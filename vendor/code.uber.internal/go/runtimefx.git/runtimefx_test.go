package runtimefx

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	versionfx "code.uber.internal/go/versionfx.git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/config"
	"go.uber.org/fx/fxtest"
)

func TestCollector(t *testing.T) {
	t.Run("emits histograms", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		testScope := tally.NewTestScope("", nil)
		rc := newCollector(testScope, time.Millisecond)
		rc.start()
		defer rc.close(ctx)

		runtime.GC()
		rc.emit()

		snapshot := testScope.Snapshot()
		histograms := snapshot.Histograms()
		assertAtLeastOneHistogramValue(t, histograms, "num-goroutines+")
		assertAtLeastOneHistogramValue(t, histograms, "gomaxprocs+")
		assertAtLeastOneHistogramValue(t, histograms, "memory.heap+")
		assertAtLeastOneHistogramValue(t, histograms, "memory.heapidle+")
		assertAtLeastOneHistogramValue(t, histograms, "memory.heapinuse+")
		assertAtLeastOneHistogramValue(t, histograms, "memory.stack+")
		assertAtLeastOneHistogramValue(t, histograms, "memory.num-gc+")
	})

	t.Run("emits metrics every tick", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		testScope := tally.NewTestScope("", nil)
		rc := newCollector(testScope, 100*time.Millisecond)
		rc.start()
		defer rc.close(ctx)

		time.Sleep(120 * time.Millisecond)
		assertAtLeastOneHistogramValue(t, testScope.Snapshot().Histograms(), "memory.stack+")

		time.Sleep(120 * time.Millisecond)
		assertAtLeastOneHistogramValue(t, testScope.Snapshot().Histograms(), "memory.stack+")
	})

	t.Run("close timeout", func(t *testing.T) {
		testScope := tally.NewTestScope("", nil)
		rc := newCollector(testScope, 100*time.Millisecond)
		rc.start()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		cancel() // cancel right away

		err := rc.close(ctx)
		require.Error(t, err, "expected close error")
		require.Equal(t, context.Canceled, err, "error must be context.Canceled")
	})
}

func TestInvoke(t *testing.T) {
	t.Run("configuration error", func(t *testing.T) {
		cfg, cfgErr := config.NewStaticProvider(map[string]interface{}{
			"metrics": map[string]interface{}{
				"runtime": map[string]interface{}{
					"disabled": "-1",
				},
			},
		})
		require.NoError(t, cfgErr, "failed to create config")
		p := Params{
			Version: &versionfx.Reporter{},
			Config:  cfg,
		}
		err := run(p)
		require.Error(t, err, "config parsing should error out")
		assert.Contains(t, err.Error(), `"-1": invalid syntax`)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg, err := config.NewStaticProvider(map[string]interface{}{
			"metrics": map[string]interface{}{
				"runtime": map[string]interface{}{
					"disabled": "true",
				},
			},
		})
		require.NoError(t, err, "failed to create config")

		p := Params{
			Version: &versionfx.Reporter{},
			Config:  cfg,
		}
		require.NoError(t, run(p))
	})

	t.Run("default configuration", func(t *testing.T) {
		lc := fxtest.NewLifecycle(t)
		testScope := tally.NewTestScope("", nil)
		cfg, _ := config.NewStaticProvider(nil)
		p := Params{
			Version:   &versionfx.Reporter{},
			Config:    cfg,
			Lifecycle: lc,
			Scope:     testScope,
		}

		require.NoError(t, run(p))
		lc.RequireStart().RequireStop()
	})
}

func assertAtLeastOneHistogramValue(
	t *testing.T,
	h map[string]tally.HistogramSnapshot,
	n string,
) {
	atLeastOne := false
	for _, v := range h[n].Values() {
		if v != 0 {
			atLeastOne = true
			break
		}
	}

	assert.True(t, atLeastOne, fmt.Sprintf("histogram %q should have at least one non-zero value", n))
}
