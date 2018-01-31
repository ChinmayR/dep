package galileo_test

import (
	"context"
	"testing"

	"code.uber.internal/engsec/galileo-go.git"
	"code.uber.internal/engsec/galileo-go.git/galileotest"
	"code.uber.internal/engsec/galileo-go.git/internal/testhelper"

	"code.uber.internal/engsec/wonka-go.git"
	"github.com/stretchr/testify/assert"
)

func TestGetCredential(t *testing.T) {
	name := "wonkaSample:TestGetCredential"
	destination := "wonkaSample:other-service"
	tracer, ctx, _ := testhelper.SetupContext()

	// g is just any old Galileo, nothing Server specific about it.
	galileotest.WithServerGalileo(t, name, func(g galileo.Galileo) {

		t.Run("without destination", func(t *testing.T) {
			token, err := galileo.GetCredential(ctx, g)

			assert.EqualError(t, err, "WithDestinationService option is required", "destination should be required")
			assert.Empty(t, token, "token should be empty on error")
		})

		t.Run("no explicit claims", func(t *testing.T) {
			token, err := galileo.GetCredential(ctx, g, galileo.WithDestinationService(destination))

			assert.NoError(t, err, "GetCredential should suceed")
			assert.NotEmpty(t, token, "token should not be empty")

			claim, err := wonka.UnmarshalClaim(token)
			assert.NoError(t, err, "failed to unmarshal claim")

			err = claim.Inspect([]string{destination}, []string{name})
			assert.NoError(t, err, "claim should be valid")
		})

	},
		galileotest.EnrolledEntities(name),
		galileotest.Tracer(tracer),
	)
}

func TestGetCredentialWhenDisabled(t *testing.T) {
	g := galileotest.NewDisabled(t, "here")
	token, err := galileo.GetCredential(context.Background(), g, galileo.WithDestinationService("there"))

	assert.NoError(t, err, "GetCredential should suceed")
	assert.Empty(t, token, "token should be empty on error")
}
