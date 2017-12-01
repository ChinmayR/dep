package zapfx

import (
	"context"
	"io/ioutil"
	"testing"

	envfx "code.uber.internal/go/envfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/config"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func newStaticProvider(t testing.TB, data map[string]interface{}) config.Provider {
	p, err := config.NewStaticProvider(data)
	if err != nil {
		t.Fatalf("Failed to create static provider: %v", err)
	}
	return p
}

func TestEndToEnd(t *testing.T) {
	tests := []struct {
		Environment string
		Level       zapcore.Level
	}{
		{envfx.EnvProduction, zapcore.InfoLevel},
		{envfx.EnvStaging, zapcore.InfoLevel},
		{envfx.EnvTest, zapcore.DebugLevel},
		{envfx.EnvDevelopment, zapcore.DebugLevel},
	}
	for _, tt := range tests {
		t.Run(tt.Environment, func(t *testing.T) {
			f, err := ioutil.TempFile("", "zapfx")
			require.NoError(t, err, "Failed to create temp file.")
			defer f.Close()

			lc := fxtest.NewLifecycle(t)
			result, err := New(Params{
				Service:     servicefx.Metadata{Name: "foo"},
				Environment: envfx.Context{Environment: tt.Environment},
				Config: newStaticProvider(t, map[string]interface{}{
					ConfigurationKey: map[string][]string{"outputPaths": {f.Name()}},
				}),
				Lifecycle: lc,
				Reporter:  &versionfx.Reporter{},
				Sentry:    nil,
			})
			require.NoError(t, err, "Failed to create logger.")
			assert.Equal(t, tt.Level, result.Level.Level(), "Unexpected level.")
			lc.RequireStart()
			result.Logger.Info("something happened")
			lc.RequireStop()

			output, err := ioutil.ReadAll(f)
			require.NoError(t, err, "Failed to read log output.")
			assert.Contains(t, string(output), `something happened`, "Output mismatch.")
		})
	}
}

func TestDevelopmentDefaultConfig(t *testing.T) {
	cfg, err := newConfiguration(
		servicefx.Metadata{Name: "foo"},
		envfx.Context{Environment: envfx.EnvDevelopment},
		newStaticProvider(t, nil),
	)
	require.NoError(t, err, "Couldn't create configuration.")
	require.Equal(t, defaultDevConfig(), cfg, "Unexpected configuration")
}

func TestProductionDefaultConfig(t *testing.T) {
	cfg, err := newConfiguration(
		servicefx.Metadata{Name: "foo"},
		envfx.Context{
			Environment:        envfx.EnvProduction,
			RuntimeEnvironment: "tacos",
		},
		newStaticProvider(t, nil),
	)
	require.NoError(t, err, "Couldn't create configuration.")
	assert.Equal(t, 4, len(cfg.InitialFields), "Expected default fields.")
	assert.Contains(t, cfg.InitialFields, "hostname", "Hostname not set.")
	assert.Contains(t, cfg.InitialFields, "zone", "Zone not set.")
	assert.Equal(t, "tacos", cfg.InitialFields["runtimeEnvironment"], "Runtime environment incorrect.")
	assert.Equal(t, "foo", cfg.InitialFields["service_name"], "Service name incorrect.")
	cfg.InitialFields = nil
	require.Equal(t, defaultProdConfig(), cfg, "Unexpected configuration")
}

func TestOverrideDefaultConfig(t *testing.T) {
	cfg, err := newConfiguration(
		servicefx.Metadata{Name: "foo"},
		envfx.Context{Environment: envfx.EnvDevelopment},
		newStaticProvider(t, map[string]interface{}{
			ConfigurationKey: map[string]string{"level": zap.FatalLevel.String()},
		}),
	)
	require.NoError(t, err, "Couldn't create configuration.")
	expected := defaultDevConfig()
	expected.Level = zap.FatalLevel.String()
	assert.Equal(t, expected, cfg, "Unexpected configuration.")
}

func TestConfiguredFields(t *testing.T) {
	cfg, err := newConfiguration(
		servicefx.Metadata{Name: "foo"},
		envfx.Context{
			Environment:        envfx.EnvProduction,
			RuntimeEnvironment: "tacos",
		},
		newStaticProvider(t, map[string]interface{}{
			ConfigurationKey: map[string]interface{}{"initialFields": map[string]string{
				"zone":  "narnia",  // shouldn't be overwritten
				"shard": "shard01", // should be preserved
			}},
		}),
	)
	require.NoError(t, err, "Couldn't create configuration.")
	assert.Equal(t, 5, len(cfg.InitialFields), "Unexpected number of fields.")
	assert.Contains(t, cfg.InitialFields, "hostname", "Hostname not set.")
	assert.Equal(t, "foo", cfg.InitialFields["service_name"], "Service name incorrect.")
	assert.Equal(t, "narnia", cfg.InitialFields["zone"], "Zone incorrect.")
	assert.Equal(t, "tacos", cfg.InitialFields["runtimeEnvironment"], "Runtime environment incorrect.")
	assert.Equal(t, "shard01", cfg.InitialFields["shard"], "User-supplied field not preserved.")
	cfg.InitialFields = nil
	require.Equal(t, defaultProdConfig(), cfg, "Unexpected configuration")
}

func TestInvalidPath(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	_, err := New(Params{
		Service:     servicefx.Metadata{Name: "foo"},
		Environment: envfx.Context{Environment: envfx.EnvProduction},
		Config: newStaticProvider(t, map[string]interface{}{
			ConfigurationKey: map[string][]string{"outputPaths": {"/does/not/exist.log"}},
		}),
		Lifecycle: lc,
		Reporter:  &versionfx.Reporter{},
		Sentry:    nil,
	})
	assert.Error(t, err, "Logger with nonexistent output should fail.")
	lc.RequireStart().RequireStop()
}

func TestInvalidLevel(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	_, err := New(Params{
		Service:     servicefx.Metadata{Name: "foo"},
		Environment: envfx.Context{Environment: envfx.EnvProduction},
		Config: newStaticProvider(t, map[string]interface{}{
			ConfigurationKey: map[string]string{"level": "not-a-level"},
		}),
		Lifecycle: lc,
		Reporter:  &versionfx.Reporter{},
		Sentry:    nil,
	})
	assert.Error(t, err, "Logger with invalid level should fail.")
	lc.RequireStart().RequireStop()
}

func TestConsoleInProduction(t *testing.T) {
	f, err := ioutil.TempFile("", "zapfx")
	require.NoError(t, err, "Failed to create temp file.")
	defer f.Close()

	lc := fxtest.NewLifecycle(t)
	_, err = New(Params{
		Service:     servicefx.Metadata{Name: "foo"},
		Environment: envfx.Context{Environment: envfx.EnvProduction},
		Config: newStaticProvider(t, map[string]interface{}{
			ConfigurationKey: map[string]interface{}{
				"outputPaths": []string{f.Name()},
				"encoding":    "console",
			},
		}),
		Lifecycle: lc,
		Reporter:  &versionfx.Reporter{},
		Sentry:    nil,
	})
	assert.NoError(t, err, "Console encoder should be allowed in production.")
	lc.RequireStart().RequireStop()

	output, err := ioutil.ReadAll(f)
	require.NoError(t, err, "Failed to read log output.")
	assert.Contains(t, string(output), "isn't supported in production", "Expected to log warning.")
}

func TestSentryTee(t *testing.T) {
	// Don't actually care about this output, but we need a path to send output
	// to.
	f, err := ioutil.TempFile("", "zapfx")
	require.NoError(t, err, "Failed to create temp file.")
	defer f.Close()

	observer, logs := observer.New(zap.InfoLevel)

	lc := fxtest.NewLifecycle(t)
	result, err := New(Params{
		Service:     servicefx.Metadata{Name: "foo"},
		Environment: envfx.Context{Environment: envfx.EnvProduction},
		Config:      newStaticProvider(t, nil),
		Lifecycle:   lc,
		Reporter:    &versionfx.Reporter{},
		Sentry:      observer,
	})
	assert.NoError(t, err, "Failed to construct logger.")
	lc.RequireStart()
	require.Equal(t, 0, logs.Len(), "Observed logs have unexpected entries: %v.", logs.All())
	result.Logger.Info("hello")
	assert.Equal(t, 1, logs.Len(), "Expected only one logged entry, got %v.", logs.All())
	lc.RequireStop()
}

func TestVersionReporting(t *testing.T) {
	r := &versionfx.Reporter{}
	lc := fxtest.NewLifecycle(t)
	_, err := New(Params{
		Service:     servicefx.Metadata{Name: "foo"},
		Environment: envfx.Context{Environment: envfx.EnvProduction},
		Config:      newStaticProvider(t, nil),
		Lifecycle:   lc,
		Reporter:    r,
		Sentry:      nil,
	})
	assert.NoError(t, err, "Failed to construct logger.")
	assert.Equal(t, Version, r.Version(_name), "Reported unexpected version.")
	lc.RequireStart().RequireStop()
}

func TestTrace(t *testing.T) {
	// Detailed testing is already handled by the Jaeger package.
	assert.NotPanics(t, func() {
		Trace(context.Background())
	})
}
