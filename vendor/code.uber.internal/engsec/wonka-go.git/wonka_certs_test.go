package wonka_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"fmt"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"

	"github.com/stretchr/testify/require"
)

func TestCheckCert(t *testing.T) {
	var certVars = []struct {
		name    string
		host    string
		badCert bool

		signErr string
	}{
		{name: "foo", host: "foo01-sjc1.prod.uber.internal"},
		{name: "foo", host: "foo01-sjc1.prod.uber.internal", badCert: true, signErr: "error marshalling certificate"},
	}

	for _, m := range certVars {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err, "generate key: %v", err)

		keyBytes, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
		require.NoError(t, err, "marshalling pubkey: %v", err)

		c := &wonka.Certificate{
			EntityName:  m.name,
			Host:        m.host,
			Key:         keyBytes,
			ValidAfter:  uint64(time.Now().Unix()),
			ValidBefore: uint64(time.Now().Add(time.Minute).Unix()),
		}

		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err, "error generating key: %v", err)

		oldWonkaMasterKeys := wonka.WonkaMasterPublicKeys
		wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&priv.PublicKey}

		err = c.SignCertificate(priv)
		if m.signErr == "" {
			require.NoError(t, err, "error signing cert: %v", err)

			pubKey, err := c.PublicKey()
			require.NoError(t, err, "getting key shouldn't error: %v", err)

			origKey, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
			require.NoError(t, err, "marshalling pubkey: %v", err)
			newKey, err := x509.MarshalPKIXPublicKey(pubKey)
			require.NoError(t, err, "marshalling new pubkey: %v", err)
			require.True(t, bytes.Equal(origKey, newKey), "keys should equal")

		}

		err = c.CheckCertificate()
		require.NoError(t, err, "error checking cert: %v", err)

		wonka.WonkaMasterPublicKeys = oldWonkaMasterKeys
	}
}

func TestNewCert(t *testing.T) {
	var testVars = []struct {
		runTime string
		taskID  string
		name    string
	}{
		{runTime: "prod"},
		{taskID: "1234"},
		{name: "foober"},
		{name: "doober", taskID: "scoober", runTime: "floober"},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			cert, _, err := wonka.NewCertificate(wonka.CertEntityName(m.name),
				wonka.CertRuntimeTag(m.runTime), wonka.CertTaskIDTag(m.taskID))
			require.NoError(t, err)
			require.Equal(t, m.runTime, cert.Tags[wonka.TagRuntime])
			require.Equal(t, m.taskID, cert.Tags[wonka.TagTaskID])
			require.Equal(t, m.name, cert.EntityName)
		})
	}
}

func TestMarshalCert(t *testing.T) {
	c, _, err := wonka.NewCertificate(wonka.CertEntityName("foober"))
	require.NoError(t, err)

	marshalled, err := wonka.MarshalCertificate(*c)
	require.NoError(t, err)

	unmarshalled, err := wonka.UnmarshalCertificate(marshalled)
	require.NoError(t, err)
	require.Equal(t, c.EntityName, unmarshalled.EntityName)

	remarshalled, err := wonka.MarshalCertificate(*unmarshalled)
	require.NoError(t, err)

	require.Equal(t, marshalled, remarshalled)

	reunmarshalled, err := wonka.UnmarshalCertificate([]byte("foober"))
	require.Error(t, err)
	require.Nil(t, reunmarshalled)
}

func TestCertSignature(t *testing.T) {
	var testVars = []struct {
		name     string
		data     []byte
		unsigned bool

		shouldErr bool
	}{
		{name: "foober", data: []byte("something to sign")},
		{name: "foober", data: []byte("something to sign"), unsigned: true, shouldErr: true},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			oldPubKeys := wonka.WonkaMasterPublicKeys
			k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			require.NoError(t, err)
			wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&k.PublicKey}

			cert, privKey, err := wonka.NewCertificate(wonka.CertEntityName(m.name))
			require.NoError(t, err)

			if !m.unsigned {
				err := cert.SignCertificate(k)
				require.NoError(t, err)
			}

			sig, err := wonka.NewCertificateSignature(*cert, privKey, m.data)
			require.NoError(t, err)
			require.NotNil(t, sig)

			err = wonka.VerifyCertificateSignature(*sig)
			require.True(t, (err == nil) == !m.shouldErr)

			wonka.WonkaMasterPublicKeys = oldPubKeys
		})
	}
}
