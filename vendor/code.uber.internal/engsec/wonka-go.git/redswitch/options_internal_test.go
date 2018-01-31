package redswitch

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"code.uber.internal/engsec/wonka-go.git/internal/dns"

	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

func TestOptions(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	valid := readerOptions{
		log:     zap.NewNop(),
		metrics: tally.NoopScope,
		keys:    make([]crypto.PublicKey, 1),
		dns:     dns.NewMockClient([]string{"test"}),
	}
	valid.keys[0] = &k.PublicKey

	t.Run("invalid_logger", func(t *testing.T) {
		o := valid
		o.log = nil
		require.EqualError(t, o.validate(), "WithLogger option is required")
	})
	t.Run("invalid_metrics", func(t *testing.T) {
		o := valid
		o.metrics = nil
		require.EqualError(t, o.validate(), "WithMetrics option is required")
	})
	t.Run("invalid_keys", func(t *testing.T) {
		o := valid
		o.keys = nil
		require.EqualError(t, o.validate(), "WithPublicKeys option is required")
	})
	t.Run("invalid_dns", func(t *testing.T) {
		o := valid
		o.dns = nil
		require.EqualError(t, o.validate(), "the DNS client cannot be set to nil")
	})
}
