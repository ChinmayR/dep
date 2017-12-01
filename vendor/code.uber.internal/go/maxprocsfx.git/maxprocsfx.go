// Package maxprocsfx automatically adjusts the Go runtime's concurrency in
// containerized environments.
package maxprocsfx

import (
	"context"

	versionfx "code.uber.internal/go/versionfx.git"

	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/fx"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

// Version is the current package version.
const Version = "1.0.0"

// Module adjusts runtime.GOMAXPROCS to match the CPU quota configured in
// Linux containers. In non-containerized or non-Linux environments, it's a
// no-op.
var Module = fx.Invoke(Set)

// Params defines the dependencies of this module.
type Params struct {
	fx.In

	Lifecycle fx.Lifecycle
	Version   *versionfx.Reporter
	Logger    *zap.SugaredLogger
}

// Set uses the provided dependencies to alter runtime concurrency on
// application startup and undo any alterations before shutting down.
func Set(p Params) error {
	if err := multierr.Append(
		p.Version.Report("maxprocs", maxprocs.Version),
		p.Version.Report("maxprocsfx", Version),
	); err != nil {
		return err
	}
	undo := func() {}
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			var err error
			undo, err = maxprocs.Set(maxprocs.Logger(p.Logger.Infof))
			return err
		},
		OnStop: func(context.Context) error {
			undo()
			return nil
		},
	})
	return nil
}
