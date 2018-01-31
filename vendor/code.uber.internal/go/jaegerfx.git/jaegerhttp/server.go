package jaegerhttp

import (
	"net/http"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"go.uber.org/zap"
)

// ExtractSpanOption customizes the behavior of ExtractSpanMiddleware.
type ExtractSpanOption interface {
	applyExtractSpanOption(*extractSpanCfg)
}

type extractSpanCfg struct {
	Logger *zap.Logger
}

type extractSpanOptionFunc func(*extractSpanCfg)

func (f extractSpanOptionFunc) applyExtractSpanOption(c *extractSpanCfg) { f(c) }

// ExtractSpanLogger specifies the logger used by the ExtractSpanMiddleware.
func ExtractSpanLogger(log *zap.Logger) ExtractSpanOption {
	return extractSpanOptionFunc(func(cfg *extractSpanCfg) {
		cfg.Logger = log
	})
}

// ExtractSpanMiddleware builds a middleware for servers that wraps
// http.Handlers to interpret jaeger spans.
func ExtractSpanMiddleware(t opentracing.Tracer, opts ...ExtractSpanOption) func(http.Handler) http.Handler {
	cfg := extractSpanCfg{
		Logger: zap.NewNop(),
	}
	for _, opt := range opts {
		opt.applyExtractSpanOption(&cfg)
	}

	return func(h http.Handler) http.Handler {
		return extractSpan{
			tracer:  t,
			handler: h,
			log:     cfg.Logger,
		}
	}
}

type extractSpan struct {
	tracer  opentracing.Tracer
	handler http.Handler
	log     *zap.Logger
}

func (t extractSpan) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	carrier := opentracing.HTTPHeadersCarrier(r.Header)

	spanCtx, err := t.tracer.Extract(opentracing.HTTPHeaders, carrier)
	if err != nil && err != opentracing.ErrSpanContextNotFound {
		t.log.Error("malformed inbound tracing context", zap.Error(err))
	}

	span := t.tracer.StartSpan(r.Method, ext.RPCServerOption(spanCtx))
	defer span.Finish()

	ext.HTTPUrl.Set(span, r.URL.String())
	ext.HTTPMethod.Set(span, r.Method)
	ext.Component.Set(span, "jaegerhttp")
	if source := r.Header.Get("x-uber-source"); source != "" {
		ext.PeerService.Set(span, source)
	}
	if r.RemoteAddr != "" {
		ext.PeerAddress.Set(span, r.RemoteAddr)
	}

	ctx := opentracing.ContextWithSpan(r.Context(), span)

	traceW := newTraceResponseWriter(w)
	t.handler.ServeHTTP(traceW, r.WithContext(ctx))

	ext.HTTPStatusCode.Set(span, uint16(traceW.status))
	if traceW.status >= http.StatusBadRequest {
		ext.Error.Set(span, true)
	}
}

// An http.ResponseWriter that captures its status code.
type traceResponseWriter struct {
	http.ResponseWriter

	status int
}

func newTraceResponseWriter(w http.ResponseWriter) *traceResponseWriter {
	return &traceResponseWriter{
		ResponseWriter: w,
		// StatusOK is the default if WriteHeader is never called.
		status: http.StatusOK,
	}
}

func (tr *traceResponseWriter) WriteHeader(status int) {
	tr.status = status
	tr.ResponseWriter.WriteHeader(status)
}
