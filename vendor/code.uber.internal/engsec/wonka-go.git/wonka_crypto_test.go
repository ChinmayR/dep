package wonka

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"io"
	"math/rand"
	"testing"

	"go.uber.org/zap"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

var mockRandSeed int64 = 0xdeadbeef

func TestLoadSaveKey(t *testing.T) {
	key := newECDSAKey(t)
	w := newUberWonkaWithSavedKey(t, key, "foo")
	pubkey, err := w.loadPubKeyFromCache("foo")
	require.NoError(t, err)
	require.Equal(t, &key.PublicKey, pubkey)
}

func newUberWonkaWithSavedKey(t *testing.T, privkey *ecdsa.PrivateKey, entity string) *uberWonka {
	w := &uberWonka{
		log:        zap.L(),
		cachedKeys: make(map[string]entityKey),
		clientECC:  newECDSAKey(t),
	}
	w.saveKey(&privkey.PublicKey, entity)
	return w
}

func TestEncryptDecrypt(t *testing.T) {
	ctx := context.Background()
	key := newECDSAKey(t)
	entity := "foo"
	plaintext := []byte("input")
	w := newUberWonkaWithSavedKey(t, key, entity)

	ciphertext, err := w.Encrypt(ctx, plaintext, entity)
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)
	require.NotEqual(t, plaintext, ciphertext)

	output, err := w.Decrypt(ctx, ciphertext, entity)
	require.NoError(t, err)
	require.Equal(t, plaintext, output)
}

func TestSignVerify(t *testing.T) {
	key := newECDSAKey(t)
	entity := "foo"
	input := []byte("input")
	w := newUberWonkaWithSavedKey(t, key, entity)

	signature, err := w.Sign(input)
	require.NoError(t, err)

	verified := w.Verify(context.Background(), input, signature, entity)
	require.True(t, verified)
}

func TestKeyHash(t *testing.T) {
	tests := []struct {
		description string
		key         interface{}
		output      string
	}{
		{
			description: "rsa private key",
			output:      "VpgpuyofH9IYSNEPxi31Yfw5QZSURrblH5pgzfgiThw=",
			key:         newRSAKey(t),
		},
		{
			description: "rsa public key",
			output:      "g7Bfeo/n4VoAW/USfQQaLLLxwMpqlV2iWUw7osvilh8=",
			key:         newRSAPubKey(t),
		},
		{
			description: "ssh public key",
			output:      "vRJJ3iHMzroGYrOxwwZj35XM+ybLrQU+WM5Cwr0edqo=",
			key:         newSSHPubKey(t),
		},
		{
			description: "ecdsa private key",
			output:      "lpuDBGS98itmjmAnoRO5XCC8QfNwIGOvDWeUiSTYdCs=",
			key:         newECDSAKey(t),
		},
		{
			description: "invalid key",
			output:      "",
			key:         "foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			val := KeyHash(tt.key)
			require.Equal(t, tt.output, val)
		})
	}
}

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	privkey, err := rsa.GenerateKey(newMockRandReader(), 1024)
	require.NoError(t, err)
	return privkey
}

func newRSAPubKey(t *testing.T) crypto.PublicKey {
	privkey := newRSAKey(t)
	return privkey.Public()
}

func newSSHPubKey(t *testing.T) *ssh.PublicKey {
	sshPubkey, err := ssh.NewPublicKey(newRSAPubKey(t))
	require.NoError(t, err)
	return &sshPubkey
}

func newECDSAKey(t *testing.T) *ecdsa.PrivateKey {
	privkey, err := ecdsa.GenerateKey(elliptic.P256(), newMockRandReader())
	require.NoError(t, err)
	return privkey
}

func newMockRandReader() io.Reader {
	return rand.New(rand.NewSource(mockRandSeed))
}
