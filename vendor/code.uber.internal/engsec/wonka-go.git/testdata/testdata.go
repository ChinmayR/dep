package testdata

import (
	"context"
	"crypto"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
)

var (
	toSign     = make(chan []byte, 1)
	signedData = make(chan []byte, 1)
)

func PrivateKeyFromPem(s string) *rsa.PrivateKey {
	p, _ := pem.Decode([]byte(s))
	if p == nil {
		panic("empty pem block")
	}
	k, err := x509.ParsePKCS1PrivateKey(p.Bytes)
	if err != nil {
		panic(err)
	}
	return k
}

func WithTempDir(fn func(dir string)) {
	dir, err := ioutil.TempDir("", "wonka")
	if err != nil {
		panic(err)
	}

	defer os.RemoveAll(dir)
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	defer os.Chdir(cwd)
	os.Chdir(dir)

	fn(dir)
}

func signData(ctx context.Context, privKey *rsa.PrivateKey) {
	for {
		select {
		case <-ctx.Done():
			return
		case i := <-toSign:
			sigBytes, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, i)
			if err != nil {
				panic(err)
			}
			signedData <- sigBytes
		}
	}
}

func writeCertificate(k *rsa.PrivateKey, loc string) error {
	t := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "foo",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(1 * time.Minute),

		SubjectKeyId:          []byte{1, 2, 3, 4},
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	cert, e := x509.CreateCertificate(rand.Reader, &t, &t, k.Public(), k)
	if e != nil {
		panic(fmt.Sprintf("fak: %v", e))
	}

	certPem := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	}

	return ioutil.WriteFile(loc, pem.EncodeToMemory(certPem), 0440)
}

func WritePublicKey(k crypto.PublicKey, loc string) error {
	b, e := x509.MarshalPKIXPublicKey(k)
	if e != nil {
		return e
	}

	pemBlock := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: b,
	}

	e = ioutil.WriteFile(loc, pem.EncodeToMemory(&pemBlock), 0440)
	if e != nil {
		return e
	}
	return nil
}

func WritePrivateKey(k *rsa.PrivateKey, loc string) error {
	pemBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k),
	}
	e := ioutil.WriteFile(loc, pem.EncodeToMemory(&pemBlock), 0440)
	if e != nil {
		return e
	}
	return nil
}

func PublicPemFromKey(k *rsa.PrivateKey) []byte {
	b, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		panic(err)
	}

	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: b,
	}
	return pem.EncodeToMemory(pemBlock)
}

func ECCPublicFromPrivateKey(k *rsa.PrivateKey) string {
	h := crypto.SHA256.New()
	h.Write([]byte(x509.MarshalPKCS1PrivateKey(k)))
	pointKey := h.Sum(nil)

	return wonka.KeyToCompressed(elliptic.P256().ScalarBaseMult(pointKey))
}
