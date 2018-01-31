package jaegerhttp

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTripperFunc is an http.RoundTripper based on a function.
type roundTripperFunc func(r *http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper.
func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// injectorFunc implements mocktracer.Injector from a function.
type injectorFunc func(mocktracer.MockSpanContext, interface{}) error

func (f injectorFunc) Inject(ctx mocktracer.MockSpanContext, carrier interface{}) error {
	return f(ctx, carrier)
}

func TestStartSpanMiddleware(t *testing.T) {
	t.Parallel()

	okRoundTrip := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	t.Run("successful request", func(t *testing.T) {
		t.Parallel()

		tracer := mocktracer.New()
		transport := StartSpanMiddleware(tracer)(okRoundTrip)

		_, err := transport.RoundTrip(httptest.NewRequest("", "http://localhost", nil))
		require.NoError(t, err, "request should not fail")

		spans := tracer.FinishedSpans()
		require.Len(t, spans, 1, "expected exactly 1 span")

		span := spans[0]
		assert.Equal(t, ext.SpanKindEnum("client"), span.Tag("span.kind"), "span kind did not match")
		assert.Equal(t, "GET", span.OperationName, "OperationName did not match")
		assert.Equal(t, uint16(200), span.Tag("http.status_code"), "status code tag did not match")
		assert.Equal(t, "http://localhost", span.Tag("http.url"), "url tag did not match")
		assert.Equal(t, "GET", span.Tag("http.method"), "method tag did not match")
	})

	t.Run("successful request with a parent", func(t *testing.T) {
		t.Parallel()

		tracer := mocktracer.New()
		transport := StartSpanMiddleware(tracer)(okRoundTrip)

		parentSpan := tracer.StartSpan("parentOperation")
		parentSpan.SetBaggageItem("token", "42")
		ctx := opentracing.ContextWithSpan(context.Background(), parentSpan)

		_, err := transport.RoundTrip(
			httptest.NewRequest("", "http://localhost", nil).WithContext(ctx),
		)
		require.NoError(t, err, "request should not fail")

		spans := tracer.FinishedSpans()
		// parentSpan is still ongoing so we expect only one span to be finished.
		require.Len(t, spans, 1, "expected exactly 1 span")

		span := spans[0]

		parentCtx := parentSpan.(*mocktracer.MockSpan).SpanContext
		assert.Equal(t, parentCtx.TraceID, span.SpanContext.TraceID, "trace ID did not match")
		assert.Equal(t, parentCtx.SpanID, span.ParentID, "parent ID did not match parent span ID")
		assert.Equal(t, "42", span.BaggageItem("token"), "parent baggage missing")
	})

	status400RoundTrip := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest}, nil
	})

	t.Run("HTTP bad request causes an error", func(t *testing.T) {
		t.Parallel()

		tracer := mocktracer.New()
		transport := StartSpanMiddleware(tracer)(status400RoundTrip)

		_, err := transport.RoundTrip(httptest.NewRequest("", "http://localhost", nil))
		require.NoError(t, err, "request should not fail")

		spans := tracer.FinishedSpans()
		require.Len(t, spans, 1, "expected exactly 1 span")

		span := spans[0]
		// Can't use assert.True because span.Tag returns interface{}.
		assert.Equal(t, true, span.Tag("error"), "error tag on span should be true")
		assert.Equal(t, uint16(400), span.Tag("http.status_code"),
			"status code tag did not match")
	})

	status500RoundTrip := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError}, nil
	})

	t.Run("HTTP internal error causes an error", func(t *testing.T) {
		t.Parallel()

		tracer := mocktracer.New()
		transport := StartSpanMiddleware(tracer)(status500RoundTrip)

		_, err := transport.RoundTrip(httptest.NewRequest("", "http://localhost", nil))
		require.NoError(t, err, "request should not fail")

		spans := tracer.FinishedSpans()
		require.Len(t, spans, 1, "expected exactly 1 span")

		span := spans[0]
		// Can't use assert.True because span.Tag returns interface{}.
		assert.Equal(t, true, span.Tag("error"), "error tag on span should be true")
		assert.Equal(t, uint16(500), span.Tag("http.status_code"),
			"status code tag did not match")
	})

	transportErrorRoundTrip := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("great sadness")
	})
	t.Run("transport error causes an error", func(t *testing.T) {
		t.Parallel()

		tracer := mocktracer.New()
		transport := StartSpanMiddleware(tracer)(transportErrorRoundTrip)

		_, err := transport.RoundTrip(httptest.NewRequest("", "http://localhost", nil))
		require.Error(t, err, "request should fail")
		assert.Contains(t, err.Error(), "great sadness")

		spans := tracer.FinishedSpans()
		require.Len(t, spans, 1, "expected exactly 1 span")

		span := spans[0]
		// Can't use assert.True because span.Tag returns interface{}.
		assert.Equal(t, true, span.Tag("error"), "error tag on span should be true")
	})
}

