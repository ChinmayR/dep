package health

import (
	"context"

	"code.uber.internal/go/health.git/internal/meta"
	"code.uber.internal/go/health.git/internal/meta/metaserver"
	"go.uber.org/yarpc/api/transport"
)

// NewThrift returns an implementation of the Thrift Meta service, which
// serves health checks.
//
// Like the HTTP-only handler, it supports both simple job control checks and
// the more sophisticated checks used by Uber's Health Controller system. It
// works over any Unary YARPC transport, including both HTTP/1.1 and TChannel.
func NewThrift(hc *Coordinator) []transport.Procedure {
	return metaserver.New(&metaHandler{hc})
}

type metaHandler struct {
	hc *Coordinator
}

func (m *metaHandler) Health(ctx context.Context, hr *meta.HealthRequest) (*meta.HealthStatus, error) {
	current := m.hc.State()
	metaState := meta.State(current)
	msg := current.String()
	res := &meta.HealthStatus{
		Ok:      true,
		Message: &msg,
		State:   &metaState,
	}

	check := meta.RequestTypeLegacy
	if hr != nil && hr.Type != nil {
		check = *hr.Type
	}

	if check == meta.RequestTypeLegacy {
		return res, nil
	}

	res.Ok = current == AcceptingTraffic
	return res, nil
}
