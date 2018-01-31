package jaegerhttp

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type extractorFunc func(interface{}) (mocktracer.MockSpanContext, error)

func (f extractorFunc) Extract(carrier interface{}) (mocktracer.MockSpanContext, error) {
	return f(carrier)
}

func TestExtractSpanMiddleware(t *testing.T) {
	echoHandler := func(t *testing.T) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			assert.NotNil(t, opentracing.SpanFromContext(r.Context()), "expected non-nil span")
			io.Copy(w, r.Body)
		}
	}

	t.Run("successful request", func(t *testing.T) {
		tracer := mocktracer.New()
		handler := ExtractSpanMiddleware(tracer)(echoHandler(t))

		res := httptest.NewRecorder()
		handler.ServeHTTP(
			res,
			httptest.NewRequest("POST", "/hello", strings.NewReader("hello world")),
		)

		assert.Equal(t, 200, res.Code, "response code did not match")
		assert.Equal(t, "hello world", res.Body.String(), "response body did not match")

		spans := tracer.FinishedSpans()
		require.Len(t, spans, 1, "expected exactly 1 span")

		span := spans[0]
		assert.Equal(t, ext.SpanKindEnum("server"), span.Tag("span.kind"), "span kind did not match")
		assert.Equal(t, "POST", span.OperationName, "OperationName did not match")
		assert.Equal(t, uint16(200), span.Tag("http.status_code"), "status code tag did not match")
		assert.Equal(t, "/hello", span.Tag("http.url"), "url tag did not match")
		assert.Equal(t, "POST", span.Tag("http.method"), "method tag did not match")
	})

	t.Run("successful request with x-uber-source", func(t *testing.T) {
		tracer := mocktracer.New()
		handler := ExtractSpanMiddleware(tracer)(echoHandler(t))

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Uber-Source", "myservice")
		handler.ServeHTTP(httptest.NewRecorder(), req)

		spans := tracer.FinishedSpans()
		require.Len(t, spans, 1, "expected exactly 1 span")

		span := spans[0]
		assert.Equal(t, "myservice", span.Tag("peer.service"), "service name did not match")
	})

	t.Run("successful request with remote addr", func(t *testing.T) {
		tracer := mocktracer.New()
		handler := ExtractSpanMiddleware(tracer)(echoHandler(t))

		req := httptest.NewRequest("POST", "/hello", strings.NewReader("hello world"))
		req.RemoteAddr = "localhost:4040"
		handler.ServeHTTP(httptest.NewRecorder(), req)

		spans := tracer.FinishedSpans()
		require.Len(t, spans, 1, "expected exactly 1 span")

		span := spans[0]
		assert.Equal(t, "localhost:4040", span.Tag("peer.address"), "peer address did not match")
	})

	t.Run("successful request with parent context", func(t *testing.T) {
		tracer := mocktracer.New()

		parentSpan := tracer.StartSpan("parentOperation").(*mocktracer.MockSpan)
		parentSpan.SetBaggageItem("token", "42")

		extractorWasCalled := false
		tracer.RegisterExtractor(opentracing.HTTPHeaders,
			extractorFunc(func(interface{}) (mocktracer.MockSpanContext, error) {
				assert.False(t, extractorWasCalled, "extractor called too many times")
				extractorWasCalled = true
				return parentSpan.SpanContext, nil
			}),
		)

		handler := ExtractSpanMiddleware(tracer)(echoHandler(t))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		require.True(t, extractorWasCalled, "extractor was never called")

		spans := tracer.FinishedSpans()
		// parentSpan is still ongoing so we expect only one span to be finished.
		require.Len(t, spans, 1, "expected exactly 1 span")

		span := spans[0]

		parentCtx := parentSpan.SpanContext
		assert.Equal(t, parentCtx.TraceID, span.SpanContext.TraceID, "trace ID did not match")
		assert.Equal(t, parentCtx.SpanID, span.ParentID, "parent ID did not match parent span ID")
		assert.Equal(t, "42", span.BaggageItem("token"), "parent baggage missing")
	})

	var status400Handler http.HandlerFunc = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}

	t.Run("HTTP bad request causes an error", func(t *testing.T) {
		tracer := mocktracer.New()

		handler := ExtractSpanMiddleware(tracer)(status400Handler)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

		spans := tracer.FinishedSpans()
		require.Len(t, spans, 1, "expected exactly 1 span")

		span := spans[0]
		// Can't use assert.True because span.Tag returns interface{}.
		assert.Equal(t, true, span.Tag("error"), "error tag on span should be true")
		assert.Equal(t, uint16(400), span.Tag("http.status_code"),
			"status code tag did not match")
	})

	var status500Handler http.HandlerFunc = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}

	t.Run("HTTP internal error causes an error", func(t *testing.T) {
		tracer := mocktracer.New()

		handler := ExtractSpanMiddleware(tracer)(status500Handler)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

		spans := tracer.FinishedSpans()
		require.Len(t, spans, 1, "expected exactly 1 span")

		span := spans[0]
		// Can't use assert.True because span.Tag returns interface{}.
		assert.Equal(t, true, span.Tag("error"), "error tag on span should be true")
		assert.Equal(t, uint16(500), span.Tag("http.status_code"),
			"status code tag did not match")
	})

	t.Run("extraction error does not affect the request", func(t *testing.T) {
		tracer := mocktracer.New()
		core, logs := observer.New(zapcore.InfoLevel)

		handler := ExtractSpanMiddleware(tracer, ExtractSpanLogger(zap.New(core)))(echoHandler(t))

		extractorWasCalled := false
		tracer.RegisterExtractor(opentracing.HTTPHeaders,
			extractorFunc(func(interface{}) (mocktracer.MockSpanContext, error) {
				assert.False(t, extractorWasCalled, "extractor called too many times")
				extractorWasCalled = true
				return mocktracer.MockSpanContext{}, errors.New("great sadness")
			}),
		)

		res := httptest.NewRecorder()
		handler.ServeHTTP(
			res,
			httptest.NewRequest("POST", "/hello", strings.NewReader("hello world")),
		)
		require.True(t, extractorWasCalled, "extractor was never called")

		assert.Equal(t, 200, res.Code, "response code did not match")
		assert.Equal(t, "hello world", res.Body.String(), "response body did not match")

		spans := tracer.FinishedSpans()
		require.Len(t, spans, 1, "expected exactly 1 span")
		span := spans[0]
		assert.Equal(t, uint16(200), span.Tag("http.status_code"), "status code tag did not match")

		assert.Equal(t, 1,
			logs.FilterField(zap.Error(errors.New("great sadness"))).Len(),
			"expected a log entry for the extraction failure")

	})
}
