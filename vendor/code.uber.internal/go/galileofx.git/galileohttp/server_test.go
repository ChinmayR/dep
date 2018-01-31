package galileohttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"code.uber.internal/engsec/galileo-go.git/galileotest"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestInMiddleware(t *testing.T) {
	type ctxKey string

	t.Run("authenticated requests are allowed", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		ctx := context.WithValue(context.Background(), ctxKey("foo"), "bar")

		g := galileotest.NewMockGalileo(mockCtrl)
		g.EXPECT().AuthenticateIn(ctx).Return(nil)

		called := false
		var handler http.Handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			assert.False(t, called, "handler already called")
			called = true
		})

		handler = AuthenticateInMiddleware(g)(handler)

		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "/", nil).WithContext(ctx))
		assert.True(t, called, "handler was never called")
		assert.Equal(t, 200, res.Code, "response code did not match")
	})

	t.Run("unauthenticated requests are rejected", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		core, logs := observer.New(zapcore.InfoLevel)

		ctx := context.WithValue(context.Background(), ctxKey("foo"), "bar")

		g := galileotest.NewMockGalileo(mockCtrl)
		g.EXPECT().AuthenticateIn(ctx).Return(errors.New("no bueno"))

		dontCallMe := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "handler must never be called")
		})
		handler := AuthenticateInMiddleware(g,
			AuthenticateInLogger(zap.New(core)),
		)(dontCallMe)

		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "/", nil).WithContext(ctx))
		assert.Equal(t, 403, res.Code, "response code did not match")
		assert.Equal(t, "EVERYONE", res.Header().Get("X-Wonka-Requires"),
			"expected X-Wonka-Requires header")

		assert.Equal(t, 1,
			logs.FilterField(zap.Error(errors.New("no bueno"))).Len(),
			"expected a log entry for auth failure")
	})
}

func TestAuthenticateInIgnorePaths(t *testing.T) {
	type ctxKey string

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mux := http.NewServeMux()

	ignoreCalls := 0
	ignoreHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		ignoreCalls++
	})

	mux.Handle("/ignore", ignoreHandler)
	mux.Handle("/ignore/", ignoreHandler)
	mux.Handle("/", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		assert.Fail(t, "/ must never be called")
	}))

	g := galileotest.NewMockGalileo(mockCtrl)
	handler := AuthenticateInMiddleware(g, AuthenticateInIgnorePaths("/ignore"))(mux)

	t.Run("ignored paths allow all requests", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ctxKey("foo"), "bar")

		t.Run("request with slash", func(t *testing.T) {
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest("GET", "/ignore/", nil).WithContext(ctx))
			assert.Equal(t, 1, ignoreCalls, "ignorewithslash must be called once")
			assert.Equal(t, 200, res.Code, "response code did not match")
		})

		t.Run("request without slash", func(t *testing.T) {
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest("GET", "/ignore", nil).WithContext(ctx))
			assert.Equal(t, 2, ignoreCalls, "ignorewithslash must be called twice")
			assert.Equal(t, 200, res.Code, "response code did not match")
		})
	})

	t.Run("other paths require authentication", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ctxKey("baz"), "qux")
		g.EXPECT().AuthenticateIn(ctx).Return(errors.New("great sadness"))

		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "/", nil).WithContext(ctx))
		assert.Equal(t, 403, res.Code, "response code did not match")
	})
}
