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
	"os"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
)

func TestTaskErrorsWhenBothSignAndVerifyArePresent(t *testing.T) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(cli.NewApp(), fs, nil)
	err := Task(c)
	require.NotNil(t, err)
}

func TestDoTaskErrorsWhenSignAndVerifyAreBothPresent(t *testing.T) {
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("sign")
	cliContext.On("String", "verify").Return("verify")
	cliContext.On("String", "cgc").Return("cgc")
	err := doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDoTaskErrorsWhenSignIsPresentButCertificateIsMissing(t *testing.T) {
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("sign")
	cliContext.On("String", "verify").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "certificate").Return("")
	err := doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDoTaskErrorsWhenSignIsPresentButKeyIsMissing(t *testing.T) {
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("sign")
	cliContext.On("String", "verify").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "certificate").Return("cert")
	cliContext.On("String", "key").Return("")
	err := doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDoTaskErrorsWhenSignIsPresentButCertIsNotARealFile(t *testing.T) {
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("sign")
	cliContext.On("String", "verify").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "certificate").Return("/some/fake/file/path")
	cliContext.On("String", "key").Return("/some/fake/file/path")
	err := doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDoTaskErrorsWhenSignIsPresentButCertIsNotWonkaCertificate(t *testing.T) {
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("sign")
	cliContext.On("String", "verify").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "certificate").Return("./task_test.go")
	cliContext.On("String", "key").Return("/some/fake/file/path")
	err := doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func writeCertToTempFile(t *testing.T, tmpfile *os.File, c wonka.Certificate) {
	content, err := wonka.MarshalCertificate(c)
	require.NoError(t, err, "error marshalling test cert: %v", err)
	_, err = tmpfile.Write(content)
	require.NoError(t, err, "error writing to temp file: %v", err)
	err = tmpfile.Close()
	require.NoError(t, err, "error closing temp file: %v", err)
}

func writeKeyToTempFile(t *testing.T, tmpfile *os.File, k *ecdsa.PrivateKey) {
	content, err := x509.MarshalECPrivateKey(k)
	require.NoError(t, err, "error marshalling private key: %v", err)
	encoded := base64.StdEncoding.EncodeToString(content)
	require.NoError(t, err, "error base64 encoding content: %v", err)
	_, err = tmpfile.Write([]byte(encoded))
	require.NoError(t, err, "error writing to temp file: %v", err)
	err = tmpfile.Close()
	require.NoError(t, err, "error closing temp file: %v", err)
}
func TestDoTaskErrorsWhenSignIsPresentButKeyFileIsMissing(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "testing")
	require.NoError(t, err, "error creating temp file: %v", err)
	defer os.Remove(tmpfile.Name())
	c, _, _, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	writeCertToTempFile(t, tmpfile, *c)

	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("sign")
	cliContext.On("String", "verify").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "certificate").Return(tmpfile.Name())
	cliContext.On("String", "key").Return("/some/fake/file/path")
	err = doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDoTaskErrorsWhenSignIsPresentButKeyFileIsAJunkFile(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "testing")
	require.NoError(t, err, "error creating temp file: %v", err)
	defer os.Remove(tmpfile.Name())
	c, _, _, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	writeCertToTempFile(t, tmpfile, *c)

	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("sign")
	cliContext.On("String", "verify").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "certificate").Return(tmpfile.Name())
	cliContext.On("String", "key").Return(tmpfile.Name())
	err = doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDoTaskSucceedsWhenSignIsPresentAndCertAndKeyFilesAreValid(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "testing")
	require.NoError(t, err, "error creating temp file: %v", err)
	defer os.Remove(tmpfile.Name())
	tmpkey, err := ioutil.TempFile("", "testing_key")
	require.NoError(t, err, "error creating temp file: %v", err)
	defer os.Remove(tmpkey.Name())
	c, privKey, _, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	writeCertToTempFile(t, tmpfile, *c)
	writeKeyToTempFile(t, tmpkey, privKey)

	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("sign")
	cliContext.On("String", "verify").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "certificate").Return(tmpfile.Name())
	cliContext.On("String", "key").Return(tmpkey.Name())
	err = doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.Nil(t, err)
}
func TestDoTaskErrorsWhenVerifyIsPresentButIsNotDecodable(t *testing.T) {
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "verify").Return("verify")
	err := doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDoTaskErrorsWhenVerifyIsPresentButIsNotJSON(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(`I'll get you for this,
	Wonka if it's the last thing
	I'll ever do! I've got a blueberry for a daughter.`))
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "verify").Return(encoded)
	err := doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDoTaskErrorsWhenVerifyPresentedASignatureSignedByAStranger(t *testing.T) {
	c, _, _, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	cs := wonka.CertificateSignature{
		Certificate: *c,
	}
	jsonBytes, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificate signature: %v", err)

	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "verify").Return(encoded)
	err = doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDoTaskSucceedsWhenVerifyPresentedAValidCertificateSignatureButNoKey(t *testing.T) {
	c, k, wonkaKey, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	oldWonkaMasterKeys := wonka.WonkaMasterPublicKeys
	defer func() { wonka.WonkaMasterPublicKeys = oldWonkaMasterKeys }()
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&wonkaKey.PublicKey}
	cs := wonka.CertificateSignature{
		Certificate: *c,
	}
	toSign, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificatesignature: %v", err)
	sig, err := wonkacrypter.New().Sign(toSign, k)
	require.NoError(t, err)

	cs.Signature = sig
	jsonBytes, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificate signature: %v", err)

	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "verify").Return(encoded)
	cliContext.On("String", "certificate").Return("")
	cliContext.On("String", "key").Return("")
	err = doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.Nil(t, err)
}

