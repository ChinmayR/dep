package systemportfx

import (
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"testing"

	envfx "code.uber.internal/go/envfx.git"
	versionfx "code.uber.internal/go/versionfx.git"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestModuleSuccess(t *testing.T) {
	tests := []struct {
		env         string
		wantWarning bool
	}{
		{envfx.EnvDevelopment, false},
		{envfx.EnvTest, false},
		{envfx.EnvProduction, true},
		{envfx.EnvStaging, true},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			lc := fxtest.NewLifecycle(t)
			core, logs := observer.New(zap.DebugLevel)
			params := Params{
				Lifecycle:   lc,
				Environment: envfx.Context{Environment: tt.env},
				Logger:      zap.New(core),
				Version:     &versionfx.Reporter{},
			}
			res, err := New(params)
			require.NoError(t, err, "Error calling module.")
			require.NotNil(t, res.Mux, "Got a nil mux.")

			res.Mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("test"))
			})
			defer lc.RequireStart().RequireStop()

			// Get the ephemeral port from the logs, just like a user would.
			records := logs.FilterMessageSnippet("HTTP server on system port").TakeAll()
			require.Equal(t, 1, len(records), "Unexpected number of logged messages.")
			context := records[0].Context
			require.Equal(t, 2, len(context), "Unexpected number of fields on startup log.")
			assert.Equal(t, "addr", context[1].Key, "Expected second log field to be addr.")
			addr := context[1].Interface

			if tt.wantWarning {
				warnings := logs.FilterMessageSnippet("ephemeral port").TakeAll()
				require.Equal(t, 1, len(warnings), "Unexpected number of logged warnings.")
				require.Equal(t, zap.WarnLevel, warnings[0].Level, "Unexpected log level.")
			}

			resp, err := http.Get(fmt.Sprintf("http://%v/", addr))
			require.NoError(t, err, "Request to systemport failed.")
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			require.NoError(t, err, "Error reading response body.")
			assert.Equal(t, "test", string(body), "Unexpected body.")
		})
	}
}

func TestInvalidPorts(t *testing.T) {
	tests := []struct {
		port string
		err  string
	}{
		{"foo", "is not an integer"},
		{"-1", "outside the uint16 range"},
		{fmt.Sprint(math.MaxUint16 + 1), "outside the uint16 range"},
	}
	for _, tt := range tests {
		lc := fxtest.NewLifecycle(t)
		params := Params{
			Lifecycle: lc,
			Environment: envfx.Context{
				Environment: envfx.EnvDevelopment,
				SystemPort:  tt.port,
			},
			Version: &versionfx.Reporter{},
			Logger:  zap.NewNop(),
		}
		_, err := New(params)
		require.Error(t, err, "Expected error running on port %q.", tt.port)
		assert.Contains(t, err.Error(), tt.err, "Unexpected error message.")

		lc.RequireStart().RequireStop()
	}
}

func TestParsePortSuccess(t *testing.T) {
	p, err := parsePort("8080")
	require.NoError(t, err, "Failed to parse port.")
	assert.Equal(t, 8080, p, "Unexpected port.")
}

func TestVersionReportError(t *testing.T) {
	ver := &versionfx.Reporter{}
	ver.Report(_name, Version)
	params := Params{
		Environment: envfx.Context{Environment: envfx.EnvDevelopment},
		Version:     ver,
	}
	_, err := New(params)
	assert.Contains(t, err.Error(), "already registered version")
}
