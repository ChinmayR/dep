package jaegerfx

import (
	"testing"

	envfx "code.uber.internal/go/envfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	jaeger "github.com/uber/jaeger-client-go"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

func succeeds(
	t testing.TB,
	sfx servicefx.Metadata,
	env envfx.Context,
	cfg config.Provider,
) (opentracing.Tracer, *jaeger.InMemoryReporter) {
	lc := fxtest.NewLifecycle(t)
	reporter := jaeger.NewInMemoryReporter()
	result, err := New(Params{
		Service:     sfx,
		Environment: env,
		Config:      cfg,
		Scope:       tally.NoopScope,
		Logger:      zap.NewNop(),
		Lifecycle:   lc,
		Version:     &versionfx.Reporter{},
		Reporter:    reporter,
	})
	require.NoError(t, err, "Unexpected error from module.")
	require.NotNil(t, result.Tracer, "Got nil tracer.")
	lc.RequireStart().RequireStop()
	return result.Tracer, reporter
}

func generateSpans(tracer opentracing.Tracer) {
	// This is not nice but without a public API to change the
	// sampling rate, this is the only option.
	//
	// Surely, we'll emit a span with the default sampling rate after
	// 10000 requests.
	for i := 0; i < 10000; i++ {
		tracer.StartSpan("foo").Finish()
	}

}

func TestDefaults(t *testing.T) {
	for _, env := range []string{envfx.EnvProduction, envfx.EnvStaging, envfx.EnvTest, envfx.EnvDevelopment} {
		t.Run(env, func(t *testing.T) {
			cfg, err := config.NewStaticProvider(nil)
			require.NoError(t, err, "failed to create config")
			tracer, reporter := succeeds(
				t,
				servicefx.Metadata{Name: "foo"},
				envfx.Context{Environment: env},
				cfg,
			)

			generateSpans(tracer)

			spans := reporter.GetSpans()
			require.NotEmpty(t, spans, "expected spans to be reported")
		})
	}
}

func TestOverrideDefaults(t *testing.T) {
	cfg, err := config.NewStaticProvider(map[string]interface{}{
		ConfigurationKey: map[string]interface{}{"disabled": true},
	})
	require.NoError(t, err, "failed to create config")
	tracer, reporter := succeeds(
		t,
		servicefx.Metadata{Name: "foo"},
		envfx.Context{Environment: envfx.EnvProduction},
		cfg,
	)

	generateSpans(tracer)
	require.Empty(t, reporter.GetSpans(), "did not expect spans to be reported")
}

func TestJaegerConfigErrors(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	result, err := New(Params{
		Service:     servicefx.Metadata{},
		Environment: envfx.Context{Environment: envfx.EnvProduction},
		Config:      config.NopProvider{},
		Scope:       tally.NoopScope,
		Logger:      zap.NewNop(),
		Lifecycle:   lc,
		Version:     &versionfx.Reporter{},
	})
	require.Error(t, err, "Unexpected success from module.")
	require.Nil(t, result.Tracer, "Got non-nil tracer.")
	lc.RequireStart().RequireStop()
}

func TestVersionReportError(t *testing.T) {
	ver := &versionfx.Reporter{}
	ver.Report(_name, Version)
	params := Params{
		Version: ver,
	}
	_, err := New(params)
	assert.Contains(t, err.Error(), "already registered version")
}

func TestGlobalTracerIsChanged(t *testing.T) {
	opentracing.SetGlobalTracer(&opentracing.NoopTracer{})

	fxtest.New(t,
		Module,
		fx.Provide(
			func() servicefx.Metadata { return servicefx.Metadata{Name: "myservice"} },
			func() envfx.Context { return envfx.Context{Environment: envfx.EnvDevelopment} },
			func() config.Provider { return config.NopProvider{} },
			func() tally.Scope { return tally.NoopScope },
			func() *zap.Logger { return zap.NewNop() },
			func() *versionfx.Reporter { return &versionfx.Reporter{} },
		),
	)

	// We don't need to start the app because Invokes are called on New().
	tr := opentracing.GlobalTracer()
	_, ok := tr.(*opentracing.NoopTracer)
	require.False(t, ok, "global tracer must not be no-op")
}
