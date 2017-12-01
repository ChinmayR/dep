package envfx

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func file(t testing.TB, contents string) (name string, remove func()) {
	f, err := ioutil.TempFile("", "envfx-test")
	require.NoError(t, err, "Failed to create temporary file for test.")
	_, err = f.Write([]byte(contents))
	require.NoError(t, err, "Failed to write information to file.")

	return f.Name(), func() {
		require.NoError(t, f.Close(), "Failed to close temp file.")
		require.NoError(t, os.Remove(f.Name()), "Failed to remove temp file.")
	}
}

func TestEnvironment(t *testing.T) {
	tests := []struct {
		env  string
		want string
	}{
		{"", EnvDevelopment},
		{"foo", EnvDevelopment},
		{EnvProduction, EnvProduction},
		{EnvStaging, EnvStaging},
		{EnvTest, EnvTest},
		{EnvDevelopment, EnvDevelopment},
	}

	for _, tt := range tests {
		unset := env(t, _environmentKey, tt.env)
		assert.Equal(t, tt.want, New().Environment.Environment, "Unexpected result with environment variable %q.", tt.env)
		unset()
	}
}

func TestZone(t *testing.T) {
	for _, e := range []string{"", "foo", "sjc1"} {
		unset := env(t, _zoneKey, e)
		assert.Equal(t, e, New().Environment.Zone, "Unexpected result with environment variable %q.", e)
		unset()
	}
}

func TestHostname(t *testing.T) {
	assert.NotEqual(t, "", New().Environment.Hostname)
}

func TestDeployment(t *testing.T) {
	for _, e := range []string{"", "foo", "pod01"} {
		unset := env(t, _deploymentKey, e)
		assert.Equal(t, e, New().Environment.Deployment, "Unexpected result with environment variable %q.", e)
		unset()
	}
}

func TestContainerName(t *testing.T) {
	for _, e := range []string{"", "foo"} {
		unset := env(t, _containerNameKey, e)
		assert.Equal(t, e, New().Environment.ContainerName, "Unexpected result with environment variable %q.", e)
		unset()
	}
}

func TestConfigDirs(t *testing.T) {
	for c, e := range map[string][]string{
		"":        {_defaultConfigDir},
		"foo":     {"foo"},
		":":       {"", ""},
		"foo:":    {"foo", ""},
		"foo:bar": {"foo", "bar"},
	} {
		unset := env(t, _configDirKey, c)
		assert.Equal(t, e, New().Environment.ConfigDirs(), "Unexpected result with environment variable %q.", e)
		unset()
	}
}

func TestSystemPort(t *testing.T) {
	for _, port := range []string{"", "foo", "8080"} {
		unset := env(t, _portSystemKey, port)
		assert.Equal(t, port, New().Environment.SystemPort, "Unexpected result with environment variable %q.", port)
		unset()
	}
}

func TestReadValue(t *testing.T) {
	tests := []struct {
		env  bool
		file bool
		want string
	}{
		{true, true, "env"},
		{true, false, "env"},
		{false, true, "file"},
		{false, false, ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("Env%vFile%v", tt.env, tt.file), func(t *testing.T) {
			if tt.env {
				defer env(t, "TEST_ENVFX", "env")()
			}

			// We always need to pass a path to readValue, but sometimes the file
			// shouldn't be present.
			path, rm := file(t, "file")
			if tt.file {
				defer rm()
			} else {
				rm()
			}

			val, ok := readValue("TEST_ENVFX", path)
			assert.Equal(t, tt.want, val, "Unexpected value read out")
			assert.Equal(t, tt.env, ok, "Unexpected value")
		})
	}
}

func TestApplicationID(t *testing.T) {
	for _, appID := range []string{"", "fx", "tbd"} {
		unset := env(t, _appIDKey, appID)
		assert.Equal(t, appID, New().Environment.ApplicationID, "Unexpected result with environment variable %q.", appID)
		unset()
	}
}

func TestPipeline(t *testing.T) {
	for _, pipeline := range []string{"", "prod", "test"} {
		unset := env(t, _pipelineKey, pipeline)
		assert.Equal(t, pipeline, New().Environment.Pipeline, "Unexpected result with environment variable %q.", pipeline)
		unset()
	}
}

func TestCluster(t *testing.T) {
	for _, clst := range []string{"", "adhoc", "mgmt"} {
		unset := env(t, _clusterKey, clst)
		assert.Equal(t, clst, New().Environment.Cluster, "Unexpected result with environment variable %q.", clst)
		unset()
	}
}

func TestPod(t *testing.T) {
	// Without clobbering the actual pod file, we can only test the no-file
	// path.
	assert.Zero(t, New().Environment.Pod, "Unexpected result without a pod file.")
}

func TestRuntimeEnvironment(t *testing.T) {
	for _, renv := range []string{"production", "staging"} {
		unset := env(t, _runtimeEnvironmentKey, renv)
		assert.Equal(t, renv, New().Environment.RuntimeEnvironment, "Unexpected result with runtime environment variable %q.", renv)
		unset()
	}
}
