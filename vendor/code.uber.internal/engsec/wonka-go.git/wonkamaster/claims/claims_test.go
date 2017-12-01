package claims

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	. "code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
)

func TestNewClaim(t *testing.T) {
	c := NewClaimReq("test", wonka.EveryEntity)
	k := PrivateKey()
	eccKey := ECCKey()

	_, e := NewSignedClaim(c, eccKey, &k.PublicKey)
	require.NoError(t, e, "signing claim")
}

func TestClaimSigning(t *testing.T) {
	k := PrivateKey()

	// TODO(pmoody): move this to a wonka-go helper
	h := crypto.SHA256.New()
	h.Write([]byte(x509.MarshalPKCS1PrivateKey(k)))
	x, y := elliptic.P256().ScalarBaseMult(h.Sum(nil))

	c := NewClaimReq("test", wonka.EveryEntity)
	err := SignClaimRequest(&c, k)
	require.NoError(t, err, "signing shouldn't err: %v", err)

	entity := wonka.Entity{
		EntityName:   "test",
		PublicKey:    keys.RSAPemBytes(&k.PublicKey),
		ECCPublicKey: wonka.KeyToCompressed(x, y),
	}

	pubKey, _, err := VerifyClaimRequest(1, c, entity)
	require.NoError(t, err, "verify shouldn't err: %v", err)

	origPubKey, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	require.NoError(t, err, "marshal shouldn't fail: %v", err)
	returnedPubKey, err := x509.MarshalPKIXPublicKey(pubKey)
	require.NoError(t, err, "marshal shouldn't fail: %v", err)

	require.True(t, bytes.Equal(origPubKey, returnedPubKey),
		"keys should be equal:\n1. %s\n2. %s", keys.SHA256Hash(origPubKey),
		keys.SHA256Hash(returnedPubKey))
}

func TestVerifyClaimSigning(t *testing.T) {
	var reqVars = []struct {
		ver       int
		badSig    bool
		badKey    bool
		shouldErr bool
		msg       string
	}{
		{ver: 1, msg: "good v1 should succeed"},
		{ver: 2, msg: "good v2 should succeed"},
		{ver: 1, badSig: true, shouldErr: true, msg: "bad v1 should fail"},
		{ver: 2, badSig: true, shouldErr: true, msg: "bad v2 should fail"},
		{ver: 1, badKey: true, shouldErr: true, msg: "bad key for v1 should fail"},
		{ver: 2, badKey: true, shouldErr: true, msg: "bad key for v2 should fail"},
	}

	for _, m := range reqVars {
		k := PrivateKey()
		ecK := ecKeyFromRSA(k)

		entity := wonka.Entity{
			EntityName:   "e1",
			PublicKey:    keys.RSAPemBytes(&k.PublicKey),
			ECCPublicKey: wonka.KeyToCompressed(ecK.PublicKey.X, ecK.PublicKey.Y),
		}

		rsaPub, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
		require.NoError(t, err, "%v", err)
		ecPub, err := x509.MarshalPKIXPublicKey(&ecK.PublicKey)
		require.NoError(t, err, "%v", err)

		cr := wonka.ClaimRequest{
			Version:     wonka.SignEverythingVersion,
			EntityName:  "e1",
			Destination: "e1",
			Claim:       wonka.EveryEntity,
			Ctime:       0,
			Etime:       0,
			SigType:     "SHA256",
		}

		toSign, err := json.Marshal(cr)
		require.NoError(t, err, "json marshal err: %v", err)
		cr.Signature = "foober"

		if !m.badSig {
			switch m.ver {
			case 1:
				h := crypto.SHA256.New()
				h.Write([]byte(toSign))
				s, err := k.Sign(rand.Reader, h.Sum(nil), crypto.SHA256)
				require.NoError(t, err, "%v", err)
				cr.Signature = base64.StdEncoding.EncodeToString(s)
			default:
				s, err := wonkacrypter.New().Sign([]byte(toSign), ecK)
				require.NoError(t, err, "signing failure: %v", err)
				cr.Signature = base64.StdEncoding.EncodeToString(s)
			}
		}

		if m.badKey {
			entity.PublicKey = "foober"
			entity.ECCPublicKey = "foober"
		}

		pubKey, _, err := VerifyClaimRequest(m.ver, cr, entity)
		if m.shouldErr {
			require.Error(t, err, "%s", m.msg)
		} else {
			require.NoError(t, err, "%s: %v", m.msg, err)

			keyBytes, err := x509.MarshalPKIXPublicKey(pubKey)
			require.NoError(t, err, "%v", err)

			switch m.ver {
			case 1:
				require.Equal(t, keyBytes, rsaPub, "%v", err)
			case 2:
				require.Equal(t, keyBytes, ecPub, "%v", err)
			}

		}
	}
}

func ecKeyFromRSA(k *rsa.PrivateKey) *ecdsa.PrivateKey {
	h := crypto.SHA256.New()
	h.Write([]byte(x509.MarshalPKCS1PrivateKey(k)))
	pointKey := h.Sum(nil)

	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = elliptic.P256()
	priv.D = new(big.Int).SetBytes(pointKey)
	priv.PublicKey.X, priv.PublicKey.Y = elliptic.P256().ScalarBaseMult(pointKey)
	return priv
}
