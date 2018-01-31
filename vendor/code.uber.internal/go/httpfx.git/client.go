package httpfx

import (
	"net/http"
	"time"

	"code.uber.internal/go/httpfx.git/clientmiddleware"
	"code.uber.internal/go/httpfx.git/internal"
	versionfx "code.uber.internal/go/versionfx.git"
	"go.uber.org/fx"
)

// ClientModule provides a fully instrumented http.Client to an Fx
// application. This HTTP client should be used to make requests to other Uber
// services.
//
// See package documentation for more information.
var ClientModule = fx.Provide(NewClient)

// ClientParams defines the dependencies for the ClientModule.
type ClientParams struct {
	fx.In

	Middleware ClientMiddleware
	Lifecycle  fx.Lifecycle
	Reporter   *versionfx.Reporter

	// RoundTripper to use to send requests. Defaults to
	// http.DefaultTransport.
	RoundTripper http.RoundTripper `optional:"true"`
}

// ClientMiddleware defines the different middleware components supported by
// httpfx.
type ClientMiddleware struct {
	fx.In

	// The following default middlewares are supported by httpfx. Their
	// implementations are provided by jaegerfx and galileofx.
	TraceStart  func(http.RoundTripper) http.RoundTripper `optional:"true" name:"trace.start"`
	Auth        func(http.RoundTripper) http.RoundTripper `optional:"true" name:"auth"`
	TraceFinish func(http.RoundTripper) http.RoundTripper `optional:"true" name:"trace.end"`
}

// ClientResult contains the output of ClientModule.
type ClientResult struct {
	fx.Out

	// Fully instrumented HTTP client with support for auth, tracing, and
	// middleware.
	Client *http.Client

	// Instruments the given http.Client with the same functionality provided
	// by the other http.Client but with the provided HTTP client middleware
	// installed into it.
	InstrumentClient func(*http.Client, ...func(http.RoundTripper) http.RoundTripper)
}

// NewClient builds an HTTP client with all the instrumentation needed for
// Uber services.
func NewClient(p ClientParams) (ClientResult, error) {
	if err := p.Reporter.Report(_name+"-client", Version); err != nil {
		return ClientResult{}, err
	}

	var tl internal.TransportLifecycle
	p.Lifecycle.Append(fx.Hook{OnStop: tl.Shutdown})

	instrumenter := clientInstrumenter{
		Enter: []func(http.RoundTripper) http.RoundTripper{
			// Span start should always happen first.
			p.Middleware.TraceStart,
		},
		Exit: []func(http.RoundTripper) http.RoundTripper{
			// Auth adds baggage so it should get applied before tracing.
			p.Middleware.Auth,

			// Injecting data from spans into the request should happen just
			// before we start sending the request. This allows intermediate
			// middleware to modify the spans before they get serialized into
			// the request.
			p.Middleware.TraceFinish,

			// If we're actually sending the request, we should track it with
			// the TransportLifecycle.
			tl.Wrap,
		},
	}

	client := &http.Client{
		Transport: p.RoundTripper,
		// TODO(abg): Timeout should be customizable from config.
		Timeout: 30 * time.Second,
	}

	instrumenter.Instrument(client)
	return ClientResult{
		Client:           client,
		InstrumentClient: instrumenter.Instrument,
	}, nil
}

// clientInstrumenter instruments http.Clients with entry and exit middleware.
type clientInstrumenter struct {
	// Middleware that will be called first for an outgoing request.
	// Middleware specified here will be called in-order.
	Enter []func(http.RoundTripper) http.RoundTripper

	// Middleware that will be called last for an outgoing request.
	// Middleware specified here will be called in-order.
	Exit []func(http.RoundTripper) http.RoundTripper
}

// Instrument instruments the given HTTP client with entry and exit middleware
// defined on the instrumenter, optionally installing the provided extra
// middleware between the entry and exit middleware.
func (i *clientInstrumenter) Instrument(c *http.Client, extraMW ...func(http.RoundTripper) http.RoundTripper) {
	mws := make([]func(http.RoundTripper) http.RoundTripper, 0, len(i.Enter)+len(i.Exit)+len(extraMW))
	mws = append(mws, i.Enter...)
	mws = append(mws, extraMW...)
	mws = append(mws, i.Exit...)
	c.Transport = clientmiddleware.Chain(mws...)(c.Transport)
}
