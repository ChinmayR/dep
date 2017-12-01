package wonka

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestCancelRefreshCert(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := &uberWonka{cancel: cancel}

	c := make(chan bool, 1)
	go func() {
		w.refreshWonkaCert(ctx, 100*time.Millisecond)
		c <- true
	}()

	Close(w)

	good := false
	select {
	case <-time.After(time.Second):
	case <-c:
		good = true
	}

	require.True(t, good, "timedout")
}

func TestCancelCheckDerelicts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := &uberWonka{cancel: cancel}

	c := make(chan bool, 1)
	go func() {
		w.checkDerelicts(ctx, 100*time.Millisecond)
		c <- true
	}()

	Close(w)

	good := false
	select {
	case <-time.After(time.Second):
	case <-c:
		good = true
	}

	require.True(t, good, "timedout")
}

func TestCSRSignWithSSH(t *testing.T) {
	var testVars = []struct {
		name   string
		noKeys bool
		err    string
	}{{name: "foober"},
		{name: "foober", noKeys: true, err: "no ussh certs found"},
		{name: "foober"}}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			withSSHAgent(m.name, func(a agent.Agent) {
				if m.noKeys {
					a.RemoveAll()
				}

				w := &uberWonka{
					entityName: m.name,
					sshAgent:   a,
					log:        zap.L(),
				}

				cert, _, err := NewCertificate(CertEntityName(m.name))
				require.NoError(t, err)

				certSig := &CertificateSignature{
					Certificate: *cert,
					Timestamp:   int64(time.Now().Unix()),
					Data:        []byte("I'm a little teapot"),
				}

				csr, err := w.signCSRWithSSH(cert, certSig)
				if m.err == "" {
					require.NoError(t, err)
					require.NotNil(t, csr)
				} else {
					require.Nil(t, csr)
					require.Error(t, err)
					require.Contains(t, err.Error(), m.err)
				}
			})
		})
	}
}

func TestCSRSignWithCert(t *testing.T) {
	var testVars = []struct {
		name          string
		noCert        bool
		noSigningCert bool

		err string
	}{
		{name: "foober"},
		{name: "foober", noCert: true, err: "certificate is nil"},
		{name: "foober", noSigningCert: true, err: "nil cert"},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			entityCert, entityPrivKey, err := NewCertificate(CertEntityName(m.name))
			require.NoError(t, err)
			k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			require.NoError(t, err)
			err = entityCert.SignCertificate(k)
			require.NoError(t, err)

			oldKeys := WonkaMasterPublicKeys
			WonkaMasterPublicKeys = []*ecdsa.PublicKey{&k.PublicKey}

			w := &uberWonka{
				entityName:   m.name,
				certificate:  entityCert,
				clientECC:    entityPrivKey,
				clientKeysMu: &sync.RWMutex{},
				log:          zap.L(),
			}

			if m.noSigningCert {
				w.certificate = nil
			}

			cert, _, err := NewCertificate(CertEntityName(m.name))
			require.NoError(t, err)

			if m.noCert {
				cert = nil
			}

			csr, err := w.signCSRWithCert(cert)
			if m.err == "" {
				require.NoError(t, err)
				require.NotNil(t, csr)
			} else {
				require.Nil(t, csr)
				require.Error(t, err)
				require.Contains(t, err.Error(), m.err)
			}

			WonkaMasterPublicKeys = oldKeys
		})
	}
}

func TestTheHose(t *testing.T) {
	w := &uberWonka{
		log:           zap.L(),
		derelicts:     make(map[string]time.Time),
		derelictsLock: &sync.RWMutex{},
	}

	ok := IsDerelict(w, "")
	require.False(t, ok, "empty should be false")

	ok = IsDerelict(w, "foo")
	require.False(t, ok, "should not be a derelict")

	w.derelicts["foo"] = time.Now().Add(-time.Hour)
	ok = IsDerelict(w, "foo")
	require.False(t, ok, "should not be a derelict")

	w.derelicts["foo"] = time.Now().Add(24 * time.Hour)
	ok = IsDerelict(w, "foo")
	require.True(t, ok, "should be a derelict")
}

