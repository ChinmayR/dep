package servermiddleware

import "net/http"

// Chain takes a series of HTTP server middlewares and returns a single
// middleware which applies them in-order. Any entry in the list may be nil.
func Chain(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		// We need to wrap in reverse order to have the first middleware from
		// the list get the request first.
		for i := len(mws) - 1; i >= 0; i-- {
			if mw := mws[i]; mw != nil {
				h = mw(h)
			}
		}
		return h
	}
}
