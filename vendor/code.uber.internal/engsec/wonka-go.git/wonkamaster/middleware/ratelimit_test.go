package middleware_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"code.uber.internal/engsec/wonka-go.git/wonkamaster/middleware"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/middleware/mock_http"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewRateLimiter(t *testing.T) {
	f := middleware.NewRateLimiter(middleware.RateConfig{
		Global:    nil,
		Endpoints: nil,
	})
	assert.NotNil(t, f)
}

func TestGlobalRateLimiting(t *testing.T) {
	f := middleware.NewRateLimiter(middleware.RateConfig{
		Global: &middleware.RateSpec{
			R: 0,
			B: 0,
		},
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rw := mock_http.NewMockResponseWriter(ctrl)
	rw.EXPECT().WriteHeader(http.StatusTooManyRequests).Times(1)
	rw.EXPECT().Write(gomock.Any()).Times(1)
	f.Apply(context.Background(), rw, new(http.Request), h{})
}

func TestEndpointRateLimiting(t *testing.T) {
	f := middleware.NewRateLimiter(middleware.RateConfig{
		Endpoints: []middleware.RateEndpointConfig{
			{
				Path: "/health",
				R:    0,
				B:    0,
			},
		},
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Expect this to be blocked
	rw := mock_http.NewMockResponseWriter(ctrl)
	rw.EXPECT().WriteHeader(http.StatusTooManyRequests).Times(1)
	rw.EXPECT().Write(gomock.Any()).Times(1)
	f.Apply(context.Background(), rw, &http.Request{
		URL: &url.URL{
			Path: "/health",
		},
	}, h{})

	// Expect this to work since it was not in the test config.
	rw2 := mock_http.NewMockResponseWriter(ctrl)
	f.Apply(context.Background(), rw2, &http.Request{
		URL: &url.URL{
			Path: "/other",
		},
	}, h{})
}

// XXX: gomock cannot mock internal interface.  https://github.com/golang/mock/issues/29. This
// circumvents that.
type h struct{}

func (handler h) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {}
