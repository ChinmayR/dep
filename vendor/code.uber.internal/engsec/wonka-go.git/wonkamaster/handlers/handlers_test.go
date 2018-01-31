package handlers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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

func getTestConfig(t *testing.T) common.HandlerConfig {
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
	return handlerCfg
}
