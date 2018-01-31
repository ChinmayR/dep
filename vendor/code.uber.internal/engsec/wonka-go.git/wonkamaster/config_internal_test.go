package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/config"
)

func TestLoadConfig(t *testing.T) {
	testLoadConfig := func(content string) (appConfig, error) {
		p, err := config.NewYAMLProviderFromBytes([]byte(content))
		require.NoError(t, err)
		cfg, err := loadConfig(p)
		return cfg, err
	}

	t.Run("no_rate", func(t *testing.T) {
		cfg, err := testLoadConfig(`rate:`)
		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.Nil(t, cfg.Rates.Global)
		require.Empty(t, cfg.Rates.Endpoints)
	})
	t.Run("simple_rates", func(t *testing.T) {
		content := `
rate:
  global:
    events_per_second: 10000
    burst_limit: 20000
  endpoints:
  - path: /thehose
    events_per_second: 1000
    burst_limit: 1000
  - path: /health
    events_per_second: 0
    burst_limit: 0`

		cfg, err := testLoadConfig(content)
		require.NoError(t, err)
		require.NotNil(t, cfg)

		r := cfg.Rates
		require.Equal(t, float64(10000), r.Global.R)
		require.Equal(t, 20000, r.Global.B)

		require.Len(t, r.Endpoints, 2)
		require.Equal(t, "/thehose", r.Endpoints[0].Path)
		require.Equal(t, float64(1000), r.Endpoints[0].R)
		require.Equal(t, 1000, r.Endpoints[0].B)

		require.Equal(t, "/health", r.Endpoints[1].Path)
		require.Equal(t, float64(0), r.Endpoints[1].R)
		require.Equal(t, 0, r.Endpoints[1].B)
	})
	t.Run("cert_auth_override", func(t *testing.T) {
		const content = `
cert_auth_override:
  grant:
    signed_after: 2017-11-05T12:00:00Z
    signed_before: 2017-11-05T13:00:00Z
    enforce_until: 2017-11-05T15:00:00Z`

		cfg, err := testLoadConfig(content)
		require.NoError(t, err)
		require.NotNil(t, cfg)

		parse := func(s string) time.Time {
			v, err := time.Parse(time.RFC3339, s)
			require.NoError(t, err, "failed to create test time %q", s)
			return v
		}

		require.NotNil(t, cfg.CertAuthentiationOverride)
		require.Equal(t, parse("2017-11-05T12:00:00Z"), cfg.CertAuthentiationOverride.Grant.SignedAfter)
		require.Equal(t, parse("2017-11-05T13:00:00Z"), cfg.CertAuthentiationOverride.Grant.SignedBefore)
		require.Equal(t, parse("2017-11-05T15:00:00Z"), cfg.CertAuthentiationOverride.Grant.EnforceUntil)
	})
}
