package keys

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// Functions for dealing with keys.
// TODO(pmoody): make these work with other key formats.

// ParsePublicKey turns the PublicKey we store with entities into an rsa publickey.
func ParsePublicKey(pubKey string) (*rsa.PublicKey, error) {
	var b bytes.Buffer
	b.WriteString("-----BEGIN PUBLIC KEY-----\n")
	b.WriteString(pubKey)
	b.WriteString("\n-----END PUBLIC KEY-----\n")

	p, _ := pem.Decode(b.Bytes())
	if p == nil {
		return nil, fmt.Errorf("not a pem encoded public key")
	}

	k, err := x509.ParsePKIXPublicKey(p.Bytes)
	if err != nil {
		return nil, fmt.Errorf("public key can't be decoded: %v", err)
	}

	// Convert public_key to rsa.PublicKey
	rsaKey, ok := k.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an rsa key")
	}

	return rsaKey, nil
}

// RSAPemBytes returns just the pem encoded bytes of a given rsa public key.
// TODO(pmoody): make this work with other key types.
func RSAPemBytes(k *rsa.PublicKey) string {
	keyBytes, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		panic(err)
	}
	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: keyBytes,
	}
	keyString := strings.TrimPrefix(string(pem.EncodeToMemory(pemBlock)), "-----BEGIN PUBLIC KEY-----\n")
	return strings.TrimSuffix(keyString, "\n-----END PUBLIC KEY-----\n")
}

// RSAKeyFromSSH pulls the rsa public key out of an ssh-rsa key
func RSAKeyFromSSH(k ssh.PublicKey) (*rsa.PublicKey, error) {
	var w struct {
		Name string
		E    *big.Int
		N    *big.Int
	}

	if err := ssh.Unmarshal(k.Marshal(), &w); err != nil {
		return nil, err
	}

	return &rsa.PublicKey{
		N: w.N,
		E: int(w.E.Int64()),
	}, nil
}

// SignData signs toSign with the given private key using the given hashing algorithm.
func SignData(privKey *rsa.PrivateKey, hashType, toSign string) ([]byte, error) {
	var hasher crypto.Hash
	switch hashType {
	case "SHA1":
		hasher = crypto.SHA1
	case "SHA256":
		hasher = crypto.SHA256
	default:
		return nil, fmt.Errorf("unsupported hashing algorithm in SignData: '%s'", hashType)
	}

	mhash := hasher.New()
	mhash.Write([]byte(toSign))
	pkhash := mhash.Sum(nil)

	sigBytes, err := privKey.Sign(rand.Reader, pkhash, hasher)
	if err != nil {
		return nil, err
	}
	return []byte(base64.StdEncoding.EncodeToString(sigBytes)), nil
}

// VerifySignature verifys that the signature 'sig' with hash algo 'sigtype'
// was created on the data 'toVerify' by the private portion of the give pubKey.
func VerifySignature(pubKey crypto.PublicKey, sig, hashType, toVerify string) error {
	if sig == "" {
		return fmt.Errorf("empty signature")
	}

	var hasher crypto.Hash
	switch hashType {
	case "SHA1":
		hasher = crypto.SHA1
	case "SHA256":
		hasher = crypto.SHA256
	default:
		return fmt.Errorf("unsupported hashing algorithm in VerifySignature: '%s'", hashType)
	}

	h := hasher.New()
	h.Write([]byte(toVerify))
	pkhash := h.Sum(nil)

	// Decode the base64 claim signature string to a byte array
	sigbytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("signature decoding error: %v", err)
	}

	switch t := pubKey.(type) {
	case *rsa.PublicKey:
		// Verify the claim/entity signature with RSA_Verify*()
		if err := rsa.VerifyPKCS1v15(t, hasher, pkhash, sigbytes); err != nil {
			return fmt.Errorf("claim signature check failed: %v", err)
		}
	case *ecdsa.PublicKey:
		if ok := wonkacrypter.New().Verify([]byte(toVerify), sigbytes, t); !ok {
			return errors.New("ec signature check failed")
		}
	}

	return nil
}

// KeyHash returns the SHA256 hash of the given key
func KeyHash(key interface{}) string {
	var b []byte
	var e error

	switch k := key.(type) {
	case *rsa.PrivateKey:
		b = x509.MarshalPKCS1PrivateKey(k)
	case *rsa.PublicKey:
		b, e = x509.MarshalPKIXPublicKey(k)
	case *ecdsa.PrivateKey:
		b, e = x509.MarshalECPrivateKey(k)
	case *ecdsa.PublicKey:
		b = elliptic.Marshal(elliptic.P256(), k.X, k.Y)
	case ssh.PublicKey:
		b = ssh.PublicKey(k).Marshal()
	default:
		e = fmt.Errorf("unknown key type: %v", reflect.TypeOf(k).String())
	}

	if e != nil || len(b) == 0 {
		// TODO(abg): Inject logger here
		zap.L().Error("error hashing key", zap.Error(e))
		return ""
	}

	return SHA256Hash(b)
}

// SHA256Hash returns the base64 encoded SHA256 sum of a given set of bytes.
func SHA256Hash(b []byte) string {
	h := sha256.New()
	h.Write(b)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
