package xhttp

import (
	"context"
	"net/http"
)

// Filter performs filtering tasks on either the request to a server endpoint,
// or on the response from the endpoint, or both. Examples of filters may include
// authentication, logging, distributed tracing, etc.
type Filter interface {
	// Apply is called by the FilterChain each time a request/response pair is passed
	// through the filter chain. The `next` parameter allows the Filter to pass on
	// the request and response to the next entity in the chain. The Filter may
	// choose not to invoke`next` and therefore block further processing, for example
	// if the authentication headers were not present in the request. Before returning,
	// the Filter can still do additional post-processing, for example by setting
	// headers on the response.
	Apply(ctx context.Context, w http.ResponseWriter, r *http.Request, next Handler)
}

// DefaultFilter is used by xhttp.Router if no custom filter is provided.
var DefaultFilter = FilterFunc(defaultTracedServer)

// The FilterFunc type is an adapter to allow the use of ordinary functions as Filters.
// If f is a function with the appropriate signature, FilterFunc(f) is a Filter object
// that calls f.
type FilterFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, next Handler)

// Apply implements Apply of Filter
func (f FilterFunc) Apply(ctx context.Context, w http.ResponseWriter, r *http.Request, next Handler) {
	f(ctx, w, r, next)
}
