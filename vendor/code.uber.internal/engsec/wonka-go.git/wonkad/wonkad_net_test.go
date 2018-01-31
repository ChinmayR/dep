package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net"
	"sync"
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

func TestWriteCertAndKey(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	withCertificate(t, "foo", "bar", func(cert *wonka.Certificate, ck, wk *ecdsa.PrivateKey) {
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			defer server.Close()

			result, err := bufio.NewReader(server).ReadString('}')
			require.NoError(t, err, "reading should not error")
			require.NotNil(t, result)
		}()

		err := writeCertAndKey(client, cert, ck)
		require.NoError(t, err, "should write cert and key")
		wg.Wait()
	})
}

func TestHandleWonkaRequestCannotContactWonkaMasterShouldError(t *testing.T) {
	withSSHConfig(t, "handleWonkaRequest", func(configPath string) {
		oldConfigPath := sshdConfig
		sshdConfig = &configPath
		defer func() { sshdConfig = oldConfigPath }()

		log := zap.NewNop()
		uwonka, err := createWonka(log)
		require.Error(t, err, "should error when cannot contact wonkamaster")
		w := &wonkad{
			log:   log,
			wonka: uwonka,
		}
		_, _, _, err = w.handleWonkaRequest(context.Background(), wonka.WonkadRequest{})
		require.Error(t, err, "should error when cannot contact wonkamaster")
	})
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
