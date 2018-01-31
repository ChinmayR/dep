package xhttp

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilters(t *testing.T) {
	type Key int
	var contextProperty Key = 1

	var filters Filter

	l := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		// apply filters first
		filters.Apply(ctx, w, r, HandlerFunc(
			func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
				// read property value from the context and return in JSON
				property, _ := ctx.Value(contextProperty).(string)
				RespondWithJSON(w, map[string]string{"property": property})
			}))
	}))
	defer l.Close()

	validate := func(t *testing.T, expected string) {
		var response map[string]string
		err := GetJSON(context.Background(), DefaultClient, fmt.Sprintf("http://%s/filter", l.Addr().String()), &response, nil)
		require.Nil(t, err)
		assert.Equal(t, expected, response["property"])
	}

	tests := []struct {
		filter   Filter
		expected string
	}{
		{
			// no filter
			expected: "",
		},
		{
			// test with a single filter
			filter: FilterFunc(
				func(ctx context.Context, w http.ResponseWriter, r *http.Request, next Handler) {
					newCtx := context.WithValue(ctx, contextProperty, "passed-through")
					next.ServeHTTP(newCtx, w, r)
				}),
			expected: "passed-through",
		},
		{
			// test with two filters in the chain, both setting the same context property
			filter: FilterFunc(
				func(ctx context.Context, w http.ResponseWriter, r *http.Request, next Handler) {
					newCtx := context.WithValue(ctx, contextProperty, "override")
					next.ServeHTTP(newCtx, w, r)
				}),
			expected: "override",
		},
		{
			// test with 3rd filter that aborts filter chain execution and responds itself
			filter: FilterFunc(
				func(ctx context.Context, w http.ResponseWriter, r *http.Request, next Handler) {
					RespondWithJSON(w, map[string]string{"property": "aborted"})
					// do not call next
				}),
			expected: "aborted",
		},
	}

	builder := NewFilterChainBuilder()
	// incrementally add more filters to the chain
	for _, test := range tests {
		if test.filter != nil {
			builder.AddFilter(test.filter)
		}
		filters = builder.Build()
		validate(t, test.expected)
	}
}
