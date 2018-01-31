package main

import (
	"crypto/rand"
	"crypto/rsa"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal/testhelper"
	"code.uber.internal/engsec/wonka-go.git/testdata"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestLoadPrivateKey(t *testing.T) {
	f, err := ioutil.TempFile("", "loadPrivateKey")
	require.NoError(t, err, "creating temp file shouldn't error")
	defer os.Remove(f.Name())

	// invalid key
	f.WriteString("invalid")
	_, _, err = loadPrivateKey(masterKey{PrivatePem: t.Name()})
	require.Error(t, err, "invalid key should error")

	// valid key
	privKey := testdata.PrivateKeyFromPem(testdata.RSAPrivKey)
	err = testdata.WritePrivateKey(privKey, f.Name())
	require.NoError(t, err, "error writing rsa private key %v", err)

	rsaPriv, ecPriv, err := loadPrivateKey(masterKey{PrivatePem: f.Name()})
	require.NotNil(t, rsaPriv, "rsa private key should not be nil")
	require.NotNil(t, ecPriv, "ecdsa key should not be nil")
	require.NoError(t, err, "loading private key should not error")
}

func TestGetDynamicHTTPPort(t *testing.T) {
	t.Run("when env is valid should work correctly", func(t *testing.T) {
		defer testhelper.SetEnvVar("UBER_PORT_HTTP", "8008")()

		port, err := getDynamicHTTPPort(80)
		require.NoError(t, err, "should not error")
		require.Equal(t, 8008, port)
	})

	t.Run("when env is invalid value should fallback", func(t *testing.T) {
		defer testhelper.SetEnvVar("UBER_PORT_HTTP", "NaN")()

		port, err := getDynamicHTTPPort(80)
		require.Error(t, err, "invalid port number")
		require.Equal(t, 80, port)
	})

	t.Run("when env is empty should fallback", func(t *testing.T) {
		defer testhelper.SetEnvVar("UBER_PORT_HTTP", "")()

		port, err := getDynamicHTTPPort(80)
		require.NoError(t, err, "should not error when value is empty")
		require.Equal(t, 80, port)
	})

	t.Run("when env is unset should fallback", func(t *testing.T) {
		defer testhelper.UnsetEnvVar("UBER_PORT_HTTP")()

		port, err := getDynamicHTTPPort(80)
		require.NoError(t, err, "should not error when value is unset")
		require.Equal(t, 80, port)
	})
}

func TestLoadUsshInvalidValuesShouldError(t *testing.T) {
	publicKeys, hostKeyCallback, err := loadUssh("", "")
	require.Nil(t, publicKeys, "public keys should be empty")
	require.Nil(t, hostKeyCallback, "host key callback should be nil")
	require.Error(t, err, "empty values should error")
}

func TestLoadUsshValidUserCA(t *testing.T) {
	signer := createSigner(t, nil)
	cert, _ := createCert(t, "parseUserCA", signer)

	f, err := ioutil.TempFile("", "cert_file")
	require.NoError(t, err, "creating temp file should not error")
	defer os.Remove(f.Name())

	f.Write(ssh.MarshalAuthorizedKey(cert))
	keys, _, err := loadUssh(f.Name(), "")
	require.NoError(t, err, "should not error")
	require.NotZero(t, len(keys), "should find a key")
}

func TestLoadUsshInvalidCAsShouldError(t *testing.T) {
	f, err := ioutil.TempFile("", "cert_file")
	require.NoError(t, err, "creating temp file should not error")
	defer os.Remove(f.Name())

	f.WriteString("junk")
	_, _, err = loadUssh(f.Name(), f.Name())
	require.Error(t, err, "should error")
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
