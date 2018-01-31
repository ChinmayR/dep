package galileo

import (
	"net/http"
	"net/url"
	"testing"

	"code.uber.internal/engsec/galileo-go.git/internal/atomic"
	"code.uber.internal/engsec/galileo-go.git/internal/claimtools"
	"code.uber.internal/engsec/galileo-go.git/internal/contexthelper"
	"code.uber.internal/engsec/galileo-go.git/internal/testhelper"

	"code.uber.internal/engsec/wonka-go.git"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

func TestAuthenticateHTTPRequest(t *testing.T) {
	tests := []struct {
		cfg        EndpointCfg
		httpMethod string
		shouldErr  bool
		name       string
	}{
		{
			name:       "get_success",
			cfg:        EndpointCfg{AllowRead: []string{"bar"}},
			httpMethod: http.MethodGet,
		},
		{
			name:       "head_success",
			cfg:        EndpointCfg{AllowRead: []string{"bar"}},
			httpMethod: http.MethodHead,
		},
		{
			name:       "post_success",
			cfg:        EndpointCfg{AllowWrite: []string{"bar"}},
			httpMethod: http.MethodPost,
		},
		{
			name:       "put_success",
			cfg:        EndpointCfg{AllowWrite: []string{"bar"}},
			httpMethod: http.MethodPut,
		},
		{
			name:       "delete_success",
			cfg:        EndpointCfg{AllowWrite: []string{"bar"}},
			httpMethod: http.MethodDelete,
		},
		{
			name: "get_with_wrong_entity",
			cfg: EndpointCfg{
				AllowRead:  []string{"baz"},
				AllowWrite: []string{"baz"},
			},
			httpMethod: http.MethodGet,
			shouldErr:  true,
		},
		{
			name: "post_with_wrong_entity",
			cfg: EndpointCfg{
				AllowRead:  []string{"baz"},
				AllowWrite: []string{"baz"},
			},
			httpMethod: http.MethodPost,
			shouldErr:  true,
		},
	}
	endpoint := "/foo"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(_ *wonka.Claim, claimString string) {
				tracer, ctx, span := testhelper.SetupContext()

				u := &uberGalileo{
					serviceAliases:    []string{"system-under-test"},
					metrics:           tally.NoopScope,
					log:               zap.NewNop(),
					enforcePercentage: atomic.NewFloat64(1),
					tracer:            tracer,
					endpointCfg: map[string]EndpointCfg{
						endpoint: tt.cfg,
					},
					inboundClaimCache: claimtools.DisabledCache,
				}

				contexthelper.SetBaggage(span, claimString)

				req := &http.Request{
					Method: tt.httpMethod,
					URL:    &url.URL{Path: endpoint},
				}
				err := AuthenticateHTTPRequest(ctx, req, u)
				if tt.shouldErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}

			}, testhelper.Claims("bar"), testhelper.Destination("system-under-test"))
		})
	}
}
