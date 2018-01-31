// Package systemportfx provides a simple HTTP mux for registering
// administrative and introspection handlers.
package systemportfx // import "code.uber.internal/go/systemportfx.git"

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"

	envfx "code.uber.internal/go/envfx.git"
	"code.uber.internal/go/httpfx.git/httpserver"
	versionfx "code.uber.internal/go/versionfx.git"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	// Version is the current package version.
	Version = "1.3.0"

	_name = "systemportfx"
)

// Module provides a Mux for registering administrative and introspection
// handlers. On application start, the Mux is automatically served on the
// Mesos-allocated $UBER_PORT_SYSTEM in production and an ephemeral port in
// development. The systemportfx module neither requires nor accepts any
// configuration.
var Module = fx.Options(
	fx.Provide(New),
	// To automatically start the server, we invoke a function that forces
	// resolution of the mux.
	fx.Invoke(func(Mux) {}),
)

// Mux is a limited view of an *http.ServeMux bound to Uber's system port.
type Mux interface {
	http.Handler

	Handle(string, http.Handler)
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}

// Params defines the dependencies of the module.
type Params struct {
	fx.In

	Environment envfx.Context
	Lifecycle   fx.Lifecycle
	Version     *versionfx.Reporter
	Logger      *zap.Logger
}

// Result defines the objects that the module provides.
type Result struct {
	fx.Out

	Mux Mux
}

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	port, err := parsePort(p.Environment.SystemPort)
	if err != nil {
		return Result{}, err
	}
	if err := p.Version.Report(_name, Version); err != nil {
		return Result{}, err
	}
	mux := http.NewServeMux()
	logger := p.Logger.With(zap.String("component", _name))
	if port == 0 && shouldHaveSystemPort(p.Environment.Environment) {
		logger.Warn("starting on ephemeral port, other systems (including health checks) cannot query systemportfx")
	}

	server := &http.Server{
		Addr:     fmt.Sprintf(":%d", port),
		Handler:  mux,
		ErrorLog: zap.NewStdLog(logger),
	}
	handle := httpserver.NewHandle(server)
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := handle.Start(ctx); err != nil {
				return fmt.Errorf("error starting system port mux on port %d: %v", port, err)
			}
			logger.Info("started HTTP server on system port", zap.Stringer("addr", handle.Addr()))
			return nil
		},
		OnStop: handle.Shutdown,
	})

	return Result{Mux: mux}, nil
}

func shouldHaveSystemPort(env string) bool {
	switch env {
	case envfx.EnvDevelopment, envfx.EnvTest:
		return false
	default:
		return true
	}
}

func parsePort(p string) (int, error) {
	if p == "" {
		return 0, nil
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return 0, fmt.Errorf("system port %q is not an integer: %v", p, err)
	}
	if port < 0 || port > math.MaxUint16 {
		return 0, fmt.Errorf("system port %d is outside the uint16 range", port)
	}
	return port, nil
}
