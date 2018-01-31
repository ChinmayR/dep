package cmd

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"net"
	"os"
	"path"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestSignDataErrorsWhenInvalid(t *testing.T) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(cli.NewApp(), fs, nil)
	err := SignData(c)
	require.Error(t, err, "sign should err")
}

func TestVerifySignatureErrorsWhenInvalid(t *testing.T) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(cli.NewApp(), fs, nil)
	err := VerifySignature(c)
	require.Error(t, err, "sign should err")
}

func TestLoadSignData(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	b, err := loadDataToSign(cliCtx)
	require.NoError(t, err, "shouldn't error: %v", err)
	require.Equal(t, b, []byte("foober"))
}

func TestLoadSignFile(t *testing.T) {
	wonkatestdata.WithTempDir(func(dir string) {
		f := path.Join(dir, "file")
		err := ioutil.WriteFile(f, []byte("foober"), 0444)
		require.NoError(t, err, "error writing file: %v", err)

		cliCtx := new(MockCLIContext)
		cliCtx.On("StringOrFirstArg", "data").Return("")
		cliCtx.On("StringOrFirstArg", "file").Return(f)
		b, err := loadDataToSign(cliCtx)
		require.NoError(t, err, "shouldn't error: %v", err)
		require.Equal(t, b, []byte("foober"))
	})
}

func TestLoadSignDataError(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("")
	cliCtx.On("StringOrFirstArg", "file").Return("")
	b, err := loadDataToSign(cliCtx)
	require.Error(t, err, "should error")
	require.Nil(t, b, "should be empty")
}

func TestSignNoDataError(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("")
	cliCtx.On("StringOrFirstArg", "file").Return("")

	err := doSignData(cliCtx)
	require.Error(t, err, "sign should err")
}

func TestSignDataError(t *testing.T) {
	wonkaClient := new(MockWonka)
	wonkaClient.On("Sign", []byte("foober")).Return(nil, errors.New("err"))

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(false)
	cliCtx.On("Writer").Return(ioutil.Discard)
	err := doSignData(cliCtx)
	require.Error(t, err, "signing should error")
}

func TestSignData(t *testing.T) {
	wonkaClient := new(MockWonka)
	wonkaClient.On("Sign", []byte("foober")).Return([]byte("test"), nil)

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(false)
	cliCtx.On("Writer").Return(ioutil.Discard)
	err := doSignData(cliCtx)
	require.NoError(t, err, "signing shouldn't err: %v", err)
}

func TestVerifySucceeds(t *testing.T) {
	ctx := context.Background()
	signString := "foober"

	wonkaClient := new(MockWonka)
	wonkaClient.On("Verify", ctx, []byte("foober"), []byte("foober"), "foober").Return(true)

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return(signString)
	cliCtx.On("StringOrFirstArg", "signer").Return("foober")
	cliCtx.On("StringOrFirstArg", "signature").
		Return(base64.StdEncoding.EncodeToString([]byte(signString)))

	cliCtx.On("Context").Return(ctx)
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(false)

	cliCtx.On("Writer").Return(ioutil.Discard)
	err := doVerifySignature(cliCtx)
	require.NoError(t, err, "verify shouldn't err: %v", err)
}

func TestVerifyFails(t *testing.T) {
	ctx := context.Background()
	signString := "foober"

	wonkaClient := new(MockWonka)
	wonkaClient.On("Verify", ctx, []byte("foober"), []byte("foober"), "foober").Return(false)

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return(signString)
	cliCtx.On("StringOrFirstArg", "signer").Return("foober")
	cliCtx.On("StringOrFirstArg", "signature").
		Return(base64.StdEncoding.EncodeToString([]byte(signString)))

	cliCtx.On("Context").Return(ctx)
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(false)

	cliCtx.On("Writer").Return(ioutil.Discard)
	err := doVerifySignature(cliCtx)
	require.Error(t, err, "verify should err")
}

