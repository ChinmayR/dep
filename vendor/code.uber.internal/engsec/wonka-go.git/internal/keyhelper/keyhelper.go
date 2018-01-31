package keyhelper

import (
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"

	"code.uber.internal/engsec/wonka-go.git"
)

// KeyHelper is a mockable interface for generating cryptographic keys.
// Real cryptographic keys are randomly generated., which is a real pain for
// testing.
type KeyHelper interface {
	RSAAndECCFromFile(string) (*rsa.PrivateKey, string, string, error)
	RSAFromFile(string) (*rsa.PrivateKey, error)
}

type keyHelper struct {
}

// New returns a real KeyHelper implementation.
func New() KeyHelper {
	return keyHelper{}
}

// RSAAndECCFromFile reads a file in PKCS1 format, and returns the rsa private key, the
// corresponding rsa public key, and the corresponding compressed ECC public key.
func (h keyHelper) RSAAndECCFromFile(keyPath string) (*rsa.PrivateKey, string, string, error) {
	rsaKey, err := h.RSAFromFile(keyPath)
	if err != nil {
		return nil, "", "", err
	}

	eccPriv := wonka.ECCFromRSA(rsaKey)
	eccPub := eccPriv.PublicKey

	// TODO(T1397881): In some places we store public key with the PEM header and
	// footer. In other places, not. Standardize, then switch to
	// PublicPemFromKey.
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

// PublicPemFromKey converts the public key into pem format.
func PublicPemFromKey(k crypto.PublicKey) ([]byte, error) {
	b, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		return nil, fmt.Errorf("error marshalling public key: %v", err)
	}

	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: b,
	}
	return pem.EncodeToMemory(pemBlock), nil
}

// PrivatePemFromKey encodes an rsa private key into pem format.
func PrivatePemFromKey(k *rsa.PrivateKey) []byte {
	pemBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k),
	}
	return pem.EncodeToMemory(&pemBlock)
}

// WriteRsaPrivateKey writes the given private key to the given file location in
// pem format.
func WriteRsaPrivateKey(k *rsa.PrivateKey, loc string) error {
	return ioutil.WriteFile(loc, PrivatePemFromKey(k), 0400)
}

// WritePublicKey writes the publickey to loc in pem format.
func WritePublicKey(k crypto.PublicKey, loc string) error {
	b, err := PublicPemFromKey(k)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(loc, b, 0440)
}
