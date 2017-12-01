package xhttp

import (
	"context"
	"net/http"
	"regexp"
	"sync"
)

// A RequestMatcher is a matching predicate for an HTTP Request
type RequestMatcher func(r *http.Request) bool

// PathMatchesRegexp returns a matcher that evaluates the path against the given regexp
func PathMatchesRegexp(re *regexp.Regexp) RequestMatcher {
	return func(r *http.Request) bool {
		return re.MatchString(r.URL.Path)
	}
}

type route struct {
	matches RequestMatcher
	handler Handler
}

// Router is an HTTP handler that can route based on arbitrary RequestMatchers.
// It applies Filter (or filter chain) before invoking the actual routes.
// If the filter is nil, it uses DefaultFilter
type Router struct {
	mux    sync.RWMutex
	routes []route
	filter Filter
}

// NewRouter creates a new empty router
func NewRouter() *Router { return &Router{} }

// NewRouterWithFilter creates a new empty router with a pre-configured Filter or FilterChain
func NewRouterWithFilter(filter Filter) *Router {
	return &Router{filter: filter}
}

// ServeHTTP creates a new request Context, and runs the filter chain,
// eventually routing to the appropriate handler based on the incoming path.
func (h *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	filter := h.filter
	if filter == nil {
		filter = DefaultFilter
	}
	filter.Apply(ctx, w, r, HandlerFunc(h.serveHTTP))
}

// serveHTTP routes to the appropriate handler based on the incoming path
func (h *Router) serveHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var handler Handler

	h.mux.RLock()
	for i := range h.routes {
		if h.routes[i].matches(r) {
			handler = h.routes[i].handler
			break
		}
	}
	h.mux.RUnlock()

	if handler != nil {
		handler.ServeHTTP(ctx, w, r)
	} else {
		http.NotFound(w, r)
	}
}

// AddPatternRoute is a convenience function for routing to the given handler
// based on a path regex.
//
// Note: the router requires a context-aware xhttp.Handler. Please change your code
// to accept a context object and propagate it through the call stack. If your handler
// is from a 3rd party library, you can wrap it with xhttp.Wrap()
func (h *Router) AddPatternRoute(pattern string, handler Handler) {
	h.AddRoute(PathMatchesRegexp(regexp.MustCompile(pattern)), handler)
}

// AddRoute adds a new router
//
// Note: the router requires a context-aware xhttp.Handler. Please change your code
// to accept a context object and propagate it through the call stack. If your handler
// is from a 3rd party library, you can wrap it with xhttp.Wrap()
func (h *Router) AddRoute(matches RequestMatcher, handler Handler) {
	h.mux.Lock()
	defer h.mux.Unlock()

	h.routes = append(h.routes, route{matches: matches, handler: handler})
}
