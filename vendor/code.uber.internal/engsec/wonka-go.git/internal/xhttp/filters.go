package xhttp

import (
	"context"
	"net/http"
	"sync"
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
var DefaultFilter = NewFilterChainBuilder().
	AddFilter(FilterFunc(defaultTracedServer)).
	Build()

// The FilterFunc type is an adapter to allow the use of ordinary functions as Filters.
// If f is a function with the appropriate signature, FilterFunc(f) is a Filter object
// that calls f.
type FilterFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, next Handler)

// Apply implements Apply of Filter
func (f FilterFunc) Apply(ctx context.Context, w http.ResponseWriter, r *http.Request, next Handler) {
	f(ctx, w, r, next)
}

// FilterChainBuilder builds a "filter chain" that chains multiple filters together
// into a single filter.  Calling the Apply method on the "filter chain" executes
// the underlying filters in order. The last filter in the chain calls the actual `handler`,
// unless one of the steps in the chain decides to abort execution by not calling `next`.
type FilterChainBuilder interface {
	// AddFilter is used to add the next filter to the chain during construction time.
	// The calls to AddFilter can be chained.
	AddFilter(filter Filter) FilterChainBuilder

	// Build creates an immutable FilterChain.
	Build() Filter
}

// NewFilterChainBuilder creates a mutable builder for filter chain
func NewFilterChainBuilder() FilterChainBuilder {
	return &filterChainBuilder{}
}

type filterChainBuilder struct {
	filters []Filter
}

// AddFilter implements AddFilter() of FilterChainBuilder
func (b *filterChainBuilder) AddFilter(filter Filter) FilterChainBuilder {
	b.filters = append(b.filters, filter)
	return b
}

// Build implements Build() of FilterChainBuilder
func (b *filterChainBuilder) Build() Filter {
	return &filterChain{
		filters: b.filters,
		pool: sync.Pool{
			New: func() interface{} {
				return &chainExecution{}
			},
		},
	}
}

type filterChain struct {
	filters []Filter
	pool    sync.Pool
}

type chainExecution struct {
	filters       []Filter
	currentFilter int
	finalHandler  Handler
}

// Apply implements Apply() of FilterChain
func (chain *filterChain) Apply(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	handler Handler,
) {
	if len(chain.filters) == 0 {
		handler.ServeHTTP(ctx, w, r)
		return
	}
	exec := chain.pool.Get().(*chainExecution)
	exec.filters = chain.filters
	exec.currentFilter = 0
	exec.finalHandler = handler
	exec.ServeHTTP(ctx, w, r)
	chain.pool.Put(exec)
}

func (exec *chainExecution) ServeHTTP(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
) {
	if exec.currentFilter == len(exec.filters) {
		exec.finalHandler.ServeHTTP(ctx, w, r)
	} else {
		filter := exec.filters[exec.currentFilter]
		exec.currentFilter++
		filter.Apply(ctx, w, r, exec)
	}
}
