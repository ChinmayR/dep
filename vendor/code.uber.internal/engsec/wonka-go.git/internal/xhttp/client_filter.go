package xhttp

import (
	"context"
	"net/http"
)

// ClientFilter is similar to xhttp.Filter, but is applied to outbound/client calls.
// It can alter or replace the context, the outgoing request, or the resulting response.
// It can also abort execution by not calling the `next` function.
//
// Examples of filters may include authentication, logging, distributed tracing, etc.
type ClientFilter interface {
	Apply(ctx context.Context, req *http.Request, next BasicClient) (*http.Response, error)
}

// The ClientFilterFunc type is an adapter to allow the use of ordinary functions as
// ClientFilters. If f is a function with the appropriate signature,
// ClientFilterFunc(f) is a ClientFilter object that calls f.
type ClientFilterFunc func(
	ctx context.Context,
	req *http.Request,
	next BasicClient,
) (*http.Response, error)

// Apply implements Apply of ClientFilter
func (f ClientFilterFunc) Apply(
	ctx context.Context,
	req *http.Request,
	next BasicClient,
) (resp *http.Response, err error) {
	return f(ctx, req, next)
}
