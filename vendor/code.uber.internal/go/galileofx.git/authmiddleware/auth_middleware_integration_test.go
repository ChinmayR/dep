package authmiddleware_test

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"path"
	"path/filepath"
	"testing"
	"time"

	galileo "code.uber.internal/engsec/galileo-go.git"
	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"code.uber.internal/go/galileofx.git/authmiddleware"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/encoding/raw"
	"go.uber.org/yarpc/transport/http"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
)

func withGalileo(t testing.TB, name string, allowed []string, f func(galileo.Galileo)) {
	wonkatestdata.WithTempDir(func(dir string) {
		privatePem := filepath.Join(dir, "private.pem")

		privKey := wonkatestdata.PrivateKey()
		require.NoError(t,
			wonkatestdata.WritePrivateKey(privKey, privatePem),
			"error writing private key",
		)

		cfg := galileo.Configuration{
			ServiceName:       name,
			PrivateKeyPath:    privatePem,
			AllowedEntities:   append(allowed, name),
			EnforcePercentage: 1.0,
			Metrics:           tally.NoopScope,
			Logger:            zap.NewNop(),
		}

		g, err := galileo.Create(cfg)
		require.NoError(t, err, "failed to set up Galileo")

		f(g)
	})
}

// WithClientGalileo builds a new Galileo client.
func WithClientGalileo(t testing.TB, name string, f func(galileo.Galileo)) {
	withGalileo(t, name, nil /* allowed */, f)
}

// WithServerGalileo sets up a fake Wonka server and provides a Galileo
// instance for that server.
//
// Any number of Galileo instances for clients may be created inside the
// callback with the WithClientGalileo call.
func WithServerGalileo(t testing.TB, name string, f func(galileo.Galileo), opts ...GalileoServerOption) {
	defer opentracing.SetGlobalTracer(opentracing.GlobalTracer())
	opentracing.SetGlobalTracer(mocktracer.New())

	var o galileoServerOptions
	for _, opt := range opts {
		opt(&o)
	}

	wonkatestdata.WithWonkaMaster(name, func(r common.Router, handlerCfg common.HandlerConfig) {
		handlers.SetupHandlers(r, handlerCfg)

		for _, entity := range o.enrolledEntities {
			wonkatestdata.WithTempDir(func(dir string) {
				privPem := path.Join(dir, "wonka_private")
				privKey := wonkatestdata.PrivateKey()
				require.NoError(t, wonkatestdata.WritePrivateKey(privKey, privPem),
					"error writing private key")

				publicKey, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
				require.NoError(t, err, "failed to marshal public key")

				e := wonka.Entity{
					EntityName:   entity,
					PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
					ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(privKey),
				}

				require.NoError(
					t,
					handlerCfg.DB.Create(context.Background(), &e),
					"failed to enroll %q", entity,
				)
			})
		}

		withGalileo(t, name, o.allowedEntities, f)
	})
}

type galileoServerOptions struct {
	allowedEntities  []string
	enrolledEntities []string
}

type GalileoServerOption func(*galileoServerOptions)

func AllowedEntities(entities ...string) GalileoServerOption {
	return func(o *galileoServerOptions) {
		o.allowedEntities = append(o.allowedEntities, entities...)
	}
}

func EnrolledEntities(entities ...string) GalileoServerOption {
	return func(o *galileoServerOptions) {
		o.enrolledEntities = append(o.enrolledEntities, entities...)
	}
}

func TestEndToEnd(t *testing.T) {
	WithServerGalileo(t, "server", func(g galileo.Galileo) {
		inbound := http.NewTransport().NewInbound(":0")
		serverMiddleware := authmiddleware.New(g)
		serverD := yarpc.NewDispatcher(yarpc.Config{
			Name:     "server",
			Inbounds: yarpc.Inbounds{inbound},
			InboundMiddleware: yarpc.InboundMiddleware{
				Unary:  serverMiddleware,
				Oneway: serverMiddleware,
			},
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
			outbound := http.NewTransport().NewSingleOutbound(serverURL)
			clientD := yarpc.NewDispatcher(yarpc.Config{
				Name: "client",
				Outbounds: yarpc.Outbounds{
					"server": {Unary: outbound, Oneway: outbound},
				},
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
				assert.True(t, yarpcerrors.IsUnauthenticated(err), "expected UnauthenticatdeError")
				assert.Contains(t, err.Error(),
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
			WithClientGalileo(t, "client", func(g galileo.Galileo) {
				outbound := http.NewTransport().NewSingleOutbound(serverURL)
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
			})
		})

		t.Run("authenticated bad request", func(t *testing.T) {
			WithClientGalileo(t, "not-client", func(g galileo.Galileo) {
				outbound := http.NewTransport().NewSingleOutbound(serverURL)
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
					assert.True(t, yarpcerrors.IsUnauthenticated(err), "expected UnauthenticatdeError")
					assert.Contains(t, err.Error(), `access denied to procedure "echoUnary" of service "server"`)
					assert.Contains(t, err.Error(), "not permitted by configuration")
				})

				t.Run("oneway", func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()

					_, err := client.CallOneway(ctx, "echoOneway", []byte{1, 2, 3})
					require.NoError(t, err, "oneway call failed")
					// oneway can't fail because the server never gets to respond
				})
			})
		})
	}, AllowedEntities("client"), EnrolledEntities("server", "client", "not-client"))
}
