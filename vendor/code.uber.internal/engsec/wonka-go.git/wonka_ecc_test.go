package wonka_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"testing"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"github.com/stretchr/testify/require"
)

func TestECCKey(t *testing.T) {
	for i := 0; i < 100; i++ {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err, "%d generating key: %v", i, err)

		marshalledK, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
		require.NoError(t, err, "%d, marshal %v", i, err)

		compressed := wonka.KeyToCompressed(k.PublicKey.X, k.PublicKey.Y)
		fromCompressed, err := wonka.KeyFromCompressed(compressed)
		require.NoError(t, err, "%d from compressed failure: %v", i, err)

		recommpressed := wonka.KeyToCompressed(fromCompressed.X, fromCompressed.Y)
		marshalledFrom, err := x509.MarshalPKIXPublicKey(fromCompressed)
		require.NoError(t, err, "%d, marshal %v", i, err)

		require.True(t, bytes.Equal(marshalledK, marshalledFrom),
			"%d key bytes aren't equal \n%s\n%s", i, compressed, recommpressed)

		require.Equal(t, compressed, recommpressed, "test %d", i)
	}
}

func TestECCBadCompressedKey(t *testing.T) {
	_, err := wonka.KeyFromCompressed("123")
	require.Error(t, err, "fucked up key shouldn't parse")
}
