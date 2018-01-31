// Package jaegerfx configures tracing using Uber's open-source Jaeger
// library.
//
// HTTP Client Support
//
// jaegerfx automatically integrates with HTTP clients produced by httpfx.
//
// HTTP Server Support
//
// See https://go.uberinternal.com/pkg/code.uber.internal/go/httpfx.git/ for
// information on how to add Galileo and Jaeger support to your HTTP servers.
//
// Break glass
//
// If you're building your own HTTP clients instead of using the one provided
// by httpfx, you can instrument them with the functionality provided by
// httpfx by using the InstrumentClient function produced by httpfx.
//
//   type clientParams struct {
//     fx.In
//
//     InstrumentClient func(*http.Client, ...func(http.RoundTripper) http.RoundTripper)
//     ...
//   }
//
//   func newClient(p clientParams) *http.Client {
//     client := &http.Client{}
//     ...
//
//     // This adds Jaeger and Galileo support to the HTTP client.
//     client.Transport = p.InstrumentClient(client.Transport)
//   }
//
// See https://go.uberinternal.com/pkg/code.uber.internal/go/httpfx.git/ for
// more information.
//
// See Also
//
// https://go.uberinternal.com/pkg/code.uber.internal/go/httpfx.git/
// https://go.uberinternal.com/pkg/code.uber.internal/go/jaegerfx.git/jaegerhttp/.
package jaegerfx // import "code.uber.internal/go/jaegerfx.git"

import (
	"context"
	"fmt"
	"net/http"

	envfx "code.uber.internal/go/envfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber-go/tally"
	jaeger "github.com/uber/jaeger-client-go"
	jconfig "github.com/uber/jaeger-client-go/config"
	jzap "github.com/uber/jaeger-client-go/log/zap"
	jtally "github.com/uber/jaeger-lib/metrics/tally"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"code.uber.internal/go/jaegerfx.git/jaegerhttp"
)

const (
	// Version is the current package version.
	Version = "1.2.0"
	// ConfigurationKey is the portion of the service configuration that this
	// package reads.
	ConfigurationKey = "tracing"

	_name = "jaegerfx"
)

// Module provides an opentracing.Tracer, and it also configures opentracing's
// package-global state. It attempts to read a Configuration from the "tracing"
// key of the service configuration, but falls back to an
// environment-appropriate default if no configuration is specified.
//
// In production and staging, the default configuration enables tracing. In all
// other environments, the tracer is a no-op.
//
// In YAML, tracing configuration might look like this:
//  tracing:
//    disabled: true
var Module = fx.Options(
	fx.Provide(New, newHTTPMiddleware),
	fx.Invoke(setGlobalTracer),
)

// Configuration toggles a subset of the Jaeger client library's options. It
// hides most of the configurability of the open-source Jaeger library, which
// lets Uber's tracing team easily roll out updates to the default settings.
type Configuration struct {
	// The open-source Jaeger client package uses snake_case YAML keys. To
	// preserve compatibility with the upstream configuration shape, this
	// package does the same.
	Disabled   bool `yaml:"disabled"`    // no-op all tracing
	RPCMetrics bool `yaml:"rpc_metrics"` // enable per-RPC metrics
}

// Params defines the dependencies of the jaegerfx module.
type Params struct {
	fx.In

	Service     servicefx.Metadata
	Environment envfx.Context
	Config      config.Provider
	Scope       tally.Scope
	Logger      *zap.Logger
	Lifecycle   fx.Lifecycle
	Version     *versionfx.Reporter

	Reporter jaeger.Reporter `optional:"true"`
}

// Result defines the objects that the jaegerfx module provides.
type Result struct {
	fx.Out

	Tracer opentracing.Tracer
}

// HTTPMiddleware provides tracing middleware for HTTP.
type HTTPMiddleware struct {
	fx.Out

	// The Start and End middlewares for tracing. The Start middleware MUST
	// run before the End middleware. Ideally, the Start middleware is the
	// first middleware to run and End, the last.
	//
	//   client.Transport = WrapClientStart(WrapClientEnd(transport))
	WrapClientStart func(http.RoundTripper) http.RoundTripper `name:"trace.start"`
	WrapClientEnd   func(http.RoundTripper) http.RoundTripper `name:"trace.end"`

	// A tracing middleware for HTTP server handlers. Ideally, this is the
	// first middleware to run on incoming requests.
	WrapHandler func(http.Handler) http.Handler `name:"trace"`
}

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	if err := multierr.Combine(
		p.Version.Report(_name, Version),
		p.Version.Report("jaeger", jaeger.JaegerClientVersion),
	); err != nil {
		return Result{}, err
	}

	var c Configuration
	raw := p.Config.Get(ConfigurationKey)
	if err := raw.Populate(&c); err != nil {
		return Result{}, fmt.Errorf("failed to load tracing configuration: %v", err)
	}

	jaegerConfig := jconfig.Configuration{
		RPCMetrics: c.RPCMetrics,
	}

	switch p.Environment.Environment {
	case envfx.EnvProduction, envfx.EnvStaging:
	default:
		// In development and tests, use the defaults suggested by the Jaeger team
		// (https://engdocs.uberinternal.com/jaeger/menu_items/go_integration.html#testing).
		// These defaults log every span, which is helpful when debugging.
		jaegerConfig.Sampler = &jconfig.SamplerConfig{Type: "const", Param: 1}
		jaegerConfig.Reporter = &jconfig.ReporterConfig{QueueSize: 1, LogSpans: true}
	}

	// Rather than using Jaeger's standard no-op mechanism, disable reporting of
	// spans. We can't use the standard mechanism because the NoopTracer relies
	// on NoopScopes, and NoopScopes don't propagate baggage. This breaks any
	// functionality that relies on baggage propagation.
	if c.Disabled {
		jaegerConfig.Sampler = &jconfig.SamplerConfig{Type: "const", Param: 0}
	}

	opts := []jconfig.Option{
		jconfig.Metrics(jtally.Wrap(p.Scope)),
		jconfig.Logger(jzap.NewLogger(p.Logger)),
	}
	if p.Reporter != nil {
		opts = append(opts, jconfig.Reporter(p.Reporter))
	}

	tracer, closer, err := jaegerConfig.New(p.Service.Name, opts...)
	if err != nil {
		return Result{}, fmt.Errorf("failed to construct tracer: %v", err)
	}

	p.Lifecycle.Append(fx.Hook{OnStop: func(context.Context) error {
		return closer.Close()
	}})
	return Result{
		Tracer: tracer,
	}, nil
}

func newHTTPMiddleware(t opentracing.Tracer, log *zap.Logger) HTTPMiddleware {
	return HTTPMiddleware{
		WrapClientStart: jaegerhttp.StartSpanMiddleware(t),
		WrapClientEnd:   jaegerhttp.InjectSpanMiddleware(t),
		WrapHandler:     jaegerhttp.ExtractSpanMiddleware(t, jaegerhttp.ExtractSpanLogger(log)),
	}
}

func setGlobalTracer(tracer opentracing.Tracer) {
	opentracing.SetGlobalTracer(tracer)
}
