package httpfx

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.uber.internal/go/httpfx.git/internal"
	versionfx "code.uber.internal/go/versionfx.git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func newVersionReporter() *versionfx.Reporter {
	return new(versionfx.Reporter)
}

func echoHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, vs := range r.Header {
			if strings.HasPrefix(k, "X-") {
				w.Header()[k] = vs
			}
		}

		_, err := io.Copy(w, r.Body)
		assert.NoError(t, err, "failed to copy body")
		assert.NoError(t, r.Body.Close(), "failed to close request body")
	})
}

func TestClient(t *testing.T) {
	srv := httptest.NewServer(echoHandler(t))
	defer srv.Close()

	var cl struct{ *http.Client }
	app := fxtest.New(t,
		fx.Provide(NewClient, newVersionReporter),
		fx.Extract(&cl),
	)
	defer app.RequireStart().RequireStop()

	require.NotNil(t, cl.Client)

	res, err := cl.Post(srv.URL, "text/plain", bytes.NewBufferString("it's alive!"))
	require.NoError(t, err, "HTTP request failed")

	b, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err, "failed to read response body")

	assert.Equal(t, "it's alive!", string(b), "response body did not match")
	assert.NoError(t, res.Body.Close(), "failed to close response body")
}

func TestClientStopped(t *testing.T) {
	srv := httptest.NewServer(echoHandler(t))
	defer srv.Close()

	var cl struct{ *http.Client }
	app := fxtest.New(t,
		fx.Provide(NewClient, newVersionReporter),
		fx.Extract(&cl),
	)
	app.RequireStart().RequireStop()

	_, err := cl.Post(srv.URL, "text/plain", bytes.NewBufferString("it's alive!"))
	require.Error(t, err, "request with stopped client must fail")
	assert.Contains(t, err.Error(), "transport was stopped")
}

type fakeDefaultMiddleware struct {
	t *testing.T

	RanStartTrace bool
	RanAuth       bool
	RanEndTrace   bool
}

func newFakeDefaultMiddleware(t *testing.T) *fakeDefaultMiddleware {
	return &fakeDefaultMiddleware{t: t}
}

func (f *fakeDefaultMiddleware) Provide() fx.Option {
	type middlewareOut struct {
		fx.Out

		StartTrace func(http.RoundTripper) http.RoundTripper `name:"trace.start"`
		EndTrace   func(http.RoundTripper) http.RoundTripper `name:"trace.end"`
		Auth       func(http.RoundTripper) http.RoundTripper `name:"auth"`
	}

	return fx.Provide(func() middlewareOut {
		return middlewareOut{
			StartTrace: f.startTrace,
			EndTrace:   f.endTrace,
			Auth:       f.auth,
		}
	})
}

func (f *fakeDefaultMiddleware) Verify() {
	t := f.t
	assert.True(t, f.RanStartTrace, "StartTrace never ran")
	assert.True(t, f.RanAuth, "Auth never ran")
	assert.True(t, f.RanEndTrace, "EndTrace never ran")
}

func (f *fakeDefaultMiddleware) startTrace(rt http.RoundTripper) http.RoundTripper {
	t := f.t
	return internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		require.False(t, f.RanStartTrace, "StartTrace already ran")
		require.False(t, f.RanAuth, "Auth ran before StartTrace")
		require.False(t, f.RanEndTrace, "EndTrace ran before StartTrace")
		f.RanStartTrace = true
		return rt.RoundTrip(req)
	})
}

func (f *fakeDefaultMiddleware) endTrace(rt http.RoundTripper) http.RoundTripper {
	t := f.t
	return internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		require.True(t, f.RanStartTrace, "StartTrace must run before EndTrace")
		require.True(t, f.RanAuth, "Auth must run before EndTrace")
		require.False(t, f.RanEndTrace, "EndTrace already ran")
		f.RanEndTrace = true
		return rt.RoundTrip(req)
	})
}

func (f *fakeDefaultMiddleware) auth(rt http.RoundTripper) http.RoundTripper {
	t := f.t
	return internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		require.True(t, f.RanStartTrace, "StartTrace must run before Auth")
		require.False(t, f.RanAuth, "Auth already ran")
		require.False(t, f.RanEndTrace, "EndTrace already ran")
		f.RanAuth = true
		return rt.RoundTrip(req)
	})
}

func TestClientTraceAndAuthMiddleware(t *testing.T) {
	fakeMW := newFakeDefaultMiddleware(t)
	defer fakeMW.Verify()

	srv := httptest.NewServer(echoHandler(t))
	defer srv.Close()

	var cl struct{ *http.Client }
	app := fxtest.New(t,
		fx.Provide(
			NewClient,
			newVersionReporter,
		),
		fakeMW.Provide(),
		fx.Extract(&cl),
	)
	defer app.RequireStart().RequireStop()

	res, err := cl.Post(srv.URL, "text/plain", bytes.NewBufferString("it's alive!"))
	require.NoError(t, err, "HTTP request failed")

	b, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err, "failed to read response body")

	assert.Equal(t, "it's alive!", string(b), "response body did not match")
	assert.NoError(t, res.Body.Close(), "failed to close response body")

}

func TestClientInstrument(t *testing.T) {
	fakeMW := newFakeDefaultMiddleware(t)
	defer fakeMW.Verify()

	srv := httptest.NewServer(echoHandler(t))
	defer srv.Close()

	var out struct {
		Instrument func(*http.Client, ...func(http.RoundTripper) http.RoundTripper)
	}
	app := fxtest.New(t,
		fx.Provide(
			NewClient,
			newVersionReporter,
		),
		fakeMW.Provide(),
		fx.Extract(&out),
	)
	defer app.RequireStart().RequireStop()

	ranUserMW := false
	userMW := func(rt http.RoundTripper) http.RoundTripper {
		return internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			require.True(t, fakeMW.RanStartTrace, "StartTrace must run before user middleware")
			require.False(t, ranUserMW, "user middleware already ran")
			require.False(t, fakeMW.RanAuth, "Auth must not run before user middleware")
			require.False(t, fakeMW.RanEndTrace, "EndTrace must not run before user middleware")
			ranUserMW = true
			return rt.RoundTrip(req)
		})
	}

	var client http.Client
	out.Instrument(&client, userMW)

	res, err := client.Post(srv.URL, "text/plain", bytes.NewBufferString("it's alive!"))
	require.NoError(t, err, "HTTP request failed")

	b, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err, "failed to read response body")

	assert.Equal(t, "it's alive!", string(b), "response body did not match")
	assert.NoError(t, res.Body.Close(), "failed to close response body")

	require.True(t, ranUserMW, "user middleware never ran")
}
