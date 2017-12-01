package xhttp

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientFilters(t *testing.T) {
	headerKey := "X-Test-Header"
	jsonKey := "response"
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.Header.Get(headerKey)
		RespondWithJSON(w, map[string]string{jsonKey: value})
	})

	l := serve(t, server)
	defer l.Close()

	validate := func(client *Client, ctx context.Context, expected string) {
		if ctx == nil {
			ctx = context.Background()
		}
		var response map[string]string
		err := GetJSON(ctx, client, fmt.Sprintf("http://%s/", l.Addr().String()), &response, nil)
		require.Nil(t, err)
		assert.Equal(t, expected, response[jsonKey])
	}

	// test default client
	validate(nil, nil, "")

	// test client with filter that does nothing
	c2 := &Client{
		Filter: ClientFilterFunc(func(ctx context.Context, req *http.Request, next BasicClient) (resp *http.Response, err error) {
			return next.Do(ctx, req)
		}),
	}
	validate(c2, nil, "")

	// test client with filter that reads value from Context and saves to a header
	type ContextKey struct{}
	contextKey := ContextKey{}
	ctx := context.WithValue(context.Background(), contextKey, "hi there")
	c3 := &Client{
		Filter: ClientFilterFunc(func(ctx context.Context, req *http.Request, next BasicClient) (resp *http.Response, err error) {
			value := ctx.Value(contextKey).(string)
			req.Header.Set(headerKey, value)
			return next.Do(ctx, req)
		}),
	}
	validate(c3, ctx, "hi there")
}

func TestJSONClientErrors(t *testing.T) {
	err := GetJSON(context.Background(), nil, "^%$#@", nil, nil)
	assert.Error(t, err)

	err = roundTripJSONBody(context.Background(), nil, "", "", func() {}, nil, nil)
	assert.Error(t, err)

	err = roundTripJSONBody(context.Background(), nil, "", "^%$#@", 3.14, nil, nil)
	assert.Error(t, err)
}
