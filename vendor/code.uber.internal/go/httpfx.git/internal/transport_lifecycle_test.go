package internal

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
}

func TestSupportsNilTransport(t *testing.T) {
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "goodbye")
	}))

	defer h.Close()

	var tl TransportLifecycle
	client := http.Client{Transport: tl.Wrap(nil /* transport */)}

	resp, err := client.Get("http://" + h.Listener.Addr().String())
	require.NoError(t, err, "Can't make a simple request")

	bye, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "goodbye", string(bye))

	assert.NoError(t, resp.Body.Close())
}

func TestTransportDoubleWrapping(t *testing.T) {
	var tl TransportLifecycle
	rt := tl.Wrap(tl.Wrap(nil /* transport */))

	tr, ok := rt.(*transport)
	require.True(t, ok, "expected *transport")

	_, ok = tr.rt.(*transport)
	require.False(t, ok, "should not double wrap")
}

func TestPanicInTransport(t *testing.T) {
	var tl TransportLifecycle
	var roundTripper RoundTripperFunc = func(r *http.Request) (*http.Response, error) {
		panic("boom")
	}

	cl := http.Client{Transport: tl.Wrap(roundTripper)}

	require.Panics(t, func() { cl.Get("http://uber.com") })

	assert.NoError(t, tl.Shutdown(context.Background()), "stopping transport")

	_, err := cl.Get("http://uber.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transport was stopped")
}

func TestTransportConcurrency(t *testing.T) {
	var roundTripper RoundTripperFunc = func(r *http.Request) (*http.Response, error) {
		if r.Body != nil {
			assert.NoError(t, r.Body.Close())
		}
		return &http.Response{Request: r}, nil
	}

	var (
		tl TransportLifecycle
		wg sync.WaitGroup
	)

	ready := make(chan struct{})
	for i := 0; i < 10; i++ {
		cl := &http.Client{Transport: tl.Wrap(roundTripper)}
		for j := 0; j < 100; j++ {
			wg.Add(2)
			go func(cl *http.Client) {
				<-ready
				defer wg.Done()
				_, err := cl.Get("http://uber.com/")
				assert.NoError(t, err)
			}(cl)

			go func(cl *http.Client) {
				<-ready
				defer wg.Done()
				_, err := cl.PostForm("http://example.com", url.Values{})
				assert.NoError(t, err)
			}(cl)
		}
	}

	close(ready)
	wg.Wait()
}

func TestTransportStopTimeout(t *testing.T) {
	ready := make(chan struct{})

	var tl TransportLifecycle
	var roundTripper RoundTripperFunc = func(r *http.Request) (*http.Response, error) {
		// Transport received request.
		close(ready)
		select {}
	}

	transport := tl.Wrap(roundTripper)

	go func() {
		transport.RoundTrip(httptest.NewRequest("", "http://localhost", nil))
	}()

	// Wait for transport to receive request.
	<-ready

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	err := tl.Shutdown(ctx)
	assert.Equal(t, context.DeadlineExceeded, err, "request should time out")
}

func TestE2ETransportStartsAndStops(t *testing.T) {
	srv := startServer()
	defer srv.Close()

	var tl TransportLifecycle
	cl := http.Client{Transport: tl.Wrap(nil /* roundTripper */)}

	rsp, err := cl.Get(srv.URL)
	require.NoError(t, err, "execute a simple get")
	assert.Equal(t, http.StatusOK, rsp.StatusCode)

	require.NoError(t, tl.Shutdown(context.Background()), "stop transport")
	_, err = cl.Get(srv.URL)
	require.Error(t, err, "stopped transport should return an error")
	assert.Contains(t, err.Error(), "transport was stopped")

	assert.NoError(t, tl.Shutdown(context.Background()), "stop second time")
}

// We are going to send 2 requests: the first one should be drained,
// and client is going to block until the request is done.
//
// We will request a stop as soon as the server signals receipt of
// the first request using `receivedRequest`.
//
// Second request is sent while the client is in the process of draining
// previous requests and should error. Sequence diagram:
//
//   test        client      server
//     |   Req1    |           |
//     |---------->|    Req1   |
//     |           | --------->|
//     |    Stop   |           |
//     |---------->|           |
//     |           |           |
//     |   Req2    |           |
//     |---------->|           |
//     |   Req2    |           |
//     |<----------|           |
//     |           |    Req1   |
//     |           |<----------|
//     |   Req1    |           |
//     |<----------|           |
//     |           |           |
//     |    Stop   |           |
//     |<----------|           |
func TestE2ETransportDrainsRequests(t *testing.T) {
	srv := startServer()
	defer srv.Close()

	receivedRequest := make(chan struct{})
	stopClient := make(chan struct{})

	wait := sync.WaitGroup{}
	wait.Add(3)
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server received the first request, so we can stop the client
		close(receivedRequest)

		// Let a test to stop client, while processing current request.
		<-stopClient

		// Reply to the very first request
		fmt.Fprintf(w, "Hello")
	})

	var tl TransportLifecycle
	cl := http.Client{Transport: tl.Wrap(nil /* transport */)}

	// Send a request that should be waited on a before a m completely stops.
	go func() {
		rsp, err := cl.Get(srv.URL)
		require.NoError(t, err)

		hello, err := ioutil.ReadAll(rsp.Body)
		require.NoError(t, err)

		require.NoError(t, rsp.Body.Close())
		assert.Equal(t, "Hello", string(hello))

		wait.Done()
	}()

	// Stop client
	go func() {
		// Wait for a server to receive a first request
		<-receivedRequest

		// Stop client
		require.NoError(t, tl.Shutdown(context.Background()))

		// All request processing is done.
		wait.Done()
	}()

	// Issue a request on client that is in the process of draining.
	go func() {
		// wait for a client to stop
		for !tl.Stopped() {
			runtime.Gosched()
		}

		_, err := cl.Get(srv.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transport was stopped")

		// Let the first request to proceed and Stop to finish.
		close(stopClient)

		wait.Done()
	}()

	// Wait for all requests to finish.
	wait.Wait()
}
