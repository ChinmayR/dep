package wonka

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func failHostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	return fmt.Errorf("fail")
}

func successHostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	return nil
}

func TestIsUsshCertWhenKeyIsNilShouldError(t *testing.T) {
	result := isUsshCert(zap.NewNop(), nil, usshCheck{})
	require.False(t, result)
}

func TestIsUsshCertWhenNotValidCertTypeShouldError(t *testing.T) {
	signer := createSigner(t, nil)
	cert, _ := createCert(t, "isUsshCert", signer)

	cert.CertType = 10 // invalid cert type

	agentKey := agent.Key{Format: "ssh-rsa", Blob: cert.Marshal()}
	result := isUsshCert(zap.NewNop(), &agentKey, usshCheck{})
	require.False(t, result)
}

func TestIsUsshCertWhenCertCheckIsNilShouldError(t *testing.T) {
	signer := createSigner(t, nil)
	cert, _ := createCert(t, "isUsshCert", signer)

	agentKey := agent.Key{Format: "ssh-rsa", Blob: cert.Marshal()}
	result := isUsshCert(zap.NewNop(), &agentKey, usshCheck{})
	require.False(t, result)
}

func TestIsUsshCertWhenCertCheckFailsShouldReturnFalse(t *testing.T) {
	signer := createSigner(t, nil)
	cert, _ := createCert(t, "isUsshCert", signer)

	agentKey := agent.Key{Format: "ssh-rsa", Blob: cert.Marshal()}
	result := isUsshCert(zap.NewNop(), &agentKey, usshCheck{hostCB: failHostKeyCallback})
	require.False(t, result)
}

func TestIsUsshCertWhenCertCheckSucceedsShouldReturnTrue(t *testing.T) {
	signer := createSigner(t, nil)
	cert, _ := createCert(t, "isUsshCert", signer)

	agentKey := agent.Key{Format: "ssh-rsa", Blob: cert.Marshal()}
	result := isUsshCert(zap.NewNop(), &agentKey, usshCheck{hostCB: successHostKeyCallback})
	require.True(t, result)
}

func TestParseUserCA(t *testing.T) {
	signer := createSigner(t, nil)
	cert, _ := createCert(t, "parseUserCA", signer)

	f, err := ioutil.TempFile("", "cert_file")
	require.NoError(t, err, "creating temp file should not error")
	defer os.Remove(f.Name())

	f.Write(ssh.MarshalAuthorizedKey(cert))
	keys, err := parseUserCA(zap.NewNop(), f.Name())
	require.NoError(t, err, "should not error")
	require.NotZero(t, len(keys), "should find a key")
}

func createSigner(t *testing.T, key *rsa.PrivateKey) ssh.Signer {
	if key == nil {
		var err error
		key, err = rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err, "generating key should not error")
	}

	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err, "creating signer should not error")

	return signer
}

func createCert(t *testing.T, name string, signer ssh.Signer) (*ssh.Certificate, *rsa.PrivateKey) {
	privKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err, "generating key should not error")

	pubKey, err := ssh.NewPublicKey(&privKey.PublicKey)
	require.NoError(t, err, "should not error making new ssh public key")

	c := &ssh.Certificate{
		Key:             pubKey,
		CertType:        ssh.HostCert,
		ValidPrincipals: []string{name},
		Serial:          0,
		ValidBefore:     uint64(time.Now().Add(time.Minute).Unix()),
		ValidAfter:      uint64(time.Now().Add(-time.Minute).Unix()),
	}

	err = c.SignCert(rand.Reader, signer)
	require.NoError(t, err, "signing cert should not error")

	return c, privKey
}
