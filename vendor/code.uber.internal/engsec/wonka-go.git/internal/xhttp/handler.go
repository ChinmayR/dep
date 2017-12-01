package xhttp

import (
	"context"
	"net/http"
)

// Handler is a context-aware extension of http.Handler.
// All Uber services are expected to propagate request context.
// See http://t.uber.com/context-propagation for the background.
type Handler interface {
	ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request)
}

// The HandlerFunc type is an adapter to allow the use of
// ordinary functions as HTTP handlers.  If f is a function
// with the appropriate signature, HandlerFunc(f) is a
// Handler object that calls f.
type HandlerFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request)

// ServeHTTP calls f(ctx, w, r).
func (f HandlerFunc) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	f(ctx, w, r)
}
