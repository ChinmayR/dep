package wonkacrypter_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"io"
	"math/big"
	"testing"

	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"github.com/stretchr/testify/require"
)

var encryptVars = []struct {
	badKey        bool
	badCipherText bool

	decryptErr bool
	msg        string
}{
	{badKey: false, decryptErr: false, msg: "good key should work"},
	{badKey: true, decryptErr: true, msg: "bad key should fail"},
	{badCipherText: true, decryptErr: true, msg: "bad key should fail"},
}

func TestCrypterEncryptDecrypt(t *testing.T) {
	for _, m := range encryptVars {
		withKeys(t, func(k1, k2 *ecdsa.PrivateKey) {
			data := make([]byte, 64)
			_, err := rand.Read(data)
			require.NoError(t, err, "reading data: %v", err)

			pubKey := k2.PublicKey
			if m.badKey {
				pubKey = k1.PublicKey
			}

			cipherText, err := wonkacrypter.New().Encrypt(data, k1, &pubKey)
			require.NoError(t, err, "encrypt: %v", err)
			require.False(t, bytes.Equal(data, []byte(cipherText)))

			if m.badCipherText {
				cipherText = []byte("foober")
			}

			plainText, err := wonkacrypter.New().Decrypt(cipherText, k2, &k1.PublicKey)
			if m.decryptErr {
				require.Error(t, err, "%s", m.msg)
			} else {
				require.NoError(t, err, "decrypt: %v", err)
				require.True(t, bytes.Equal(plainText, data))
			}
		})
	}
}

func BenchmarkCrypterEncrypt(b *testing.B) {
	withKeys(b, func(k1, k2 *ecdsa.PrivateKey) {
		data := make([]byte, 64)
		_, err := rand.Read(data)
		require.NoError(b, err, "reading data: %v", err)

		for i := 0; i < b.N; i++ {
			cipherText, err := wonkacrypter.New().Encrypt(data, k1, &k2.PublicKey)
			require.NoError(b, err, "encrypt: %v", err)
			require.False(b, bytes.Equal(data, []byte(cipherText)))
		}
	})
}

func BenchmarkCrypterDecrypt(b *testing.B) {
	withKeys(b, func(k1, k2 *ecdsa.PrivateKey) {
		data := make([]byte, 64)
		_, err := rand.Read(data)
		require.NoError(b, err, "reading data: %v", err)

		cipherText, err := wonkacrypter.New().Encrypt(data, k1, &k2.PublicKey)
		require.NoError(b, err, "encrypt: %v", err)
		require.False(b, bytes.Equal(data, []byte(cipherText)))

		for i := 0; i < b.N; i++ {
			plainText, err := wonkacrypter.New().Decrypt(cipherText, k2, &k1.PublicKey)
			require.NoError(b, err, "decrypt: %v", err)
			require.True(b, bytes.Equal(plainText, data))
		}
	})
}

var signVerifyArgs = []struct {
	invalidSig bool
	badSigLen  bool
	badKey     bool

	shouldErr bool
	msg       string
}{
	{invalidSig: true, shouldErr: true, msg: "invalid sig shouldn't verify"},
	{badSigLen: true, shouldErr: true, msg: "invalid sig length shouldn't verify"},
	{badKey: true, shouldErr: true, msg: "invalid key shouldn't verify"},
	{shouldErr: false, msg: "good sig should verify"},
}

func TestCrypterSignVerify(t *testing.T) {
	for _, m := range signVerifyArgs {
		withKeys(t, func(k1, k2 *ecdsa.PrivateKey) {
			data := make([]byte, 64)
			_, err := rand.Read(data)
			require.NoError(t, err, "reading data: %v", err)

			sig, err := wonkacrypter.New().Sign(data, k1)
			require.NoError(t, err, "sign: %v", err)

			if m.invalidSig {
				sig = []byte("foober")
			}

			if m.badSigLen {
				sig = []byte("foober")
			}

			if m.badKey {
				sig, err = wonkacrypter.New().Sign(data, k2)
				require.NoError(t, err, "re-signing for bad key test: %v", err)
			}

			ok := wonkacrypter.New().Verify(data, sig, &k1.PublicKey)
			require.True(t, ok != m.shouldErr, "%s", m.msg)
		})
	}
}

