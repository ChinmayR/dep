package servicefx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/config"
)

func valid() map[string]interface{} {
	return map[string]interface{}{
		ConfigurationKey: map[string]interface{}{"name": "foo"},
	}
}

func TestName(t *testing.T) {
	tests := []struct {
		cfg   map[string]interface{}
		fails bool
	}{
		{cfg: nil, fails: true},
		{cfg: map[string]interface{}{}, fails: true},
		{
			cfg: map[string]interface{}{ConfigurationKey: map[string]interface{}{
				"name": "",
			}},
			fails: true,
		},
		{cfg: valid(), fails: false},
	}
	for _, tt := range tests {
		cfg, err := config.NewStaticProvider(tt.cfg)
		require.NoError(t, err, "failed to create config")
		result, err := New(Params{
			Config: cfg,
		})
		if tt.fails {
			assert.Error(t, err, "Expected an error using config %v.", tt.cfg)
		} else {
			assert.NoError(t, err, "Expected success using config %v.", tt.cfg)
			assert.NotZero(t, result.Metadata.Name, "Got empty name.")
		}
	}
}

func TestBuildInfo(t *testing.T) {
	// Expect the default values, since we're not testing with the linker flags
	// set.
	cfg, cfgErr := config.NewStaticProvider(valid())
	require.NoError(t, cfgErr, "failed to create config")
	result, err := New(Params{
		Config: cfg,
	})
	require.NoError(t, err, "Unexpected error calling module.")
	assert.Equal(t, time.Time{}, result.Metadata.BuildTime, "Unexpected build time.")
	assert.Equal(t, "unknown@unknown", result.Metadata.BuildUserHost, "Unexpected build user and host.")
	assert.Equal(t, "unknown-build-hash", result.Metadata.BuildHash, "Unexpected build hash.")
}
