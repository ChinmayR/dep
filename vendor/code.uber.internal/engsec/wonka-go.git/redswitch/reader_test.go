package redswitch_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal/dns"
	"code.uber.internal/engsec/wonka-go.git/redswitch"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

func BenchmarkIsDisabled(b *testing.B) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(b, err)

	ctime := int64(time.Now().Add(-time.Hour).Unix())
	etime := int64(time.Now().Add(time.Hour).Unix())

	msg := redswitch.DisableMessage{
		Ctime:      ctime,
		Etime:      etime,
		IsDisabled: true,
	}

	toSign, err := json.Marshal(msg)
	require.NoError(b, err)

	msg.Signature, err = wonkacrypter.New().Sign(toSign, k)
	require.NoError(b, err)

	toCheck, err := json.Marshal(msg)
	require.NoError(b, err)

	record := base64.StdEncoding.EncodeToString(toCheck)

	r, err := redswitch.NewReader(redswitch.WithPublicKeys(&k.PublicKey),
		redswitch.WithDNSClient(dns.NewMockClient([]string{record})),
		redswitch.WithLogger(zap.NewNop()),
		redswitch.WithMetrics(tally.NoopScope))
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.IsDisabled()
	}
}

func TestNewReader(t *testing.T) {
	t.Run("invalid_options", func(t *testing.T) {
		r, err := redswitch.NewReader()
		require.Error(t, err)
		require.Nil(t, r)
	})
	t.Run("invalid_ecdsa_public_key", func(t *testing.T) {
		key := ecdsa.PublicKey{
			X: big.NewInt(0),
			Y: big.NewInt(0),
		}
		key.Curve = invalidCurve{}

		r, err := redswitch.NewReader(redswitch.WithPublicKeys(&key),
			redswitch.WithLogger(zap.NewNop()),
			redswitch.WithMetrics(tally.NoopScope),
			redswitch.WithDNSClient(dns.NewMockClient(nil)))
		require.EqualError(t, err, "x509: unsupported elliptic curve")
		require.Nil(t, r)
	})
	t.Run("invalid_public_key", func(t *testing.T) {
		var key crypto.PublicKey
		r, err := redswitch.NewReader(redswitch.WithPublicKeys(key),
			redswitch.WithLogger(zap.NewNop()),
			redswitch.WithMetrics(tally.NoopScope),
			redswitch.WithDNSClient(dns.NewMockClient(nil)))
		require.EqualError(t, err, "only ecdsa.PublicKey keys are supported")
		require.Nil(t, r)
	})
	t.Run("error_dns", func(t *testing.T) {
		// If the DNS returns nothing but errors, we should always be enabled
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		r, err := redswitch.NewReader(redswitch.WithPublicKeys(&k.PublicKey),
			redswitch.WithDNSClient(errorDNS{}),
			redswitch.WithLogger(zap.NewNop()),
			redswitch.WithMetrics(tally.NoopScope))
		require.NoError(t, err)
		require.NotNil(t, r)

		require.False(t, r.IsDisabled())
		require.False(t, r.(redswitch.CachelessReader).ForceCheckIsDisabled(context.Background()))
		require.False(t, r.IsDisabled())
	})
	t.Run("multiple_keys", func(t *testing.T) {
		k1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		k2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		r, err := redswitch.NewReader(redswitch.WithPublicKeys(&k1.PublicKey, &k2.PublicKey),
			redswitch.WithDNSClient(dns.NewMockClient(nil)),
			redswitch.WithLogger(zap.NewNop()),
			redswitch.WithMetrics(tally.NoopScope))
		require.NoError(t, err)
		require.NotNil(t, r)

		require.False(t, r.IsDisabled())
		require.False(t, r.(redswitch.CachelessReader).ForceCheckIsDisabled(context.Background()))
		require.False(t, r.IsDisabled())
	})
}

type errorDNS struct{}

func (e errorDNS) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return nil, errors.New("test DNS error")
}

type invalidCurve struct{}

func (c invalidCurve) Params() *elliptic.CurveParams {
	return &elliptic.CurveParams{
		P:       big.NewInt(4),
		N:       big.NewInt(5),
		B:       big.NewInt(3),
		Gx:      big.NewInt(0),
		Gy:      big.NewInt(1),
		BitSize: 192,
		Name:    "fake",
	}
}

func (c invalidCurve) IsOnCurve(x, y *big.Int) bool {
	return false
}

func (c invalidCurve) Add(x1, y1, x2, y2 *big.Int) (x, y *big.Int) {
	return big.NewInt(0), big.NewInt(0)
}

func (c invalidCurve) Double(x1, y1 *big.Int) (x, y *big.Int) {
	return big.NewInt(0), big.NewInt(0)
}

func (c invalidCurve) ScalarMult(x1, y1 *big.Int, k []byte) (x, y *big.Int) {
	return big.NewInt(0), big.NewInt(0)
}

func (c invalidCurve) ScalarBaseMult(k []byte) (x, y *big.Int) {
	return big.NewInt(0), big.NewInt(0)
}
