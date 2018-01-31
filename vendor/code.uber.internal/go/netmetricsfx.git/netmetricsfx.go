// Package netmetricsfx provides a metrics registry for both Prometheus and Tally.
package netmetricsfx

import (
	"context"
	"time"

	versionfx "code.uber.internal/go/versionfx.git"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/multierr"
	"go.uber.org/net/metrics"
	"go.uber.org/net/metrics/tallypush"
)

const (
	_metricPackageName = "netmetrics"
	_packageName       = "netmetricsfx"

	// Todo expose configuration variable
	_tallyPushInterval = 500 * time.Millisecond

	// Version is the current package version.
	Version = "1.0.0"
)

// Module provides netmetrics integration for services.
var Module = fx.Provide(New)

// Params defines the dependencies of this module.
type Params struct {
	fx.In

	Lifecycle fx.Lifecycle
	Reporter  *versionfx.Reporter `optional:"true"`
	Scope     tally.Scope         `optional:"true"`
}

// Result defines the values produced by this module.
type Result struct {
	fx.Out

	Scope *metrics.Scope
}

// New creates a new net/metrics scope.
func New(p Params) (Result, error) {
	if err := multierr.Combine(
		p.Reporter.Report(_packageName, Version),
		p.Reporter.Report(_metricPackageName, metrics.Version),
	); err != nil {
		return Result{}, err
	}

	root := metrics.New()
	scope := root.Scope()

	if p.Scope == nil {
		return Result{Scope: scope}, nil
	}

	stop := func() {}
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			var err error
			stop, err = root.Push(tallypush.New(p.Scope), _tallyPushInterval)
			return err
		},
		OnStop: func(context.Context) error {
			stop()
			return nil
		},
	})

	return Result{Scope: scope}, nil
}
