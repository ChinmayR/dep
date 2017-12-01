package galileo_test

import (
	"context"
	"crypto"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"io/ioutil"
	"os"
	"path"
	"testing"

	. "code.uber.internal/engsec/galileo-go.git"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func startSpan(ctx context.Context, tracer opentracing.Tracer) (_ context.Context, finish func()) {
	span := tracer.StartSpan("test-span")
	return opentracing.ContextWithSpan(ctx, span), span.Finish
}

func Test_NewGalileo_OK(t *testing.T) {
	t.Run("without ServiceName", func(t *testing.T) {
		var cfg Configuration
		_, err := Create(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Configuration must have ServiceName parameter set")
	})

	t.Run("with ServiceName", func(t *testing.T) {
		cfg := Configuration{ServiceName: "foo", Tracer: mocktracer.New()}
		g, err := Create(cfg)
		assert.NoError(t, err)
		assert.NotNil(t, g)
	})

	t.Run("without tracer", func(t *testing.T) {
		cfg := Configuration{ServiceName: "foo"}
		_, err := Create(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "jaeger must be initialized before calling galileo")
	})

	t.Run("with NoopTracer", func(t *testing.T) {
		cfg := Configuration{ServiceName: "foo", Tracer: opentracing.NoopTracer{}}
		_, err := Create(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "jaeger must be initialized before calling galileo")
	})
}

func TestDisabled(t *testing.T) {
	type ctxKey string

	cfg := Configuration{ServiceName: "foo", Disabled: true}

	g, err := Create(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, g)

	wonkatestdata.WithWonkaMaster("foo", func(r common.Router, handlerCfg common.HandlerConfig) {
		handlers.SetupHandlers(r, handlerCfg)
		ctx := context.WithValue(context.Background(), ctxKey("key"), "value")
		newCtx, err := g.AuthenticateOut(ctx, "foo", "EVERYONE")
		require.NoError(t, err, "calling authenticate out when disabled shouldn't error: %v", err)
		require.Equal(t, newCtx.Value("foo"), ctx.Value("foo"))
	})
}

var createVars = []struct {
	name  string
	noKey bool

	errMsg string
}{
	{name: "wonkaSample:foober"},
	{errMsg: "Configuration must have ServiceName parameter set"},
}

func TestCreate(t *testing.T) {
	log := zap.L()
	for idx, m := range createVars {
		wonkatestdata.WithWonkaMaster(m.name, func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)

			wonkatestdata.WithTempDir(func(dir string) {
				oldAuthSock := os.Getenv("SSH_AUTH_SOCK")
				os.Unsetenv("SSH_AUTH_SOCK")

				log.Debug("tempdir", zap.String("path", dir))
				privatePem := path.Join(dir, "private.pem")
				privKey := wonkatestdata.PrivateKey()
				err := wonkatestdata.WritePrivateKey(privKey, privatePem)
				require.NoError(t, err, "%d writing private key: %v", idx, err)

				cfg := Configuration{
					ServiceName:     m.name,
					PrivateKeyPath:  privatePem,
					AllowedEntities: []string{m.name},
					Tracer:          mocktracer.New(),
				}

				if m.noKey {
					cfg.PrivateKeyPath = ""
				}

				_, err = Create(cfg)
				if m.errMsg != "" {
					require.Error(t, err, "%d should error, name %s, msg %s", idx, cfg.ServiceName, m.errMsg)
					require.Contains(t, err.Error(), m.errMsg, "%d", idx)
				} else {
					require.NoError(t, err, "%d, err %v", idx, err)
				}

				// cleanup
				os.Setenv("SSH_AUTH_SOCK", oldAuthSock)
			})
		})
	}
}

var galileoVars = []struct {
	name          string
	attribute     string
	ctxClaim      string
	explicitClaim []interface{}
	enrolled      bool
	noDest        bool

	errMsg string
}{
	{name: "wonkaSample:foober", attribute: "test_attribute"},
	{name: "wonkaSample:foober", attribute: "test_attribute", enrolled: true},
	{name: "wonkaSample:foober", attribute: "test_attribute", ctxClaim: "AD:engsec", enrolled: true},
	{
		name:          "wonkaSample:foober", // Verifies that the explicit claim(s) provided via `AuthenticateOut` takes precedence over the `WithClaim` result.
		attribute:     "test_attribute",
		ctxClaim:      "AD:engsec",
		explicitClaim: []interface{}{"AD:engineering", "EVERYTHING"},
		enrolled:      true,
		errMsg:        "only one explicit claim is supported",
	},
	{name: "wonkaSample:foober", enrolled: true, noDest: true, errMsg: "no destination"},
}

