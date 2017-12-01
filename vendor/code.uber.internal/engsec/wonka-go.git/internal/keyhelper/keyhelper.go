package keyhelper

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"

	wonka "code.uber.internal/engsec/wonka-go.git"
)

// KeyHelper is a mockable interface for generating cryptographic keys.
// Real cryptographic keys are randomly generated., which is a real pain for
// testing.
type KeyHelper interface {
	RSAAndECC(string) (*rsa.PrivateKey, string, string, error)
	RSAFromFile(string) (*rsa.PrivateKey, error)
}

type keyHelper struct {
}

// New returns a real KeyHelper implementation.
func New() KeyHelper {
	return keyHelper{}
}

// RSAAndECC reads a file in PKCS1 format, and returns the rsa private key, the
// corresponding rsa public key, and the corresponding compressed ECC public key.
func (h keyHelper) RSAAndECC(keyPath string) (*rsa.PrivateKey, string, string, error) {
	rsaKey, err := h.RSAFromFile(keyPath)
	if err != nil {
		return nil, "", "", err
	}

	eccPriv := wonka.ECCFromRSA(rsaKey)
	eccPub := eccPriv.PublicKey

	rsaPub, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	if err != nil {
		return nil, "", "", fmt.Errorf("error marshalling rsa pubkey: %v", err)
	}

	return rsaKey, base64.StdEncoding.EncodeToString(rsaPub), wonka.KeyToCompressed(eccPub.X, eccPub.Y), nil
}

// RSAFromFile reads the given file and decodes the rsa private key pem it contains.
func (keyHelper) RSAFromFile(keyPath string) (*rsa.PrivateKey, error) {
	keyBytes, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	// PEM Decode the private key buffer into pem.Block object
	pemBlock, _ := pem.Decode(keyBytes)
	if pemBlock == nil {
		return nil, errors.New("failed to decode pem")
	}

	// Parse the RSA private key from the pem.Block object
	rsaKey, err := x509.ParsePKCS1PrivateKey(pemBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("error parsing rsa private key: %v", err)
	}

	return rsaKey, nil
}
