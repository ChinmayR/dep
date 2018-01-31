// Package tallyfx configures telemetry using Uber's open-source Tally
// library.
package tallyfx // import "code.uber.internal/go/tallyfx.git"

import (
	"context"
	"fmt"
	"time"

	envfx "code.uber.internal/go/envfx.git"
	"code.uber.internal/go/servicefx.git"
	"github.com/uber-go/tally"
	"github.com/uber-go/tally/m3"
	"go.uber.org/config"
	"go.uber.org/fx"
)

const (
	// Version is the current package version.
	Version = "1.2.2"
	// ConfigurationKey is the portion of the service configuration that this
	// package reads.
	ConfigurationKey = "metrics"

	_hostPort      = "127.0.0.1:9052"
	_maxPacketSize = 1440
	_interval      = 500 * time.Millisecond
	_maxQueueSize  = 4096
	_runtimeEnvTag = "runtime_env"
)

// Module provides a Tally scope for service telemetry. It attempts to read a
// Configuration from the "metrics" key of the service configuration, but falls
// back to an environment-appropriate default if no configuration is specified.
//
// In production and staging, the default configuration sends M3 metrics
// without a hostname tag. In all other environments, the default configuration
// is a no-op.
//
// In YAML, metrics configuration might look like this:
//
//  metrics:
//    includeHost: true
//    tags:
//      foo: bar
//      baz: quux
var Module = fx.Provide(New)

// Configuration toggles common options for Tally's M3-based telemetry. All
// fields are optional, and most services need not supply any configuration at
// all.
type Configuration struct {
	Disabled    bool              `yaml:"disabled"`    // no-op all metrics
	IncludeHost bool              `yaml:"includeHost"` // tag all metrics with hostname
	Tags        map[string]string `yaml:"tags"`        // common tags
}

func newConfiguration(env envfx.Context, cfg config.Provider) (Configuration, error) {
	raw := cfg.Get(ConfigurationKey)

	switch env.Environment {
	case envfx.EnvProduction, envfx.EnvStaging:
	default:
		var err error
		raw, err = raw.WithDefault(Configuration{Disabled: true})
		if err != nil {
			return Configuration{}, fmt.Errorf("failed to set up configuration: %v", err)
		}
	}

	var c Configuration
	if err := raw.Populate(&c); err != nil {
		return Configuration{}, fmt.Errorf("failed to load metrics configuration: %v", err)
	}
	if _, ok := c.Tags[_runtimeEnvTag]; !ok && env.RuntimeEnvironment != "" {
		if c.Tags == nil {
			c.Tags = make(map[string]string)
		}
		c.Tags[_runtimeEnvTag] = env.RuntimeEnvironment
	}
	return c, nil
}

// Params defines the dependencies of the tallyfx module.
type Params struct {
	fx.In

	Service     servicefx.Metadata
	Environment envfx.Context
	Config      config.Provider
	Lifecycle   fx.Lifecycle
}

// Result defines the objects that the tallyfx module provides.
type Result struct {
	fx.Out

	Scope tally.Scope
}

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	c, err := newConfiguration(p.Environment, p.Config)
	if err != nil {
		return Result{}, fmt.Errorf("failed to load metrics configuration: %v", err)
	}

	if c.Disabled {
		return Result{
			Scope: tally.NoopScope,
		}, nil
	}

	m3Cfg := m3.Configuration{
		HostPort:    _hostPort,
		Service:     p.Service.Name,
		Env:         p.Environment.Environment,
		CommonTags:  c.Tags,
		Queue:       _maxQueueSize,
		PacketSize:  _maxPacketSize,
		IncludeHost: c.IncludeHost,
	}
	reporter, err := m3Cfg.NewReporter()
	if err != nil {
		return Result{}, fmt.Errorf("failed to create M3 reporter: %v", err)
	}

	scope, closer := tally.NewRootScope(tally.ScopeOptions{
		CachedReporter:  reporter,
		Separator:       tally.DefaultSeparator,
		SanitizeOptions: &m3.DefaultSanitizerOpts,
	}, _interval)

	p.Lifecycle.Append(fx.Hook{OnStop: func(context.Context) error {
		return closer.Close()
	}})

	return Result{Scope: scope}, nil
}
