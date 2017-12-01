package galileofx

import (
	"context"
	"fmt"

	galileo "code.uber.internal/engsec/galileo-go.git"
)

type galileoNoop struct {
	name string
}

var _ galileo.Galileo = galileoNoop{}

func (g galileoNoop) Name() string {
	return g.name
}

func (galileoNoop) Endpoint(endpoint string) (galileo.EndpointCfg, error) {
	return galileo.EndpointCfg{}, fmt.Errorf("no configuration found for endpoint %q", endpoint)
}

func (galileoNoop) AuthenticateOut(ctx context.Context, destination string, explicitClaim ...interface{}) (context.Context, error) {
	return ctx, nil
}

func (galileoNoop) AuthenticateIn(ctx context.Context, allowedEntities ...interface{}) error {
	return nil
}
