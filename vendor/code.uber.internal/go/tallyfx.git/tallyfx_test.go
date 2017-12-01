package tallyfx

import (
	"testing"

	envfx "code.uber.internal/go/envfx.git"
	"code.uber.internal/go/servicefx.git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/config"
	"go.uber.org/fx/fxtest"
)

func succeeds(t testing.TB, sfx servicefx.Metadata, env envfx.Context, cfg config.Provider) tally.Scope {
	lc := fxtest.NewLifecycle(t)
	result, err := New(Params{
		Service:     sfx,
		Environment: env,
		Config:      cfg,
		Lifecycle:   lc,
	})
	require.NoError(t, err, "Unexpected error from module.")
	require.NotNil(t, result.Scope, "Got nil scope.")
	lc.RequireStart().RequireStop()
	return result.Scope
}

func fails(t testing.TB, errMsg string, sfx servicefx.Metadata, env envfx.Context, cfg config.Provider) {
	lc := fxtest.NewLifecycle(t)
	result, err := New(Params{
		Service:     sfx,
		Environment: env,
		Config:      cfg,
		Lifecycle:   lc,
	})
	require.Error(t, err, "Unexpected success from module.")
	require.Contains(t, err.Error(), errMsg, "Unexpected error.")
	require.Nil(t, result.Scope, "Got non-nil scope.")
	lc.RequireStart().RequireStop()
}

func TestDefaults(t *testing.T) {
	tests := []struct {
		env  string
		noop bool
	}{
		{envfx.EnvProduction, false},
		{envfx.EnvStaging, false},
		{envfx.EnvTest, true},
		{envfx.EnvDevelopment, true},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			scope := succeeds(
				t,
				servicefx.Metadata{Name: "some-service"},
				envfx.Context{Environment: tt.env},
				newStaticProvider(t, nil),
			)
			if tt.noop {
				assert.True(t, scope == tally.NoopScope, "Expected no-op scope, got %v.", scope)
			} else {
				assert.False(t, scope == tally.NoopScope, "Expected real scope, got no-op.")
			}
		})
	}
}

func TestConfigIncomplete(t *testing.T) {
	scope := succeeds(
		t,
		servicefx.Metadata{Name: "some-service"},
		envfx.Context{Environment: envfx.EnvDevelopment},
		newStaticProvider(t, map[string]interface{}{ConfigurationKey: map[string]interface{}{
			"includeHost": true,
		}}),
	)
	assert.True(t, scope == tally.NoopScope, "Expected no-op scope.")
}

func TestTallyError(t *testing.T) {
	fails(
		t,
		"service common tag is required",
		servicefx.Metadata{}, // Tally requires service name
		envfx.Context{Environment: envfx.EnvProduction},
		newStaticProvider(t, nil),
	)
}

func TestConfigDisabled(t *testing.T) {
	for _, env := range []string{envfx.EnvDevelopment, envfx.EnvProduction} {
		scope := succeeds(
			t,
			servicefx.Metadata{Name: "some-service"},
			envfx.Context{Environment: env},
			newStaticProvider(t, map[string]interface{}{
				ConfigurationKey: Configuration{Disabled: true},
			}),
		)
		assert.True(t, scope == tally.NoopScope, "Expected no-op scope.")
	}
}

func TestConfigHostname(t *testing.T) {
	cfg, err := newConfiguration(
		envfx.Context{Environment: envfx.EnvProduction},
		newStaticProvider(t, map[string]interface{}{
			ConfigurationKey: Configuration{IncludeHost: true},
		}),
	)
	require.NoError(t, err, "Unexpected error creating Configuration.")
	assert.Equal(
		t,
		Configuration{IncludeHost: true, Tags: map[string]string{}},
		cfg,
		"Unexpected Configuration.",
	)
}

func TestConfigTags(t *testing.T) {
	t.Run("user tags", func(t *testing.T) {
		cfg, err := newConfiguration(
			envfx.Context{
				Environment:        envfx.EnvProduction,
				RuntimeEnvironment: "tacos",
			},
			newStaticProvider(t, map[string]interface{}{
				ConfigurationKey: Configuration{Tags: map[string]string{"foo": "bar"}},
			}),
		)
		require.NoError(t, err, "Unexpected error creating Configuration.")
		assert.Equal(
			t,
			Configuration{Tags: map[string]string{
				"foo":          "bar",
				_runtimeEnvTag: "tacos",
			}},
			cfg,
			"Unexpected Configuration.",
		)
	})
	t.Run("no user tags", func(t *testing.T) {
		cfg, err := newConfiguration(
			envfx.Context{
				Environment:        envfx.EnvProduction,
				RuntimeEnvironment: "tacos",
			},
			newStaticProvider(t, map[string]interface{}{}),
		)
		require.NoError(t, err, "Unexpected error creating Configuration.")
		assert.Equal(
			t,
			Configuration{Tags: map[string]string{_runtimeEnvTag: "tacos"}},
			cfg,
			"Unexpected Configuration.",
		)
	})
}

func newStaticProvider(t *testing.T, m map[string]interface{}) config.Provider {
	p, err := config.NewStaticProvider(m)
	require.NoError(t, err, "failed to build Provider")
	return p
}
