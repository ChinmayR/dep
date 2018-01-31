package cmd

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal/testhelper"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
	"golang.org/x/crypto/ssh"
)

var pemKeyParams = `-----BEGIN EC PARAMETERS-----
BggqhkjOPQMBBw==
-----END EC PARAMETERS-----
`

var pemPrivateKey = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIDQYuK6beN3c+QMaGFCnSlVRiOcjxYXsE/8vvopEYs/moAoGCCqGSM49
AwEHoUQDQgAEvBm2K6E493PpLGGP2w+XeFFAQv2JINiVuCeNY868by8GuFUzIrqv
7G2AIQr/Xv26r8m6P0/89A1GPGMNQFzYrQ==
-----END EC PRIVATE KEY-----
`

func TestSaveP256ECDSAPrivateKeyToPem(t *testing.T) {
	expected := new(bytes.Buffer)
	expected.WriteString(pemKeyParams)
	expected.WriteString(pemPrivateKey)

	block, rest := pem.Decode([]byte(pemPrivateKey))
	require.Empty(t, rest)

	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	require.NoError(t, err)

	result := new(bytes.Buffer)
	saveP256ECDSAPrivateKeyToPem(privateKey, result)

	require.Equal(t, expected.String(), result.String())
}

func TestWriteCertAndPrivateKeyNoKeyPathShouldError(t *testing.T) {
	cert, key, _, err := createTestCert(t)
	require.NoError(t, err)

	cliCtx := new(MockCLIContext)
	cliCtx.On("String", "key-path").Return("")

	err = writeCertAndPrivateKey(cert, key, cliCtx)
	require.Error(t, err, "should error")
	cliCtx.AssertExpectations(t)
}

func TestWriteCertAndPrivateKeyNoCertPathShouldError(t *testing.T) {
	cert, key, _, err := createTestCert(t)
	require.NoError(t, err)

	cliCtx := new(MockCLIContext)
	cliCtx.On("String", "key-path").Return("ok")
	cliCtx.On("String", "cert-path").Return("")

	err = writeCertAndPrivateKey(cert, key, cliCtx)
	require.Error(t, err, "should error")
	cliCtx.AssertExpectations(t)
}

func TestWriteCertAndPrivateKeyInvalidKeyShouldError(t *testing.T) {
	cert, key, _, err := createTestCert(t)
	require.NoError(t, err)
	key.Curve = nil // invalidate the curve

	cliCtx := new(MockCLIContext)
	cliCtx.On("String", "key-path").Return("ok")
	cliCtx.On("String", "cert-path").Return("ok")

	err = writeCertAndPrivateKey(cert, key, cliCtx)
	require.Error(t, err, "should error")
	cliCtx.AssertExpectations(t)
}

func TestWriteCertAndPrivateKey(t *testing.T) {
	defer func() {
		os.Remove("key-path")
		os.Remove("cert-path")
		os.Remove("x509-path")
	}()

	cert, key, _, err := createTestCert(t)
	require.NoError(t, err, "should not error")

	cliCtx := new(MockCLIContext)
	cliCtx.On("String", "key-path").Return("key-path")
	cliCtx.On("String", "cert-path").Return("cert-path")
	cliCtx.On("String", "x509-path").Return("x509-path")

	err = writeCertAndPrivateKey(cert, key, cliCtx)
	require.NoError(t, err, "should not error")

	cliCtx.AssertExpectations(t)
}

func TestNoSelfFails(t *testing.T) {
	if testhelper.IsProductionEnvironment {
		t.Skip()
	}

	cliCtx := new(MockCLIContext)
	cliCtx.On("GlobalString", "self").Return("")
	err := doRequestCertificate(cliCtx)
	require.Error(t, err, "should error")
}

func TestNoTaskIDFails(t *testing.T) {
	if testhelper.IsProductionEnvironment {
		t.Skip()
	}

	cliCtx := new(MockCLIContext)
	cliCtx.On("GlobalString", "self").Return("foo")
	cliCtx.On("String", "taskid").Return("")
	err := doRequestCertificate(cliCtx)
	require.Error(t, err, "should error")
}

func TestNoKeyPathFails(t *testing.T) {
	if testhelper.IsProductionEnvironment {
		t.Skip()
	}

	cliCtx := new(MockCLIContext)
	cliCtx.On("GlobalString", "self").Return("foo")
	cliCtx.On("String", "taskid").Return("foo-1234")
	cliCtx.On("String", "key-path").Return("")
	err := doRequestCertificate(cliCtx)
	require.Error(t, err, "should error")
}

func TestNoCertPathFails(t *testing.T) {
	if testhelper.IsProductionEnvironment {
		t.Skip()
	}

	cliCtx := new(MockCLIContext)
	cliCtx.On("GlobalString", "self").Return("foo")
	cliCtx.On("String", "taskid").Return("foo-1234")
	cliCtx.On("String", "key-path").Return("foo.key")
	cliCtx.On("String", "cert-path").Return("")
	err := doRequestCertificate(cliCtx)
	require.Error(t, err, "should error")
}

func TestRequestCertificateFailsWithoutAnyArgs(t *testing.T) {
	if testhelper.IsProductionEnvironment {
		t.Skip()
	}

	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(cli.NewApp(), fs, nil)
	err := RequestCertificate(c)
	require.Error(t, err, "should error")
}

func TestNoWonkadPathFails(t *testing.T) {
	if testhelper.IsProductionEnvironment {
		t.Skip()
	}

	cliCtx := new(MockCLIContext)
	cliCtx.On("GlobalString", "self").Return("foo")
	cliCtx.On("String", "taskid").Return("foo-1234")
	cliCtx.On("String", "key-path").Return("foo.key")
	cliCtx.On("String", "cert-path").Return("foo.crt")
	cliCtx.On("String", "wonkad-path").Return("")
	err := doRequestCertificate(cliCtx)
	require.Error(t, err, "should error")
}

func TestDoRequestCertificate(t *testing.T) {
	withSSHConfig(t, "doRequestCertificate", func(configPath string) {
		// prep the ssh_config path and reset it when the test is over
		oldSshdConfigPath := sshdConfigPath
		defer func() { sshdConfigPath = oldSshdConfigPath }()
		sshdConfigPath = configPath

		// delete files we make writing out the cert and key
		defer func() {
			os.Remove("foo.crt")
			os.Remove("foo.key")
		}()

		wonkaClient := new(MockWonka)
		wonkaClient.On("CertificateSignRequest",
			mock.AnythingOfType("*context.emptyCtx"),
			mock.AnythingOfType("*wonka.Certificate"),
			mock.AnythingOfType("*wonka.CertificateSignature")).Return(nil)

		ctx := context.Background()

		cliCtx := new(MockCLIContext)
		cliCtx.On("GlobalString", "self").Return("foo")
		cliCtx.On("String", "taskid").Return("foo-1234")
		cliCtx.On("String", "x509-path").Return("")
		cliCtx.On("String", "key-path").Return("foo.key")
		cliCtx.On("String", "cert-path").Return("foo.crt")
		cliCtx.On("String", "wonkad-path").Return("foo.sock")
		cliCtx.On("NewWonkaClientFromConfig", mock.AnythingOfType("wonka.Config")).Return(wonkaClient, nil)
		cliCtx.On("Context").Return(ctx)

		err := doRequestCertificate(cliCtx)
		require.NoError(t, err, "should request certificate")
	})
}

func withSSHConfig(t *testing.T, name string, fn func(string)) {
	authority := wonkatestdata.AuthorityKey()
	signer, err := ssh.NewSignerFromKey(authority)
	require.NoError(t, err, "error creating signer: %v", err)
	cert, privKey := createCert(t, name, signer)

	defer testhelper.SetEnvVar("WONKA_USSH_HOST_CA",
		fmt.Sprintf("@cert-authority * %s", ssh.MarshalAuthorizedKey(signer.PublicKey())))()

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

func createCert(t *testing.T, name string, signer ssh.Signer) (*ssh.Certificate, crypto.PrivateKey) {
	privKey := wonkatestdata.PrivateKey()

	pubKey, err := ssh.NewPublicKey(&privKey.PublicKey)
	require.NoError(t, err, "creating ssh public key should not error")

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
