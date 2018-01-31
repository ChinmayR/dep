package galileofx

import (
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	galileo "code.uber.internal/engsec/galileo-go.git"
	"code.uber.internal/engsec/galileo-go.git/galileotest"
	envfx "code.uber.internal/go/envfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	"github.com/golang/mock/gomock"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/yarpc/api/middleware"
	"go.uber.org/zap"
)

func TestConfiguration(t *testing.T) {
	tests := []struct {
		desc       string
		giveConfig map[string]interface{}
		wantConfig galileo.Configuration
	}{
		{
			desc:       "empty",
			giveConfig: map[string]interface{}{},
			wantConfig: galileo.Configuration{ServiceName: "myservice"},
		},
		{
			desc: "allowedEntities",
			giveConfig: map[string]interface{}{
				"allowedEntities": []interface{}{"EVERYONE"},
			},
			wantConfig: galileo.Configuration{
				ServiceName:     "myservice",
				AllowedEntities: []string{galileo.EveryEntity},
			},
		},
		{
			desc: "enforceRatio",
			giveConfig: map[string]interface{}{
				"enforceRatio": 0.3,
			},
			wantConfig: galileo.Configuration{
				ServiceName:       "myservice",
				EnforcePercentage: 0.3,
			},
		},
		{
			desc: "privateKeyPath",
			giveConfig: map[string]interface{}{
				"privateKeyPath": "private.pem",
			},
			wantConfig: galileo.Configuration{
				ServiceName:    "myservice",
				PrivateKeyPath: "private.pem",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			logger := zap.NewNop()

			tracer := mocktracer.New()

			wantConfig := tt.wantConfig
			wantConfig.Metrics = tally.NoopScope
			wantConfig.Logger = logger
			wantConfig.Tracer = tracer

			defer expectGalileoConfig(t, mockCtrl, wantConfig)()

			cfg := map[string]interface{}{"galileo": tt.giveConfig}

			var out struct {
				Galileo galileo.Galileo

				UnaryInbound   middleware.UnaryInbound   `name:"auth"`
				UnaryOutbound  middleware.UnaryOutbound  `name:"auth"`
				OnewayInbound  middleware.OnewayInbound  `name:"auth"`
				OnewayOutbound middleware.OnewayOutbound `name:"auth"`
			}
			app := fxtest.New(t,
				Module,
				fx.Provide(
					staticProvider(cfg),
					func() envfx.Context { return envfx.Context{Environment: envfx.EnvProduction} },
					func() tally.Scope { return tally.NoopScope },
					func() *zap.Logger { return logger },
					func() servicefx.Metadata { return servicefx.Metadata{Name: "myservice"} },
					func() *versionfx.Reporter { return new(versionfx.Reporter) },
					func() opentracing.Tracer { return tracer },
				),
				fx.Extract(&out),
			)
			app.RequireStart().RequireStop()

			assert.NotNil(t, out.Galileo, "a galileo client must be constructed")

			assert.NotNil(t, out.UnaryInbound, "a unary inbound middleware for YARPC is expected")
			assert.NotNil(t, out.UnaryOutbound, "a unary outbound middleware for YARPC is expected")
			assert.NotNil(t, out.OnewayInbound, "a oneway inbound middleware for YARPC is expected")
			assert.NotNil(t, out.OnewayOutbound, "a oneway outbound middleware for YARPC is expected")
		})
	}

}

