// Package debugfx registers debug handlers on the system port, including the
// pprof profiling and runtime introspection handlers.
package debugfx

import (
	"fmt"
	"net/http"
	_ "net/http/pprof" // registers on DefaultServeMux

	systemportfx "code.uber.internal/go/systemportfx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	"go.uber.org/fx"
)

const (
	_name = "debugfx"

	// Version is the current package version.
	Version = "1.0.0"
)

// Params defines the dependencies of this module.
type Params struct {
	fx.In

	Version *versionfx.Reporter
	Mux     systemportfx.Mux
}

// Module registers debug handlers on the system port mux. It includes
// pprof handlers.
//
// Note: debugfx imports net/http/pprof which has the side effect of
// also registering the pprof handlers on http.DefaultServeMux.
var Module = fx.Invoke(run)

func run(p Params) error {
	if err := p.Version.Report(_name, Version); err != nil {
		return fmt.Errorf("failed to report debugfx version: %v", err)
	}

	registerPProf(p.Mux)
	return nil
}

func registerPProf(mux systemportfx.Mux) {
	// Since all the pprof handlers are registered on http.DefaultServeMux
	// as a side effect of importing net/http/pprof, let's redirect everything
	// under /debug/pprof to the default serve mux.
	mux.Handle("/debug/pprof/", http.DefaultServeMux)
}
