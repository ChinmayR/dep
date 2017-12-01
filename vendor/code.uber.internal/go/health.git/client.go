package health

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/encoding/thrift"

	"code.uber.internal/go/health.git/internal/meta"
	"code.uber.internal/go/health.git/internal/meta/metaclient"
)

// Response is an encoding-agnostic representation of the the job control and
// RPC health state of a server.
type Response struct {
	JobHealth bool
	RPCHealth State
}

// A ThriftClient queries a Thrift-encoded health check procedure (over HTTP
// or TChannel).
type ThriftClient struct {
	client metaclient.Interface
}

// NewThriftClient constructs a Thrift client.
func NewThriftClient(cfg transport.ClientConfig, opts ...thrift.ClientOption) *ThriftClient {
	return &ThriftClient{metaclient.New(cfg, opts...)}
}

// Health queries the Meta::health procedure.
func (t *ThriftClient) Health(ctx context.Context) (*Response, error) {
	res, err := t.client.Health(ctx, &meta.HealthRequest{})
	if err != nil {
		return nil, err
	}
	if res.State == nil {
		// Our health procedures always return RPC health state.
		return nil, errors.New("no RPC health state reported")
	}
	return &Response{
		JobHealth: res.Ok,
		RPCHealth: State(*res.State),
	}, nil
}

// A PlainClient queries a plain-text Nagios/HAProxy-style HTTP health
// endpoint.
type PlainClient struct {
	url string
}

// NewPlainClient constructs a plain-text client for the specified URL.
func NewPlainClient(url string) *PlainClient {
	return &PlainClient{url}
}

// Health queries the URL specified at client construction.
func (n *PlainClient) Health(ctx context.Context) (*Response, error) {
	res, err := http.Get(n.url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}

	var returnErr error
	defer func() { returnErr = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP response has non-200 status code %d", res.StatusCode)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body failed: %v", err)
	}
	r := &Response{
		JobHealth: string(body) == "OK",
	}
	hc := res.Header.Get("Health-Status")
	if err := r.RPCHealth.UnmarshalText([]byte(hc)); err != nil {
		return nil, fmt.Errorf("unknown RPC health %q", hc)
	}
	return r, returnErr
}
