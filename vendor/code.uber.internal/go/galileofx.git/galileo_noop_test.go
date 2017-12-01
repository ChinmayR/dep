package galileofx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func TestNoop(t *testing.T) {
	type ctxKey string

	g := galileoNoop{name: "foo"}
	require.Equal(t, "foo", g.Name(), "name must match")

	t.Run("Endpoint", func(t *testing.T) {
		_, err := g.Endpoint("/")
		require.Error(t, err, "expected failure looking up per-endpoint configuration")
	})

	t.Run("AuthenticateOut", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ctxKey("foo"), "bar")
		gotCtx, err := g.AuthenticateOut(ctx, "someservice", "someone")
		require.NoError(t, err, "AuthenticateOut should succeed")
		assert.Equal(t, "bar", gotCtx.Value(ctxKey("foo")), "context value must not change")
	})

	t.Run("AuthenticateIn", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ctxKey("foo"), "bar")
		require.NoError(t, g.AuthenticateIn(ctx, "someone"), "AuthenticateIn must not fail")
	})
}