func TestDisabled(t *testing.T) {
	tests := []struct {
		desc     string
		cfg      map[string]interface{}
		env      string
		wantNoop bool
	}{
		{desc: "prod", env: envfx.EnvProduction},
		{desc: "staging", env: envfx.EnvStaging},
		{desc: "dev", env: envfx.EnvDevelopment, wantNoop: true},
		{desc: "test", env: envfx.EnvTest, wantNoop: true},
		{desc: "custom", env: "custom", wantNoop: true},
		{
			desc: "dev explicitly enabled",
			cfg:  map[string]interface{}{"enabled": true},
			env:  envfx.EnvDevelopment,
		},
		{
			desc: "test explicitly enabled",
			cfg:  map[string]interface{}{"enabled": true},
			env:  envfx.EnvTest,
		},
		{
			desc:     "staging explicitly disabled",
			cfg:      map[string]interface{}{"enabled": false},
			env:      envfx.EnvStaging,
			wantNoop: true,
		},
		{
			desc:     "prod explicitly disabled",
			cfg:      map[string]interface{}{"enabled": false},
			env:      envfx.EnvProduction,
			wantNoop: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			defer setGalileoCreate(func(galileo.Configuration) (galileo.Galileo, error) {
				return galileotest.NewMockGalileo(mockCtrl), nil
			})()

			cfg := map[string]interface{}{"galileo": tt.cfg}

			var out struct{ Galileo galileo.Galileo }
			app := fxtest.New(t,
				Module,
				fx.Provide(
					zap.NewNop,
					staticProvider(cfg),
					func() envfx.Context { return envfx.Context{Environment: tt.env} },
					func() tally.Scope { return tally.NoopScope },
					func() servicefx.Metadata { return servicefx.Metadata{Name: "myservice"} },
					func() *versionfx.Reporter { return new(versionfx.Reporter) },
					func() opentracing.Tracer { return mocktracer.New() },
				),
				fx.Extract(&out),
			)
			app.RequireStart().RequireStop()

			require.NotNil(t, out.Galileo, "galileo must be constructed")
			if _, ok := out.Galileo.(galileoNoop); tt.wantNoop {
				require.True(t, ok, "expected a no-op implementation for Galileo")
				assert.Equal(t, "myservice", out.Galileo.Name(), "galileo service name must match")
			} else {
				assert.False(t, ok, "did not expect a no-op implementation for Galileo")
			}
		})
	}
}

func expectGalileoConfig(t *testing.T, ctrl *gomock.Controller, want galileo.Configuration) (done func()) {
	return setGalileoCreate(func(got galileo.Configuration) (galileo.Galileo, error) {
		assert.Equal(t, want, got, "galileo configuration must match")
		return galileotest.NewMockGalileo(ctrl), nil
	})
}

func staticProvider(data map[string]interface{}) func() (config.Provider, error) {
	return func() (config.Provider, error) {
		return config.NewStaticProvider(data)
	}
}

func TestHTTPMiddlewareRoundTrip(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	g := galileotest.NewMockGalileo(mockCtrl)
	var out struct {
		Client  func(http.RoundTripper) http.RoundTripper `name:"auth"`
		Handler func(http.Handler) http.Handler           `name:"auth"`
	}

	app := fxtest.New(
		t,
		fx.Provide(
			func() galileo.Galileo { return g },
			zap.NewNop,
			newHTTPMiddleware,
		),
		fx.Extract(&out),
	)
	defer app.RequireStart().RequireStop()

	client := &http.Client{Transport: out.Client(http.DefaultTransport)}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	})

	server := httptest.NewServer(out.Handler(mux))
	defer server.Close()

	t.Run("health does not need authentication", func(t *testing.T) {
		res, err := client.Get(server.URL + "/health")
		require.NoError(t, err, "request should succeed")
		assert.Equal(t, 200, res.StatusCode, "status code should match")
		body, err := ioutil.ReadAll(res.Body)
		require.NoError(t, err, "failed to read response body")
		assert.Equal(t, "ok", string(body), "response body did not match")
	})

	t.Run("unauthenticated requests are rejected", func(t *testing.T) {
		g.EXPECT().
			AuthenticateIn(gomock.Any()).
			Return(errors.New("unauthenticated request"))

		res, err := client.Get(server.URL)
		require.NoError(t, err, "request should succeed")
		assert.Equal(t, 403, res.StatusCode, "status code should match")
	})

	t.Run("authenticated requests are allowed", func(t *testing.T) {
		g.EXPECT().
			AuthenticateIn(gomock.Any()).
			Return(nil)

		res, err := client.Post(server.URL, "text/plain", strings.NewReader("hello"))
		assert.Equal(t, 200, res.StatusCode, "status code should match")
		body, err := ioutil.ReadAll(res.Body)
		require.NoError(t, err, "failed to read response body")
		assert.Equal(t, "hello", string(body), "response body did not match")
	})
}
