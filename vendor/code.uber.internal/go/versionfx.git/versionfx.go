// Package versionfx provides simple version heartbeating for packages.
package versionfx

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	configfx "code.uber.internal/go/configfx.git"
	envfx "code.uber.internal/go/envfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	tallyfx "code.uber.internal/go/tallyfx.git"
	"github.com/uber-go/tally"
	"go.uber.org/config"
	"go.uber.org/dig"
	"go.uber.org/fx"
	"go.uber.org/multierr"
)

const (
	// Version is the current package version.
	Version = "1.1.0"

	_name     = "versionfx"
	_interval = 10 * time.Second
)

// Module provides a *Reporter, which allows package authors to track the
// versions of their package running in production. It neither requires nor
// accepts any configuration.
var Module = fx.Provide(New)

type spec struct {
	str     string
	counter tally.Counter
}

// A Reporter periodically reports the running version of a package to Uber's
// central telemetry systems. It's designed to let package authors track the
// versions of their code running in production, and potentially identify
// services using deprecated or buggy releases.
type Reporter struct {
	versionsMu sync.RWMutex
	versions   map[string]spec
	scope      tally.Scope
	tickC      <-chan time.Time
	stopC      chan struct{}
	stoppedC   chan struct{}
}

func newReporter(scope tally.Scope, ticks <-chan time.Time) *Reporter {
	r := &Reporter{
		scope:    scope.Tagged(map[string]string{"component": _name}),
		tickC:    ticks,
		stopC:    make(chan struct{}),
		stoppedC: make(chan struct{}),
	}
	r.Report(_name, Version)
	return r
}

// Report adds a version hearbeat for a particular package. It returns an
// error if the package already has a version registered.
func (r *Reporter) Report(pkg, version string) error {
	r.versionsMu.Lock()
	defer r.versionsMu.Unlock()

	if r.versions == nil {
		r.versions = make(map[string]spec)
	}
	if r.scope == nil {
		r.scope = tally.NoopScope
	}

	if spec, ok := r.versions[pkg]; ok {
		return fmt.Errorf("already registered version %q for package %q", spec.str, pkg)
	}

	r.versions[pkg] = spec{
		str:     version,
		counter: r.scope.Tagged(map[string]string{"package": pkg, "version": version}).Counter("heartbeat"),
	}
	return nil
}

// Version returns the reported version for a package.
func (r *Reporter) Version(pkg string) string {
	r.versionsMu.RLock()
	defer r.versionsMu.RUnlock()
	if r.versions == nil {
		return ""
	}
	return r.versions[pkg].str
}

func (r *Reporter) stop() {
	close(r.stopC)
	<-r.stoppedC
}

func (r *Reporter) start() {
	defer close(r.stoppedC)
	for {
		select {
		case <-r.tickC:
			r.push()
		case <-r.stopC:
			return
		}
	}
}

func (r *Reporter) push() {
	r.versionsMu.Lock()
	defer r.versionsMu.Unlock()

	for _, spec := range r.versions {
		spec.counter.Inc(1)
	}
}

// Params defines the dependencies of the versionfx module.
type Params struct {
	fx.In

	Lifecycle fx.Lifecycle
	Scope     tally.Scope
	Metadata  servicefx.Metadata
}

// Result defines the objects that the versionfx module provides.
type Result struct {
	fx.Out

	Reporter *Reporter
}

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	timer := time.NewTicker(_interval)
	r := newReporter(p.Scope, timer.C)

	if err := multierr.Combine(
		// Since versionfx's dependencies can't use it without introducing cycles,
		// report on their behalf.
		r.Report("dig", dig.Version),
		r.Report("fx", fx.Version),
		r.Report("envfx", envfx.Version),
		r.Report("servicefx", servicefx.Version),
		r.Report("tallyfx", tallyfx.Version),
		r.Report("config", config.Version),
		r.Report("configfx", configfx.Version),

		// Also report the build hash of the running service.
		r.Report(p.Metadata.Name, p.Metadata.BuildHash),

		// We'll also want to monitor the penetration of new Go releases.
		r.Report("go", runtime.Version()),
	); err != nil {
		// Since we're the first ones reporting version info, we can't get into
		// this branch unless *we* mistakenly report the same package name twice.
		return Result{}, err
	}

	p.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go r.start()
			return nil
		},
		OnStop: func(context.Context) error {
			r.stop()
			timer.Stop()
			return nil
		},
	})
	return Result{
		Reporter: r,
	}, nil
}
