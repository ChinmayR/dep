package jaegerhttp

import (
	"net/http"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	"go.uber.org/multierr"
)

// StartSpanMiddleware builds a middleware that wraps an http.RoundTripper to
// start and emit traces for all outgoing requests to the given tracer.
//
// This middleware DOES NOT alter the request to include the tracing
// information over the wire. The InjectSpanMiddleware must be used for that.
//
// This middleware should usually go before any other middleware.
//
//   StartSpanMiddleware(tracer)(InjectSpanMiddleware(tracer)(transport))
func StartSpanMiddleware(t opentracing.Tracer) func(http.RoundTripper) http.RoundTripper {
	return func(rt http.RoundTripper) http.RoundTripper {
		if rt == nil {
			rt = http.DefaultTransport
		}
		return startSpan{
			tracer:    t,
			transport: rt,
		}
	}
}

type startSpan struct {
	tracer    opentracing.Tracer
	transport http.RoundTripper
}

func (t startSpan) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	var parent opentracing.SpanContext
	if s := opentracing.SpanFromContext(ctx); s != nil {
		parent = s.Context()
	}

	span := t.tracer.StartSpan(req.Method, opentracing.ChildOf(parent))
	defer span.Finish()

	ext.SpanKindRPCClient.Set(span)
	ext.HTTPUrl.Set(span, req.URL.String())
	ext.HTTPMethod.Set(span, req.Method)
	ext.Component.Set(span, "jaegerhttp")

	ctx = opentracing.ContextWithSpan(ctx, span)

	resp, err := t.transport.RoundTrip(req.WithContext(ctx))
	if err != nil {
		ext.Error.Set(span, true)

		// TODO(abg): Should we use log.Error instead?
		span.LogFields(
			log.String("event", "error"),
			log.String("message", err.Error()),
		)
	} else if resp != nil {
		ext.HTTPStatusCode.Set(span, uint16(resp.StatusCode))
		if resp.StatusCode >= http.StatusBadRequest {
			ext.Error.Set(span, true)
		}
	}

	return resp, err
}

// InjectSpanMiddleware builds a middleware that reads the tracing information
// for the current request from its context and includes it in the outgoing
// request. Note that this will alter the outgoing request in-place.
//
// The span for the outgoing request must already exist on the context to be
// included in the request. Make sure that StartSpanMiddleware was applied
// first.
//
//   StartSpanMiddleware(tracer)(InjectSpanMiddleware(tracer)(transport))
func InjectSpanMiddleware(t opentracing.Tracer) func(http.RoundTripper) http.RoundTripper {
	return func(rt http.RoundTripper) http.RoundTripper {
		if rt == nil {
			rt = http.DefaultTransport
		}
		return injectSpan{
			tracer:    t,
			transport: rt,
		}
	}
}

type injectSpan struct {
	tracer    opentracing.Tracer
	transport http.RoundTripper
}

func (t injectSpan) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return t.transport.RoundTrip(req)
	}

	carrier := opentracing.HTTPHeadersCarrier(req.Header)
	if err := t.tracer.Inject(span.Context(), opentracing.HTTPHeaders, carrier); err != nil {
		if req.Body != nil {
			// We need to close the request body if we return early.
			err = multierr.Append(err, req.Body.Close())
		}
		return nil, err
	}

	return t.transport.RoundTrip(req)
}
