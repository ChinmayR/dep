package galileo_test

import (
	"testing"

	"code.uber.internal/engsec/galileo-go.git"
	"code.uber.internal/engsec/galileo-go.git/galileotest"
	"code.uber.internal/engsec/galileo-go.git/internal/testhelper"

	"code.uber.internal/engsec/wonka-go.git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCredential(t *testing.T) {
	name := "wonkaSample:TestValidateCredential"
	//destination := "wonkaSample:other-service"

	// g is just any old Galileo, nothing Server specific about it.
	galileotest.WithServerGalileo(t, name, func(g galileo.Galileo) {

		t.Run("malformed token", func(t *testing.T) {
			err := galileo.ValidateCredential(g, "totally-bonkers")
			require.Error(t, err, "token should not be valid")
			assert.Contains(t, err.Error(), "unmarshalling claim token", "unexpected error")
		})

		t.Run("valid token", func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(_ *wonka.Claim, token string) {

				err := galileo.ValidateCredential(g, token)
				assert.NoError(t, err, "token should be valid")
			},
				testhelper.Destination("wonkaSample:TestValidateCredential"),
				testhelper.Claims("wonkaSample:TestValidateCredential"),
			)
		})

		t.Run("mismatched destination", func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(_ *wonka.Claim, token string) {

				err := galileo.ValidateCredential(g, token)
				require.Error(t, err, "token should not be valid")
				assert.EqualError(t, err,
					`not permitted by configuration: claim token destination "wonkaSample:Not-TestValidateCredential" is not among allowed destinations ["wonkaSample:TestValidateCredential"]`,
					"unexpected error")

			}, testhelper.Destination("wonkaSample:Not-TestValidateCredential"))
		})

		t.Run("no matching claims", func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(_ *wonka.Claim, token string) {

				err := galileo.ValidateCredential(g, token, galileo.AllowedEntities("larry", "moe", "curly"))
				require.Error(t, err, "token should not be valid")
				assert.Contains(t, err.Error(),
					"not permitted by configuration: no common claims",
					"unexpected error")

			},
				testhelper.Claims("tastes-great", "less-filling"),
				testhelper.Destination("wonkaSample:TestValidateCredential"),
			)
		})

		t.Run("everyone allows identity claim", func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(_ *wonka.Claim, token string) {

				// When allowed entities is EVERYONE...
				err := galileo.ValidateCredential(g, token, galileo.AllowedEntities(wonka.EveryEntity))
				assert.NoError(t, err, "token should be valid")
			},
				testhelper.EntityName("a-named-entity"),
				testhelper.Claims("a-named-entity"), // ...any identity claim should be allowed.
				testhelper.Destination("wonkaSample:TestValidateCredential"),
			)
		})

		t.Run("everyone does not allow arbitrary claim", func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(_ *wonka.Claim, token string) {
				// Even when allowed entities is EVERYONE...
				err := galileo.ValidateCredential(g, token, galileo.AllowedEntities(wonka.EveryEntity))
				require.Error(t, err, "token should not be valid")
				assert.Contains(t, err.Error(),
					"not permitted by configuration: no common claims",
					"unexpected error")

			},
				testhelper.Claims("anything"), // ...an arbitrary claim is still denied.
				testhelper.Destination("wonkaSample:TestValidateCredential"),
			)
		})

		t.Run("CallerName", func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(_ *wonka.Claim, token string) {

				t.Run("mis-matched", func(t *testing.T) {
					err := galileo.ValidateCredential(g, token,
						galileo.AllowedEntities(wonka.EveryEntity),
						galileo.CallerName("alice"), // Eve is pretending to be alice
					)

					assert.NoError(t, err, "token should be valid") // We're not currently enforcing caller name matching.
					// require.Error(t, err, "token should not be valid")
					// assert.Contains(t, err.Error(),
					//	`remote entity name mismatch. caller_name="alice"; remote_entity="eve"`,
					//	"unexpected error")

				})

				t.Run("matching", func(t *testing.T) {
					err := galileo.ValidateCredential(g, token,
						galileo.AllowedEntities(wonka.EveryEntity),
						galileo.CallerName("eve"), // Eve is being honest about her identity.
					)

					assert.NoError(t, err, "token should be valid")
				})

			},
				testhelper.EntityName("eve"), // Token affirms caller identity is eve.
				testhelper.Destination("wonkaSample:TestValidateCredential"),
			)
		})

		t.Run("globally derelict", func(t *testing.T) {
			err := galileo.ValidateCredential(g, "", galileo.CallerName("crufty"))
			assert.NoError(t, err, "empty token should be valid for globally configured derelict entity")
		})

	},
		galileotest.GlobalDerelictEntities("crufty"),
		galileotest.EnrolledEntities(name),
	)
}

func TestValidateCredentialWhenDisabled(t *testing.T) {
	g := galileotest.NewDisabled(t, "here")
	err := galileo.ValidateCredential(g, "complete-junk")

	assert.NoError(t, err, "token should be valid")
}
