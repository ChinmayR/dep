package xhttp

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTracing(t *testing.T) {
	baggageKey := "some-baggage-item"
	jsonTraceID := "trace_id"
	jsonBaggage := "baggage"

	tracer := mocktracer.New()

	getTraceID := func(span opentracing.Span) string {
		ctx := span.Context().(mocktracer.MockSpanContext)
		return fmt.Sprintf("%v", ctx.TraceID)
	}

	// enable tracing filter in the router, without affecting DefaultFilter
	tr := &Tracer{tracer}
	r := NewRouterWithFilter(FilterFunc(tr.TracedServer))
	r.AddRoute(PathMatchesRegexp(regexp.MustCompile("/trace")),
		HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
			if span := opentracing.SpanFromContext(ctx); span != nil {
				RespondWithJSON(w, map[string]string{
					jsonTraceID: getTraceID(span),
					jsonBaggage: span.BaggageItem(baggageKey),
				})
			} else {
				RespondWithJSON(w, map[string]string{})
			}
		}))

	l := serve(t, r)
	defer l.Close()

	// enable tracing filter on a client, without using DefaultClientFilter
	client := &Client{Filter: ClientFilterFunc(tr.TracedClient)}

	// start a trace before invoking the client to simulate in-process context propagation
	span := tracer.StartSpan("client-main")
	span.SetBaggageItem(baggageKey, "howdy")

	testCases := []struct {
		url        string
		serverSpan bool
		statusCode int
		err        bool
	}{
		{
			url:        fmt.Sprintf("http://%s/trace", l.Addr().String()),
			serverSpan: true,
			statusCode: 200,
		},
		{
			url:        fmt.Sprintf("http://%s/wrong", l.Addr().String()),
			serverSpan: true,
			statusCode: 404,
			err:        true,
		},
		{
			url:        fmt.Sprintf("http-invalid://%s/trace", l.Addr().String()),
			serverSpan: false,
			err:        true,
		},
	}

	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(fmt.Sprintf("%v", testCase.statusCode), func(t *testing.T) {
			tracer.Reset()
			ctx := opentracing.ContextWithSpan(context.Background(), span)

			var response map[string]string
			err := GetJSON(ctx, client, testCase.url, &response, &CallOptions{
				Headers: map[string]string{
					"X-Uber-Source": "i-am-client",
				},
			})
			if !testCase.err {
				require.NoError(t, err)
				traceVal := response[jsonTraceID]
				baggageVal := response[jsonBaggage]
				assert.Equal(t, getTraceID(span), traceVal)
				assert.Equal(t, "howdy", baggageVal)
			}

			spans := tracer.FinishedSpans()
			if testCase.serverSpan {
				assert.Len(t, spans, 2)
			} else {
				assert.Len(t, spans, 1)
			}

			// verify that proper tags have been written
			var serverSpan, clientSpan *mocktracer.MockSpan
			for _, span := range spans {
				if span.Tag("span.kind") == ext.SpanKindRPCClientEnum {
					clientSpan = span
				}
				if span.Tag("span.kind") == ext.SpanKindRPCServerEnum {
					serverSpan = span
				}
				if testCase.serverSpan {
					assert.EqualValues(t, testCase.statusCode, span.Tag("http.status_code"))
				}
				if testCase.err {
					assert.EqualValues(t, true, span.Tag("error"))
				}
			}
			if testCase.serverSpan {
				if assert.NotNil(t, serverSpan, "expecting serverSpan") {
					assert.NotEmpty(t, serverSpan.Tag("peer.address"))
					assert.Equal(t, "i-am-client", serverSpan.Tag("peer.service"))
				}
			}
			require.NotNil(t, clientSpan, "expecting clientSpan")
		})
	}
}

func TestWrapForStatusCodeTracking(t *testing.T) {
	r := NewRouter()
	r.AddRoute(PathMatchesRegexp(regexp.MustCompile("/wrapper")),
		HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
			// check that w has the interfaces we expect
			_ = w.(http.Flusher)
			_ = w.(http.Hijacker)
			_ = w.(http.CloseNotifier)
			tracker := wrapForStatusCodeTracking(w)
			// check that the wrapper has the same interfaces
			_, flusher := tracker.ResponseWriter.(http.Flusher)
			_, hijacker := tracker.ResponseWriter.(http.Hijacker)
			_, closeNotifier := tracker.ResponseWriter.(http.CloseNotifier)
			RespondWithJSON(w, map[string]bool{
				"flusher":       flusher,
				"hijacker":      hijacker,
				"closeNotifier": closeNotifier,
			})
		}))

	l := serve(t, r)
	defer l.Close()

	var response map[string]bool
	url := fmt.Sprintf("http://%s/wrapper", l.Addr().String())
	err := GetJSON(context.Background(), nil, url, &response, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{
		"flusher":       true,
		"hijacker":      true,
		"closeNotifier": true,
	}, response)
}
