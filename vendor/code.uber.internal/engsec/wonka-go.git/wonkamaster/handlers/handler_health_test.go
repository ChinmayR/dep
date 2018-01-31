package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

func TestHealthHandler(t *testing.T) {
	wonkatestdata.WithHTTPListener(func(ln net.Listener, r *xhttp.Router) {
		handlerCfg := common.HandlerConfig{
			Logger:  zap.L(),
			Metrics: tally.NoopScope,
		}

		r.AddPatternRoute("/health", newHealthHandler(handlerCfg))
		url := fmt.Sprintf("http://%s/health", ln.Addr().String())
		client := &http.Client{}
		req, _ := http.NewRequest("GET", url, nil)

		resp, e := client.Do(req)
		require.NoError(t, e, "get: %v", e)

		body, e := ioutil.ReadAll(resp.Body)
		require.NoError(t, e, "%d, reading body: %v", e)
		require.Contains(t, string(body), "OK")
	})
}

func TestHealthHandlerUnits(t *testing.T) {
	validHandler := newHealthHandler(getTestConfig(t))

	var testCases = []struct {
		name       string
		statusCode int
		makeCall   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:       "returns ok via the json route",
			statusCode: http.StatusOK,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				r := &http.Request{
					URL: &url.URL{Path: "/health/json"},
					Body: ioutil.NopCloser(
						bytes.NewReader([]byte("The suspense is terrible...I hope It'll last.")))}
				validHandler.ServeHTTP(context.Background(), w, r)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.makeCall(t, w)
			resp := w.Result()
			assert.Equal(t, tc.statusCode, resp.StatusCode, "status code did not match expected")
		})
	}
}
