// Package zapfx provides a structured logger configured to match the ELK
// team's preferred schema.
package zapfx

import (
	"context"
	"fmt"

	envfx "code.uber.internal/go/envfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// Version is the current package version.
	Version = "1.1.0"
	// ConfigurationKey is the portion of the service configuration that this
	// package reads.
	ConfigurationKey = "logging"

	_name = "zapfx"
)

// Module provides a zap logger for structured logging. All logs are written to
// standard out; on Uber's compute cluster, these logs are then captured in
// files on the local filesystem, surfaced in uDeploy, and forwarded to Kafka
// and ElasticSearch. Note that only JSON-encoded log output is supported in
// production.
//
// In production and staging, the default configuration logs at zap.InfoLevel,
// uses the JSON encoder, and enables sampling. In all other environments, the
// default configuration logs at zap.DebugLevel, uses the console encoder, and
// disables sampling.
//
// In YAML, logging configuration might look like this:
//
//  logging:
//    level: info
//    development: false
//    sampling:
//      initial: 100
//      thereafter: 100
//    encoding: json
var Module = fx.Provide(New)

// Trace extracts Jaeger tracing information from a context and returns it as a
// zap field. The returned field is usable with both plain and sugared loggers.
//
// With a *zap.Logger, both of these work:
//  logger.With(
//    zapfx.Trace(ctx),
//    zap.String("something", "else"),
//  ).Info("hello")
//  logger.Info("hello",
//    zapfx.Trace(ctx),
//    zap.String("something", "else"),
//  )
//
// With a *zap.SugaredLogger, usage is similar:
//  sugar.With(
//    zapfx.Trace(ctx),
//    "something", "else",
//  ).Info("hello")
//  sugar.Infow("hello",
//    zapfx.Trace(ctx),
//    "something", "else",
//  )
func Trace(ctx context.Context) zapcore.Field {
	return jaegerzap.Trace(ctx)
}

// Params defines the dependencies of the zapfx module.
type Params struct {
	fx.In

	Service     servicefx.Metadata
	Environment envfx.Context
	Config      config.Provider
	Lifecycle   fx.Lifecycle
	Reporter    *versionfx.Reporter

	Sentry zapcore.Core `optional:"true" name:"sentry"`
}

// Result defines the objects that the zapfx module provides.
type Result struct {
	fx.Out

	Level         zap.AtomicLevel
	Logger        *zap.Logger
	SugaredLogger *zap.SugaredLogger
}

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(params Params) (Result, error) {
	if err := params.Reporter.Report(_name, Version); err != nil {
		return Result{}, err
	}
	c, err := newConfiguration(params.Service, params.Environment, params.Config)
	if err != nil {
		return Result{}, err
	}
	lvl, logger, err := c.build()
	if err != nil {
		return Result{}, err
	}
	if params.Sentry != nil {
		logger = logger.WithOptions(zap.WrapCore(func(base zapcore.Core) zapcore.Core {
			return zapcore.NewTee(base, params.Sentry)
		}))
	}
	undoGlobals := zap.ReplaceGlobals(logger)
	undoHijack := zap.RedirectStdLog(logger)
	params.Lifecycle.Append(fx.Hook{OnStop: func(context.Context) error {
		undoHijack()
		undoGlobals()
		logger.Sync()
		return nil
	}})

	// Warn if we're using an unsupported encoding in production.
	if c.Encoding == "json" {
		return Result{
			Level:         lvl,
			Logger:        logger,
			SugaredLogger: logger.Sugar(),
		}, nil
	}
	switch params.Environment.Environment {
	case envfx.EnvProduction, envfx.EnvStaging:
		logger.Warn(
			"Current log encoding isn't supported in production.",
			zap.String("encoding", c.Encoding),
		)
	}
	return Result{
		Level:         lvl,
		Logger:        logger,
		SugaredLogger: logger.Sugar(),
	}, nil
}

func newConfiguration(
	sfx servicefx.Metadata,
	env envfx.Context,
	cfg config.Provider,
) (Configuration, error) {
	raw := cfg.Get(ConfigurationKey)
	var setDefaultErr error
	switch env.Environment {
	case envfx.EnvProduction, envfx.EnvStaging:
		raw, setDefaultErr = raw.WithDefault(defaultProdConfig())
	default:
		raw, setDefaultErr = raw.WithDefault(defaultDevConfig())
	}
	if setDefaultErr != nil {
		return Configuration{}, setDefaultErr
	}

	var c Configuration
	if err := raw.Populate(&c); err != nil {
		return Configuration{}, fmt.Errorf("failed to load logging config: %v", err)
	}

	// TODO: Config library isn't using nil as the zero value of pointers and maps.
	if c.Sampling.Initial == 0 && c.Sampling.Thereafter == 0 {
		c.Sampling = nil
	}
	if len(c.InitialFields) == 0 {
		c.InitialFields = nil
	}

	// In production, set Panama-mandated fields if the user hasn't already
	// done so. Don't set these fields in development, where they just clutter
	// console output.
	if c.Development {
		return c, nil
	}
	if c.InitialFields == nil {
		c.InitialFields = make(map[string]interface{})
	}
	c.defaultField("service_name", sfx.Name) // snake-case for ELK schema
	c.defaultField("hostname", env.Hostname)
	c.defaultField("zone", env.Zone)
	c.defaultField("runtimeEnvironment", env.RuntimeEnvironment)
	return c, nil
}

func defaultDevConfig() Configuration {
	return Configuration{
		Level:       zap.DebugLevel.String(),
		Development: true,
		Encoding:    "console",
		OutputPaths: []string{"stdout"},
	}
}

func defaultProdConfig() Configuration {
	return Configuration{
		Level:    zap.InfoLevel.String(),
		Encoding: "json",
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		OutputPaths: []string{"stdout"},
	}
}
