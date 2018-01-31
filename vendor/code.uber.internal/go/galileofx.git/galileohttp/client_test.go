package galileohttp

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"code.uber.internal/engsec/galileo-go.git/galileotest"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOutMiddleware(t *testing.T) {
	type ctxKey string

	t.Run("successful request", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		g := galileotest.NewMockGalileo(mockCtrl)

		ctx := context.WithValue(context.Background(), ctxKey("foo"), "bar")
		authCtx := context.WithValue(context.Background(), ctxKey("auth"), true)
		g.EXPECT().
			AuthenticateOut(ctx, "myservice").
			Return(authCtx, nil)

		var okRoundTrip roundTripperFunc = func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK}, nil
		}

		rt := AuthenticateOutMiddleware(g)(okRoundTrip)
		req := httptest.NewRequest("GET", "http://localhost", nil /* body */).WithContext(ctx)
		req.Header.Set("rpc-service", "myservice")

		res, err := rt.RoundTrip(req)
		require.NoError(t, err, "expected success")
		assert.Equal(t, 200, res.StatusCode, "status code did not match")
	})

	t.Run("request without Rpc-Service header", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		ctx := context.WithValue(context.Background(), ctxKey("foo"), "bar")

		roundTripperCalled := false
		var roundTripper roundTripperFunc = func(req *http.Request) (*http.Response, error) {
			require.False(t, roundTripperCalled, "round tripper called too many times")
			roundTripperCalled = true
			assert.Equal(t, ctx, req.Context(), "context mismatch")
			return &http.Response{StatusCode: http.StatusOK}, nil
		}

		g := galileotest.NewMockGalileo(mockCtrl)
		req := httptest.NewRequest("GET", "http://localhost", nil /* body */).WithContext(ctx)

		res, err := AuthenticateOutMiddleware(g)(roundTripper).RoundTrip(req)
		require.True(t, roundTripperCalled, "round tripper was never called")
		require.NoError(t, err, "expected success")
		assert.Equal(t, 200, res.StatusCode, "status code did not match")
	})

	t.Run("authentication error", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		g := galileotest.NewMockGalileo(mockCtrl)

		ctx := context.WithValue(context.Background(), ctxKey("foo"), "bar")
		g.EXPECT().
			AuthenticateOut(ctx, "myservice").
			Return(nil, errors.New("no bueno"))

		var dontCallMe roundTripperFunc = func(*http.Request) (*http.Response, error) {
			require.FailNow(t, "round tripper must never be called")
			return nil, errors.New("don't call me")
		}

		rt := AuthenticateOutMiddleware(g)(dontCallMe)
		req := httptest.NewRequest("GET", "http://localhost", nil /* body */).WithContext(ctx)
		req.Header.Set("rpc-service", "myservice")

		_, err := rt.RoundTrip(req)
		assert.Equal(t, errors.New("no bueno"), err, "expected failure")
	})
}

func TestOutMiddlewareWithDefaultTransport(t *testing.T) {
	type ctxKey string

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "hello")
	}))
	defer server.Close()

	ctx := context.WithValue(context.Background(), ctxKey("auth"), true)
	g := galileotest.NewMockGalileo(mockCtrl)
	g.EXPECT().
		AuthenticateOut(gomock.Any(), "myservice").
		Return(ctx, nil)

	var client http.Client
	client.Transport = AuthenticateOutMiddleware(g)(client.Transport)

	req, err := http.NewRequest("GET", server.URL, nil /* body */)
	require.NoError(t, err, "failed to build request")
	req.Header.Set("rpc-service", "myservice")

	res, err := client.Do(req)
	require.NoError(t, err, "failed to make request")
	defer res.Body.Close()
	assert.Equal(t, 200, res.StatusCode, "status code did not match")

	body, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(body), "response body did not match")
}