func TestCrypterVerifyAny(t *testing.T) {
	withKeys(t, func(k1, k2 *ecdsa.PrivateKey) {
		data := make([]byte, 64)
		_, err := rand.Read(data)
		require.NoError(t, err, "reading data: %v", err)

		sig, err := wonkacrypter.New().Sign(data, k1)
		require.NoError(t, err, "sign: %v", err)

		ok := wonkacrypter.VerifyAny(data, sig, []*ecdsa.PublicKey{&k2.PublicKey, &k1.PublicKey})
		require.True(t, ok, "should verify any")

		ok = wonkacrypter.VerifyAny(data, sig, []*ecdsa.PublicKey{&k2.PublicKey, &k2.PublicKey})
		require.False(t, ok, "should not verify any")
	})
}

func TestSharedSecretNilKeysShouldError(t *testing.T) {
	_, err := wonkacrypter.SharedSecret(nil, nil)
	require.Error(t, err, "shared secret with nil private key should error")

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generating key: %v", err)

	_, err = wonkacrypter.SharedSecret(key, nil)
	require.Error(t, err, "shared secret with nil public key should error")
}

func TestCrypterEncryptWithNilKeysShouldError(t *testing.T) {
	_, err := wonkacrypter.New().Encrypt([]byte("hello"), nil, nil)
	require.Error(t, err, "should not permit nil private key")

	withKeys(t, func(k1, _ *ecdsa.PrivateKey) {
		_, err := wonkacrypter.New().Encrypt([]byte("hello"), k1, nil)
		require.Error(t, err, "should not permit nil public key")
	})
}

func TestCrypterDecryptWithNilKeysShouldError(t *testing.T) {
	_, err := wonkacrypter.New().Decrypt([]byte("hello"), nil, nil)
	require.Error(t, err, "should not permit nil private key")

	withKeys(t, func(k1, _ *ecdsa.PrivateKey) {
		_, err := wonkacrypter.New().Decrypt([]byte("hello"), k1, nil)
		require.Error(t, err, "should not permit nil public key")
	})
}

func TestCrypterSigningWithNilKeyShouldError(t *testing.T) {
	_, err := wonkacrypter.New().Sign([]byte("hello"), nil)
	require.Error(t, err, "should not permit nil private key")
}

func TestCrypterVerifyingWithNilKeyShouldError(t *testing.T) {
	result := wonkacrypter.New().Verify([]byte("hello"), []byte("world"), nil)
	require.False(t, result, "should not permit nil public key")
}

func TestCrypterDecryptAny(t *testing.T) {
	withKeys(t, func(k1, k2 *ecdsa.PrivateKey) {
		data := make([]byte, 64)
		_, err := rand.Read(data)
		require.NoError(t, err, "reading data: %v", err)

		cipherText, err := wonkacrypter.New().Encrypt(data, k1, &k2.PublicKey)
		require.NoError(t, err, "encrypt: %v", err)
		require.False(t, bytes.Equal(data, []byte(cipherText)))

		plainText, err := wonkacrypter.DecryptAny(cipherText, k1, []*ecdsa.PublicKey{&k1.PublicKey, &k2.PublicKey})
		require.NoError(t, err, "decrypt: %v", err)
		require.True(t, bytes.Equal(plainText, data))

		plainText, err = wonkacrypter.DecryptAny(cipherText, k1, []*ecdsa.PublicKey{&k1.PublicKey, &k1.PublicKey})
		require.Error(t, err, "decrypt: %v", err)
		require.False(t, bytes.Equal(plainText, data))
	})
}

func BenchmarkCrypterSign(b *testing.B) {
	withKeys(b, func(k1, _ *ecdsa.PrivateKey) {
		data := make([]byte, 64)
		_, err := rand.Read(data)
		require.NoError(b, err, "reading data: %v", err)

		for i := 0; i < b.N; i++ {
			_, err := wonkacrypter.New().Sign(data, k1)
			require.NoError(b, err, "sign: %v", err)
		}
	})
}

