package wonka_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path"
	"testing"

	wonka "code.uber.internal/engsec/wonka-go.git"
	. "code.uber.internal/engsec/wonka-go.git/testdata"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	Alice = "wonkaSample:alice"
	Bob   = "wonkaSample:bob"
)

var signVars = []struct {
	from string

	badSig       bool
	badSigLength bool
	verifies     bool
}{
	{verifies: true},
	{badSig: true, verifies: false},
	{from: Bob, verifies: false},
	{from: Bob, badSigLength: true, verifies: false},
}

func TestCryptoSign(t *testing.T) {
	for idx, m := range signVars {
		setupWonka(t, func(alice, bob wonka.Wonka) {
			data := make([]byte, 64)
			_, err := rand.Read(data)
			require.NoError(t, err, "%d rand read err: %v", idx, err)

			sig, err := alice.Sign(data)
			require.NoError(t, err, "%d sign error: %v", idx, err)

			if m.badSig {
				sig = []byte("foober")
			}

			if m.badSigLength {
				sig = []byte("foober")
			}

			from := Alice
			if m.from != "" {
				from = m.from
			}
			ok := bob.Verify(context.Background(), data, sig, from)
			require.Equal(t, m.verifies, ok, "test %d verify error", idx)
		})
	}
}
func BenchmarkCryptoEncrypt(b *testing.B) {
	defer zap.ReplaceGlobals(zap.NewNop())()
	setupWonka(b, func(alice, bob wonka.Wonka) {
		data := make([]byte, 1024)
		_, err := rand.Read(data)
		require.NoError(b, err, "reading random data: %v", err)

		ctx := context.Background()
		// prime the key loading
		_, err = alice.Encrypt(ctx, data, bob.EntityName())
		require.NoError(b, err, "encrypt error: %v", err)

		for i := 0; i < b.N; i++ {
			_, err = alice.Encrypt(ctx, data, bob.EntityName())
			require.NoError(b, err, "encrypt error: %v", err)
		}
	})
}

func BenchmarkCryptoDecrypt(b *testing.B) {
	defer zap.ReplaceGlobals(zap.NewNop())()
	setupWonka(b, func(alice, bob wonka.Wonka) {
		ctx := context.Background()
		data := make([]byte, 1025)
		_, err := rand.Read(data)
		require.NoError(b, err, "reading random data: %v", err)
		cipherText, err := alice.Encrypt(ctx, data, bob.EntityName())
		require.NoError(b, err, "encrypt error: %v", err)

		// prime this once
		plainText, err := bob.Decrypt(ctx, cipherText, alice.EntityName())
		require.NoError(b, err, "decrypt error: %v", err)
		require.True(b, bytes.Equal(data, plainText), "text should be equal")

		for i := 0; i < b.N; i++ {
			plainText, err := bob.Decrypt(ctx, cipherText, alice.EntityName())
			require.NoError(b, err, "decrypt error: %v", err)
			require.True(b, bytes.Equal(data, plainText), "text should be equal")
		}
	})
}

func BenchmarkCryptoSign(b *testing.B) {
	defer zap.ReplaceGlobals(zap.NewNop())()
	setupWonka(b, func(alice, _ wonka.Wonka) {
		data := make([]byte, 1024)
		_, err := rand.Read(data)
		require.NoError(b, err, "reading random data: %v", err)

		for i := 0; i < b.N; i++ {
			_, err = alice.Sign(data)
			require.NoError(b, err, "signing data: %v", err)
		}
	})
}

func BenchmarkCryptoVerify(b *testing.B) {
	defer zap.ReplaceGlobals(zap.NewNop())()
	setupWonka(b, func(alice, _ wonka.Wonka) {
		data := make([]byte, 1024)
		_, err := rand.Read(data)
		require.NoError(b, err, "reading random data: %v", err)
		sig, err := alice.Sign(data)
		require.NoError(b, err, "signing data: %v", err)

		for i := 0; i < b.N; i++ {
			ok := alice.Verify(context.Background(), data, sig, alice.EntityName())
			require.True(b, ok, "signature should verify")
		}
	})
}

