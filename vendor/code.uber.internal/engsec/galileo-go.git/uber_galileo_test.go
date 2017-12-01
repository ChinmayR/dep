package galileo

import (
	"crypto/ecdsa"
	"encoding/json"
	"sync"
	"testing"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSkipDestination(t *testing.T) {
	u := &uberGalileo{
		skippedEntities: make(map[string]skippedEntity, 1),
		skipLock:        &sync.RWMutex{},
		log:             zap.L(),
	}

	require.Equal(t, len(u.skippedEntities), 0, "entities should be empty")

	u.addSkipDest("foober")
	require.Equal(t, len(u.skippedEntities), 1, "entities should have one entity")
	ok := u.shouldSkipDest("foober")
	require.True(t, ok, "should skip")

	e, ok := u.skippedEntities["foober"]
	require.True(t, ok, "foober should be skipped")
	require.Equal(t, initialSkipDuration, e.until, "default skip time")

	e.start = time.Now().Add(-time.Hour)
	u.skippedEntities["foober"] = e
	ok = u.shouldSkipDest("foober")
	require.False(t, ok, "should not skip")

	u.addSkipDest("foober")
	require.Equal(t, len(u.skippedEntities), 1, "entities should have one entity")
	ok = u.shouldSkipDest("foober")
	require.True(t, ok, "should skip")

	e, ok = u.skippedEntities["foober"]
	require.True(t, ok, "should skip")
	require.Equal(t, initialSkipDuration, e.until, "reset skip time")

	u.addSkipDest("foober")
	e, ok = u.skippedEntities["foober"]
	require.True(t, ok, "should skip")
	require.Equal(t, 2*initialSkipDuration, e.until, "2x default skip time")

	e.until = 2 * maxSkipDuration
	u.skippedEntities["foober"] = e

	u.addSkipDest("foober")
	e, ok = u.skippedEntities["foober"]
	require.True(t, ok, "should skip")
	require.Equal(t, maxSkipDuration, e.until, "max skip time")
}

func TestClaimUsableUnsignedClaim(t *testing.T) {
	now := time.Now()
	claim := &wonka.Claim{
		EntityName:  "TestClaimUsable",
		ValidAfter:  now.Add(-10 * time.Minute).Unix(),
		ValidBefore: now.Add(10 * time.Minute).Unix(),
		Claims:      []string{wonka.EveryEntity},
		Destination: wonka.NullEntity,
	}

	err := claimUsable(claim, "")
	assert.Error(t, err, "unsigned claim token should not be usable")
	assert.Contains(t, err.Error(), "invalid signature", "not the error we expected")
}

func TestClaimUsable(t *testing.T) {
	now := time.Now()

	var testVars = []struct {
		descr    string             // describes the test case
		errMsg   string             // expected error message. Leave empty if you expect no error.
		claimReq string             // explicity required claim
		before   func(*wonka.Claim) // Modify the claim before the test
	}{
		{descr: "allow any claims"},
		{descr: "explicit required claim", claimReq: "probable-claim", before: func(c *wonka.Claim) { c.Claims = append(c.Claims, "probable-claim") }},
		{descr: "require ungranted claim", errMsg: "claim token does not grant \"wildly-improbable-claim\"", claimReq: "wildly-improbable-claim"},
		{descr: "already expired", errMsg: "claim token expired", before: func(c *wonka.Claim) { c.ValidBefore = now.Add(-5 * time.Minute).Unix() }},
		{descr: "expiring soon", errMsg: "claim token will expire soon", before: func(c *wonka.Claim) { c.ValidBefore = now.Add(2 * time.Minute).Unix() }},
	}

	for _, m := range testVars {
		t.Run(m.descr, func(t *testing.T) {
			eccKey := wonkatestdata.ECCKey()
			defer setTempWonkaMasterPublicKey(&eccKey.PublicKey)()

			claim := &wonka.Claim{
				EntityName:  "TestClaimUsable",
				ValidAfter:  now.Add(-10 * time.Minute).Unix(),
				ValidBefore: now.Add(10 * time.Minute).Unix(),
				Claims:      []string{wonka.EveryEntity},
				Destination: wonka.NullEntity,
			}

			if m.before != nil {
				m.before(claim) // potentially modify claim for this test case
			}
			signClaim(t, claim, eccKey)

			err := claimUsable(claim, m.claimReq)
			if m.errMsg == "" {
				require.NoError(t, err, "should be usable")
			} else {
				require.Error(t, err, "should not be usable")
				assert.Contains(t, err.Error(), m.errMsg, "not the error we expected")
			}
		})
	}
}

// this sets a temp wonkamaster public key for the duration of a single test
// it returns a function that should be deferred for cleanup, e.g.:
//  defer setTempWonkaMasterPublicKey(eccKey)()
func setTempWonkaMasterPublicKey(newKey *ecdsa.PublicKey) func() {
	oldWonkaMasterPublicKeys := wonka.WonkaMasterPublicKeys
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{newKey}
	return func() {
		wonka.WonkaMasterPublicKeys = oldWonkaMasterPublicKeys
	}
}

func signClaim(t *testing.T, claim *wonka.Claim, ecPrivKey *ecdsa.PrivateKey) {
	claim.Signature = nil // allows easy resigning

	toHash, err := json.Marshal(claim)
	require.NoError(t, err, "error marshalling claim")

	sig, err := wonkacrypter.New().Sign(toHash, ecPrivKey)
	require.NoError(t, err, "error signing claim")

	claim.Signature = sig
}