func TestDisabled(t *testing.T) {
	w := &uberWonka{
		isGloballyDisabled: atomic.NewBool(false),
	}
	ok := IsGloballyDisabled(w)
	require.False(t, ok)

	w.isGloballyDisabled.Store(true)
	ok = IsGloballyDisabled(w)
	require.True(t, ok)
}

func TestIsCurrentlyDisabled(t *testing.T) {
	d := keyInfo{}
	ok := isCurrentlyEnabled(d)
	require.True(t, ok)

	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	d.key = &k.PublicKey

	ok = isCurrentlyEnabled(d)
	require.False(t, ok)
}

type disabledTestVars struct {
	badDecode bool
	eTime     time.Duration
	cTime     time.Duration
	badKey    bool

	disabled bool
}

func TestIsDisabled(t *testing.T) {
	var testVars = []disabledTestVars{
		{badDecode: false, disabled: true},
		{badDecode: true, disabled: false},
		{eTime: 25 * time.Hour, disabled: false},
		{eTime: -time.Hour, disabled: false},
		{cTime: time.Hour, disabled: false},
		{badKey: true, disabled: false},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			w := &uberWonka{metrics: tally.NoopScope, log: zap.L(), isGloballyDisabled: atomic.NewBool(false)}
			msg, key := newDisableMessage(t, m)

			// assume we're disabled and this is the message
			ok := w.isDisabled(msg, key)
			require.Equal(t, m.disabled, ok)

			k := w.shouldDisable(msg, key)
			if m.disabled {
				require.NotNil(t, k)
			} else {
				require.Nil(t, k)
			}

			ok = w.shouldReEnable(msg, key)
			require.Equal(t, !m.disabled, ok)
		})
	}
}

// newDisableMessage returns a signed disable message and the pubkey that can be
// used to validate it.
func newDisableMessage(t *testing.T, d disabledTestVars) (string, *ecdsa.PublicKey) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	ctime := int64(time.Now().Add(-time.Minute).Unix())
	if d.cTime != time.Duration(0) {
		ctime = int64(time.Now().Add(d.cTime).Unix())
	}

	etime := int64(time.Now().Add(time.Minute).Unix())
	if d.eTime != time.Duration(0) {
		etime = int64(time.Now().Add(d.eTime).Unix())
	}

	msg := DisableMessage{
		Ctime:      ctime,
		Etime:      etime,
		IsDisabled: true,
	}

	toSign, err := json.Marshal(msg)
	require.NoError(t, err)

	msg.Signature, err = wonkacrypter.New().Sign(toSign, k)
	require.NoError(t, err)

	toCheck, err := json.Marshal(msg)
	require.NoError(t, err)

	toRet := base64.StdEncoding.EncodeToString(toCheck)
	if d.badDecode {
		toRet = "foober"
	}

	if d.badKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		return toRet, &k.PublicKey
	}

	return toRet, &k.PublicKey
}

func withSSHAgent(name string, fn func(agent.Agent)) {
	a := agent.NewKeyring()
	authority, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	signer, err := ssh.NewSignerFromKey(authority)
	if err != nil {
		panic(err)
	}

	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	pubKey, err := ssh.NewPublicKey(&k.PublicKey)
	if err != nil {
		panic(err)
	}

	cert := &ssh.Certificate{
		CertType:        ssh.UserCert,
		Key:             pubKey,
		Serial:          1,
		ValidPrincipals: []string{name},
		ValidAfter:      0,
		ValidBefore:     uint64(time.Now().Add(time.Minute).Unix()),
	}

	if err := cert.SignCert(rand.Reader, signer); err != nil {
		panic(err)
	}

	if err := a.Add(agent.AddedKey{PrivateKey: k}); err != nil {
		panic(err)
	}

	if err := a.Add(agent.AddedKey{PrivateKey: k, Certificate: cert}); err != nil {
		panic(err)
	}

	fn(a)
}
