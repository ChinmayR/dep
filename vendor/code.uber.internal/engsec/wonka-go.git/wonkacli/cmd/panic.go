package cmd

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/keyhelper"
	"code.uber.internal/engsec/wonka-go.git/internal/url"
	"code.uber.internal/engsec/wonka-go.git/redswitch"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func performSignDisableMessage(c CLIContext) error {
	exp := c.Duration("expiration")
	if exp <= 0 {
		return cli.NewExitError(errors.New("expiration duration must be greater than 0"), 1)
	}
	msg := redswitch.DisableMessage{
		Ctime:      time.Now().Unix(),
		Etime:      time.Now().Add(exp).Unix(),
		IsDisabled: true,
	}

	toSign, err := json.Marshal(msg)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	//privKey, ecPriv/ec
	privKey, err := keyhelper.New().RSAFromFile(c.GlobalString("private-key"))
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	ecPriv := wonka.ECCFromRSA(privKey)
	sig, err := wonkacrypter.New().Sign(toSign, ecPriv)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	msg.Signature = sig
	toPrint, err := json.Marshal(msg)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	pubKey, err := publicKeyHash(&ecPriv.PublicKey)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	logrus.WithField("pubkey_hash", pubKey).Info()

	if c.GlobalBool("json") {
		c.Writer().Write(toPrint)
	} else {
		c.Writer().Write([]byte(base64.StdEncoding.EncodeToString(toPrint)))
	}

	return nil
}

// SignDisableMessage signs a disable message with the globally supplied rsa private key.
func SignDisableMessage(c *cli.Context) error {
	return performSignDisableMessage(cliWrapper{inner: c})
}

func publicKeyHash(pubKey *ecdsa.PublicKey) (string, error) {
	b, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("error marshalling publickey: %v", err)
	}

	h := crypto.SHA256.New()
	h.Write(b)

	return url.Base32WithoutPadding(h.Sum(nil)), nil
}
