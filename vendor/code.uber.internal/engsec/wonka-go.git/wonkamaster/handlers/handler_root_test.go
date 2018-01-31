package handlers

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootHandlerUnits(t *testing.T) {
	validHandler := NewRootHandler(getTestConfig(t))

	var testCases = []struct {
		name       string
		statusCode int
		makeCall   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:       "returns ok",
			statusCode: http.StatusOK,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				r := &http.Request{Body: ioutil.NopCloser(
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
