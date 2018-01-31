package internal

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"go.uber.org/atomic"
)

// TransportLifecycle wraps http.RoundTrippers with support for graceful
// shutdown.
//
//   var tl TransportLifecycle
//   rt := tl.Wrap(http.DefaultTransport)
//   defer tl.Shutdown(ctx)
//
// Any number of http.RoundTrippers may be wrapped using a single
// TransportLifecycle. The TransportLifecycle can shut down all dependent
// http.RoundTrippers, waiting for ongoing requests to finish executing.
type TransportLifecycle struct {
	wg      sync.WaitGroup
	stopped atomic.Bool
}

// Wrap wraps the given http.RoundTripper with support for graceful shutdown.
//
// If rt isn nil, http.DefaultTransport is used.
func (l *TransportLifecycle) Wrap(rt http.RoundTripper) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}

	if _, ok := rt.(*transport); ok {
		// Already wrapped.
		return rt
	}

	return &transport{l: l, rt: rt}
}

// Shutdown disallows any new requests through all wrapped transports and
// waits until all ongoing requests on those transports have finished, or
// until the given context finishes.
func (l *TransportLifecycle) Shutdown(ctx context.Context) error {
	l.stopped.Store(true)

	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// Stopped returns true if the TransportLifecycle has been stopped.
func (l *TransportLifecycle) Stopped() bool {
	return l.stopped.Load()
}

func (l *TransportLifecycle) startRequest()  { l.wg.Add(1) }
func (l *TransportLifecycle) finishRequest() { l.wg.Done() }

// transport is an http.RoundTripper that supports waiting for ongoing
// requests to finish.
type transport struct {
	l  *TransportLifecycle
	rt http.RoundTripper
}

var _ http.RoundTripper = (*transport)(nil)

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.l.startRequest()
	defer t.l.finishRequest()

	// We need to mark the request has started before checking whether we've
	// been stopped. Otherwise there's a race between setting the flag and
	// calling wg.Wait in Shutdown().
	if t.l.Stopped() {
		return nil, errors.New("transport was stopped")
	}

	return t.rt.RoundTrip(req)
}
