package galileotest_test

import (
	context "context"
	"testing"

	galileo "code.uber.internal/engsec/galileo-go.git"
	"code.uber.internal/engsec/galileo-go.git/galileotest"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGalileoTest(t *testing.T) {
	type ctxKey string
	tracer := mocktracer.New()

	galileotest.WithServerGalileo(t, "server", func(serverG galileo.Galileo) {
		t.Run("failure", func(t *testing.T) {
			ctx := context.WithValue(context.Background(), ctxKey("key"), "foo")
			require.Error(t, serverG.AuthenticateIn(ctx), "unauthenticated request should fail")
		})

		t.Run("success", func(t *testing.T) {
			galileotest.WithClientGalileo(t, "client", func(clientG galileo.Galileo) {
				ctx := context.WithValue(context.Background(), ctxKey("key"), "foo")
				ctx, err := clientG.AuthenticateOut(ctx, "server")
				require.NoError(t, err, "AuthenticateOut should suceed")
				require.NoError(t, serverG.AuthenticateIn(ctx), "authenticated request should succeed")
			}, galileotest.Tracer(tracer))
		})

		t.Run("authenticated for different destination server", func(t *testing.T) {
			galileotest.WithClientGalileo(t, "client", func(clientG galileo.Galileo) {
				ctx := context.WithValue(context.Background(), ctxKey("key"), "foo")
				ctx, err := clientG.AuthenticateOut(ctx, "not-server")
				require.NoError(t, err, "AuthenticateOut should suceed")
				require.Error(t, serverG.AuthenticateIn(ctx), "authenticated request should fail")
			}, galileotest.Tracer(tracer))
		})

		t.Run("authenticated as a disallowed client", func(t *testing.T) {
			galileotest.WithClientGalileo(t, "not-client", func(clientG galileo.Galileo) {
				ctx := context.WithValue(context.Background(), ctxKey("key"), "foo")
				ctx, err := clientG.AuthenticateOut(ctx, "server")
				require.NoError(t, err, "AuthenticateOut should suceed")
				require.Error(t, serverG.AuthenticateIn(ctx), "authenticated request should fail")
			}, galileotest.Tracer(tracer))
		})

	}, galileotest.AllowedEntities("client"),
		galileotest.EnrolledEntities("server", "not-server", "client", "not-client"),
		galileotest.Tracer(tracer),
	)
}

func TestNewDisabled(t *testing.T) {
	g := galileotest.NewDisabled(t, "Testy-McTestface")
	ctx := context.Background()
	ctx, err := g.AuthenticateOut(ctx, "Antarctica")
	assert.NoError(t, err, "AuthenticateOut should succeed when Galileo is disabled")
	err = g.AuthenticateIn(ctx)
	assert.NoError(t, err, "AuthenticateIn should succeed when Galileo is disabled")
}
