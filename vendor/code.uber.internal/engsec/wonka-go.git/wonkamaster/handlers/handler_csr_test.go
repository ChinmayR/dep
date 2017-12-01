package handlers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestCSR(t *testing.T) {
	var csrVars = []struct {
		name    string
		badName bool
		vb      time.Duration

		err string
	}{
		{name: "foo01-sjc1.prod.uber.internal"},
		{name: "foo01-sjc1.prod.uber.internal", vb: 21 * time.Hour},
		{name: "foo01-sjc1.prod.uber.internal", badName: true,
			err: wonka.BadCertificateSigningRequest},
	}

	for idx, m := range csrVars {
		wonkatestdata.WithUSSHHostAgent(m.name, func(agentPath string, caKey ssh.PublicKey) {
			wonkatestdata.WithWonkaMaster(m.name, func(r common.Router, handlerCfg common.HandlerConfig) {
				oldCA := os.Getenv("WONKA_USSH_HOST_CA")
				os.Setenv("WONKA_USSH_HOST_CA",
					fmt.Sprintf("@cert-authority * %s", ssh.MarshalAuthorizedKey(caKey)))

				SetupHandlers(r, handlerCfg)
				require.NotNil(t, r, "%d setup handlers returned nil", idx)
				a, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
				require.NoError(t, err, "%d, ssh-agent dial error: %v", idx, err)

				w, err := wonka.Init(wonka.Config{EntityName: m.name, Agent: agent.NewClient(a)})
				require.NoError(t, err, "%d, error initializing wonka: %v", idx, err)

				cert, _, err := wonka.NewCertificate(wonka.CertEntityName(m.name), wonka.CertHostname(m.name))
				require.NoError(t, err, "%d, error generating a cert: %v", idx, err)
				cert.ValidBefore = uint64(time.Now().Add(time.Minute).Unix())

				if m.badName {
					cert.Host = cert.Host + "bad"
				}

				if m.vb != 0 {
					cert.ValidBefore = uint64(time.Now().Add(m.vb).Unix())
				}

				err = w.CertificateSignRequest(context.Background(), cert, nil)
				if m.err != "" {
					require.Error(t, err, "should error")
					require.Contains(t, err.Error(), m.err, "%d, should contain %s", idx, m.err)
				} else {
					require.NoError(t, err, "%d, signing error: %v", idx, err)
				}

				if m.vb != 0 {
					certVB := time.Unix(int64(cert.ValidBefore), 0)
					vb := time.Now().Add(m.vb).Add(-time.Minute)
					require.True(t, certVB.Before(vb), "time should've been replaced")
				}

				os.Setenv("WONKA_USSH_HOST_CA", oldCA)
			})
		})
	}
}

func TestRefreshCert(t *testing.T) {
	wonkaPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "error generating key: %v", err)

	oldWonkaKeys := wonka.WonkaMasterPublicKeys
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&wonkaPriv.PublicKey}
	defer func() {
		wonka.WonkaMasterPublicKeys = oldWonkaKeys
	}()

	signingCert, signingKey, err := wonka.NewCertificate(wonka.CertEntityName("foo"), wonka.CertHostname("host"))
	signingCertBytes, err := wonka.MarshalCertificate(*signingCert)
	require.NoError(t, err)

	signingCert.Signature, err = wonkacrypter.New().Sign(signingCertBytes, wonkaPriv)
	require.NoError(t, err)

	err = signingCert.CheckCertificate()
	require.NoError(t, err, "signing cert doesn't verify: %v", err)

	signingCertBytes, err = wonka.MarshalCertificate(*signingCert)
	require.NoError(t, err)

	cert, _, err := wonka.NewCertificate(
		wonka.CertEntityName(signingCert.EntityName),
		wonka.CertHostname(signingCert.Host),
		wonka.CertTaskIDTag(signingCert.Tags[wonka.TagTaskID]))
	require.NoError(t, err)

	certBytes, err := wonka.MarshalCertificate(*cert)
	require.NoError(t, err)

	csr := wonka.CertificateSigningRequest{
		Certificate:        certBytes,
		SigningCertificate: signingCertBytes,
	}

	toSign, err := json.Marshal(csr)
	require.NoError(t, err)

	csr.Signature, err = wonkacrypter.New().Sign(toSign, signingKey)
	require.NoError(t, err)

	h := csrHandler{
		eccPrivateKey: wonkaPriv,
		log:           zap.L(),
	}

	// everything above here is setup.
	// TODO(pmoody): move this setup code into a helper that can be used by other tests.
	// this doesn't actually sign the new cert, it just verifies the request is good
	err = h.existingCertVerify(csr)
	require.NoError(t, err)
}