func TestVerifyBadSignature(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("StringOrFirstArg", "signer").Return("foober")
	cliCtx.On("StringOrFirstArg", "signature").Return("foober")

	wonkaClient := new(MockWonka)
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(false)

	cliCtx.On("Writer").Return(ioutil.Discard)
	err := doVerifySignature(cliCtx)
	require.Error(t, err, "verify should err")
}

func TestVerifyNoDataError(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("")
	cliCtx.On("StringOrFirstArg", "file").Return("")

	err := doVerifySignature(cliCtx)
	require.Error(t, err, "verify should err")
}
func TestCertVerifyFailsWhenSignStringFailsToUnMarshal(t *testing.T) {
	signString := "foober"

	wonkaClient := new(MockWonka)

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return(signString)
	cliCtx.On("StringOrFirstArg", "signer").Return("foober")
	cliCtx.On("StringOrFirstArg", "signature").
		Return(base64.StdEncoding.EncodeToString([]byte(signString)))

	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(true)

	err := doVerifySignature(cliCtx)
	assert.NotNil(t, err)
}

func TestCertVerifyFailsWhenSignCertificateIsInvalid(t *testing.T) {
	c, _, _, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	cs := wonka.CertificateSignature{
		Certificate: *c,
	}
	jsonBytes, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificate signature: %v", err)

	signString := "foober"

	wonkaClient := new(MockWonka)

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return(signString)
	cliCtx.On("StringOrFirstArg", "signer").Return("foober")
	cliCtx.On("StringOrFirstArg", "signature").
		Return(base64.StdEncoding.EncodeToString(jsonBytes))

	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(true)

	err = doVerifySignature(cliCtx)
	assert.NotNil(t, err)
}

func TestCertVerifyFailsWhenCertIsValidButSignerIsInvalid(t *testing.T) {
	c, _, wonkaKey, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	oldWonkaMasterKeys := wonka.WonkaMasterPublicKeys
	defer func() { wonka.WonkaMasterPublicKeys = oldWonkaMasterKeys }()
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&wonkaKey.PublicKey}
	cs := wonka.CertificateSignature{
		Certificate: *c,
	}
	jsonBytes, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificate signature: %v", err)

	signString := "foober"

	wonkaClient := new(MockWonka)

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return(signString)
	cliCtx.On("StringOrFirstArg", "signer").Return("foober")
	cliCtx.On("StringOrFirstArg", "signature").
		Return(base64.StdEncoding.EncodeToString(jsonBytes))

	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(true)

	err = doVerifySignature(cliCtx)
	assert.NotNil(t, err)
}

func TestCertVerifySucceedsWhenEverythingIsValid(t *testing.T) {
	c, k, wonkaKey, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	oldWonkaMasterKeys := wonka.WonkaMasterPublicKeys
	defer func() { wonka.WonkaMasterPublicKeys = oldWonkaMasterKeys }()
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&wonkaKey.PublicKey}
	signedData, err := wonkacrypter.New().Sign([]byte("foober"), k)
	require.NoError(t, err, "error signing data: %v", err)

	var certSigner = struct {
		Certificate wonka.Certificate
		Data        []byte
	}{*c, []byte("foober")}

	cs := wonka.CertificateSignature{
		Certificate: *c,
	}
	toSign, err := json.Marshal(certSigner)
	require.NoError(t, err, "error marshalling certificatesignature: %v", err)
	sig, err := wonkacrypter.New().Sign(toSign, k)
	require.NoError(t, err, "error signing: %v", err)

	cs.Signature = sig

	jsonBytes, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificate signature: %v", err)

	wonkaClient := new(MockWonka)

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("StringOrFirstArg", "signer").Return(string(signedData))
	cliCtx.On("StringOrFirstArg", "signature").
		Return(base64.StdEncoding.EncodeToString(jsonBytes))

	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(true)
	cliCtx.On("StringOrFirstArg", "cert-path").Return("")
	cliCtx.On("StringOrFirstArg", "key-path").Return("")

	err = doVerifySignature(cliCtx)
	assert.Nil(t, err)
}

