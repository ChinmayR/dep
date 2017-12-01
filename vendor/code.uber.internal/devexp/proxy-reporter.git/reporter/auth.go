package reporter

import (
	"context"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
)

// Claimer is a simplification of wonka's interface surface.
type Claimer interface {
	ClaimResolveTTL(ctx context.Context, entityName string, ttl time.Duration) (*wonka.Claim, error)
}

// wk lets us to use default wonka instance
// if it wasn't provided and make code testable.
type wk struct {
	Claimer
}

var _ Claimer = wk{nil}

func (w wk) ClaimResolveTTL(ctx context.Context, entityName string, ttl time.Duration) (*wonka.Claim, error) {
	if w.Claimer == nil {
		var err error
		w.Claimer, err = wonka.Init(wonka.Config{
			EntityName:     entityName,
			WonkaMasterURL: "https://wonkabar.uberinternal.com",
		})

		if err != nil {
			return nil, err
		}
	}

	return w.Claimer.ClaimResolveTTL(ctx, entityName, ttl)
}
