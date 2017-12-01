// Package healthfx registers health check handlers on the server provided by
// systemportfx. It also provides a means for multiple network servers within
// a process to maintain a consistent notion of application health.
package healthfx

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	envfx "code.uber.internal/go/envfx.git"
	health "code.uber.internal/go/health.git"
	servicefx "code.uber.internal/go/servicefx.git"
	systemportfx "code.uber.internal/go/systemportfx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	"go.uber.org/fx"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

// Version is the current package version.
const Version = "1.1.0"

// Module provides a *health.Coordinator and a *WaitSet, which allow network
// servers to coordinate their notions of application health, and also
// registers health check handlers on the systemportfx mux.
//
// Servers should call the WaitSet's Add method at construction time, then use
// the returned Readyer once their OnStart hooks complete. They may use the
// Coordinator to serve their health endpoints.
//
// Module will invoke New to start reporting health by default and users don't
// have to explicitly depend on *health.Coordinator or *WaitSet.
var Module = fx.Options(
	fx.Provide(New),
	fx.Invoke(registerHandlers),
)

// A Readyer allows a server to indicate that it's ready to receive traffic.
type Readyer interface {
	Ready()
}

type readyFunc func()

func (f readyFunc) Ready() { f() }

// A WaitSet tracks all the network servers in the application, allowing them
// to start up (and potentially warm a cache) before receiving traffic. Once
// all servers have warmed up, the application's health.Coordinator
// automatically signals to Uber's Health Controller system that the
// application is ready to receive traffic.
type WaitSet struct {
	wg     sync.WaitGroup
	done   chan struct{}
	logger *zap.Logger

	namesMu sync.Mutex
	names   map[string]struct{}
}

// Add adds a named server that needs time to warm up before receiving
// traffic. It returns a Readyer that the server must use when it's done
// warming up.
func (ws *WaitSet) Add(name string) (Readyer, error) {
	ws.namesMu.Lock()
	defer ws.namesMu.Unlock()

	if _, ok := ws.names[name]; ok {
		ws.logger.Error(
			"Can't block an already-blocked name.",
			zap.String("name", name),
			zap.Any("blockers", ws.blockers()),
		)
		return nil, fmt.Errorf("can't block name %q a second time", name)
	}

	ws.logger.Info(
		"Network server needs time to warm up.",
		zap.String("name", name),
		zap.Any("blockers", ws.blockers()),
	)
	ws.names[name] = struct{}{}
	ws.wg.Add(1)

	return readyFunc(func() { ws.unblock(name) }), nil
}

func (ws *WaitSet) unblock(name string) {
	ws.namesMu.Lock()
	defer ws.namesMu.Unlock()

	delete(ws.names, name)
	ws.logger.Info(
		"Network server done warming up.",
		zap.String("name", name),
		zap.Any("blockers", ws.blockers()),
	)
	ws.wg.Done()
}

// Wait blocks until all servers added to the WaitSet are ready.
func (ws *WaitSet) Wait() {
	<-ws.done
}

func (ws *WaitSet) blockers() []string {
	// must be called under the lock
	known := make([]string, 0, len(ws.names))
	for name := range ws.names {
		known = append(known, name)
	}
	sort.Strings(known)
	return known
}

// Params defines the dependencies of the module.
type Params struct {
	fx.In

	Environment envfx.Context
	Service     servicefx.Metadata
	Lifecycle   fx.Lifecycle
	Logger      *zap.Logger
	Version     *versionfx.Reporter
	Mux         systemportfx.Mux
}

// Result defines the objects that the module provides.
type Result struct {
	fx.Out

	Coordinator *health.Coordinator
	WaitSet     *WaitSet
}

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	if err := multierr.Append(
		p.Version.Report("health", health.Version),
		p.Version.Report("healthfx", Version),
	); err != nil {
		return Result{}, err
	}

	var coordinator *health.Coordinator
	switch p.Environment.Environment {
	case envfx.EnvDevelopment, envfx.EnvTest:
		coordinator = health.NewCoordinator(p.Service.Name, health.CoolDown(10*time.Millisecond))
	default:
		coordinator = health.NewCoordinator(p.Service.Name)
	}

	ws := &WaitSet{
		done:   make(chan struct{}),
		logger: p.Logger,
		names:  make(map[string]struct{}),
	}

	p.Lifecycle.Append(fx.Hook{OnStart: func(context.Context) error {
		// Coordinator and WaitSet start before the servers, so we spawn a
		// goroutine that waits on the servers.

		go func() {
			// Since the servers add themselves to the WaitSet during construction,
			// the WaitSet has already had all servers added to it and this won't
			// immediately unblock.
			ws.wg.Wait()
			coordinator.AcceptTraffic()
			close(ws.done)
		}()
		return nil
	}})

	return Result{
		Coordinator: coordinator,
		WaitSet:     ws,
	}, nil
}

func registerHandlers(c *health.Coordinator, m systemportfx.Mux) {
	handler := health.NewPlain(c)
	m.Handle("/health", handler)
	m.Handle("/health/", handler)
}
