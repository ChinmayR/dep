package handlers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAuthenticateCertificate(t *testing.T) {
	t.Run("unmarshal_error", func(t *testing.T) {
		cert, err := authenticateCertificate(nil, "foo", nil, zap.L())
		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "error unmarshalling cert reply"))
		assert.Nil(t, cert)
	})
	t.Run("entity_mismatch", func(t *testing.T) {
		c := wonka.Certificate{EntityName: "foo"}
		b, err := wonka.MarshalCertificate(c)
		require.NoError(t, err)

		cert, err := authenticateCertificate(b, "bar", nil, zap.L())
		require.Error(t, err)
		assert.EqualError(t, err, `certificate entity "foo" does not match requesting entity "bar"`)
		assert.Nil(t, cert)
	})
	t.Run("unsigned", func(t *testing.T) {
		c := wonka.Certificate{
			EntityName:  "foo",
			ValidAfter:  uint64(time.Now().Unix()),
			ValidBefore: uint64(time.Now().Add(time.Hour).Unix()),
		}
		b, err := wonka.MarshalCertificate(c)
		require.NoError(t, err, "failed to marshal test cert")

		_, err = authenticateCertificate(b, "foo", nil, zap.L())
		require.Error(t, err, "expected unsigned certificate to fail")
	})
	t.Run("valid_no_override", func(t *testing.T) {
		c := wonka.Certificate{
			EntityName:  "foo",
			ValidAfter:  uint64(time.Now().Unix()),
			ValidBefore: uint64(time.Now().Add(time.Hour).Unix()),
		}

		b, restore := signedCertificate(t, &c)
		defer restore()

		cert, err := authenticateCertificate(b, "foo", nil, zap.L())
		require.NoError(t, err)
		require.NotNil(t, cert)
	})
	t.Run("valid_with_override", func(t *testing.T) {
		now := time.Now().AddDate(-1, 0, 0)
		c := wonka.Certificate{
			EntityName:  "foo",
			ValidAfter:  uint64(now.Unix()),
			ValidBefore: uint64(now.Add(time.Hour).Unix()),
		}
		b, restore := signedCertificate(t, &c)
		defer restore()

		o := &common.CertAuthOverride{
			Grant: common.AuthGrantOverride{
				SignedAfter:  now.Add(-time.Second),
				SignedBefore: now.Add(time.Second),
				EnforceUntil: time.Now().Add(time.Hour),
			},
		}

		cert, err := authenticateCertificate(b, "foo", o, zap.L())
		require.NoError(t, err)
		require.NotNil(t, cert)
	})
	t.Run("not_yet_valid", func(t *testing.T) {
		c := wonka.Certificate{
			EntityName: "foo",
			ValidAfter: uint64(time.Now().Add(time.Hour).Unix()),
		}
		b, restore := signedCertificate(t, &c)
		defer restore()
		cert, err := authenticateCertificate(b, "foo", nil, zap.L())
		require.EqualError(t, err, "certificate is not yet valid")
		require.Nil(t, cert)
	})
	t.Run("expired", func(t *testing.T) {
		now := time.Now()
		c := wonka.Certificate{
			EntityName:  "foo",
			ValidAfter:  uint64(now.Unix()),
			ValidBefore: uint64(now.Add(-time.Hour).Unix()),
		}
		b, restore := signedCertificate(t, &c)
		defer restore()
		cert, err := authenticateCertificate(b, "foo", nil, zap.L())
		require.EqualError(t, err, "certificate has expired")
		require.Nil(t, cert)
	})
	t.Run("overide_enfore_until_expired", func(t *testing.T) {
		now := time.Now()
		c := wonka.Certificate{
			EntityName:  "foo",
			ValidAfter:  uint64(now.Unix()),
			ValidBefore: uint64(now.Add(-time.Hour).Unix()),
		}
		b, restore := signedCertificate(t, &c)
		defer restore()
		o := &common.CertAuthOverride{
			Grant: common.AuthGrantOverride{
				SignedAfter:  now.Add(-time.Second),
				SignedBefore: now.Add(time.Second),
				EnforceUntil: time.Now().Add(-time.Hour),
			},
		}
		cert, err := authenticateCertificate(b, "foo", o, zap.L())
		require.EqualError(t, err, "certificate has expired")
		require.Nil(t, cert)
	})
	t.Run("override_cert_signature_out_of_range", func(t *testing.T) {
		now := time.Now()
		c := wonka.Certificate{
			EntityName:  "foo",
			ValidAfter:  uint64(now.Unix()),
			ValidBefore: uint64(now.Add(-time.Hour).Unix()),
		}
		b, restore := signedCertificate(t, &c)
		defer restore()
		o := &common.CertAuthOverride{
			Grant: common.AuthGrantOverride{
				SignedAfter:  now.Add(time.Second),
				SignedBefore: now.Add(2 * time.Second),
				EnforceUntil: time.Now().Add(time.Hour),
			},
		}
		cert, err := authenticateCertificate(b, "foo", o, zap.L())
		require.EqualError(t, err, "certificate has expired")
		require.Nil(t, cert)
	})
}

func TestIsADGroupClaim(t *testing.T) {
	var testCases = []struct {
		in  string
		out bool
	}{
		{
			"ad:",
			true,
		},
		{
			"ad",
			false,
		},
		{
			"AD:stuff",
			true,
		},
		{
			"AD:s",
			true,
		},
		{
			"  AD:s",
			true,
		},
		{
			"",
			false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.out, isADGroupClaim(tt.in))
		})
	}
}

func TestIsPersonnelClaim(t *testing.T) {
	var testCases = []struct {
		in  string
		out bool
	}{
		{
			"@",
			true,
		},
		{
			"bradb@uber.com",
			true,
		},
		{
			"me@ext.uber.com",
			true,
		},
		{
			"",
			false,
		},
		{
			"usecret",
			false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.out, isPersonnelClaim(tt.in))
		})
	}
}

func signedCertificate(t *testing.T, c *wonka.Certificate) (b []byte, restore func()) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "failed to create test keys")

	o := wonka.WonkaMasterPublicKeys
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&k.PublicKey}
	restore = func() { wonka.WonkaMasterPublicKeys = o }

	// Sign the cert with the new wonkamaster key
	sign := func() {
		defer func() {
			if r := recover(); r != nil {
				// If we are panicing out of this helper, make sure we restore
				// global state
				restore()
				restore = func() {}
				panic(r)
			}
		}()

		inputCert, err := wonka.MarshalCertificate(*c)
		require.NoError(t, err, "failed to marshal test cert")
		s, err := wonkacrypter.New().Sign(inputCert, k)
		require.NoError(t, err, "failed to sign test cert")
		c.Signature = s
		b, err = wonka.MarshalCertificate(*c)
		require.NoError(t, err, "failed to marshal test cert with signature")
	}

	sign()
	return
}
