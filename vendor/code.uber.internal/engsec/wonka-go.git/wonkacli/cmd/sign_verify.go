package cmd

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"go.uber.org/zap"
)

func loadDataToSign(c CLIContext) ([]byte, error) {
	log := zap.L()

	dataToSign := []byte(c.StringOrFirstArg("data"))
	if len(dataToSign) == 0 {
		fileToSign := c.StringOrFirstArg("file")
		if fileToSign == "" {
			log.Error("no data provided")
			return nil, cli.NewExitError("", 1)
		}
		var err error
		dataToSign, err = ioutil.ReadFile(fileToSign)
		if err != nil {
			log.Error("error reading file to sign")
			return nil, cli.NewExitError(err.Error(), 1)
		}
	}

	return dataToSign, nil
}

func doCertSign(c CLIContext, toSign []byte) ([]byte, error) {
	self := c.GlobalString("self")
	if self == "" {
		return nil, fmt.Errorf("-self not provided")
	}

	wonkadPath := c.String("wonkad-path")
	if wonkadPath == "" {
		return nil, fmt.Errorf("-wonkad-path not provided")
	}

	req := wonka.WonkadRequest{
		Service: self,
		// should be the mesos task id. just the pid for now.
		TaskID: fmt.Sprintf("%d", os.Getpid()),
	}

	done := make(chan struct{})
	var repl wonka.WonkadReply
	var err error
	go func() {
		repl, err = wonkadRequest(wonkadPath, req)
		close(done)
	}()

	to := c.Duration("timeout")
	select {
	case <-done:
		if err != nil {
			return nil, err
		}
	case <-time.After(to):
		return nil, fmt.Errorf("timeout: %v", to.String())
	}

	cert, err := wonka.UnmarshalCertificate(repl.Certificate)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	privKey, err := x509.ParseECPrivateKey(repl.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("error reading private key: %v", err)
	}

	var certSigner = struct {
		Certificate wonka.Certificate
		Data        []byte
	}{*cert, toSign}

	toSign, err = json.Marshal(certSigner)
	if err != nil {
		return nil, fmt.Errorf("error marshalling to sign: %v", err)
	}

	sig, err := wonkacrypter.New().Sign(toSign, privKey)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	certSig := wonka.CertificateSignature{
		Certificate: *cert,
		Signature:   sig,
	}

	return json.Marshal(certSig)
}

func doSignData(c CLIContext) error {
	dataToSign, err := loadDataToSign(c)
	if err != nil {
		return err
	}

	w, err := c.NewWonkaClient(DefaultClient)
	if err != nil {
		return cli.NewExitError("", 1)
	}

	var sig []byte
	if c.Bool("cert") {
		sig, err = doCertSign(c, dataToSign)
	} else {
		sig, err = w.Sign(dataToSign)
	}

	if err != nil {
		zap.L().Error("error signing data", zap.Error(err))
		return cli.NewExitError(err.Error(), 1)
	}

	c.Writer().Write([]byte(base64.StdEncoding.EncodeToString(sig)))
	return nil
}

func doCertVerify(c CLIContext, dataToSign, sig []byte) error {
	var certSig wonka.CertificateSignature
	if err := json.Unmarshal(sig, &certSig); err != nil {
		return fmt.Errorf("error unmarshalling certificate signature: %v", err)
	}

	if err := certSig.Certificate.CheckCertificate(); err != nil {
		return fmt.Errorf("signing certificate is not valid: %v", err)
	}

	pubKey, err := certSig.Certificate.PublicKey()
	if err != nil {
		return fmt.Errorf("error reading publickey from cert: %v", err)
	}

	var certSigner = struct {
		Certificate wonka.Certificate
		Data        []byte
	}{certSig.Certificate, dataToSign}

	toVerify, err := json.Marshal(certSigner)
	if err != nil {
		return fmt.Errorf("error marshalling data to verify: %v", err)
	}

	if ok := wonkacrypter.New().Verify(toVerify, certSig.Signature, pubKey); !ok {
		return fmt.Errorf("signature doesn't verify")
	}

	certPath := c.StringOrFirstArg("cert-path")
	keyPath := c.StringOrFirstArg("key-path")

	if certPath != "" && keyPath != "" {
		if err := doRequestCertificate(c); err != nil {
			return fmt.Errorf("error requesting certificate from verify: %v", err)
		}
	}

	return nil
}

func doVerifySignature(c CLIContext) error {
	dataToSign, err := loadDataToSign(c)
	if err != nil {
		return err
	}

	sigStr := c.StringOrFirstArg("signature")
	sig, err := base64.StdEncoding.DecodeString(sigStr)
	if err != nil {
		return cli.NewExitError("", 1)
	}
	entity := c.StringOrFirstArg("signer")

	w, err := c.NewWonkaClient(DefaultClient)
	if err != nil {
		return cli.NewExitError("", 1)
	}

	if c.Bool("cert") {
		if err := doCertVerify(c, dataToSign, sig); err != nil {
			zap.L().Error("error verifying certificate", zap.Error(err))
			return cli.NewExitError(err.Error(), 1)
		}
	} else {
		if ok := w.Verify(c.Context(), dataToSign, sig, entity); !ok {
			return cli.NewExitError("invalid signature", 1)
		}
	}

	logrus.Info("signature valid")
	return nil
}

// SignData signs data with the entity private key.
func SignData(c *cli.Context) error {
	return doSignData(cliWrapper{inner: c})
}

// VerifySignature verifies that some bit of data was signed by
// a particular entity privat key.
func VerifySignature(c *cli.Context) error {
	return doVerifySignature(cliWrapper{inner: c})
}
