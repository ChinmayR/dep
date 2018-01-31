package servermiddleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestChainServerMiddleware(t *testing.T) {
	t.Run("no args is noop", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		h := panicHandler{}

		// == instead of assert.Equal because we don't want a deep match.
		assert.True(t, h == Chain()(h),
			"round tripper must be unchanged")
	})

	t.Run("middleware is called in-order", func(t *testing.T) {
		var (
			counter int
			mw1     = checkPositionMiddleware(t, 0, &counter)
			mw2     = checkPositionMiddleware(t, 1, &counter)
			mw3     = checkPositionMiddleware(t, 2, &counter)
			mw4     = checkPositionMiddleware(t, 3, &counter)

			h http.HandlerFunc = func(w http.ResponseWriter, req *http.Request) {
				assert.Equal(t, 4, counter, "counter did not match")
				counter--
				io.WriteString(w, "hello")
			}
		)

		handler := Chain(mw1, mw2, mw3, mw4)(h)

		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "/", nil /* body */))

		assert.Equal(t, -1, counter)
		assert.Equal(t, "hello", res.Body.String(), "response body did not match")
	})
}

func checkPositionMiddleware(t *testing.T, want int, counter *int) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, want, *counter, "counter did not match on entry")
			*counter++

			h.ServeHTTP(w, r)

			assert.Equal(t, want, *counter, "counter did not match on exit")
			*counter--
		})
	}
}

type panicHandler struct{}

func (panicHandler) ServeHTTP(http.ResponseWriter, *http.Request) {
	panic("not implemented")
}
