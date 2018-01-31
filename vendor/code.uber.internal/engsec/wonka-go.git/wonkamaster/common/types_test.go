package common_test

import (
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"

	"github.com/stretchr/testify/require"
	"go.uber.org/config"
)

func TestUnmarshalText(t *testing.T) {
	regExp := common.TaskRegexp{}
	err := regExp.UnmarshalText([]byte("^mesos-master\\d{2}-\\w{3}\\d\\.prod\\.uber\\.internal$"))
	require.Nil(t, err)
}

func TestUnmarshalTextHandlesErrors(t *testing.T) {
	regExp := common.TaskRegexp{}
	err := regExp.UnmarshalText([]byte("$^\\Ks"))
	require.NotNil(t, err)
}

func TestAuthGrantOverrideUnmarshalYAML(t *testing.T) {
	loadFromYAML := func(content string) (*common.AuthGrantOverride, error) {
		p, err := config.NewYAMLProviderFromBytes([]byte(content))
		require.NoError(t, err, "could not create new YAML config provider")

		ag := new(common.AuthGrantOverride)
		err = p.Get(config.Root).Populate(ag)
		return ag, err
	}

	parse := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		require.NoError(t, err, "failed to create test time %q", s)
		return v
	}

	t.Run("valid", func(t *testing.T) {
		ag, err := loadFromYAML(`
signed_after: 2017-11-05T12:00:00Z
signed_before: 2017-11-05T13:00:00Z
enforce_until: 2017-11-05T15:00:00Z`)
		require.NoError(t, err)
		require.NotNil(t, ag)
		require.Equal(t, parse("2017-11-05T12:00:00Z"), ag.SignedAfter)
		require.Equal(t, parse("2017-11-05T13:00:00Z"), ag.SignedBefore)
		require.Equal(t, parse("2017-11-05T15:00:00Z"), ag.EnforceUntil)
	})
	t.Run("bad_signed_after", func(t *testing.T) {
		_, err := loadFromYAML(`
signed_after: foo
signed_before: 2017-11-05T13:00:00Z
enforce_until: 2017-11-05T15:00:00Z`)
		require.Error(t, err)
	})
	t.Run("bad_signed_before", func(t *testing.T) {
		_, err := loadFromYAML(`
signed_after: 2017-11-05T12:00:00Z
signed_before: foo
enforce_until: 2017-11-05T15:00:00Z`)
		require.Error(t, err)
	})
	t.Run("bad_enforce_until", func(t *testing.T) {
		_, err := loadFromYAML(`
signed_after: 2017-11-05T12:00:00Z
signed_before: 2017-11-05T13:00:00Z
enforce_until: foo`)
		require.Error(t, err)
	})
}