var encryptVars = []struct {
	to   string
	from string

	cipherErr  bool
	encryptErr bool
	decryptErr bool
	errMsg     string
}{
	{to: Bob, from: Alice},
	{to: Alice, from: Alice, decryptErr: true},
	{to: "wonkaSample:NoSuchUser", encryptErr: true},
}

func TestCryptoEncrypt(t *testing.T) {
	for idx, m := range encryptVars {
		setupWonka(t, func(alice, bob wonka.Wonka) {
			ctx := context.Background()
			data := make([]byte, 64)
			_, err := rand.Read(data)
			require.NoError(t, err, "%d rand read err: %v", idx, err)

			to := Bob
			if m.to != "" {
				to = m.to
			}
			cipherText, err := alice.Encrypt(ctx, data, to)
			require.Equal(t, !m.encryptErr, err == nil, "%d, encrypt %v", idx, err)
			if !m.encryptErr {
				from := Alice
				if m.from != "" {
					from = m.from
				}
				plainText, err := bob.Decrypt(ctx, cipherText, from)
				require.Equal(t, !m.decryptErr, err == nil, "%d, decrypt %v", idx, err)
				if !m.decryptErr {
					require.True(t, bytes.Equal(data, plainText), "%d text should be equal", idx)
				}
			}
		})
	}
}

// setupWonka sets up a wonkamaster instance and enrolls two entities, alice and bob,
// suitable for communicating with each other like any two wonka entities.
func setupWonka(t testing.TB, fn func(alice, bob wonka.Wonka)) {
	os.Unsetenv("SSH_AUTH_SOCK")
	WithTempDir(func(dir string) {
		alicePrivPem := path.Join(dir, "alice.private.pem")
		aliceK := PrivateKeyFromPem(RSAPrivKey)
		err := WritePrivateKey(aliceK, alicePrivPem)
		require.NoError(t, err, "error writing alice private %v", err)

		bobPrivPem := path.Join(dir, "bob.private.pem")
		bobK := PrivateKeyFromPem(RSAPriv2)
		err = WritePrivateKey(bobK, bobPrivPem)
		require.NoError(t, err, "error writing bob private %v", err)

		wonkatestdata.WithWonkaMaster("wonkaSample:test", func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)
			ctx := context.TODO()

			aliceEntity := wonka.Entity{
				EntityName:   "wonkaSample:alice",
				PublicKey:    string(PublicPemFromKey(aliceK)),
				ECCPublicKey: ECCPublicFromPrivateKey(aliceK),
			}
			err := handlerCfg.DB.Create(ctx, &aliceEntity)
			require.NoError(t, err, "create alice failed")

			aliceCfg := wonka.Config{
				EntityName:     "wonkaSample:alice",
				PrivateKeyPath: alicePrivPem,
			}
			alice, err := wonka.Init(aliceCfg)
			require.NoError(t, err, "alice wonka init error: %v", err)

			bobEntity := wonka.Entity{
				EntityName:   "wonkaSample:bob",
				PublicKey:    string(PublicPemFromKey(bobK)),
				ECCPublicKey: ECCPublicFromPrivateKey(bobK),
			}
			err = handlerCfg.DB.Create(ctx, &bobEntity)
			require.NoError(t, err, "create bob failed")

			bobCfg := wonka.Config{
				EntityName:     "wonkaSample:bob",
				PrivateKeyPath: bobPrivPem,
			}
			bob, err := wonka.Init(bobCfg)
			require.NoError(t, err, "bob wonka init error: %v", err)

			// now run our test
			fn(alice, bob)

			// cleanup
			handlerCfg.DB.Delete(ctx, aliceEntity.Name())
			handlerCfg.DB.Delete(ctx, bobEntity.Name())
		})
	})
}
