package handlers

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net/http"
	"testing"

	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

type mockResponseWriter struct{ Resp bytes.Buffer }

func (mockResponseWriter) Header() http.Header { return http.Header{} }

func (w *mockResponseWriter) Write(r []byte) (int, error) {
	w.Resp.Write(r)
	return len(r), nil
}

func (mockResponseWriter) WriteHeader(int) {}

func TestSetupHandlers(t *testing.T) {
	eccPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	rsaPrivateKey := wonkatestdata.PrivateKey()
	pubKey, err := ssh.NewPublicKey(&rsaPrivateKey.PublicKey)
	require.NoError(t, err, "generating pbkey: %v", err)
	db := wonkadb.NewMockEntityDB()

	handlerCfg := common.HandlerConfig{
		Metrics:    tally.NoopScope,
		ECPrivKey:  eccPrivateKey,
		RSAPrivKey: rsaPrivateKey,
		Ussh:       []ssh.PublicKey{pubKey},
		DB:         db,
		Logger:     zap.L(),
	}

	SetupHandlers(xhttp.NewRouter(), handlerCfg)
}
