package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/rpc"
	"code.uber.internal/engsec/wonka-go.git/internal/testhelper"
	"code.uber.internal/engsec/wonka-go.git/testdata"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var adminHandlerVars = []struct {
	mem    string
	entity string
	toDel  string

	badBaggage bool
	errMsg     string
}{
	{entity: "e1@uber.com", toDel: "e2@uber.com", mem: "AD:wonka-admins"},
	{entity: "e1@uber.com", toDel: "e2@uber.com", mem: "AD:engineering", errMsg: wonka.AdminAccessDenied},
}

func TestAdminHandler(t *testing.T) {
	log := zap.S()

	for idx, m := range adminHandlerVars {
		wonkatestdata.WithUSSHAgent(m.entity, func(agentPath string, caKey ssh.PublicKey) {
			wonkatestdata.WithWonkaMaster(m.entity, func(r common.Router, handlerCfg common.HandlerConfig) {
				defer testhelper.SetEnvVar("SSH_AUTH_SOCK", agentPath)()
				aSock, err := net.Dial("unix", agentPath)
				if err != nil {
					panic(err)
				}

				a := agent.NewClient(aSock)
				k, err := a.List()
				if err != nil {
					panic(err)
				}
				if len(k) != 1 {
					log.Fatalf("invalid keys: %d\n", len(k))
				}

				mem := make(map[string][]string, 0)
				mem[m.entity] = []string{m.mem}
				handlerCfg.Pullo = rpc.NewMockPulloClient(mem,
					rpc.Logger(handlerCfg.Logger, zap.NewAtomicLevel()))
				handlerCfg.Ussh = []ssh.PublicKey{caKey}

				// put it in the ether
				defer testhelper.SetEnvVar("WONKA_USSH_CA", string(ssh.MarshalAuthorizedKey(caKey)))()

				ctx := context.Background()

				// here we should be able to request wonka-admin personnel claims
				testdata.EnrollEntity(ctx, t, handlerCfg.DB, m.entity, wonkatestdata.PrivateKey())
				wonkaCfg := wonka.Config{
					EntityName: m.entity,
				}

				SetupHandlers(r, handlerCfg)

				w, err := wonka.Init(wonkaCfg)
				require.NoError(t, err, "test %d, wonka init error: %v", idx, err)

				testdata.EnrollEntity(ctx, t, handlerCfg.DB, m.toDel, wonkatestdata.PrivateKey())

				req := wonka.AdminRequest{
					Action:     wonka.DeleteEntity,
					EntityName: m.entity,
					ActionOn:   m.toDel,
				}

				err = w.Admin(ctx, req)
				if m.errMsg == "" {
					require.NoError(t, err, "test %d, Admin error: %v", idx, err)
				} else {
					require.True(t, err != nil, "test %d should error", idx)
					require.Contains(t, err.Error(), m.errMsg, "test %d error not equal", idx)
				}
			})
		})
	}
}

func TestAdminHandlerUnits(t *testing.T) {
	validHandler := newAdminHandler(getTestConfig(t))

	var testCases = []struct {
		name       string
		statusCode int
		Message    string
		makeCall   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:       "returns error when method is not post",
			statusCode: http.StatusMethodNotAllowed,
			Message:    wonka.AdminInvalidCmd,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				r := &http.Request{
					Method: "GOLDEN TICKET",
					Body: ioutil.NopCloser(
						bytes.NewReader([]byte("I'm extraordinary busy sir.")))}
				validHandler.ServeHTTP(context.Background(), w, r)
			},
		},
		{
			name:       "returns error when req fails to deserialize",
			statusCode: http.StatusBadRequest,
			Message:    wonka.DecodeError,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				r := &http.Request{
					Method: "POST",
					Body: ioutil.NopCloser(
						bytes.NewReader([]byte("I SAID GOOD DAY!")))}
				validHandler.ServeHTTP(context.Background(), w, r)
			},
		},
		{
			name:       "returns error when ussh verify fails to verify",
			statusCode: http.StatusForbidden,
			Message:    wonka.SignatureVerifyError,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				req := wonka.AdminRequest{}
				data, err := json.Marshal(req)
				require.NoError(t, err, "failed to marshal resolve request: %v", err)
				r := &http.Request{
					Method: "POST",
					Body:   ioutil.NopCloser(bytes.NewReader(data))}
				validHandler.ServeHTTP(context.Background(), w, r)
			},
		},
		{
			name:       "returns error when ussh verify fails because the key is not an ssh cert",
			statusCode: http.StatusForbidden,
			Message:    wonka.SignatureVerifyError,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				wonkatestdata.WithUSSHAgent("testEntity@uber.com", func(agentPath string, caKey ssh.PublicKey) {
					req := wonka.AdminRequest{
						Ussh: string(ssh.MarshalAuthorizedKey(caKey)),
					}
					data, err := json.Marshal(req)
					require.NoError(t, err, "failed to marshal resolve request: %v", err)
					r := &http.Request{
						Method: "POST",
						Body:   ioutil.NopCloser(bytes.NewReader(data))}
					validHandler.ServeHTTP(context.Background(), w, r)
				})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.makeCall(t, w)
			resp := w.Result()
			decoder := json.NewDecoder(resp.Body)
			var m wonka.GenericResponse
			err := decoder.Decode(&m)
			assert.Nil(t, err, "err was not nil")
			assert.Equal(t, tc.statusCode, resp.StatusCode, "status code did not match expected")
			assert.Equal(t, tc.Message, m.Result, "message did not match expected")
		})
	}
}
