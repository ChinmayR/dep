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
	"golang.org/x/crypto/ssh"

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
	certPath := c.String("certificate")
	if certPath == "" {
		return errors.New("no signing certificate specified")
	}

	keyPath := c.String("key")
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

// doCertGrantingCert turns this launch request into a cert-granting certificate.
func doCertGrantingCert(c CLIContext, toCGC string) error {
	cgcBytes, err := base64.StdEncoding.DecodeString(toCGC)
	if err != nil {
		return fmt.Errorf("error base64 decoding request: %v", err)
	}

	var cgcReq wonka.CertificateSignature
	if err := json.Unmarshal(cgcBytes, &cgcReq); err != nil {
		return fmt.Errorf("error unmarshalling signature: %v", err)
	}

	var lr wonka.LaunchRequest
	if err := json.Unmarshal(cgcReq.Data, &lr); err != nil {
		return fmt.Errorf("error parsing launch request: %v", err)
	}

	taskID := lr.TaskID
	if taskID == "" {
		taskID = lr.InstID
	}
	if taskID == "" {
		taskID = "no task id for this service"
	}

	sshAgent, hostName, err := usshHostAgent()
	if err != nil {
		return fmt.Errorf("error getting host ussh cert: %v", err)
	}

	keys, err := sshAgent.List()
	if err != nil {
		return fmt.Errorf("error listing keys task agent: %v", err)
	}

	if len(keys) != 1 {
		return fmt.Errorf("task ssh agent should only have one key, got %d", len(keys))
	}

	pubKey, err := ssh.ParsePublicKey(keys[0].Blob)
	if err != nil {
		return fmt.Errorf("error parsing key: %v", err)
	}
	usshCert := ssh.MarshalAuthorizedKey(pubKey)

	cert, privKey, err := wonka.NewCertificate(
		wonka.CertLaunchRequestTag(toCGC),
		wonka.CertEntityName(lr.SvcID),
		wonka.CertHostname(hostName),
		wonka.CertTaskIDTag(taskID),
		wonka.CertUSSHCertTag(string(usshCert)))

	if err != nil {
		return fmt.Errorf("error generating key and cert: %v", err)
	}

	certToSign, err := wonka.MarshalCertificate(*cert)
	if err != nil {
		return fmt.Errorf("error marshalling cert to sign: %v", err)
	}

	sshSig, err := sshAgent.Sign(pubKey, certToSign)
	if err != nil {
		return fmt.Errorf("error signing cert with ssh host key: %v", err)
	}
	cert.Signature = ssh.Marshal(sshSig)

	if err := writeCertificate(cert, c.String("certificate")); err != nil {
		return err
	}

	return writePrivateKey(privKey, c.String("key"))
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
	certPath := c.String("certificate")
	keyPath := c.String("key")
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
	toSign := c.String("sign")
	toVerify := c.String("verify")
	toCGC := c.String("cgc")

	args := 0
	for _, a := range []string{toSign, toVerify, toCGC} {
		if a != "" {
			args++
		}
	}

	if args != 1 {
		return errors.New("only one of sign, verify or cgc can be set")
	}

	if toSign != "" {
		return doSign(c, toSign)
	} else if toVerify != "" {
		return doVerify(c, toVerify)
	} else if toCGC != "" {
		return doCertGrantingCert(c, toCGC)
	}

	return errors.New("wut")
}

// Task implements the subcommands for signing/verifying task launch requests.
func Task(c *cli.Context) error {
	if err := doTask(cliWrapper{inner: c}); err != nil {
		return cli.NewExitError(err, 1)
	}
	return nil
}
