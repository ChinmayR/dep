package health

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/yarpc"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/transport/http"

	"code.uber.internal/go/health.git/internal/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThriftSimple(t *testing.T) {
	c := NewCoordinator("foo")
	defer c.cleanup()
	handler := &metaHandler{c}
	req := &meta.HealthRequest{}

	res, err := handler.Health(context.Background(), req)
	assert.NoError(t, err, "Failed to call health procedure.")
	assert.True(t, res.Ok, "Expected job control to always be okay.")
	assert.Equal(t, RefusingTraffic.String(), *res.Message, "Unexpected message.")
	assert.Equal(t, meta.StateRefusing, *res.State, "Expected to be refusing.")

	require.NoError(t, c.AcceptTraffic(), "Couldn't accept traffic.")
	res, err = handler.Health(context.Background(), req)
	assert.NoError(t, err, "Failed to call health procedure.")
	assert.True(t, res.Ok, "Expected job control to always be okay.")
	assert.Equal(t, AcceptingTraffic.String(), *res.Message, "Unexpected message.")
	assert.Equal(t, meta.StateAccepting, *res.State, "Expected to be accepting.")
}

func TestThriftHealthController(t *testing.T) {
	c := NewCoordinator("foo")
	defer c.cleanup()
	handler := &metaHandler{c}
	checkType := meta.RequestTypeTraffic
	req := &meta.HealthRequest{Type: &checkType}

	res, err := handler.Health(context.Background(), req)
	assert.NoError(t, err, "Failed to call health procedure.")
	assert.False(t, res.Ok, "Expected not OK while refusing.")
	assert.Equal(t, RefusingTraffic.String(), *res.Message, "Unexpected message.")
	assert.Equal(t, meta.StateRefusing, *res.State, "Expected to be refusing.")

	require.NoError(t, c.AcceptTraffic(), "Couldn't accept traffic.")
	res, err = handler.Health(context.Background(), req)
	assert.NoError(t, err, "Failed to call health procedure.")
	assert.True(t, res.Ok, "Expected OK while accepting.")
	assert.Equal(t, AcceptingTraffic.String(), *res.Message, "Unexpected message.")
	assert.Equal(t, meta.StateAccepting, *res.State, "Expected to be accepting.")
}

func TestThriftClient(t *testing.T) {
	hc := NewCoordinator("foo")
	defer hc.cleanup()
	require.NoError(t, hc.AcceptTraffic(), "Couldn't accept traffic.")

	inbound := http.NewTransport().NewInbound(":0")
	server := yarpc.NewDispatcher(yarpc.Config{
		Name:     "server",
		Inbounds: yarpc.Inbounds{inbound},
	})
	server.Register(NewThrift(hc))
	require.NoError(t, server.Start(), "Couldn't start server.")
	defer func() {
		require.NoError(t, server.Stop(), "Server didn't stop cleanly.")
	}()

	tr := http.NewTransport()
	serverURI := fmt.Sprintf("http://%s", inbound.Addr())
	client := yarpc.NewDispatcher(yarpc.Config{
		Name: "client",
		Outbounds: yarpc.Outbounds{
			"server": transport.Outbounds{
				ServiceName: "server",
				Unary:       tr.NewSingleOutbound(serverURI),
			},
			"nonexistent": transport.Outbounds{
				ServiceName: "server",
				Unary:       tr.NewSingleOutbound("http://localhost:1234"),
			},
		},
	})
	require.NoError(t, client.Start(), "Couldn't start client.")
	defer func() {
		require.NoError(t, client.Stop(), "Client didn't stop cleanly.")
	}()

	t.Run("Success", func(t *testing.T) {
		tc := NewThriftClient(client.ClientConfig("server"))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		res, err := tc.Health(ctx)
		require.NoError(t, err, "Couldn't call health procedure.")
		assert.True(t, res.JobHealth, "Expected job health to be OK.")
		assert.Equal(t, AcceptingTraffic, res.RPCHealth, "Expected to be accepting RPCs.")
	})

	t.Run("RequestFailure", func(t *testing.T) {
		tc := NewThriftClient(client.ClientConfig("nonexistent"))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err := tc.Health(ctx)
		require.Error(t, err, "Unexpected success calling nonexistent server.")
	})
}
