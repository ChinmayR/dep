package httpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _testPort = 8989

func init() {
	// See D1334359
	if portStr, ok := os.LookupEnv("HTTPSERVER_PORT"); ok {
		port, err := strconv.ParseInt(portStr, 10, 64)
		if err != nil {
			log.Printf("ERROR: Could not parse HTTPSERVER_PORT=%q: %v", portStr, err)
		} else {
			_testPort = int(port)
		}
	}
}

func testAddr() string {
	return fmt.Sprintf(":%d", _testPort)
}

func testURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/", _testPort)
}

// Returns an http.Handler that fails the test if it is ever called.
func dontCallMeHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Errorf("handler must not be called")
	})
}

func echoHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(w, r.Body)
		assert.NoError(t, err, "failed to copy request body")
	})
}

func postRoundTripSuccess(t *testing.T, url string, body string) string {
	res, err := http.Post(url, "text/plain", bytes.NewBufferString(body))
	require.NoError(t, err, "request should succeed")

	resBody, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err, "failed to read response body")
	assert.NoError(t, res.Body.Close(), "failed to close response body")

	return string(resBody)
}

func TestStartRoundtrip(t *testing.T) {
	t.Run("with implicit address", func(t *testing.T) {
		ctx := context.Background()
		h := NewHandle(&http.Server{Handler: echoHandler(t)})
		err := h.Start(ctx)
		require.NoError(t, err, "failed to start server")

		defer func() {
			assert.NoError(t, h.Shutdown(ctx), "error stopping server")
		}()

		resBody := postRoundTripSuccess(t,
			fmt.Sprintf("http://%s/", h.Addr()), "hello")
		assert.Equal(t, "hello", resBody, "response body did not match")
	})

	t.Run("with explicit address", func(t *testing.T) {
		ctx := context.Background()
		h := NewHandle(&http.Server{
			Handler: echoHandler(t),
			Addr:    testAddr(),
		})
		err := h.Start(ctx)
		require.NoError(t, err, "failed to start server")
		defer func() {
			assert.NoError(t, h.Shutdown(ctx), "error stopping server")
		}()

		resBody := postRoundTripSuccess(t, testURL(), "hello")
		assert.Equal(t, "hello", resBody, "response body did not match")
	})
}

func TestStartAndStop(t *testing.T) {
	t.Run("with timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		h := NewHandle(&http.Server{Handler: dontCallMeHandler(t)})
		err := h.Start(ctx)
		require.NoError(t, err)
		assert.NoError(t, h.Shutdown(ctx))
	})

	t.Run("without timeout", func(t *testing.T) {
		ctx := context.Background()
		h := NewHandle(&http.Server{Handler: dontCallMeHandler(t)})
		err := h.Start(ctx)
		require.NoError(t, err)
		assert.NoError(t, h.Shutdown(ctx))
	})

	t.Run("really short timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()

		h := NewHandle(&http.Server{
			Addr:    testAddr(),
			Handler: dontCallMeHandler(t),
		})
		err := h.Start(ctx)
		if !assert.Error(t, err, "Start should fail") {
			// If we ended up starting the server, we should stop it right
			// away.
			h.Shutdown(context.Background())
		}

		assert.Equal(t, context.DeadlineExceeded, err)

		_, err = http.Get(testURL())
		require.Error(t, err, "request must fail")
	})
}

