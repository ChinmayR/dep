package uberfx

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx/fxtest"
)

func env(t testing.TB, key string, val string) func() {
	prev, present := os.LookupEnv(key)
	assert.NoError(t, os.Setenv(key, val), "Failed to override environment variable %q.", key)
	return func() {
		if !present {
			assert.NoError(t, os.Unsetenv(key), "Failed to unset environment variable %q.", key)
			return
		}
		assert.NoError(t, os.Setenv(key, prev), "Failed to restore environment variable %q.", key)
	}
}

func TestStartStop(t *testing.T) {
	unset := env(t, "UBER_CONFIG_DIR", "./testdata/config")
	defer unset()
	app := fxtest.New(t, Module)
	app.RequireStart().RequireStop()
}
