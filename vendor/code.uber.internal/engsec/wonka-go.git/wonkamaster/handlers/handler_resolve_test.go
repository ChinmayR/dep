package handlers

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/rpc"
	"code.uber.internal/engsec/wonka-go.git/internal/testhelper"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"go.uber.org/zap"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestResolve(t *testing.T) {
	var testVars = []struct {
		e1     string
		e1In   []string
		e1TTL  int64
		badReq bool

		e2         string
		e2Requires []string
		expect     []string
		err        string
	}{
		{e1: "test@uber.com", e2: "foober", expect: []string{wonka.EveryEntity, "test@uber.com"}},
		{e1: "test@uber.com", e2: "doober", e2Requires: []string{"foo"},
			expect: []string{wonka.EveryEntity, "test@uber.com"}},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d: %+v", idx, m), func(t *testing.T) {
			wonkatestdata.WithUSSHAgent(m.e1, func(agentPath string, caKey ssh.PublicKey) {
				wonkatestdata.WithWonkaMaster("foober", func(r common.Router, handlerCfg common.HandlerConfig) {
					mem := make(map[string][]string)
					mem[m.e1] = m.e1In
					handlerCfg.Pullo = rpc.NewMockPulloClient(mem,
						rpc.Logger(handlerCfg.Logger, zap.NewAtomicLevel()))
					handlerCfg.Ussh = []ssh.PublicKey{caKey}

					if len(m.e2Requires) != 0 {
						entity := &wonka.Entity{
							EntityName: m.e2,
							Requires:   strings.Join(m.e2Requires, ","),
						}
						handlerCfg.DB.Create(context.TODO(), entity)
					}
					SetupHandlers(r, handlerCfg)

					a, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
					require.NoError(t, err, "ssh-agent dial error: %v", err)

					w, err := wonka.Init(wonka.Config{EntityName: m.e1, Agent: agent.NewClient(a)})
					require.NoError(t, err, "error initializing wonka: %v", err)

					if m.badReq {
						defer testhelper.SetEnvVar("WONKA_USSH_CA", fmt.Sprintf("foo"))()
					}

					c, err := w.ClaimResolve(context.Background(), m.e2)
					if m.err != "" {
						require.Error(t, err)
						require.Contains(t, err.Error(), "failed to load user ca file")
					} else {
						require.NoError(t, err, "shouldn't err: %v", err)

						sort.Strings(m.expect)
						claims := c.Claims
						sort.Strings(claims)
						require.Equal(t, m.expect, claims, "should be equal")
					}
				})
			})
		})
	}
}

func serializeResolveRequestAndMakeCall(t *testing.T,
	w *httptest.ResponseRecorder,
	h xhttp.Handler,
	req wonka.ResolveRequest) {
	data, err := json.Marshal(req)
	require.NoError(t, err, "failed to marshal resolve request: %v", err)
	r := &http.Request{Body: ioutil.NopCloser(bytes.NewReader(data))}
	h.ServeHTTP(context.Background(), w, r)
}

func TestResolveHandlerUnits(t *testing.T) {
	validHandler := newResolveHandler(getTestConfig(t))
	eccPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "failed to create a private key: %v", err)

	var testCases = []struct {
		name         string
		statusCode   int
		errorMessage string
		makeCall     func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:         "errors when request is invalid json",
			statusCode:   http.StatusBadRequest,
			errorMessage: wonka.DecodeError,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				r := &http.Request{Body: ioutil.NopCloser(
					bytes.NewReader([]byte("Snozzberries? Who ever heard of a snozzberry?")))}
				validHandler.ServeHTTP(context.Background(), w, r)
			},
		},
		{
			name:         "errors when request is missing auth",
			statusCode:   http.StatusBadRequest,
			errorMessage: wonka.SignatureVerifyError,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				req := wonka.ResolveRequest{}
				serializeResolveRequestAndMakeCall(t, w, validHandler, req)
			},
		},
		{
			name:         "errors when request is missing auth via authEnrolledEntity because signature is wrong",
			statusCode:   http.StatusBadRequest,
			errorMessage: wonka.SignatureVerifyError,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				pubKey := wonka.KeyToCompressed(eccPrivateKey.PublicKey.X, eccPrivateKey.PublicKey.Y)

				req := wonka.ResolveRequest{
					PublicKey: pubKey,
					Signature: []byte("You're turning violet, Violet!"),
				}
				serializeResolveRequestAndMakeCall(t, w, validHandler, req)
			},
		},
		{
			name:         "errors when request is having an invalid certificate",
			statusCode:   http.StatusBadRequest,
			errorMessage: wonka.SignatureVerifyError,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				pubKey := wonka.KeyToCompressed(eccPrivateKey.PublicKey.X, eccPrivateKey.PublicKey.Y)

				req := wonka.ResolveRequest{
					PublicKey:   pubKey,
					Certificate: []byte("Strike that! Reverse it!"),
				}
				serializeResolveRequestAndMakeCall(t, w, validHandler, req)
			},
		},
		{
			name:         "succeeds when the cert request is valid",
			statusCode:   http.StatusOK,
			errorMessage: wonka.ResultOK,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				pubKey := wonka.KeyToCompressed(eccPrivateKey.PublicKey.X, eccPrivateKey.PublicKey.Y)

				req := wonka.ResolveRequest{
					PublicKey: pubKey,
					Etime:     time.Now().Add(2 * maxClaimTime).Unix(),
					Claims:    "charlie,wonka",
				}

				toSign, err := json.Marshal(req)
				require.NoError(t, err, "failed to marshal request for signing: %v", err)
				req.Signature, err = wonkacrypter.New().Sign(toSign, eccPrivateKey)
				require.NoError(t, err, "failed to sign request: %v", err)

				serializeResolveRequestAndMakeCall(t, w, validHandler, req)
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
			assert.Equal(t, tc.errorMessage, m.Result, "error message did not match expected")
		})
	}
}
