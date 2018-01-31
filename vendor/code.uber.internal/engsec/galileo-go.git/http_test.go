package galileo_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	galileo "code.uber.internal/engsec/galileo-go.git"
	"code.uber.internal/engsec/galileo-go.git/galileotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticateHTTPRequestWithDerelict(t *testing.T) {
	tests := []struct {
		desc        string
		header      http.Header
		expectedErr string
	}{
		{
			desc: "derelict rpc caller",
			header: http.Header{
				"Rpc-Caller": {"crufty-derelict-service"},
			},
		},
		{
			desc: "derelict uber source",
			header: http.Header{
				"X-Uber-Source": {"crufty-derelict-service"},
			},
		},
		{
			desc: "rpc caller overrides uber source",
			header: http.Header{
				"X-Uber-Source": {"eve"},
				"Rpc-Caller":    {"crufty-derelict-service"},
			},
		},
		{
			// Ensure "Rpc-Caller" header doesn't magically grant all access.
			desc: "allowed entity is denied without claim",
			header: http.Header{
				"Rpc-Caller": {"alice"},
			},
			expectedErr: "unauthenticated request: no wonka token in baggage",
		},
		{
			// Ensure "Rpc-Caller" header doesn't magically grant all access.
			desc: "non-derelict entity is denied without claim",
			header: http.Header{
				"Rpc-Caller": {"eve"},
			},
			expectedErr: "unauthenticated request: no wonka token in baggage",
		},
	}
	endpoint := "/foo"

	galileotest.WithServerGalileo(t, "system-under-test", func(g galileo.Galileo) {
		time.Sleep(1 * time.Second) // Wonka client loads derelict list asynchronously

		for _, tt := range tests {
			t.Run(tt.desc, func(t *testing.T) {
				req := &http.Request{
					Method: http.MethodPost,
					URL:    &url.URL{Path: endpoint},
					Header: tt.header,
				}

				// No claim in context.
				err := galileo.AuthenticateHTTPRequest(context.Background(), req, g)

				if tt.expectedErr == "" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedErr)
				}
			})
		}
	},
		galileotest.GlobalDerelictEntities("crufty-derelict-service"),
		galileotest.AllowedEntities("alice"),
		galileotest.EnrolledEntities("alice", "eve", "system-under-test"),
	)
}
