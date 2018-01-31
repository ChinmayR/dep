package claimhelper

import (
	"testing"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/require"
)

func TestNewClaim(t *testing.T) {
	c := wonkatestdata.NewClaimReq("test", wonka.EveryEntity)
	k := wonkatestdata.PrivateKey()
	eccKey := wonkatestdata.ECCKey()

	claim, e := NewSignedClaim(c, eccKey)
	require.NoError(t, e, "signing claim")

	_, e = EncryptClaim(claim, eccKey, &k.PublicKey)
	require.NoError(t, e)
}

func TestNewClaimWhenSigningFailsShouldError(t *testing.T) {
	c := wonkatestdata.NewClaimReq("test", wonka.EveryEntity)

	_, e := NewSignedClaim(c, nil)
	require.Error(t, e, "should error signing claim")
}

func TestNewClaimWithEccEncryption(t *testing.T) {
	c := wonkatestdata.NewClaimReq("test", wonka.EveryEntity)
	eccKey := wonkatestdata.ECCKey()

	claim, e := NewSignedClaim(c, eccKey)
	require.NoError(t, e, "signing claim")

	_, e = EncryptClaim(claim, eccKey, &eccKey.PublicKey)
	require.NoError(t, e)
}

func TestEncryptClaimWithNilPrivateKeyShouldFail(t *testing.T) {
	c := wonkatestdata.NewClaimReq("test", wonka.EveryEntity)
	k := wonkatestdata.PrivateKey()
	eccKey := wonkatestdata.ECCKey()

	claim, e := NewSignedClaim(c, eccKey)
	require.NoError(t, e, "signing claim")

	_, e = EncryptClaim(claim, nil, &k.PublicKey)
	require.Error(t, e, "should error when using nil private key")
}

func TestEncryptClaimWithNilPublicKeyShouldFail(t *testing.T) {
	c := wonkatestdata.NewClaimReq("test", wonka.EveryEntity)
	eccKey := wonkatestdata.ECCKey()

	claim, e := NewSignedClaim(c, eccKey)
	require.NoError(t, e, "signing claim")

	_, e = EncryptClaim(claim, eccKey, nil)
	require.Error(t, e, "should error when using nil public key")
}

func TestClaimValidateInvalidClaimShouldError(t *testing.T) {
	err := ClaimValidate("invalid")
	require.Error(t, err, "should error when claim is invalid string")
}

func TestClaimCheckInvalidClaimShouldError(t *testing.T) {
	err := ClaimCheck([]string{"lol"}, "lol", "invalid")
	require.Error(t, err, "should error when claim is invalid string")
}
