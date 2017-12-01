// Package galileofx provides Galileo integration for Fx applications.
package galileofx

// TODO(abg): Explain that this integrates with yarpcfx automagically.

import (
	"code.uber.internal/go/galileofx.git/authmiddleware"

	galileo "code.uber.internal/engsec/galileo-go.git"
	envfx "code.uber.internal/go/envfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber-go/tally"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/multierr"
	"go.uber.org/yarpc/api/middleware"
	"go.uber.org/zap"
)

const (
	// Version is the current package version.
	Version = "1.1.0"

	// ConfigurationKey is the key under which the Galileo configuration must
	// be present in the YAML.
	ConfigurationKey = "galileo"

	_name = "galileofx"
)

// Module provides a Galileo object and integrates it with YARPC.
var Module = fx.Provide(New, newYARPCMiddleware)

// TODO(abg): httpfx integration?

// Configuration configures the Galileo Fx module.
//
//   galileo:
//     allowedEntities: [EVERYONE]
//     enforceRatio: 0.5
//
// All parameters are optional.
//
// By default, in production, all outbound requests are signed and all inbound
// requests are allowed through. In non-production environments, Galileo is
// completely disabled by default. You may override this by specifying
// `enabled: true` in your development.yaml. We recommend that you do this if
// you are using Cerberus to make requests to other services.
//
// An empty configuration will satisfy Gold Star requirements for most
// services.
type Configuration struct {
	// Whether Galileo signing and authentication is enabled. By default, this
	// is false in development and testing and true in production.
	Enabled bool

	// List of entities that are allowed to call this service. This field is
	// optional.
	//
	// Defaults to allowing requests from everyone.
	AllowedEntities []string `yaml:"allowedEntities"`

	// Value in the range [0.0, 1.0] controlling the likelihood of
	// authentication being enforced on incoming requests.
	//
	// A value of 0.0 (the default) states that authentication is not enforced
	// on any request. A value of 1.0 states that authentication is enforced
	// on all requests.
	EnforceRatio float32 `yaml:"enforceRatio"`

	// Path to the private key uniquely identifying this service. This
	// parameter is optional.
	//
	// In production, this will be inferred automatically.
	PrivateKeyPath string `yaml:"privateKeyPath"`
}

// Params provides inputs for the galileofx module.
type Params struct {
	fx.In

	Environment envfx.Context
	Metrics     tally.Scope
	Config      config.Provider
	Logger      *zap.Logger
	Service     servicefx.Metadata
	Reporter    *versionfx.Reporter
	Tracer      opentracing.Tracer
}

// Result is the output of the galileofx module.
type Result struct {
	fx.Out

	Galileo galileo.Galileo
}

// YARPCMiddleware provides authentication middleware for YARPC.
type YARPCMiddleware struct {
	fx.Out

	UnaryInbound   middleware.UnaryInbound   `name:"auth"`
	UnaryOutbound  middleware.UnaryOutbound  `name:"auth"`
	OnewayInbound  middleware.OnewayInbound  `name:"auth"`
	OnewayOutbound middleware.OnewayOutbound `name:"auth"`
}

// New exports the functionality of Module as a callable function.
func New(p Params) (Result, error) {
	var cfg Configuration
	if err := p.Config.Get(ConfigurationKey).Populate(&cfg); err != nil {
		return Result{}, err
	}

	enabled := cfg.Enabled
	switch p.Environment.Environment {
	case envfx.EnvProduction, envfx.EnvStaging:
		enabled = true
	}

	if err := multierr.Append(
		p.Reporter.Report("galileo", galileo.Version),
		p.Reporter.Report(_name, Version),
	); err != nil {
		return Result{}, err
	}

	if !enabled {
		return Result{Galileo: galileoNoop{name: p.Service.Name}}, nil
	}

	g, err := _galileoCreate(galileo.Configuration{
		AllowedEntities: cfg.AllowedEntities,
		// Galileo calls this percentage but it's in the range [0, 1].
		EnforcePercentage: cfg.EnforceRatio,
		PrivateKeyPath:    cfg.PrivateKeyPath,
		// TODO: Endpoints: ...,
		Logger:      p.Logger,
		Metrics:     p.Metrics,
		ServiceName: p.Service.Name,
		Tracer:      p.Tracer,
	})
	if err != nil {
		return Result{}, err
	}

	return Result{Galileo: g}, nil
}

var _galileoCreate = galileo.Create

// Testing helper to temporarily replace the galileo.Create function.
func setGalileoCreate(f func(galileo.Configuration) (galileo.Galileo, error)) (restore func()) {
	original := _galileoCreate
	_galileoCreate = f
	return func() { _galileoCreate = original }
}

func newYARPCMiddleware(g galileo.Galileo) YARPCMiddleware {
	mw := authmiddleware.New(g)
	return YARPCMiddleware{
		UnaryInbound:   mw,
		UnaryOutbound:  mw,
		OnewayInbound:  mw,
		OnewayOutbound: mw,
	}
}
