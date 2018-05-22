package cmd

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"

	wonka "code.uber.internal/engsec/wonka-go.git"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func readCertAndKey(certPath, keyPath string) (*wonka.Certificate, *ecdsa.PrivateKey, error) {
	certBytes, err := ioutil.ReadFile(certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading certificate: %v", err)
	}

	cert, err := wonka.UnmarshalCertificate(certBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	keyB64, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading key: %v", err)
	}

	keyBytes, err := base64.StdEncoding.DecodeString(string(keyB64))
	if err != nil {
		return nil, nil, fmt.Errorf("error decoding private key: %v", err)
	}

	key, err := x509.ParseECPrivateKey(keyBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing private key: %v", err)
	}

	return cert, key, nil
}

func doSign(c CLIContext, launchReq string) error {
	certPath := c.StringOrFirstArg("certificate")
	if certPath == "" {
		return errors.New("no signing certificate specified")
	}

	keyPath := c.StringOrFirstArg("key")
	if keyPath == "" {
		return errors.New("no signing key specified")
	}

	cert, key, err := readCertAndKey(certPath, keyPath)
	if err != nil {
		return err
	}

	certSignature, err := wonka.NewCertificateSignature(*cert, key, []byte(launchReq))
	if err != nil {
		return err
	}

	toPrint, err := json.Marshal(certSignature)
	if err != nil {
		return fmt.Errorf("error marshalling signature for printing: %v", err)
	}

	fmt.Println(base64.StdEncoding.EncodeToString(toPrint))
	return nil
}

func doVerify(c CLIContext, toVerify string) error {
	toVerifyBytes, err := base64.StdEncoding.DecodeString(toVerify)
	if err != nil {
		return fmt.Errorf("unable to parse signature: %v", err)
	}

	var certSignature wonka.CertificateSignature
	if err := json.Unmarshal(toVerifyBytes, &certSignature); err != nil {
		return fmt.Errorf("unable to unmarshal signature: %v", err)
	}

	if err := wonka.VerifyCertificateSignature(certSignature); err != nil {
		return err
	}

	logrus.Info("signature verifies, generating new certificate")
	certPath := c.StringOrFirstArg("certificate")
	keyPath := c.StringOrFirstArg("key")
	if certPath == "" || keyPath == "" {
		logrus.Info("no cert and/or key requested, exiting")
		return nil
	}

	var req wonka.LaunchRequest
	if err := json.Unmarshal(certSignature.Data, &req); err != nil {
		return fmt.Errorf("error unmarshalling launch request: %v", err)
	}

	logrus.WithFields(logrus.Fields{
		"service": req.SvcID,
		"taskid":  req.TaskID,
		"host":    req.Hostname,
	}).Info("request a new certificate")

	// here we will request a certificate
	cert, key, err := wonka.NewCertificate(wonka.CertEntityName(req.SvcID),
		wonka.CertHostname(req.Hostname), wonka.CertTaskIDTag(req.TaskID))
	if err != nil {
		return fmt.Errorf("error generating new certificate: %v", err)
	}

	w, err := c.NewWonkaClient(TaskLauncherClient)
	if err != nil {
		return fmt.Errorf("error creating wonka client: %v", err)
	}

	if err := w.CertificateSignRequest(context.Background(), cert, &certSignature); err != nil {
		return fmt.Errorf("error signing certificate: %v", err)
	}

	certBytes, err := wonka.MarshalCertificate(*cert)
	if err != nil {
		return fmt.Errorf("eror marshalling certificate to write: %v", err)
	}

	if err := ioutil.WriteFile(certPath, certBytes, 0444); err != nil {
		return fmt.Errorf("error writing certificate: %v", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("error marshalling private key: %v", err)
	}

	if err := ioutil.WriteFile(keyPath,
		[]byte(base64.StdEncoding.EncodeToString(keyBytes)), 0444); err != nil {
		return fmt.Errorf("error writing private key: %v", err)
	}

	return nil
}

func doTask(c CLIContext) error {
	toSign := c.StringOrFirstArg("sign")
	toVerify := c.StringOrFirstArg("verify")
	if toSign != "" && toVerify != "" {
		return fmt.Errorf("cannot specify both -sign and -verify")
	}

	if toSign != "" {
		return doSign(c, toSign)
	}

	return doVerify(c, toVerify)
}

// Task implements the subcommands for signing/verifying task launch requests.
func Task(c *cli.Context) error {
	if err := doTask(cliWrapper{inner: c}); err != nil {
		return cli.NewExitError(err, 1)
	}
	return nil
}
