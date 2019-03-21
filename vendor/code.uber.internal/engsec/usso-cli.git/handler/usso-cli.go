package handler

import (
	"context"
	"fmt"

	ussocligen "code.uber.internal/engsec/usso-cli.git/.gen/go/engsec/usso-cli/usso_cli"
	"code.uber.internal/engsec/usso-cli.git/.gen/go/engsec/usso-cli/usso_cli/ussocliserver"
	zapfx "code.uber.internal/go/zapfx.git"
	"go.uber.org/zap"
)

// NewUssoCli creates the impl for the UssoCli service in usso_cli.thrift.
func NewUssoCli(logger *zap.SugaredLogger) ussocliserver.Interface {
	return &ussoCli{logger: logger}
}

type ussoCli struct {
	logger *zap.SugaredLogger
}

func (h *ussoCli) Hello(ctx context.Context, request *ussocligen.HelloRequest) (*ussocligen.HelloResponse, error) {
	message := fmt.Sprintf("Hello, %v!", request.GetName())
	h.logger.Infow("hello called", zapfx.Trace(ctx), "message", message)

	return &ussocligen.HelloResponse{Message: &message}, nil
}
