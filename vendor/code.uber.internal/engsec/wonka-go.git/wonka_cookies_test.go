package wonka

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCookieMarshal(t *testing.T) {
	c := &Cookie{Destination: "foo"}
	marshalled, err := MarshalCookie(c)
	require.NoError(t, err)

	unmarshalledC, err := UnmarshalCookie(marshalled)
	require.NoError(t, err)

	require.Equal(t, c.Destination, unmarshalledC.Destination)
}

func TestCookieNewAndVerify(t *testing.T) {
	goodCookieSetup := func(t *testing.T, w Wonka) *Cookie {
		c, e := NewCookie(w, w.EntityName())
		require.NoError(t, e)
		return c
	}
	var testVars = []struct {
		testName string
		setup    func(*testing.T, Wonka) *Cookie
		test     func(*testing.T, Wonka, *Cookie)
	}{
		{
			testName: "good_cookie",
			setup:    goodCookieSetup,
			test: func(t *testing.T, w Wonka, c *Cookie) {
				require.NoError(t, CheckCookie(w, c))
			},
		},
		{
			testName: "wrong_dest",
			setup:    goodCookieSetup,
			test: func(t *testing.T, w Wonka, c *Cookie) {
				c.Destination = fmt.Sprintf("not-%s", w.EntityName())
				e := CheckCookie(w, c)
				require.Error(t, e)
				require.Contains(t, e.Error(), "cookie not for me")
			},
		},
		{
			testName: "bad_signature",
			setup:    goodCookieSetup,
			test: func(t *testing.T, w Wonka, c *Cookie) {
				c.Ctime = 0
				e := CheckCookie(w, c)
				require.Error(t, e)
				require.Contains(t, e.Error(), "cookie signature doesn't verify")
			},
		},
		{
			testName: "no_key",
			setup:    goodCookieSetup,
			test: func(t *testing.T, w Wonka, c *Cookie) {
				c.Certificate.Key = nil
				e := CheckCookie(w, c)
				require.Error(t, e)
				require.Contains(t, e.Error(), "error getting key from certificate")
			},
		},
	}
	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d_%s", idx, m.testName), func(t *testing.T) {
			withUberWonka("foober", func(w *uberWonka) {
				cookie := m.setup(t, w)
				m.test(t, w, cookie)
			})
		})
	}
}

func withUberWonka(name string, fn func(w *uberWonka)) {
	w := &uberWonka{
		entityName: name,
		log:        zap.L(),
	}

	signer, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	oldWMKeys := WonkaMasterPublicKeys
	WonkaMasterPublicKeys = []*ecdsa.PublicKey{&signer.PublicKey}
	defer func() { WonkaMasterPublicKeys = oldWMKeys }()

	cert, key, err := NewCertificate(CertEntityName(name))
	if err != nil {
		panic(err)
	}

	if err := cert.SignCertificate(signer); err != nil {
		panic(err)
	}
	w.writeCertAndKey(cert, key)
	fn(w)
}