func BenchmarkCrypterVerify(b *testing.B) {
	withKeys(b, func(k1, _ *ecdsa.PrivateKey) {
		data := make([]byte, 64)
		_, err := rand.Read(data)
		require.NoError(b, err, "reading data: %v", err)

		sig, err := wonkacrypter.New().Sign(data, k1)
		require.NoError(b, err, "sign: %v", err)
		for i := 0; i < b.N; i++ {
			ok := wonkacrypter.New().Verify(data, sig, &k1.PublicKey)
			require.True(b, ok, "verify shouldn't fail")
		}
	})
}

func TestSharedSecret(t *testing.T) {
	k1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generate key: %v", err)

	k2, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	require.NoError(t, err, "generate key: %v", err)

	_, err = wonkacrypter.SharedSecret(k1, &k2.PublicKey)
	require.Error(t, err, "invalid curves should fail")

	k2, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generate key: %v", err)

	_, err = wonkacrypter.SharedSecret(k1, &k2.PublicKey)
	require.NoError(t, err, "valid curves should not fail: %v", err)
}

func TestSharedSecretWhenBadKeysThenShouldError(t *testing.T) {
	k1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generate key: %v", err)

	k2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generate key: %v", err)

	// += 1 so it's probably no longer on the curve
	x := big.NewInt(1)
	k2.X.Add(k2.X, x)

	_, err = wonkacrypter.SharedSecret(k1, &k2.PublicKey)
	require.Error(t, err, "bad keys should fail")
}

func TestSharedSecretWhenSeedIsIdentityThenShouldError(t *testing.T) {
	k1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generate key: %v", err)

	k2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generate key: %v", err)

	// force 0 * (x, y) = (0, 0)
	// which is Inf according to https://golang.org/src/crypto/elliptic/elliptic.go#L85
	k1.D = big.NewInt(0)

	_, err = wonkacrypter.SharedSecret(k1, &k2.PublicKey)
	require.Error(t, err, "infinite seed should fail")
}

func TestEntityCrypter(t *testing.T) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "failed to generate test keypair")

	e := wonkacrypter.NewEntityCrypter(private)
	require.NotNil(t, e, "failed to create entity crypter")

	t.Run("encrypt/decrypt roundtrip", func(t *testing.T) {
		plaintext := make([]byte, 128)
		_, err := io.ReadFull(rand.Reader, plaintext)
		require.NoError(t, err, "failed to generate plaintext")

		ciphertext, err := e.Encrypt(plaintext, &private.PublicKey)
		require.NoError(t, err, "failed to encrypt")
		require.NotEqual(t, plaintext, ciphertext)

		round, err := e.Decrypt(ciphertext, &private.PublicKey)
		require.NoError(t, err, "failed to decrypt")
		require.Equal(t, plaintext, round)

		ciphertext[0]++
		_, err = e.Decrypt(ciphertext, &private.PublicKey)
		require.Error(t, err, "decrypt succeeded on mutated ciphertext")
	})
	t.Run("sign/verify roundtrip", func(t *testing.T) {
		data := make([]byte, 128)
		_, err := io.ReadFull(rand.Reader, data)
		require.NoError(t, err, "failed to generate data")

		sig, err := e.Sign(data)
		require.NoError(t, err, "failed to sign")
		require.Len(t, sig, 64)

		require.True(t, e.Verify(data, sig, &private.PublicKey), "verify failed")

		data[0]++
		require.False(t, e.Verify(data, sig, &private.PublicKey), "verify succeeded on mutated data")
	})
}

func TestNewEntityCrypter(t *testing.T) {
	t.Run("nil private key", func(t *testing.T) {
		ec := wonkacrypter.NewEntityCrypter(nil)
		require.Nil(t, ec, "expected a nil EntityCrypter when private key is nil")

		// Ensure the operations fails
		private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err, "failed to generate test keypair")

		b, err := ec.Encrypt(make([]byte, 10), &private.PublicKey)
		require.Empty(t, b)
		require.Error(t, err)

		b, err = ec.Sign(make([]byte, 10))
		require.Empty(t, b)
		require.Error(t, err)
	})
	t.Run("valid private key", func(t *testing.T) {
		private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err, "failed to generate test keypair")

		ec := wonkacrypter.NewEntityCrypter(private)
		require.NotNil(t, ec)
	})
}

func withKeys(t testing.TB, fn func(k1, k2 *ecdsa.PrivateKey)) {
	k1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "k1: %v", err)

	k2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "k2: %v", err)

	fn(k1, k2)
}
