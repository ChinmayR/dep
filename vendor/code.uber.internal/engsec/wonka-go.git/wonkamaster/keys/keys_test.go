package keys

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"strings"
	"testing"

	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func TestBadHash(t *testing.T) {
	err := VerifySignature(&rsa.PublicKey{}, "foo", "", "foober")
	require.Error(t, err, "empty hash algorithm should error")
	require.Equal(t, err.Error(), "unsupported hashing algorithm in VerifySignature: ''")

	err = VerifySignature(&rsa.PublicKey{}, "foo", "420", "foober")
	require.Error(t, err, "empty hash algorithm should error")
	require.Equal(t, err.Error(), "unsupported hashing algorithm in VerifySignature: '420'")
}

func TestRSAKey(t *testing.T) {
	k, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err, "generating key: %v", err)

	pubKey, err := ssh.NewPublicKey(&k.PublicKey)
	require.NoError(t, err, "ssh pubkey key: %v", err)

	rsaPub, err := RSAKeyFromSSH(pubKey)
	require.NoError(t, err, "rsa key: %v", err)

	k1, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	require.NoError(t, err, "marshal original key: %v", err)
	k2, err := x509.MarshalPKIXPublicKey(rsaPub)
	require.NoError(t, err, "marshal ssh key: %v", err)

	require.True(t, bytes.Equal(k1, k2), "keys should match")
}

func TestSigning(t *testing.T) {
	data := make([]byte, 64)
	n, err := rand.Read(data)
	require.NoError(t, err, "reading random data: %v", err)
	require.Equal(t, len(data), n, "wrong number of bytes read")
	toSign := base64.StdEncoding.EncodeToString(data)

	rsaKey := wonkatestdata.PrivateKey()
	eccKey := wonkatestdata.ECCKey()

	for _, algo := range []string{"SHA1", "SHA256"} {
		sig, err := SignData(rsaKey, algo, toSign)
		require.NoError(t, err, "signing: %v", err)

		err = VerifySignature(&rsaKey.PublicKey, string(sig), algo, toSign)
		require.NoError(t, err, "data should verify: %v", err)

		// verify ec signed stuff
		rawSig, err := wonkacrypter.New().Sign([]byte(toSign), eccKey)
		require.NoError(t, err, "signing: %v", err)

		sig = []byte(base64.StdEncoding.EncodeToString(rawSig))

		err = VerifySignature(&eccKey.PublicKey, string(sig), algo, toSign)
		require.NoError(t, err, "data should verify: %v", err)
	}

	_, err = SignData(rsaKey, "foo", toSign)
	require.Error(t, err, "invalid algorithm should err: %v", err)
	require.Contains(t, err.Error(), "unsupported hashing algorithm in SignData: 'foo'")

	err = VerifySignature(&rsaKey.PublicKey, "sig", "foo", toSign)
	require.Error(t, err, "invalid algorithm should err: %v", err)
	require.Contains(t, err.Error(), "unsupported hashing algorithm in VerifySignature: 'foo'")
}

func TestKeyHashing(t *testing.T) {
	log := zap.L()

	rsaKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err, "rsa key: %v", err)
	ret := KeyHash(rsaKey)
	require.NotEmpty(t, ret, "rsa key should hash")
	log.Info("rsa private key", zap.Any("hash", ret))

	ret = KeyHash(&rsaKey.PublicKey)
	require.NotEmpty(t, ret, "rsa public key should hash")
	log.Info("rsa public key", zap.Any("hash", ret))

	sshPub, err := ssh.NewPublicKey(&rsaKey.PublicKey)
	require.NoError(t, err)
	ret = KeyHash(sshPub)
	require.NotEmpty(t, ret, "ssh public key should hash")
	log.Info("ssh public key", zap.Any("hash", ret))

	eccKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generate ecdsa key: %v", err)
	ret = KeyHash(eccKey)
	require.NotEmpty(t, ret, "ecdsa public key should hash")
	log.Info("ecdsa private key", zap.Any("hash", ret))

	ret = KeyHash("foo")
	require.Empty(t, ret, "invalid key should not hash")
}

func TestVerifySignatureEmptySignatureShouldError(t *testing.T) {
	k, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err, "generating key: %v", err)

	err = VerifySignature(k.PublicKey, "", "", "")
	require.Error(t, err, "empty signature should error")
}

func TestParsePublicKeyInvalidPemShouldError(t *testing.T) {
	_, err := ParsePublicKey("invalid")
	require.Error(t, err, "invalid key should error")
}

func TestParsePublicKeyInvalidKeyShouldError(t *testing.T) {
	_, err := ParsePublicKey("dGVzdA==")
	require.Error(t, err, "invalid key should error")
}

func TestParsePublicKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err, "generating keys should not fail")

	pubKey, err := x509.MarshalPKIXPublicKey(key.Public())
	require.NoError(t, err, "public key should marshal")

	buf := new(bytes.Buffer)
	encoder := base64.NewEncoder(base64.StdEncoding, buf)
	encoder.Write(pubKey)
	encoder.Close()

	result, err := ParsePublicKey(buf.String())
	require.NoError(t, err, "valid key should parse")
	require.Equal(t, key.PublicKey, *result, "key should be equal")
}

func TestRSAPemBytes(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err, "generating keys should not fail")

	pubKey, err := x509.MarshalPKIXPublicKey(key.Public())
	require.NoError(t, err, "public key should marshal")

	buf := new(bytes.Buffer)
	encoder := base64.NewEncoder(base64.StdEncoding, buf)
	encoder.Write(pubKey)
	encoder.Close()

	encodedPem := RSAPemBytes(&key.PublicKey)
	noNewlines := strings.Replace(encodedPem, "\n", "", -1)
	require.Equal(t, buf.String(), noNewlines)
}
