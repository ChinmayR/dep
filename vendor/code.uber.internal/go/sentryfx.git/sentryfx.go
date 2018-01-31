// Package sentryfx integrates zap's structured logging with Sentry, an
// open-source exception-tracking system commonly used within Uber.
//
// See https://engdocs.uberinternal.com/uSentry/index.html for more
// information on Uber's Sentry setup.
package sentryfx // import "code.uber.internal/go/sentryfx.git"

import (
	"context"
	"fmt"

	envfx "code.uber.internal/go/envfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	raven "github.com/getsentry/raven-go"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap/zapcore"
)

const (
	// Version is the current package version.
	Version = "1.2.0"
	// ConfigurationKey is the portion of the service configuration that this
	// package reads.
	ConfigurationKey = "sentry"

	_name              = "sentryfx"
	_platform          = "go"
	_traceContextLines = 3
	_traceSkipFrames   = 2
)

// Module provides Sentry integration for zap-based structured logging. The
// returned zapcore.Core is unlikely to be directly useful to service owners,
// but it automatically integrates with zapfx.
//
// By default, the returned core is a no-op. To enable Sentry, add your
// project's DSN to your service configuration under the "sentry" top-level
// key. A minimal production YAML configuration might look like this:
//
//  sentry:
//    dsn: http://user:pass@sentry.local.uber.internal/123
//
// Note that sending errors to Sentry is quite slow compared to logging only
// to standard out. This is only a concern for applications serving thousands
// of requests per second per process, but if profiling shows bottlenecks in
// logging consider disabling Sentry and relying on Kibana dashboards instead.
var Module = fx.Provide(New)

// Configuration sets options on the module's underlying Sentry client. All
// parameters are optional.
type Configuration struct {
	// Level is the minimum logging level at which messages should be sent to
	// Sentry. Valid options are "debug", "info", "warn", "error", "dpanic",
	// "panic", and "fatal". The default level is "error".
	Level string `yaml:"level"`
	// DSN sets the Sentry DSN. Leaving this blank disables Sentry integration.
	// See https://engdocs.uberinternal.com/uSentry/index.html for details.
	DSN string `yaml:"dsn"`
	// InAppPrefixes sets the package prefixes which should be considered
	// "in-app" in Sentry. By default, the Sentry UI highlights these stack
	// frames.
	InAppPrefixes []string `yaml:"inAppPrefixes"`
}

func ravenSeverity(lvl zapcore.Level) raven.Severity {
	switch lvl {
	case zapcore.DebugLevel:
		return raven.INFO
	case zapcore.InfoLevel:
		return raven.INFO
	case zapcore.WarnLevel:
		return raven.WARNING
	case zapcore.ErrorLevel:
		return raven.ERROR
	case zapcore.DPanicLevel:
		return raven.FATAL
	case zapcore.PanicLevel:
		return raven.FATAL
	case zapcore.FatalLevel:
		return raven.FATAL
	default:
		// Unrecognized levels are fatal.
		return raven.FATAL
	}
}

type client interface {
	Capture(*raven.Packet, map[string]string) (string, chan error)
	Wait()
}

type core struct {
	zapcore.LevelEnabler

	client client
	fields *node
}

func (c *core) With(fs []zapcore.Field) zapcore.Core {
	return c.with(fs)
}

func (c *core) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *core) Write(ent zapcore.Entry, fs []zapcore.Field) error {
	packet := &raven.Packet{
		Message:   ent.Message,
		Timestamp: raven.Timestamp(ent.Time),
		Level:     ravenSeverity(ent.Level),
		Platform:  _platform,
		Extra:     c.with(fs).extra(),
		Interfaces: []raven.Interface{
			raven.NewStacktrace(_traceSkipFrames, _traceContextLines, nil /* in-app prefixes */),
		},
	}

	c.client.Capture(packet, nil /* tags */)

	// We may be crashing the program, so should flush any buffered events.
	if ent.Level > zapcore.ErrorLevel {
		c.Sync()
	}
	return nil
}

func (c *core) Sync() error {
	c.client.Wait()
	return nil
}

func (c *core) with(fs []zapcore.Field) *core {
	head := c.fields
	for _, f := range fs {
		head = &node{
			next:  head,
			field: f,
		}
	}

	return &core{
		client:       c.client,
		LevelEnabler: c.LevelEnabler,
		fields:       head,
	}
}

func (c *core) extra() map[string]interface{} {
	if c.fields == nil {
		return nil
	}
	enc := newEncoder()
	m := enc.encode(c.fields)
	enc.free()
	return m
}

// modifierClient a wrapper around client that modifies the packet with packetModifier
type modifierClient struct {
	client
	packetModifier func(*raven.Packet)
}

func (m *modifierClient) Capture(packet *raven.Packet, tags map[string]string) (string, chan error) {
	m.packetModifier(packet)
	return m.client.Capture(packet, tags)
}

// Params defines the dependencies of the sentryfx module.
type Params struct {
	fx.In

	Service     servicefx.Metadata
	Environment envfx.Context
	Config      config.Provider
	Lifecycle   fx.Lifecycle
	Reporter    *versionfx.Reporter

	// If specified, all Sentry packets will be transformed with this function before being posted.
	// Existing implementations of packet modifiers are found in the packetmodifier package.
	PacketModifier func(*raven.Packet) `optional:"true"`
}

// Result defines the objects that the sentryfx module provides.
type Result struct {
	fx.Out

	Core zapcore.Core `name:"sentry"`
}

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	if err := p.Reporter.Report(_name, Version); err != nil {
		return Result{}, err
	}

	var cfg Configuration
	raw, err := p.Config.Get(ConfigurationKey).WithDefault(Configuration{Level: "error"})
	if err != nil {
		return Result{}, err
	}
	if err := raw.Populate(&cfg); err != nil {
		return Result{}, err
	}

	if cfg.DSN == "" {
		// Sentry is disabled.
		return Result{Core: zapcore.NewNopCore()}, nil
	}

	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return Result{}, fmt.Errorf("%q isn't a recognized zap logging level", cfg.Level)
	}

	client, err := raven.NewWithTags(cfg.DSN, map[string]string{
		"deployment": p.Environment.Deployment,
		"service":    p.Service.Name,
		"hostname":   p.Environment.Hostname,
		"zone":       p.Environment.Zone,
	})
	if err != nil {
		return Result{}, fmt.Errorf("failed to create Sentry client: %v", err)
	}
	client.SetEnvironment(p.Environment.Environment)
	client.SetIncludePaths(cfg.InAppPrefixes)
	client.SetRelease(p.Service.BuildHash)

	p.Lifecycle.Append(fx.Hook{OnStop: func(context.Context) error {
		client.Close()
		return nil
	}})

	core := &core{LevelEnabler: level, client: client}

	if p.PacketModifier != nil {
		// install a wrapper client which invokes packetModifier
		core.client = &modifierClient{client: client, packetModifier: p.PacketModifier}
	}

	return Result{Core: core}, nil
}