func TestStartSpanMiddlewareWithDefaultTransport(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "hello")
	}))
	defer server.Close()

	var client http.Client

	tracer := mocktracer.New()
	client.Transport = StartSpanMiddleware(tracer)(client.Transport)

	res, err := client.Get(server.URL + "/")
	require.NoError(t, err, "failed to make request")
	defer res.Body.Close()
	assert.Equal(t, 200, res.StatusCode, "status code did not match")

	body, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(body), "response body did not match")

	spans := tracer.FinishedSpans()
	require.Len(t, spans, 1, "expected exactly 1 span")

	span := spans[0]
	assert.Equal(t, ext.SpanKindEnum("client"), span.Tag("span.kind"), "span kind did not match")
	assert.Equal(t, "GET", span.OperationName, "OperationName did not match")
	assert.Equal(t, uint16(200), span.Tag("http.status_code"), "status code tag did not match")
	assert.Equal(t, server.URL+"/", span.Tag("http.url"), "url tag did not match")
	assert.Equal(t, "GET", span.Tag("http.method"), "method tag did not match")
}

func TestInjectSpanMiddlewareTracer(t *testing.T) {
	t.Parallel()

	okRoundTrip := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	badInjector := injectorFunc(func(mocktracer.MockSpanContext, interface{}) error {
		return errors.New("very bad tracer")
	})

	t.Run("ignores missing span", func(t *testing.T) {
		t.Parallel()

		tracer := mocktracer.New()
		tracer.RegisterInjector(opentracing.HTTPHeaders, badInjector)
		transport := InjectSpanMiddleware(tracer)(okRoundTrip)

		_, err := transport.RoundTrip(httptest.NewRequest("", "http://localhost", nil))
		require.NoError(t, err, "request should not fail")
	})

	t.Run("injects spans into headers", func(t *testing.T) {
		t.Parallel()

		injectorWasCalled := false
		injectIntoHeaders := injectorFunc(func(ctx mocktracer.MockSpanContext, carrier interface{}) error {
			require.False(t, injectorWasCalled, "injector called too many times")
			injectorWasCalled = true

			assert.Equal(t, map[string]string{"token": "42"}, ctx.Baggage, "baggage mismatch")

			_, ok := carrier.(opentracing.HTTPHeadersCarrier)
			assert.True(t, ok,
				"carrier must be opentracing.HTTPHeadersCarrier, got %T", carrier)

			return nil
		})

		tracer := mocktracer.New()
		tracer.RegisterInjector(opentracing.HTTPHeaders, injectIntoHeaders)
		transport := InjectSpanMiddleware(tracer)(okRoundTrip)

		span := tracer.StartSpan("myrequest")
		span.SetBaggageItem("token", "42")
		ext.HTTPMethod.Set(span, "GET")
		ctx := opentracing.ContextWithSpan(context.Background(), span)

		_, err := transport.RoundTrip(
			httptest.NewRequest("", "http://localhost", nil).WithContext(ctx),
		)
		require.NoError(t, err, "request should not fail")
		assert.True(t, injectorWasCalled, "span injector was never called")
	})

	t.Run("span injection failure", func(t *testing.T) {
		t.Parallel()

		tracer := mocktracer.New()
		tracer.RegisterInjector(opentracing.HTTPHeaders, badInjector)
		transport := InjectSpanMiddleware(tracer)(okRoundTrip)

		span := tracer.StartSpan("test_method")
		span.SetBaggageItem("token", "42")
		ctx := opentracing.ContextWithSpan(context.Background(), span)

		_, err := transport.RoundTrip(
			httptest.NewRequest("", "http://localhost", nil).WithContext(ctx),
		)
		assert.EqualError(t, err, "very bad tracer")
	})
}

func TestInjectSpanMiddlewareWithDefaultTransport(t *testing.T) {
	t.Parallel()

	tracer := mocktracer.New()
	tracer.RegisterInjector(opentracing.HTTPHeaders,
		&mocktracer.TextMapPropagator{HTTPHeaders: true})

	span := tracer.StartSpan("myrequest")
	span.SetBaggageItem("token", "42")
	ext.HTTPMethod.Set(span, "GET")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := r.Header
		assert.NotEmpty(t, headers.Get("Mockpfx-Ids-Spanid"), "missing span id from request")
		assert.NotEmpty(t, headers.Get("Mockpfx-Ids-Traceid"), "missing trace id from request")
		assert.Equal(t, "42", headers.Get("Mockpfx-Baggage-Token"), "baggage did not match")

		io.WriteString(w, "hello")
	}))
	defer server.Close()

	var client http.Client
	client.Transport = InjectSpanMiddleware(tracer)(client.Transport)

	req, err := http.NewRequest("GET", server.URL, nil /* body */)
	require.NoError(t, err, "falied to build request")

	ctx := opentracing.ContextWithSpan(context.Background(), span)
	res, err := client.Do(req.WithContext(ctx))
	require.NoError(t, err, "failed to make request")
	defer res.Body.Close()
	assert.Equal(t, 200, res.StatusCode, "status code did not match")

	body, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(body), "response body did not match")
}
