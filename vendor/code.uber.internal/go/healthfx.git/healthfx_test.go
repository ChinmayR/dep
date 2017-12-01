package healthfx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	envfx "code.uber.internal/go/envfx.git"
	health "code.uber.internal/go/health.git"
	servicefx "code.uber.internal/go/servicefx.git"
	systemportfx "code.uber.internal/go/systemportfx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

func assertHealthy(t testing.TB, mux *http.ServeMux, path string) {
	request := httptest.NewRequest(http.MethodGet, path, nil /* body */)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code, "Expected 200 from job control health check.")
	assert.Equal(t, "OK", recorder.Body.String(), "Unexpected job control response body.")
}

// Combine all module dependencies.
// Can't use Params directly, because it embeds fx.In, not fx.Out.
type testParams struct {
	fx.Out

	envfx.Context
	servicefx.Metadata
	*zap.Logger
	*versionfx.Reporter
	systemportfx.Mux
}

func TestModule(t *testing.T) {
	mux := http.NewServeMux()

	var res struct {
		*health.Coordinator
		*WaitSet
	}

	// Add a named server.
	var lifeline = func(name string) interface{} {
		return func(w *WaitSet, lc fx.Lifecycle) {
			ready, err := res.WaitSet.Add(name)
			require.NoError(t, err, `Error adding %s.`, name)
			lc.Append(fx.Hook{OnStart: func(context.Context) error {
				ready.Ready()
				return nil
			}})
		}
	}

	app := fxtest.New(t,
		fx.Provide(func() testParams {
			return testParams{
				Context:  envfx.Context{Environment: envfx.EnvProduction},
				Metadata: servicefx.Metadata{Name: "foo"},
				Logger:   zap.NewNop(),
				Reporter: &versionfx.Reporter{},
				Mux:      mux,
			}
		}),
		Module,
		fx.Extract(&res),
		fx.Invoke(lifeline("server-one"), lifeline("server-two")),
	)

	require.Equal(
		t,
		health.RefusingTraffic,
		res.Coordinator.State(),
		"Unexpected initial health state.",
	)

	_, err := res.WaitSet.Add("server-one")
	assert.Error(t, err, "Should have failed to add same name a second time.")

	defer app.RequireStart().RequireStop()
	res.WaitSet.Wait()

	require.Equal(
		t,
		health.AcceptingTraffic,
		res.Coordinator.State(),
		"Coordinator should have automatically started accepting traffic.",
	)

	assertHealthy(t, mux, "/health")
	assertHealthy(t, mux, "/health/")
}

func TestVersionReportError(t *testing.T) {
	ver := &versionfx.Reporter{}
	ver.Report("health", Version)
	_, err := New(Params{Version: ver})
	assert.Contains(t, err.Error(), "already registered version")
}

func TestHealthEndpointInvokedByDefault(t *testing.T) {
	var r struct{ *versionfx.Reporter }

	fxtest.New(
		t,
		fx.Provide(func() testParams {
			return testParams{
				Context:  envfx.Context{Environment: envfx.EnvDevelopment},
				Logger:   zap.NewNop(),
				Reporter: &versionfx.Reporter{},
				Mux:      http.NewServeMux(),
			}
		}),
		Module,
		fx.Extract(&r),
	).RequireStart().RequireStop()

	assert.Equal(t, Version, r.Reporter.Version("healthfx"))
}