func TestStartErrors(t *testing.T) {
	t.Run("invalid address", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := NewHandle(&http.Server{
			Handler: dontCallMeHandler(t),
			Addr:    ":wrong",
		}).Start(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `error starting HTTP server on ":wrong"`)
	})

	t.Run("failure to listen", func(t *testing.T) {
		listenCalled := false
		listen := func(network, addr string) (net.Listener, error) {
			listenCalled = true
			require.Equal(t, "tcp", network, "incorrect network requested")
			require.Equal(t, ":0", addr, "incorrect address requested")
			return nil, errors.New("great sadness")
		}

		err := NewHandle(&http.Server{}, listenFunc(listen)).Start(context.Background())
		require.True(t, listenCalled, "listen was never called")
		require.Error(t, err, "expected failure")
		assert.Contains(t, err.Error(), `error starting HTTP server on ":0": great sadness`)
	})

	t.Run("serve error returns original error", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		listener := NewMockListener(mockCtrl)
		listen := func(string, string) (net.Listener, error) {
			return listener, nil
		}

		mockDialer := NewMockDialer(mockCtrl)
		newDialer := func() dialer {
			return mockDialer
		}

		giveErr := errors.New("great sadness")
		listener.EXPECT().
			Accept().
			Return(nil, giveErr)
		listener.EXPECT().
			Addr().
			Return(&net.TCPAddr{
				IP:   net.ParseIP("198.51.100.1"), // TEST-NET-2
				Port: 18888,
			}).AnyTimes()
		listener.EXPECT().Close()

		mockDialer.EXPECT().
			DialContext(gomock.Any(), "tcp", "198.51.100.1:18888").
			Do(func(context.Context, string, string) {
				// The Dial should take long enough that the system can
				// realize that the server failed to start up.
				time.Sleep(100 * time.Millisecond)
			}).
			Return(nil, errors.New("this error should not be shown to the user"))

		h := NewHandle(&http.Server{}, listenFunc(listen), newDialerFunc(newDialer))
		err := h.Start(context.Background())
		require.Error(t, err, "expected failure")
		assert.Contains(t, err.Error(), "error starting HTTP server: great sadness")
	})
}

func TestStartTwice(t *testing.T) {
	ctx := context.Background()
	h := NewHandle(&http.Server{Handler: dontCallMeHandler(t)})
	require.NoError(t, h.Start(ctx))
	defer func() {
		assert.NoError(t, h.Shutdown(ctx))
	}()

	err := h.Start(ctx)
	require.Error(t, err, "second Start should fail")
	assert.Contains(t, err.Error(), "server is already running")
}

func TestShutdownTwice(t *testing.T) {
	ctx := context.Background()
	h := NewHandle(&http.Server{Handler: dontCallMeHandler(t)})
	err := h.Start(ctx)
	require.NoError(t, err)

	require.NoError(t, h.Shutdown(ctx))
	require.NoError(t, h.Shutdown(ctx))
}

func TestStartAndShutdownManyTimes(t *testing.T) {
	ctx := context.Background()
	h := NewHandle(&http.Server{Handler: echoHandler(t)})
	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			require.NoError(t, h.Start(ctx), "failed to start server")
			defer func() {
				assert.NoError(t, h.Shutdown(ctx), "error stopping server")
			}()

			resBody := postRoundTripSuccess(t, fmt.Sprintf("http://%s/", h.Addr()), "hello")
			assert.Equal(t, "hello", resBody, "response body did not match")
		})
	}
}

func TestShutdownErrors(t *testing.T) {
	t.Run("serve error", func(t *testing.T) {
		ctx := context.Background()
		h := NewHandle(&http.Server{Handler: echoHandler(t)})
		err := h.Start(ctx)
		require.NoError(t, err, "failed to start server")

		postRoundTripSuccess(t, fmt.Sprintf("http://%s/", h.Addr()), "hello")

		// Get a working server and kill the listener.
		require.NoError(t, h.ln.Close(), "failed to close listener")

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		err = h.Shutdown(ctx)
		require.Error(t, err, "expected failure")
		assert.Contains(t, err.Error(), "use of closed network connection")
	})

	t.Run("shutdown timeout", func(t *testing.T) {
		ctx := context.Background()
		h := NewHandle(&http.Server{Handler: dontCallMeHandler(t)})
		err := h.Start(ctx)
		require.NoError(t, err)

		// Create a bunch of connections to the server that will prevent it
		// from shutting down in time.
		for i := 0; i < 100; i++ {
			conn, err := net.Dial("tcp", h.Addr().String())
			require.NoError(t, err, "failed to connect to server")
			defer conn.Close()

			// This will take 250 milliseconds to write but we'll wait only 50
			// for shutdown.
			go dripWrite(conn, 50*time.Millisecond, "GET /")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err = h.Shutdown(ctx)
		assert.Equal(t, context.DeadlineExceeded, err, "expected deadline exceeded error")
	})
}

func dripWrite(w io.Writer, interval time.Duration, s string) {
	bs := []byte(s)
	for _, b := range bs {
		w.Write([]byte{b})
		time.Sleep(interval)
	}
}

func TestHandleAddrWithoutStarting(t *testing.T) {
	h := NewHandle(&http.Server{})
	assert.Nil(t, h.Addr(), "addr should be nil")
}
