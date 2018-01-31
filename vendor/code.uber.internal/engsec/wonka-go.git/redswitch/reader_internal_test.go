package redswitch

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

type mockDNS struct {
	records []string
}

func (m *mockDNS) LookupTXT(context.Context, string) ([]string, error) {
	return m.records, nil
}

func TestIsDisabledRefreshCycle(t *testing.T) {
	record, pubKey := newDisableMessage(t, disabledTestVars{
		eTime: time.Hour,
	})

	// Disable wonka at first, then re-enable
	dns := mockDNS{records: []string{record}}
	rec := make(chan time.Time, 1)

	r, err := NewReader(WithLogger(zap.NewNop()),
		WithMetrics(tally.NoopScope),
		WithPublicKeys(pubKey),
		WithDNSClient(&dns),
		WithRecoveryNotification(rec),
	)
	require.NoError(t, err)

	// need to manually reset the timer since we don't want to wait for
	// the full internal.DisableCheckPeriod value.
	r.(*reader).timer.Stop()
	r.(*reader).timer.Reset(0)

	// Now poll until the disabled status is updated
	for !r.IsDisabled() {
	}
	require.True(t, r.IsDisabled())

	// Now re-enable Wonka
	dns.records = []string{"enabled placeholder"}
	r.(*reader).timer.Stop()
	r.(*reader).timer.Reset(0)

	// Wait for our recovery notification
	hasNotification := false
	for !hasNotification {
		select {
		case <-rec:
			hasNotification = true
		default:
			r.IsDisabled()
		}
	}

	require.False(t, r.IsDisabled())
}

func TestLookupReturnsNameAndValue(t *testing.T) {
	expectedRecordValue := "TXT value"
	pubkey := ecdsa.PublicKey{X: big.NewInt(123), Y: big.NewInt(456)}
	dns := mockDNS{records: []string{expectedRecordValue}}

	k := keyRecordTuple{
		recordName: "pubkeyhash.uberinternal.com",
		key:        &pubkey,
	}

	keys := lookup(context.Background(), &dns, []keyRecordTuple{k})
	require.Len(t, keys, 1)

	key := keys[0]
	require.Equal(t, expectedRecordValue, key.recordValue)
	require.Equal(t, k.recordName, key.recordName)
	require.Equal(t, k.key, key.key)
}

func TestForceCheckIsDisabled(t *testing.T) {
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
			msg, key := newDisableMessage(t, m)
			r, err := NewReader(WithLogger(zap.NewNop()),
				WithMetrics(tally.NoopScope),
				WithPublicKeys(key),
				WithDNSClient(&mockDNS{records: []string{msg}}))
			require.NoError(t, err)

			// assume we're disabled and this is the message
			require.Equal(t, m.disabled, r.(CachelessReader).ForceCheckIsDisabled(context.Background()))
		})
	}
}

func TestIsDisabled(t *testing.T) {
	t.Run("invalid_encoding", func(t *testing.T) {
		require.False(t, isDisabled(keyRecordTuple{
			recordValue: strings.Repeat("b", 65),
			key:         &ecdsa.PublicKey{},
		}, zap.NewNop(), tally.NoopScope))
	})
	t.Run("invalid_marshalling", func(t *testing.T) {
		b := make([]byte, 256)
		_, err := rand.Read(b)
		require.NoError(t, err)

		s := base64.StdEncoding.EncodeToString(b)
		require.False(t, isDisabled(keyRecordTuple{
			recordValue: s,
			key:         &ecdsa.PublicKey{},
		}, zap.NewNop(), tally.NoopScope))
	})
	t.Run("nil_key", func(t *testing.T) {
		require.False(t, isDisabled(keyRecordTuple{
			key: nil,
		}, zap.NewNop(), tally.NoopScope))
	})
}

func TestVerifySignature(t *testing.T) {
	t.Run("invalid_serialization", func(t *testing.T) {
		serial := func(interface{}) ([]byte, error) {
			return nil, errors.New("test serialization error")
		}

		require.False(t, verifySignature(DisableMessage{}, serial, zap.NewNop(), tally.NoopScope, nil))
	})
}

type disabledTestVars struct {
	badDecode bool
	eTime     time.Duration
	cTime     time.Duration
	badKey    bool

	disabled bool
}

// newDisableMessage returns a signed disable message and the pubkey that can be
// used to validate it.
func newDisableMessage(t require.TestingT, d disabledTestVars) (string, *ecdsa.PublicKey) {
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
