package clientmiddleware

import "net/http"

// Chain takes a series of HTTP Client middlewares and returns a single
// middleware which applies them in-order. Any entry in the list may be nil.
func Chain(mws ...func(http.RoundTripper) http.RoundTripper) func(http.RoundTripper) http.RoundTripper {
	return func(rt http.RoundTripper) http.RoundTripper {
		// We need to wrap in reverse order to have the first middleware from
		// the list get the request first.
		for i := len(mws) - 1; i >= 0; i-- {
			if mw := mws[i]; mw != nil {
				rt = mw(rt)
			}
		}
		return rt
	}
}
