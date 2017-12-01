package xhttp

import (
	"context"
	"fmt"
	stdlog "log"
	"net/http"

	"github.com/felixge/httpsnoop"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
)

// DefaultTracer is used by DefaultFilter and DefaultClientFilter
var DefaultTracer = &Tracer{}

// Tracer implements functions used as server-side and client-side xhttp filters.
type Tracer struct {
	Tracer opentracing.Tracer
}

// GetTracer function returns a reference to the real Tracer instance.
// It either returns the instance this provider has been instantiated with,
// or delegates to opentracing.GlobalTracer().
func (t *Tracer) GetTracer() opentracing.Tracer {
	if t.Tracer == nil {
		return opentracing.GlobalTracer()
	}
	return t.Tracer
}

// TracedServer can be used as xhttp.Filter in the server endpoints to start a new trace
// or resume a trace if request headers contain the tracing context. The new tracing span
// is stored in the Context before forwarding to the next step in the chain.
func (t *Tracer) TracedServer(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	next Handler,
) {
	carrier := opentracing.HTTPHeadersCarrier(r.Header)
	spanCtx, err := t.GetTracer().Extract(opentracing.HTTPHeaders, carrier)
	if err != nil && err != opentracing.ErrSpanContextNotFound {
		// cannot use x/log due to circular dependency: x/log->x/kafka->xhttp
		// TODO: how about storing this in the span itself?
		stdlog.Printf("Malformed inbound tracing context: %s", err.Error())
	}
	span := t.GetTracer().StartSpan(operationName(r), ext.RPCServerOption(spanCtx))
	ext.HTTPUrl.Set(span, r.URL.String())
	ext.HTTPMethod.Set(span, r.Method)
	ext.Component.Set(span, "xhttp")
	if source := r.Header.Get("x-uber-source"); source != "" {
		ext.PeerService.Set(span, source)
	}
	if r.RemoteAddr != "" {
		span.SetTag("peer.address", r.RemoteAddr)
	}
	defer span.Finish()

	ctx = opentracing.ContextWithSpan(ctx, span)

	statusTracker := wrapForStatusCodeTracking(w)
	next.ServeHTTP(ctx, statusTracker.ResponseWriter, r)

	ext.HTTPStatusCode.Set(span, uint16(statusTracker.status))
	if statusTracker.status >= http.StatusBadRequest {
		// treat anything from 400 and up as an error
		ext.Error.Set(span, true)
	}
}

type statusCodeTracker struct {
	http.ResponseWriter
	status int
}

func wrapForStatusCodeTracking(w http.ResponseWriter) *statusCodeTracker {
	tracker := &statusCodeTracker{
		status: http.StatusOK, // default in case WriteHeader is never called
	}
	tracker.ResponseWriter = httpsnoop.Wrap(w, httpsnoop.Hooks{
		// TODO would be nice to avoid 2 extra allocations here
		WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
			return func(code int) {
				tracker.status = code
				next(code)
			}
		},
	})
	return tracker
}

// TracedClient can be used as xhttp.ClientFilter to register OpenTracing spans for
// outbound HTTP requests and encode tracing context and baggage in the HTTP headers.
// The new tracing span is stored in the Context before forwarding to the next step
// in the chain.
func (t *Tracer) TracedClient(
	ctx context.Context,
	req *http.Request,
	next BasicClient,
) (resp *http.Response, err error) {
	var parent opentracing.SpanContext // ok to be nil
	if s := opentracing.SpanFromContext(ctx); s != nil {
		parent = s.Context()
	}
	span := t.GetTracer().StartSpan(operationName(req), opentracing.ChildOf(parent))
	ext.SpanKindRPCClient.Set(span)
	ext.HTTPUrl.Set(span, req.URL.String())
	ext.HTTPMethod.Set(span, req.Method)
	ext.Component.Set(span, "xhttp")
	defer span.Finish()

	ctx = opentracing.ContextWithSpan(ctx, span)
	carrier := opentracing.HTTPHeadersCarrier(req.Header)
	span.Tracer().Inject(span.Context(), opentracing.HTTPHeaders, carrier)

	resp, err = next.Do(ctx, req)

	errorSet := false
	if resp != nil {
		ext.HTTPStatusCode.Set(span, uint16(resp.StatusCode))
		if resp.StatusCode >= http.StatusBadRequest {
			span.SetTag("error", true)
			errorSet = true
		}
	}
	if err != nil {
		if !errorSet {
			span.SetTag("error", true)
		}
		span.LogFields(
			log.String("event", "error"),
			log.String("message", err.Error()),
		)
	}
	return resp, err
}

// This function is used by DefaultFilter to avoid early binding to DefaultTracer
func defaultTracedServer(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	next Handler,
) {
	DefaultTracer.TracedServer(ctx, w, r, next)
}

// This function is used by DefaultClientFilter to avoid early binding to DefaultTracer
func defaultTracedClient(
	ctx context.Context,
	req *http.Request,
	next BasicClient,
) (resp *http.Response, err error) {
	return DefaultTracer.TracedClient(ctx, req, next)
}

func operationName(req *http.Request) string {
	return fmt.Sprintf("%s %s", req.Method, req.URL.Path)
}
