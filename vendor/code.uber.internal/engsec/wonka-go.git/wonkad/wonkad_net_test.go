package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"testing"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestVerifySignature(t *testing.T) {
	var sigVars = []struct {
		taskID   string
		certSig  bool
		wonkaSig bool

		shouldErr bool
	}{
		{taskID: "foo-1234", certSig: true, wonkaSig: true},
		{taskID: "foo-1234", certSig: true, shouldErr: true},
		{taskID: "foo-1234", wonkaSig: true, shouldErr: true},
	}

	for idx, m := range sigVars {
		withRequest(t, "", "", "", "", func(r wonka.WonkadRequest) {
			if !m.certSig {
				r.Certificate.Signature = nil
			}

			if !m.wonkaSig {
				r.Signature = nil
			}

			ok := verifySignature(zap.NewNop(), r)
			require.Equal(t, !m.shouldErr, ok, "test %d", idx)
		})
	}
}

func withRequest(t *testing.T, host, proc, taskid, dest string, fn func(r wonka.WonkadRequest)) {
	withCertificate(t, host, host, func(c *wonka.Certificate, certKey, wonkaKey *ecdsa.PrivateKey) {
		r := wonka.WonkadRequest{
			Process:     proc,
			TaskID:      taskid,
			Service:     taskid,
			Destination: dest,
			Certificate: *c,
		}

		toSign, err := json.Marshal(r)
		require.NoError(t, err, "error marshalling: %v", err)

		r.Signature, err = wonkacrypter.New().Sign(toSign, certKey)
		require.NoError(t, err, "error signing: %v", err)

		fn(r)
	})
}

func withCertificate(t *testing.T, host, entity string, fn func(c *wonka.Certificate, ck, wk *ecdsa.PrivateKey)) {
	wonkaMasterKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "error generating ecdsa key: %v", err)

	oldWonkaMasterKeys := wonka.WonkaMasterPublicKeys
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&wonkaMasterKey.PublicKey}
	defer func() { wonka.WonkaMasterPublicKeys = oldWonkaMasterKeys }()

	cert, certKey, err := wonka.NewCertificate(wonka.CertEntityName(entity), wonka.CertHostname(host))
	require.NoError(t, err, "error generating certificate: %v", err)

	toSign, err := json.Marshal(cert)
	require.NoError(t, err, "error marshalling cert: %v", err)

	cert.Signature, err = wonkacrypter.New().Sign(toSign, wonkaMasterKey)
	require.NoError(t, err, "error signing certificate: %v", err)

	fn(cert, certKey, wonkaMasterKey)
}
