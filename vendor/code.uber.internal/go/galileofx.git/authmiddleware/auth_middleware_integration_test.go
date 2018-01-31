package authmiddleware_test

import (
	"context"
	"testing"
	"time"

	"code.uber.internal/engsec/galileo-go.git"
	"code.uber.internal/engsec/galileo-go.git/galileotest"
	"code.uber.internal/go/galileofx.git/authmiddleware"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/encoding/raw"
	"go.uber.org/yarpc/transport/http"
	"go.uber.org/yarpc/yarpcerrors"
)

func TestEndToEnd(t *testing.T) {
	tracer := mocktracer.New()

	galileotest.WithServerGalileo(t, "server", func(g galileo.Galileo) {
		inbound := http.NewTransport(http.Tracer(tracer)).NewInbound(":0")
		serverMiddleware := authmiddleware.New(g)
		serverD := yarpc.NewDispatcher(yarpc.Config{
			Name:     "server",
			Inbounds: yarpc.Inbounds{inbound},
			InboundMiddleware: yarpc.InboundMiddleware{
				Unary:  serverMiddleware,
				Oneway: serverMiddleware,
			},
			Tracer: tracer,
		})

		serverD.Register(raw.Procedure("echoUnary", func(ctx context.Context, b []byte) ([]byte, error) {
			return b, nil
		}))
		serverD.Register(raw.Procedure("Meta::health", func(ctx context.Context, b []byte) ([]byte, error) {
			return b, nil
		}))
		serverD.Register(raw.OnewayProcedure("echoOneway", func(ctx context.Context, b []byte) error {
			return nil
		}))

		require.NoError(t, serverD.Start(), "failed to start YARPC server")
		defer func() {
			assert.NoError(t, serverD.Stop(), "failed to stop YARPC server")
		}()

		serverURL := "http://" + inbound.Addr().String()

		t.Run("unauthenticated request", func(t *testing.T) {
			outbound := http.NewTransport(http.Tracer(tracer)).NewSingleOutbound(serverURL)
			clientD := yarpc.NewDispatcher(yarpc.Config{
				Name: "client",
				Outbounds: yarpc.Outbounds{
					"server": {Unary: outbound, Oneway: outbound},
				},
				Tracer: tracer,
			})

			require.NoError(t, clientD.Start(), "failed to start YARPC client")
			defer func() {
				assert.NoError(t, clientD.Stop(), "failed to stop YARPC client")
			}()

			client := raw.New(clientD.ClientConfig("server"))

			t.Run("unary", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()

				_, err := client.Call(ctx, "echoUnary", []byte{1, 2, 3})
				require.Error(t, err, "unary call must fail")
				status := yarpcerrors.FromError(err)
				assert.Equal(t, yarpcerrors.CodeUnauthenticated, status.Code(), "expected status error with CodeUnauthenticated")
				assert.Contains(t, status.Message(),
					`access denied to procedure "echoUnary" of service "server"`)
			})

			t.Run("health check", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()

				_, err := client.Call(ctx, "Meta::health", nil)
				require.NoError(t, err, "unauthenticated health check should succeed")
			})

			t.Run("oneway", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()

				_, err := client.CallOneway(ctx, "echoOneway", []byte{1, 2, 3})
				require.NoError(t, err, "oneway call failed")
				// oneway can't fail because the server never gets to respond
			})
		})

		t.Run("authenticated request", func(t *testing.T) {
			galileotest.WithClientGalileo(t, "client", func(g galileo.Galileo) {
				outbound := http.NewTransport(http.Tracer(tracer)).NewSingleOutbound(serverURL)
				clientMiddleware := authmiddleware.New(g)
				clientD := yarpc.NewDispatcher(yarpc.Config{
					Name: "client",
					Outbounds: yarpc.Outbounds{
						"server": {Unary: outbound, Oneway: outbound},
					},
					OutboundMiddleware: yarpc.OutboundMiddleware{
						Unary:  clientMiddleware,
						Oneway: clientMiddleware,
					},
					Tracer: tracer,
				})

				require.NoError(t, clientD.Start(), "failed to start YARPC client")
				defer func() {
					assert.NoError(t, clientD.Stop(), "failed to stop YARPC client")
				}()

				client := raw.New(clientD.ClientConfig("server"))

				t.Run("unary", func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()

					res, err := client.Call(ctx, "echoUnary", []byte{1, 2, 3})
					require.NoError(t, err, "unary call failed")
					assert.Equal(t, []byte{1, 2, 3}, res, "response must match")
				})

				t.Run("oneway", func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()

					_, err := client.CallOneway(ctx, "echoOneway", []byte{1, 2, 3})
					require.NoError(t, err, "oneway call failed")
				})
			}, galileotest.Tracer(tracer))
		})

		t.Run("authenticated bad request", func(t *testing.T) {
			galileotest.WithClientGalileo(t, "not-client", func(g galileo.Galileo) {
				outbound := http.NewTransport(http.Tracer(tracer)).NewSingleOutbound(serverURL)
				clientMiddleware := authmiddleware.New(g)
				clientD := yarpc.NewDispatcher(yarpc.Config{
					Name: "not-client",
					Outbounds: yarpc.Outbounds{
						"server": {Unary: outbound, Oneway: outbound},
					},
					OutboundMiddleware: yarpc.OutboundMiddleware{
						Unary:  clientMiddleware,
						Oneway: clientMiddleware,
					},
					Tracer: tracer,
				})

				require.NoError(t, clientD.Start(), "failed to start YARPC client")
				defer func() {
					assert.NoError(t, clientD.Stop(), "failed to stop YARPC client")
				}()

				client := raw.New(clientD.ClientConfig("server"))

				t.Run("unary", func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()

					_, err := client.Call(ctx, "echoUnary", []byte{1, 2, 3})
					require.Error(t, err, "unary call must fail")
					status := yarpcerrors.FromError(err)
					assert.Equal(t, yarpcerrors.CodeUnauthenticated, status.Code(), "expected status error with CodeUnauthenticated")
					assert.Contains(t, status.Message(), `access denied to procedure "echoUnary" of service "server"`)
					assert.Contains(t, status.Message(), "not permitted by configuration")
				})

				t.Run("oneway", func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()

					_, err := client.CallOneway(ctx, "echoOneway", []byte{1, 2, 3})
					require.NoError(t, err, "oneway call failed")
					// oneway can't fail because the server never gets to respond
				})
			}, galileotest.Tracer(tracer))
		})
	},
		galileotest.AllowedEntities("client"),
		galileotest.EnrolledEntities("server", "client", "not-client"),
		galileotest.Tracer(tracer),
	)
}
