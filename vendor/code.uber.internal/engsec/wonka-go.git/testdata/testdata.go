package testdata

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"testing"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/keyhelper"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"

	"github.com/stretchr/testify/require"
)

var (
	toSign     = make(chan []byte, 1)
	signedData = make(chan []byte, 1)
)

// EnrollEntity creates a database entry for an entity with the given name and
// private key.
func EnrollEntity(ctx context.Context, t testing.TB, db wonkadb.EntityDB, name string, privkey *rsa.PrivateKey) {
	privPem, err := keyhelper.PublicPemFromKey(&privkey.PublicKey)
	require.NoError(t, err, "failed to encode private key for %q to pem", name)
	entity := wonka.Entity{
		EntityName:   name,
		PublicKey:    string(privPem),
		ECCPublicKey: ECCPublicFromPrivateKey(privkey),
		Ctime:        int(time.Now().Unix()),
		Etime:        int(time.Now().Add(time.Hour).Unix()),
	}
	err = db.Create(ctx, &entity)
	require.NoError(t, err, "failed to enroll %q", name)
}

// PrivateKeyFromPem decodes a pem string into an rsa private key.
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

// WithTempDir runs function in an ephemeral directory and cleans up after itself.
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

// WritePublicKey writes the publickey to loc in pem format.
// This is part of Wonka's public API so we can't remove it, and we
// do want to have only one implementation.
var WritePublicKey = keyhelper.WritePublicKey

// WritePrivateKey writes the given private key to the given file location in
// pem format. This is part of Wonka's public API so we can't remove it, and we
// do want to have only one implementation.
var WritePrivateKey = keyhelper.WriteRsaPrivateKey

// PublicPemFromKey extracts the public key from an rsa private key and encodes
// it to pem format.
// This is part of Wonka's public API so we can't remove it, and we
// do want to have only one implementation.
// Deprecated: Avoid this function because it panics. Prefer wonkatestdata.PublicPemFromKey
func PublicPemFromKey(k *rsa.PrivateKey) []byte {
	b, err := keyhelper.PublicPemFromKey(&k.PublicKey)
	if err != nil {
		panic(err)
	}
	return b
}

// ECCPublicFromPrivateKey turns an rsa private key into a compressed
// ecdsa public key on the p256 curve. This is mostly used to make it easier
// to do things like create test entities.
func ECCPublicFromPrivateKey(k *rsa.PrivateKey) string {
	eccKey := wonka.ECCFromRSA(k)
	return wonka.KeyToCompressed(eccKey.PublicKey.X, eccKey.PublicKey.Y)
}
