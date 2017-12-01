package maxprocsfx

import (
	"testing"

	versionfx "code.uber.internal/go/versionfx.git"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSet(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	app := fxtest.New(t, Module, fx.Provide(
		func() *zap.SugaredLogger { return zap.New(core).Sugar() },
		func() *versionfx.Reporter { return &versionfx.Reporter{} },
	))
	app.RequireStart().RequireStop()
	entries := logs.All()
	require.Equal(t, 2, len(entries), "expected exactly two log entries")

	setEntry := entries[0] // from OnStart
	assert.Equal(t, zap.InfoLevel, setEntry.Level)
	// This message changes depending on host machine, so just assert that it
	// likely came from this library.
	assert.Contains(t, setEntry.Message, "GOMAXPROCS")

	restoreEntry := entries[1] // from OnStop
	assert.Equal(t, zap.InfoLevel, restoreEntry.Level)
	assert.Contains(t, restoreEntry.Message, "resetting GOMAXPROCS")
}

func TestVersionError(t *testing.T) {
	reporter := &versionfx.Reporter{}
	reporter.Report("maxprocsfx", "foo")

	lc := fxtest.NewLifecycle(t)
	params := Params{
		Lifecycle: lc,
		Logger:    zap.S(),
		Version:   reporter,
	}
	err := Set(params)
	require.Error(t, Set(params), "expected error calling Set")
	assert.Contains(t, err.Error(), `already registered version "foo"`, "unexpected error message")
	lc.RequireStart().RequireStop()
}
