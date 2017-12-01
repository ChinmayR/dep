package runtimefx

import (
	"context"
	"fmt"
	"runtime"
	"time"

	envfx "code.uber.internal/go/envfx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	"github.com/uber-go/tally"
	"go.uber.org/config"
	"go.uber.org/fx"
)

const (
	// _numGCThreshold comes from the PauseNs buffer size https://golang.org/pkg/runtime/#MemStats
	_numGCThreshold = uint32(len(runtime.MemStats{}.PauseEnd))

	// by default how often these metrics are reported
	_defaultInterval = time.Second

	// Version is the current package version
	Version = "v1.0.0"

	// ConfigurationKey is the portion of service configuration that this module reads
	ConfigurationKey = "metrics.runtime"
)

// Params defines the dependencies of the runtimefx
type Params struct {
	fx.In

	Config    config.Provider
	Scope     tally.Scope
	Env       envfx.Context
	Lifecycle fx.Lifecycle
	Version   *versionfx.Reporter
}

// Module queues an Invoke that on service startup will start
// emitting metrics on memory usage, numbers of goroutines, GC pauses, etc.
//
// By default, the reporting is turned off in develpoment environment.
//
// In YAML, the configuration section may look something like this:
//
//   metrics:
//     runtime:
//       disabled: false
var Module = fx.Invoke(run)

// RuntimeConfig contains configuration for initializing runtime metrics
type RuntimeConfig struct {
	Disabled bool `yaml:"disabled"`
}

func run(p Params) error {
	if err := p.Version.Report("runtimefx", Version); err != nil {
		return fmt.Errorf("failed to report runtimefx version: %v", err)
	}

	cfg := RuntimeConfig{}
	if p.Env.Environment == envfx.EnvDevelopment {
		cfg.Disabled = true
	}

	if err := p.Config.Get(ConfigurationKey).Populate(&cfg); err != nil {
		return fmt.Errorf("failed to parse metrics.runtime config: %v", err)
	}

	if cfg.Disabled {
		return nil
	}

	rc := newCollector(p.Scope, _defaultInterval)
	rc.start()

	p.Lifecycle.Append(fx.Hook{OnStop: rc.close})
	return nil
}

func newCollector(scope tally.Scope, interval time.Duration) *collector {
	var memstats runtime.MemStats
	runtime.ReadMemStats(&memstats)

	// not sure if this is the best scale to use, open to suggestions
	memBuckets := tally.MustMakeExponentialValueBuckets(2, 2, 37) // 2bytes-128GiB

	return &collector{
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		interval: interval,
		numGoRoutines: scope.Histogram(
			"num-goroutines",
			combineValues(
				tally.MustMakeLinearValueBuckets(0, 10, 10),
				tally.MustMakeLinearValueBuckets(100, 100, 10),
				tally.MustMakeLinearValueBuckets(1000, 1000, 5),
				tally.MustMakeLinearValueBuckets(5000, 2500, 15), // up to 40k
			),
		),
		goMaxProcs: scope.Histogram(
			"gomaxprocs",
			// number of cores should be pretty small, but some stuff runs dedicated hardware
			tally.MustMakeLinearValueBuckets(1, 1, 128),
		),
		memoryHeap:      scope.Histogram("memory.heap", memBuckets),
		memoryHeapIdle:  scope.Histogram("memory.heapidle", memBuckets),
		memoryHeapInuse: scope.Histogram("memory.heapinuse", memBuckets),
		memoryStack:     scope.Histogram("memory.stack", memBuckets),
		numGC: scope.Histogram(
			"memory.num-gc",
			tally.ValueBuckets{},
		),
		gcPauseMs: scope.Histogram(
			"memory.gc-pause",
			combineDurations(
				tally.MustMakeLinearDurationBuckets(0, 100, 10),
				tally.MustMakeLinearDurationBuckets(time.Microsecond, time.Microsecond*50, 20),
				tally.MustMakeLinearDurationBuckets(time.Millisecond, time.Millisecond, 10),
				tally.MustMakeLinearDurationBuckets(time.Millisecond*5, time.Millisecond*5, 9),
				tally.MustMakeLinearDurationBuckets(time.Millisecond*10, time.Millisecond*10, 9),
				tally.MustMakeLinearDurationBuckets(time.Millisecond*100, time.Millisecond*100, 10),
			),
		),
		lastNumGC: memstats.NumGC,
	}
}

type collector struct {
	stop     chan struct{} // stop collecting metrics
	done     chan struct{} // collection fully stopped, safe to release the stop hook
	interval time.Duration

	numGoRoutines   tally.Histogram
	goMaxProcs      tally.Histogram
	memoryHeap      tally.Histogram
	memoryHeapIdle  tally.Histogram
	memoryHeapInuse tally.Histogram
	memoryStack     tally.Histogram
	numGC           tally.Histogram
	lastNumGC       uint32
	gcPauseMs       tally.Histogram
}

func (r *collector) emit() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	r.numGoRoutines.RecordValue(float64(runtime.NumGoroutine()))
	r.goMaxProcs.RecordValue(float64(runtime.GOMAXPROCS(0)))
	r.memoryHeap.RecordValue(float64(memStats.HeapAlloc))
	r.memoryHeapIdle.RecordValue(float64(memStats.HeapIdle))
	r.memoryHeapInuse.RecordValue(float64(memStats.HeapInuse))
	r.memoryStack.RecordValue(float64(memStats.StackInuse))

	// memStats.NumGC is a perpetually incrementing counter (unless it wraps at 2^32)
	num := memStats.NumGC
	lastNum := r.lastNumGC
	r.lastNumGC = num
	if delta := num - lastNum; delta > 0 {
		r.numGC.RecordValue(float64(delta))
		// Match the MemStats buffer and generate only the last _numGCThreshold
		if delta >= _numGCThreshold {
			lastNum = num - _numGCThreshold
		}
		for i := lastNum; i != num; i++ {
			pause := memStats.PauseNs[i%uint32(len(memStats.PauseNs))]
			r.gcPauseMs.RecordDuration(time.Duration(pause))
		}
	}
}

func (r *collector) start() {
	go func() {
		defer close(r.done)

		ticker := time.NewTicker(r.interval)
		for {
			select {
			case <-ticker.C:
				r.emit()
			case <-r.stop:
				ticker.Stop()
				return
			}
		}
	}()
}

func (r *collector) close(ctx context.Context) error {
	close(r.stop)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
	}
	return nil
}