func TestDoTaskErrorsWhenVerifyPresentedAValidCertificateSignatureAndKeyButNoData(t *testing.T) {
	c, k, wonkaKey, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	oldWonkaMasterKeys := wonka.WonkaMasterPublicKeys
	defer func() { wonka.WonkaMasterPublicKeys = oldWonkaMasterKeys }()
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&wonkaKey.PublicKey}
	cs := wonka.CertificateSignature{
		Certificate: *c,
	}
	toSign, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificatesignature: %v", err)
	sig, err := wonkacrypter.New().Sign(toSign, k)
	require.NoError(t, err)

	cs.Signature = sig
	jsonBytes, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificate signature: %v", err)

	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "verify").Return(encoded)
	cliContext.On("String", "certificate").Return("cert")
	cliContext.On("String", "key").Return("key")
	err = doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDoTaskErrorsWhenVerifyPresentedAValidCertificateSignatureAndKeyButCSRErrors(t *testing.T) {
	wonkaClient := new(MockWonka)
	c, k, wonkaKey, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	oldWonkaMasterKeys := wonka.WonkaMasterPublicKeys
	defer func() { wonka.WonkaMasterPublicKeys = oldWonkaMasterKeys }()
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&wonkaKey.PublicKey}
	req := wonka.LaunchRequest{}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	cs := wonka.CertificateSignature{
		Certificate: *c,
		Data:        data,
	}
	toSign, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificatesignature: %v", err)
	sig, err := wonkacrypter.New().Sign(toSign, k)
	require.NoError(t, err)

	cs.Signature = sig
	jsonBytes, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificate signature: %v", err)

	wonkaClient.On("CertificateSignRequest", context.Background(),
		mock.Anything, mock.Anything).Return(errors.New("csr failed"))
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "verify").Return(encoded)
	cliContext.On("String", "certificate").Return("cert")
	cliContext.On("String", "key").Return("key")
	cliContext.On("NewWonkaClient", TaskLauncherClient).Return(wonkaClient, nil)
	err = doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDoTaskSucceedsWhenVerifyPresentedAValidCertificateSignatureAndKeyAndCSRSucceeds(t *testing.T) {
	tmpCert, err := ioutil.TempFile("", "testing")
	require.NoError(t, err, "error setting up a testfile: %v", err)
	defer os.Remove(tmpCert.Name())
	tmpKey, err := ioutil.TempFile("", "testingKey")
	require.NoError(t, err, "error setting up a testfile: %v", err)
	defer os.Remove(tmpKey.Name())

	wonkaClient := new(MockWonka)
	c, k, wonkaKey, err := createTestCert(t)
	require.NoError(t, err, "error creating test cert: %v", err)
	oldWonkaMasterKeys := wonka.WonkaMasterPublicKeys
	defer func() { wonka.WonkaMasterPublicKeys = oldWonkaMasterKeys }()
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&wonkaKey.PublicKey}
	req := wonka.LaunchRequest{}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	cs := wonka.CertificateSignature{
		Certificate: *c,
		Data:        data,
	}
	toSign, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificatesignature: %v", err)
	sig, err := wonkacrypter.New().Sign(toSign, k)
	require.NoError(t, err)

	cs.Signature = sig
	jsonBytes, err := json.Marshal(cs)
	require.NoError(t, err, "error marshalling certificate signature: %v", err)

	wonkaClient.On("CertificateSignRequest", context.Background(),
		mock.Anything, mock.Anything).Return(nil)
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	cliContext := new(MockCLIContext)
	cliContext.On("String", "sign").Return("")
	cliContext.On("String", "cgc").Return("")
	cliContext.On("String", "verify").Return(encoded)
	cliContext.On("String", "certificate").Return(tmpCert.Name())
	cliContext.On("String", "key").Return(tmpKey.Name())
	cliContext.On("NewWonkaClient", TaskLauncherClient).Return(wonkaClient, nil)
	err = doTask(cliContext)
	cliContext.AssertExpectations(t)
	require.Nil(t, err)
}
