package cmd

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"io"
	"io/ioutil"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/sshhelper"

	"github.com/sirupsen/logrus"
	"github.com/uber-go/tally"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh/agent"
)

var sshdConfigPath = "/etc/ssh/sshd_config"

func saveP256ECDSAPrivateKeyToPem(privatekey *ecdsa.PrivateKey, out io.Writer) error {
	marshalledprivkey, err := x509.MarshalECPrivateKey(privatekey)

	if err != nil {
		return err
	}

	// This is equivalent the named curve for P256, but unfortunately asn1 package does not export it yet :-(
	// taking the trick from https://stackoverflow.com/questions/24022946/how-to-write-out-ecdsa-keys-using-golang-crypto
	secp256r1, err := asn1.Marshal(asn1.ObjectIdentifier{1, 2, 840, 10045, 3, 1, 7})
	if err != nil {
		return err
	}

	pem.Encode(out, &pem.Block{Type: "EC PARAMETERS", Bytes: secp256r1})
	pem.Encode(out, &pem.Block{Type: "EC PRIVATE KEY", Bytes: marshalledprivkey})

	return nil
}

func writeCertAndPrivateKey(cert *wonka.Certificate, key *ecdsa.PrivateKey, c CLIContext) error {
	keyPath := c.String("key-path")
	if keyPath == "" {
		return cli.NewExitError("no key path", 1)
	}

	certPath := c.String("cert-path")
	if certPath == "" {
		return cli.NewExitError("no cert path", 1)
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	key64 := base64.StdEncoding.EncodeToString(keyBytes)
	if err := ioutil.WriteFile(keyPath, []byte(key64), 0644); err != nil {
		return cli.NewExitError("error writing key", 1)
	}

	if x509Path := c.String("x509-path"); len(x509Path) > 0 {
		key, err := x509.ParseECPrivateKey(keyBytes)
		if err != nil {
			return cli.NewExitError("error parsing key", 1)
		}
		buf := new(bytes.Buffer)
		if err := saveP256ECDSAPrivateKeyToPem(key, buf); err != nil {
			return cli.NewExitError("error encoding x509 key", 1)
		}

		if err := ioutil.WriteFile(x509Path, []byte(buf.String()), 0644); err != nil {
			return cli.NewExitError("error writing x509 key", 1)
		}
	}

	certBytes, err := wonka.MarshalCertificate(*cert)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	if err := ioutil.WriteFile(certPath, certBytes, 0644); err != nil {
		return cli.NewExitError("error writing certificate", 1)
	}

	return nil
}

func doRequestCertificate(c CLIContext) error {
	usshCert, usshKey, err := sshhelper.UsshHostCert(zap.L(), sshdConfigPath)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	a := agent.NewKeyring()
	ak := agent.AddedKey{
		PrivateKey:  usshKey,
		Certificate: usshCert,
	}

	if err := a.Add(ak); err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	host := usshCert.ValidPrincipals[0]
	cfg := wonka.Config{
		EntityName: host,
		Agent:      a,
		Logger:     zap.L(),
		Metrics:    tally.NoopScope,
	}

	w, err := c.NewWonkaClientFromConfig(cfg)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	cert, privKey, err := wonka.NewCertificate(wonka.CertEntityName(host),
		wonka.CertHostname(host))
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	err = w.CertificateSignRequest(c.Context(), cert, nil)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	err = writeCertAndPrivateKey(cert, privKey, c)
	if err == nil {
		logrus.Info("successfully requested certificate")
	}

	return err
}

// RequestCertificate makes a certificate csr.
func RequestCertificate(c *cli.Context) error {
	return doRequestCertificate(cliWrapper{inner: c})
}
