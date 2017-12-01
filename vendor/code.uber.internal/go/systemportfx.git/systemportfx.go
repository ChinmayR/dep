// Package systemportfx provides a simple HTTP mux for registering
// administrative and introspection handlers.
package systemportfx

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"

	envfx "code.uber.internal/go/envfx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	// Version is the current package version.
	Version = "1.1.0"

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
		Handler:  mux,
		ErrorLog: zap.NewStdLog(logger),
	}

	// The standard library's net/http.Server really, really wants to live in
	// the main function - the usual server.ListenAndServe blocks until
	// shutdown. Fitting it into Fx's lifecycle requires closing over some
	// shared state.
	errCh := make(chan error, 1)
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			// Most errors that occur when starting an http.Server are actually
			// Listen errors. If we encounter one of those, we can abort
			// immediately.
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				return fmt.Errorf("error starting system port mux on port %d: %v", port, err)
			}
			go func() {
				// Serve blocks until it encounters an error or until the server shuts
				// down, so we need to call it in a separate goroutine. Errors here
				// (apart from http.ErrServerClosed) are rare.
				err := server.Serve(ln)
				if err != http.ErrServerClosed {
					// Log errors immediately instead of waiting until OnStop. If this
					// happens following a deploy, Sentry and M3 should trigger a
					// rollback.
					logger.Error("error serving on system port", zap.Error(err))
				}
				errCh <- err
			}()
			logger.Info("started HTTP server on system port", zap.Stringer("port", ln.Addr()))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if err := server.Shutdown(ctx); err != nil {
				return err
			}
			if err := <-errCh; err != http.ErrServerClosed {
				return err
			}
			return nil
		},
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
