package versionfx

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	envfx "code.uber.internal/go/envfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	tallyfx "code.uber.internal/go/tallyfx.git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/dig"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func assertReported(t testing.TB, scope tally.TestScope, pkg, version string) {
	key := fmt.Sprintf("heartbeat+component=%s,package=%s,version=%s", _name, pkg, version)
	snap := scope.Snapshot().Counters()[key]
	require.NotNil(t, snap, "No counter for package %q.", pkg)
	assert.Equal(t, int64(1), snap.Value(), "Unexpected count for package %q.", pkg)
}

func TestModule(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	result, err := New(Params{
		Lifecycle: lc,
		Scope:     tally.NoopScope,
		Metadata:  servicefx.Metadata{Name: "test-service", BuildHash: "abc"},
	})
	require.NoError(t, err, "Module failed.")
	require.NotNil(t, result.Reporter, "Got a nil reporter.")

	assert.Equal(t, dig.Version, result.Reporter.Version("dig"), "Unexpected version for dig.")
	assert.Equal(t, fx.Version, result.Reporter.Version("fx"), "Unexpected version for fx.")
	assert.Equal(t, envfx.Version, result.Reporter.Version("envfx"), "Unexpected version for envfx.")
	assert.Equal(t, servicefx.Version, result.Reporter.Version("servicefx"), "Unexpected version for servicefx.")
	assert.Equal(t, tallyfx.Version, result.Reporter.Version("tallyfx"), "Unexpected version for tallyfx.")
	assert.Equal(t, "abc", result.Reporter.Version("test-service"), "Unexpected version for service.")
	assert.Equal(t, runtime.Version(), result.Reporter.Version("go"), "Unexpected version for Go runtime.")

	lc.RequireStart().RequireStop()
}

func TestZeroValue(t *testing.T) {
	ver := &Reporter{}
	assert.Equal(t, "", ver.Version("foo"))
	ver.Report("foo", "1.0.0")
	assert.Equal(t, "1.0.0", ver.Version("foo"))
}

func TestSuccess(t *testing.T) {
	scope := tally.NewTestScope("" /* prefix */, nil /* tags */)
	ticks := make(chan time.Time)

	r := newReporter(scope, ticks)
	r.Report("foo", "1.0.0")
	assert.Equal(t, "", r.Version("bar"), "Unexpected version for unknown package.")
	assert.Equal(t, "1.0.0", r.Version("foo"), "Unexpected version for known package.")

	go r.start()
	ticks <- time.Now()
	r.stop()

	counters := scope.Snapshot().Counters()
	require.Equal(t, 2, len(counters), "Unexpected number of counters reported to Tally.")
	assertReported(t, scope, _name, Version)
	assertReported(t, scope, "foo", "1.0.0")
}

func TestLibraryNameCollision(t *testing.T) {
	r := newReporter(tally.NoopScope, nil /* ticks */)
	assert.NoError(t, r.Report("foo", "1.2.0"), "Unexpected error registering package first time.")
	assert.Error(t, r.Report("foo", "1.3.1"), "Expected error re-registering package.")
}
