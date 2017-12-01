package sshhelper

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func TestLoadHostCert(t *testing.T) {
	var hostCertVars = []struct {
		name string
	}{{name: "foo01"}}

	for _, m := range hostCertVars {
		withSSHConfig(t, m.name, func(configPath string) {
			cert, privKey, err := UsshHostCert(zap.NewNop(), configPath)
			require.NoError(t, err, "error reading host cert and key: %v", err)
			require.Equal(t, cert.ValidPrincipals[0], m.name)

			signer, err := ssh.NewSignerFromKey(privKey)
			require.NoError(t, err, "error getting signing key: %v", err)

			// now verify the cert and privkey corespond to each other.
			data, sig := signData(signer)
			err = cert.Verify(data, sig)
			require.NoError(t, err, "verify error: %v", err)
		})
	}
}

func signData(privKey ssh.Signer) ([]byte, *ssh.Signature) {
	b := make([]byte, 128)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}

	sig, err := privKey.Sign(rand.Reader, b)
	if err != nil {
		panic(err)
	}

	return b, sig
}

func withSSHConfig(t *testing.T, name string, fn func(string)) {
	authority := wonkatestdata.AuthorityKey()
	signer, err := ssh.NewSignerFromKey(authority)
	require.NoError(t, err, "error creating signer: %v", err)
	cert, privKey := createCert(name, signer)

	oldHostCA := os.Getenv("WONKA_USSH_HOST_CA")
	os.Setenv("WONKA_USSH_HOST_CA",
		fmt.Sprintf("@cert-authority * %s", ssh.MarshalAuthorizedKey(signer.PublicKey())))

	defer func() {
		if oldHostCA == "" {
			os.Unsetenv("WONKA_USSH_HOST_CA")
		} else {
			os.Setenv("WONKA_USSH_HOST_CA", oldHostCA)
		}
	}()

	rsaPriv, ok := privKey.(*rsa.PrivateKey)
	require.True(t, ok, "privkey not an rsa key")

	wonkatestdata.WithTempDir(func(dir string) {
		certPath := path.Join(dir, "ssh_host_rsa_key-cert.pub")

		err := ioutil.WriteFile(certPath, ssh.MarshalAuthorizedKey(cert), 0666)
		require.NoError(t, err, "error writing cert: %v", err)

		privKeyPath := path.Join(dir, "ssh_host_rsa_key")
		pemBlock := &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(rsaPriv),
		}
		err = ioutil.WriteFile(privKeyPath, pem.EncodeToMemory(pemBlock), 0666)
		require.NoError(t, err, "error writing private key: %v", err)

		configPath := path.Join(dir, "sshd_config")
		configLine := fmt.Sprintf("HostCertificate %s\nHostKey %s", certPath, privKeyPath)

		err = ioutil.WriteFile(configPath, []byte(configLine), 0666)
		require.NoError(t, err, "error writing config file: %v", err)

		fn(configPath)
	})

}

func createCert(name string, signer ssh.Signer) (*ssh.Certificate, crypto.PrivateKey) {
	privKey := wonkatestdata.PrivateKey()

	pubKey, err := ssh.NewPublicKey(&privKey.PublicKey)
	if err != nil {
		panic(err)
	}

	c := &ssh.Certificate{
		Key:             pubKey,
		CertType:        ssh.HostCert,
		ValidPrincipals: []string{name},
		Serial:          0,
		ValidBefore:     uint64(time.Now().Add(time.Minute).Unix()),
		ValidAfter:      uint64(time.Now().Add(-time.Minute).Unix()),
	}

	if err := c.SignCert(rand.Reader, signer); err != nil {
		panic(err)
	}

	return c, privKey
}
