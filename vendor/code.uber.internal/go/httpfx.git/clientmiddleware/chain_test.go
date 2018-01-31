package clientmiddleware

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"code.uber.internal/go/httpfx.git/internal"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChainClientMiddleware(t *testing.T) {
	t.Run("no args is noop", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		rt := panicRoundTripper{}

		// == instead of assert.Equal because we don't want a deep match.
		assert.True(t, rt == Chain()(rt),
			"round tripper must be unchanged")
	})

	t.Run("middleware is called in-order", func(t *testing.T) {
		var (
			counter int
			mw1     = checkPositionMiddleware(t, 0, &counter)
			mw2     = checkPositionMiddleware(t, 1, &counter)
			mw3     = checkPositionMiddleware(t, 2, &counter)
			mw4     = checkPositionMiddleware(t, 3, &counter)

			rt internal.RoundTripperFunc = func(req *http.Request) (*http.Response, error) {
				require.Equal(t, 4, counter, "counter did not match")
				counter--
				return &http.Response{Body: ioutil.NopCloser(bytes.NewBufferString("hello"))}, nil
			}
		)

		transport := Chain(mw1, mw2, mw3, mw4)(rt)
		res, err := transport.RoundTrip(&http.Request{})
		require.NoError(t, err, "request must not fail")
		defer res.Body.Close()

		gotBody, err := ioutil.ReadAll(res.Body)
		require.NoError(t, err, "failed to read response body")

		assert.Equal(t, "hello", string(gotBody), "response body did not match")
	})
}

func checkPositionMiddleware(t *testing.T, want int, counter *int) func(http.RoundTripper) http.RoundTripper {
	return func(rt http.RoundTripper) http.RoundTripper {
		return internal.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			require.Equal(t, want, *counter, "counter did not match on entry")
			*counter++

			res, err := rt.RoundTrip(r)

			require.Equal(t, want, *counter, "counter did not match on exit")
			*counter--

			return res, err
		})
	}
}

type panicRoundTripper struct{}

func (panicRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	panic("not implemented")
}
