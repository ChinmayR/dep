package testhelper

import (
	"crypto/ecdsa"
	"encoding/json"
	"testing"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
)

// SetTempWonkaMasterPublicKey sets wonkamaster public key for the duration of a
// single test. It returns a function that should be deferred for cleanup:
//  defer setTempWonkaMasterPublicKey(eccKey)()
func SetTempWonkaMasterPublicKey(newKey *ecdsa.PublicKey) func() {
	oldWonkaMasterPublicKeys := wonka.WonkaMasterPublicKeys
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{newKey}
	return func() {
		wonka.WonkaMasterPublicKeys = oldWonkaMasterPublicKeys
	}
}

// SignClaim adds a valid signature to the given Wonka claim struct.
func SignClaim(t testing.TB, claim *wonka.Claim, ecPrivKey *ecdsa.PrivateKey) {
	claim.Signature = nil // allows easy resigning

	toHash, err := json.Marshal(claim)
	require.NoError(t, err, "error marshalling claim")

	sig, err := wonkacrypter.New().Sign(toHash, ecPrivKey)
	require.NoError(t, err, "error signing claim")

	claim.Signature = sig
}

// ClaimOption allows modification of the claim provided by for WithSignedClaim
type ClaimOption func(*wonka.Claim)

// Claims sets the claims field of a Wonka claim token.
func Claims(claims ...string) ClaimOption {
	return func(c *wonka.Claim) {
		c.Claims = claims
	}
}

// Destination sets the destination field of a Wonka claim token.
func Destination(d string) ClaimOption {
	return func(c *wonka.Claim) {
		c.Destination = d
	}
}

// EntityName sets the entity name of a Wonka claim token.
func EntityName(e string) ClaimOption {
	return func(c *wonka.Claim) {
		c.EntityName = e
	}
}

// WithSignedClaim calls provided function with a signed valid Wonka claim
// token. Token is signed after applying provided options. Provides both struct
// and marshalled string form.
func WithSignedClaim(t testing.TB, fn func(*wonka.Claim, string), opts ...ClaimOption) {
	privKey := wonkatestdata.ECCKey()
	defer SetTempWonkaMasterPublicKey(&privKey.PublicKey)()

	claim := &wonka.Claim{
		EntityName:  "TestAuthenticateIn",
		ValidAfter:  time.Now().Add(-10 * time.Minute).Unix(),
		ValidBefore: time.Now().Add(10 * time.Minute).Unix(),
		Claims:      []string{wonka.EveryEntity},
		Destination: "",
	}

	for _, opt := range opts {
		if opt != nil {
			opt(claim)
		}
	}

	SignClaim(t, claim, privKey)

	claimString, err := wonka.MarshalClaim(claim)
	require.NoError(t, err)

	fn(claim, claimString)
}
