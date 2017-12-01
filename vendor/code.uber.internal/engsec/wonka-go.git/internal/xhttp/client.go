package xhttp

import (
	"context"
	"net/http"
)

// Client is a context-aware extension of http.Client with support for pluggable filters.
// It is now required by many components in go-common.
//
// Upgrade path:
//
// in most cases you can use xhttp.DefaultClient singleton. However,
// if your code was previously instantiating http.Client, e.g.
//
//    client := &http.Client{Transport: transport}
//
// you can replace it with xhttp.Client constructed as follows:
//
//    client := &xhttp.Client{Client: http.Client{Transport: transport}}
//
type Client struct {
	// Client is a reference to the real, context-unaware http.Client
	http.Client

	// Filter is a ClientFilter or a filter chain to be applied before each request is
	// executed via http.Client.
	//
	// If nil, xhttp.DefaultClientFilter will be used
	Filter ClientFilter
}

// DefaultClient is the default context-aware client that uses http.Client{}
// and xhttp.DefaultFilter
var DefaultClient = &Client{}

// DefaultClientFilter is used by xhttp.Client if no other filters are defined.
var DefaultClientFilter = ClientFilterFunc(defaultTracedClient)

// Do is a context-aware, filter-enabled extension of Do() in http.Client
func (c *Client) Do(ctx context.Context, req *http.Request) (resp *http.Response, err error) {
	filter := c.Filter
	if filter == nil {
		filter = DefaultClientFilter
	}
	return filter.Apply(ctx, req, BasicClientFunc(c.do))
}

func (c *Client) do(ctx context.Context, req *http.Request) (resp *http.Response, err error) {
	return c.Client.Do(req.WithContext(ctx))
}

// BasicClient is the simplest, context-aware HTTP client with a single method Do.
type BasicClient interface {
	// Do sends an HTTP request and returns an HTTP response, following
	// policy (e.g. redirects, cookies, auth) as configured on the client.
	Do(ctx context.Context, req *http.Request) (resp *http.Response, err error)
}

// The BasicClientFunc type is an adapter to allow the use of ordinary functions as
// BasicClient. If f is a function with the appropriate signature,
// BasicClientFunc(f) is a BasicClient object that calls f.
type BasicClientFunc func(ctx context.Context, req *http.Request) (resp *http.Response, err error)

// Do implements Do of BasicClient
func (f BasicClientFunc) Do(ctx context.Context, req *http.Request) (resp *http.Response, err error) {
	return f(ctx, req)
}