func TestSignNoSelfErrors(t *testing.T) {
	wonkaClient := new(MockWonka)
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("StringOrFirstArg", "file").Return("foober")
	cliCtx.On("Bool", "cert").Return(true)
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("GlobalString", "self").Return("")
	err := doSignData(cliCtx)
	assert.NotNil(t, err)
}

func TestSignNoWonkadErrors(t *testing.T) {
	wonkaClient := new(MockWonka)
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("StringOrFirstArg", "file").Return("foober")
	cliCtx.On("Bool", "cert").Return(true)
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("GlobalString", "self").Return("self")
	cliCtx.On("String", "wonkad-path").Return("")
	err := doSignData(cliCtx)
	assert.NotNil(t, err)
}

func TestSignReturnsErrorWhenWonkadReplyMissingCert(t *testing.T) {
	wonkadFile := "testUnixConnection"
	ln, err := net.Listen("unix", wonkadFile)
	require.NoError(t, err, "error setting up the listener: %v", err)
	defer ln.Close()
	defer os.Remove(wonkadFile)

	repl := wonka.WonkadReply{}
	go serverFunc(t, ln, repl)

	wonkaClient := new(MockWonka)
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("StringOrFirstArg", "file").Return("foober")
	cliCtx.On("Bool", "cert").Return(true)
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("GlobalString", "self").Return("self")
	cliCtx.On("String", "wonkad-path").Return(wonkadFile)
	cliCtx.On("Duration", "timeout").Return(time.Second)
	cliCtx.On("Writer").Return(ioutil.Discard)
	err = doSignData(cliCtx)
	assert.NotNil(t, err)
}

func TestSignReturnsErrorWhenWonkadReplyMissingCertKey(t *testing.T) {
	c, _, _, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	wonkadFile := "testUnixConnection"
	ln, err := net.Listen("unix", wonkadFile)
	require.NoError(t, err, "error setting up the listener: %v", err)
	defer ln.Close()
	defer os.Remove(wonkadFile)

	certBytes, err := json.Marshal(c)
	require.NoError(t, err, "error marshalling certificate: %v", err)
	repl := wonka.WonkadReply{Certificate: certBytes}
	go serverFunc(t, ln, repl)

	wonkaClient := new(MockWonka)
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("StringOrFirstArg", "file").Return("foober")
	cliCtx.On("Bool", "cert").Return(true)
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("GlobalString", "self").Return("self")
	cliCtx.On("String", "wonkad-path").Return(wonkadFile)
	cliCtx.On("Duration", "timeout").Return(time.Second)
	cliCtx.On("Writer").Return(ioutil.Discard)
	err = doSignData(cliCtx)
	assert.NotNil(t, err)
}

func TestSignReturnsCertFromWonkad(t *testing.T) {
	c, k, _, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)

	wonkadFile := "testUnixConnection"
	ln, err := net.Listen("unix", wonkadFile)
	require.NoError(t, err, "error setting up the listener: %v", err)
	defer ln.Close()
	defer os.Remove(wonkadFile)

	certBytes, err := json.Marshal(c)
	require.NoError(t, err, "error marshalling certificate: %v", err)

	keyBytes, err := x509.MarshalECPrivateKey(k)
	require.NoError(t, err, "error marshalling private key: %v", err)

	repl := wonka.WonkadReply{Certificate: certBytes, PrivateKey: keyBytes}
	go serverFunc(t, ln, repl)

	wonkaClient := new(MockWonka)
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("StringOrFirstArg", "file").Return("foober")
	cliCtx.On("Bool", "cert").Return(true)
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("GlobalString", "self").Return("self")
	cliCtx.On("String", "wonkad-path").Return(wonkadFile)
	cliCtx.On("Duration", "timeout").Return(time.Second)
	cliCtx.On("Writer").Return(ioutil.Discard)
	err = doSignData(cliCtx)
	assert.Nil(t, err)
}
