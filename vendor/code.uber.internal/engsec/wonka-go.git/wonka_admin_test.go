package wonka_test

import (
	"context"
	"testing"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/rpc"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func TestAdminDelete(t *testing.T) {
	tests := []struct {
		userGroup   string
		errString   string
		description string
	}{
		{
			userGroup:   "AD:wonka-admins",
			description: "success",
		},
		{
			userGroup:   "AD:engineering",
			errString:   wonka.AdminAccessDenied,
			description: "bad admin group",
		},
	}

	ctx := context.Background()
	adminName := "fuser@uber.com"
	entityName := "foo"
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			wonkatestdata.WithWonkaMaster(adminName, func(r common.Router, handlerCfg common.HandlerConfig) {
				wonkatestdata.WithUSSHAgent(adminName, func(agentPath string, caKey ssh.PublicKey) {

					mem := map[string][]string{adminName: {tt.userGroup}}
					handlerCfg.Pullo = rpc.NewMockPulloClient(mem,
						rpc.Logger(handlerCfg.Logger, zap.NewAtomicLevel()))
					handlerCfg.Ussh = []ssh.PublicKey{caKey}
					err := handlerCfg.DB.Create(ctx, &wonka.Entity{
						EntityName: entityName,
					})
					require.NoError(t, err)

					handlers.SetupHandlers(r, handlerCfg)

					cfg := wonka.Config{EntityName: adminName}

					w, err := wonka.Init(cfg)
					require.NoError(t, err, "init %v", err)

					req := wonka.AdminRequest{
						EntityName: adminName,
						Action:     wonka.DeleteEntity,
						ActionOn:   entityName,
					}

					err = w.Admin(ctx, req)
					if tt.errString != "" {
						require.Contains(t, err.Error(), tt.errString, "unexpected error: %v", err)
					} else {
						require.NoError(t, err, "should not error: %v", err)
						_, err := handlerCfg.DB.Get(ctx, entityName)
						require.Error(t, wonkadb.ErrNotFound, err)
					}
				})
			})
		})
	}
}
