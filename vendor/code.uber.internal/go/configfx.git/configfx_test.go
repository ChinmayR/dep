package configfx

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"code.uber.internal/go/configfx.git/load"
	envfx "code.uber.internal/go/envfx.git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestConfigModuleZeroInitializedContext(t *testing.T) {
	_, err := New(Params{
		Environment: envfx.Context{},
	})

	// this works only because current dir doesn't have any config files.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no providers were loaded")
}

func TestConfigFilesBasedOnContext(t *testing.T) {
	type result struct {
		env   map[string]string
		files []load.FileInfo
	}

	tests := map[string]result{
		"laptop": {
			env: map[string]string{"UBER_ENVIRONMENT": ""},
			files: []load.FileInfo{
				{Name: "base.yaml", Interpolate: true},
				{Name: "development.yaml", Interpolate: true},
				{Name: "secrets.yaml", Interpolate: false},
			},
		},
		"mesos": {
			env: map[string]string{
				"UBER_ENVIRONMENT":     envfx.EnvProduction,
				"MESOS_CONTAINER_NAME": "mesos-was-here123",
				"UBER_DATACENTER":      "sjc1",
			},
			files: []load.FileInfo{
				{Name: "base.yaml", Interpolate: true},
				{Name: "production.yaml", Interpolate: true},
				{Name: "production-sjc1.yaml", Interpolate: true},
				{Name: "mesos.yaml", Interpolate: true},
				{Name: "mesos-production.yaml", Interpolate: true},
				{Name: "mesos-production-sjc1.yaml", Interpolate: true},
				{Name: "secrets.yaml", Interpolate: false},
				{Name: "secrets-sjc1.yaml", Interpolate: false},
			},
		},
		"deployment": {
			env: map[string]string{
				"UBER_ENVIRONMENT":        envfx.EnvStaging,
				"UDEPLOY_DEPLOYMENT_NAME": "my_lucky_numbers_are_3_and_5",
				"UBER_DATACENTER":         "DCA9",
			},
			files: []load.FileInfo{
				{Name: "base.yaml", Interpolate: true},
				{Name: "staging.yaml", Interpolate: true},
				{Name: "staging-DCA9.yaml", Interpolate: true},
				{Name: "deployment-my_lucky_numbers_are__and_.yaml", Interpolate: true},
				{Name: "deployment-my_lucky_numbers_are__and_-DCA9.yaml", Interpolate: true},
				{Name: "secrets.yaml", Interpolate: false},
				{Name: "secrets-DCA9.yaml", Interpolate: false},
			},
		},
		"staging_in_prod": {
			env: map[string]string{
				"UBER_ENVIRONMENT":         envfx.EnvProduction,
				"UBER_RUNTIME_ENVIRONMENT": envfx.EnvStaging,
				"UDEPLOY_DEPLOYMENT_NAME":  "my_lucky_numbers_are_3_and_5",
				"UBER_DATACENTER":          "DCA9",
			},
			files: []load.FileInfo{
				{Name: "base.yaml", Interpolate: true},
				{Name: "staging.yaml", Interpolate: true},
				{Name: "staging-DCA9.yaml", Interpolate: true},
				{Name: "deployment-my_lucky_numbers_are__and_.yaml", Interpolate: true},
				{Name: "deployment-my_lucky_numbers_are__and_-DCA9.yaml", Interpolate: true},
				{Name: "secrets.yaml", Interpolate: false},
				{Name: "secrets-DCA9.yaml", Interpolate: false},
			},
		},
	}

	for name, info := range tests {
		t.Run(name, func(t *testing.T) {
			for k, v := range info.env {
				defer setEnv(k, v)()
			}

			var x struct{ Context envfx.Context }
			app := fx.New(
				envfx.Module,
				fx.Extract(&x),
			)

			require.NoError(t, app.Start(context.Background()))
			defer func() { assert.NoError(t, app.Stop(context.Background())) }()
			assert.Equal(t, info.files, getFiles(x.Context))
		})
	}
}

func setEnv(key, value string) func() {
	res := func() { os.Unsetenv(key) }
	if oldVal, ok := os.LookupEnv(key); ok {
		res = func() { os.Setenv(key, oldVal) }
	}

	os.Setenv(key, value)
	return res
}

func writeFile(t *testing.T, dir, name, contents string) {
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(dir, name),
		[]byte(contents),
		os.ModePerm))
}

func TestLookupFunc(t *testing.T) {
	dir, err := ioutil.TempDir("", t.Name())
	require.NoError(t, err)
	defer func() { assert.NoError(t, os.RemoveAll(dir)) }()

	writeFile(t, dir, "base.yaml", "source: base\nbase: ${INTERPOLATE:13}")
	file := []load.FileInfo{{Name: "base.yaml", Interpolate: true}}
	lookUp := func(key string) (string, bool) {
		if key == "INTERPOLATE" {
			return "VALUE", true
		}
		return "", false
	}
	p, err := load.FromFiles([]string{dir}, file, lookUp)
	require.NoError(t, err)
	assert.Equal(t, "VALUE", p.Get("base").String())
}

func TestE2ELaptopLoad(t *testing.T) {
	dir, err := ioutil.TempDir("", t.Name())
	require.NoError(t, err)
	defer func() { assert.NoError(t, os.RemoveAll(dir)) }()

	writeFile(t, dir, "base.yaml", "source: base\nbase: ${INTERPOLATE:13}")
	writeFile(t, dir, "development.yaml", "source: development\ndev: ${INTERPOLATE:42}")
	writeFile(t, dir, "secrets.yaml", "password: ${SECRET:1111}")

	defer setEnv("INTERPOLATE", "666")()
	defer setEnv("UBER_CONFIG_DIR", dir)()

	// `make test` sets the environment to "test"
	defer setEnv("UBER_ENVIRONMENT", "")()
	var x struct{ Context envfx.Context }
	app := fx.New(
		envfx.Module,
		fx.Extract(&x),
	)

	require.NoError(t, app.Start(context.Background()))
	defer func() { assert.NoError(t, app.Stop(context.Background())) }()

	cfg, err := New(Params{Environment: x.Context})
	require.NoError(t, err)

	assert.Equal(t, "development", cfg.Provider.Get("source").String())
	assert.Equal(t, "666", cfg.Provider.Get("dev").String())
	assert.Equal(t, "666", cfg.Provider.Get("base").String())
	assert.Equal(t, "${SECRET:1111}", cfg.Provider.Get("password").String())
}