func TestAttributes(t *testing.T) {
	log := zap.L()
	for idx, m := range galileoVars {
		wonkatestdata.WithWonkaMaster(m.name, func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)

			wonkatestdata.WithTempDir(func(dir string) {
				log.Debug("tempdir", zap.String("path", dir))
				privatePem := path.Join(dir, "private.pem")
				privKey := wonkatestdata.PrivateKey()
				err := wonkatestdata.WritePrivateKey(privKey, privatePem)
				require.NoError(t, err, "%d writing private key: %v", idx, err)

				tracer := mocktracer.New()
				originalSpan := tracer.StartSpan("test-span")
				ctx := opentracing.ContextWithSpan(context.Background(), originalSpan)
				defer originalSpan.Finish()

				cfg := Configuration{
					ServiceName:     m.name,
					PrivateKeyPath:  privatePem,
					AllowedEntities: []string{m.name},
					Tracer:          tracer,
				}

				g, err := CreateWithContext(ctx, cfg)
				require.NoError(t, err, "creating: %v", err)
				require.False(t, g == nil, "galileo shouldn't be nil")

				if m.ctxClaim != "" {
					ctx = WithClaim(ctx, m.ctxClaim)
				}

				if m.enrolled {
					dest := m.name
					if m.noDest {
						dest = ""
					}

					ctx, err := g.AuthenticateOut(ctx, dest, m.explicitClaim...)
					if m.errMsg != "" {
						require.Error(t, err, "%d should error", idx)
						require.Contains(t, err.Error(), m.errMsg, "%d", idx)
					} else {
						newSpan := opentracing.SpanFromContext(ctx)
						assert.NotEqual(t, newSpan, originalSpan)

						err = g.AuthenticateIn(ctx)
						require.NoError(t, err, "%d, authorize: %v", idx, err)
					}
				}

			})
		})
	}
}

// tiny helper for hiding the ugliness of setting up a wonkamaster for a test.
func withWM(name string, fn func(wonkadb.EntityDB)) {
	wonkatestdata.WithWonkaMaster(name, func(rtr common.Router, handlerCfg common.HandlerConfig) {
		handlers.SetupHandlers(rtr, handlerCfg)
		fn(handlerCfg.DB)
	})
}

func createEntity(name string, db wonkadb.WonkaDB) {
	withTempDir(func(dir string) {
		privPem := path.Join(dir, "wonka_private")
		k := wonkatestdata.PrivateKey()
		err := writePrivateKey(k, privPem)
		if err != nil {
			panic(err)
		}

		e := wonka.Entity{
			EntityName:   name,
			PublicKey:    string(publicPemFromKey(k)),
			ECCPublicKey: eccPublicFromPrivateKey(k),
		}

		ok := db.CreateEntity(e)
		if !ok {
			panic("error creating entity")
		}
	})
}

func withTempDir(fn func(dir string)) {
	dir, err := ioutil.TempDir("", "galileo")
	if err != nil {
		panic(err)
	}

	defer os.RemoveAll(dir)
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	defer os.Chdir(cwd)
	os.Chdir(dir)

	fn(dir)
}

func publicPemFromKey(k *rsa.PrivateKey) string {
	b, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}

func eccPublicFromPrivateKey(k *rsa.PrivateKey) string {
	h := crypto.SHA256.New()
	h.Write([]byte(x509.MarshalPKCS1PrivateKey(k)))
	pointKey := h.Sum(nil)

	return wonka.KeyToCompressed(elliptic.P256().ScalarBaseMult(pointKey))
}

func writePrivateKey(k *rsa.PrivateKey, loc string) error {
	pemBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k),
	}
	e := ioutil.WriteFile(loc, pem.EncodeToMemory(&pemBlock), 0440)
	if e != nil {
		return e
	}
	return nil
}
