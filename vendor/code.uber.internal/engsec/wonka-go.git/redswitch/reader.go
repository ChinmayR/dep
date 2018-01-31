package redswitch

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal"
	"code.uber.internal/engsec/wonka-go.git/internal/dns"
	"code.uber.internal/engsec/wonka-go.git/internal/url"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

var _ CachelessReader = (*reader)(nil)

// DisableMessage is signed by wonkamaster and put as a json blob in a TXT record
// under uberinternal.com. It's used to globally disable Wonka authentication for some period of time.
// This is a hack and may go away when Flipr supports disabling Galileo.
type DisableMessage struct {
	Ctime      int64  `json:"ctime,omitempty"`
	Etime      int64  `json:"etime,omitempty"`
	IsDisabled bool   `json:"is_disabled,omitmpty"`
	Signature  []byte `json:"signature,omitempty"`
}

// Reader provides a means for querying the globally disabled status.
//
// The status may be internally cached for a period of time.
type Reader interface {
	IsDisabled() bool
}

// CachelessReader provides a means for querying the globally disabled status
// that always bypasses any local caching, though local caches may be updated
// as a result of the check.
type CachelessReader interface {
	Reader
	ForceCheckIsDisabled(context.Context) bool
}

// keyRecordTuple encapsulates the relationship between wonkamaster
// public keys and their associated DNS records for the globally
// disabled switch.
type keyRecordTuple struct {
	key         *ecdsa.PublicKey
	recordName  string
	recordValue string
}

type reader struct {
	disabled       uint32
	dns            dns.Client
	keys           []keyRecordTuple
	log            *zap.Logger
	metrics        tally.Scope
	recoveryNotify chan<- time.Time
	timer          *time.Timer
}

// NewReader creates a new Reader which can be used to query the redwitch status.
func NewReader(opts ...ReaderOption) (Reader, error) {
	o := newReaderOptions()
	for _, opt := range opts {
		opt(o)
	}

	if err := o.validate(); err != nil {
		return nil, err
	}

	pubKeys, err := o.publicKeys()
	if err != nil {
		return nil, err
	}

	keys, err := setupKeyInfo(pubKeys)
	if err != nil {
		return nil, err
	}

	return &reader{
		timer:          time.NewTimer(internal.DisableCheckPeriod),
		log:            o.log,
		metrics:        o.metrics,
		recoveryNotify: o.recovery,
		keys:           keys,
		dns:            o.dns,
	}, nil
}

// IsDisabled returns true if Wonka authentication should be globally disabled. This state
// may be cached for performance reasons.
func (d *reader) IsDisabled() bool {
	select {
	case e := <-d.timer.C:
		d.log.With(zap.Time("expired", e), zap.Time("now", time.Now())).Debug("Refreshing global disabled status")
		go d.refreshStatus(context.Background())
		d.timer.Reset(internal.DisableCheckPeriod)
	default:
	}

	return atomic.LoadUint32(&d.disabled) == 1
}

// ForceCheckIsDisabled synchronously checks the global disabled status and returns
// the result. It also updates the sentinel field which caches the disabled state.
func (d *reader) ForceCheckIsDisabled(ctx context.Context) bool {
	d.refreshStatus(ctx)
	return atomic.LoadUint32(&d.disabled) == 1
}

func (d *reader) refreshStatus(ctx context.Context) {
	defer d.log.Debug("wonka global disabled status refreshed")
	currentlyDisabled := d.queryDisabledStatus(ctx)
	if !currentlyDisabled && atomic.CompareAndSwapUint32(&d.disabled, 1, 0) {
		// Notify any consumers that we have now recovered from a global panic.
		d.log.Info("re-enabling wonka")
		if d.recoveryNotify != nil {
			go func() {
				d.recoveryNotify <- time.Now()
			}()
		}
	} else if currentlyDisabled && atomic.CompareAndSwapUint32(&d.disabled, 0, 1) {
		d.log.Warn("disabling wonka; may willie have mercy on our souls")
	}
}

func (d *reader) queryDisabledStatus(ctx context.Context) bool {
	keys := lookup(ctx, d.dns, d.keys)
	for _, key := range keys {
		if isDisabled(key, d.log, d.metrics) {
			return true
		}
	}
	return false
}

// setupKeyInfo takes the list of wonkamster publickeys and turns them into a slice
// of keyRecordTuple structs.
func setupKeyInfo(masterKeys []*ecdsa.PublicKey) ([]keyRecordTuple, error) {
	kiSlice := make([]keyRecordTuple, 0, len(masterKeys))

	for _, k := range masterKeys {
		record, err := marshallKeyToRecord(k)
		if err != nil {
			return nil, err
		}

		ki := keyRecordTuple{
			key:        k,
			recordName: record,
		}

		kiSlice = append(kiSlice, ki)
	}

	return kiSlice, nil
}

func marshallKeyToRecord(k *ecdsa.PublicKey) (string, error) {
	keyBytes, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		return "", err
	}

	h := crypto.SHA256.New()
	h.Write(keyBytes)
	record := fmt.Sprintf("%s.uberinternal.com", url.Base32WithoutPadding(h.Sum(nil)))
	return record, nil
}

// lookup refreshes the values of all the keys.
func lookup(ctx context.Context, dnsClient dns.Client, keys []keyRecordTuple) []keyRecordTuple {
	rval := make([]keyRecordTuple, len(keys))
	copy(rval, keys)
	for i, k := range keys {
		if r, err := dnsClient.LookupTXT(ctx, k.recordName); err == nil && len(r) > 0 {
			// there shouldn't be more than one record here, ignore anything that's not
			// the first record all the same.
			rval[i].recordValue = r[0]
		}
	}

	return rval
}

// isDisabled checks the validity of a potential signed disable message.
func isDisabled(key keyRecordTuple, log *zap.Logger, metrics tally.Scope) bool {
	if key.key == nil {
		return false
	}

	record := key.recordValue

	// All valid disabled records will be greater than 64 chars.
	const _minValidLength = 64
	if len(record) < _minValidLength {
		metrics.Tagged(map[string]string{
			"disabled": "false",
		}).Counter("disabled").Inc(1)
		return false
	}

	reply, err := base64.StdEncoding.DecodeString(record)
	if err != nil {
		metrics.Tagged(map[string]string{
			"error": "base64_decode_record",
		}).Counter("disabled").Inc(1)

		log.Warn("error base64 decoding",
			zap.String("record", record),
			zap.Error(err))
		return false
	}

	var msg DisableMessage
	if err := json.Unmarshal(reply, &msg); err != nil {
		metrics.Tagged(map[string]string{
			"error": "json_unmarshal",
		}).Counter("disabled").Inc(1)
		log.Warn("error unmarshalling disable txt record", zap.Error(err))
		return false
	}

	// TODO(pmoody): allow for clock skew.
	now := time.Now()
	cTime := time.Unix(msg.Ctime, 0)
	eTime := time.Unix(msg.Etime, 0)
	// check that it was created before now
	if cTime.After(now) {
		metrics.Tagged(map[string]string{
			"error": "not_yet_valid",
		}).Counter("disabled").Inc(1)
		log.Debug("disable message not yet valid", zap.Time("ctime", cTime))
		return false
	}

	// check that it hasn't expired
	if eTime.Before(now) {
		metrics.Tagged(map[string]string{
			"error": "expired",
		}).Counter("disabled").Inc(1)
		log.Debug("disable message expired", zap.Time("etime", eTime))
		return false
	}

	// check that the disable message isn't good for more than 24 hours
	if !cTime.Add(internal.MaxDisableDuration).After(eTime) {
		metrics.Tagged(map[string]string{
			"error": "disable_too_long",
		}).Counter("disabled").Inc(1)
		log.Error("disable message is good for too long", zap.Time("etime", eTime))
		return false
	}

	if !verifySignature(msg, json.Marshal, log, metrics, key.key) {
		return false
	}

	metrics.Tagged(map[string]string{
		"disabled": "true",
	}).Counter("disabled").Inc(1)

	// we're disabled
	return true
}

func verifySignature(msg DisableMessage,
	serialize func(interface{}) ([]byte, error),
	log *zap.Logger,
	metrics tally.Scope,
	key *ecdsa.PublicKey) bool {

	verify := msg
	verify.Signature = nil
	toVerify, err := serialize(verify)
	if err != nil {
		metrics.Tagged(map[string]string{
			"error": "json_marshal",
		}).Counter("disabled").Inc(1)
		log.Debug("unable to marshal msg to verify", zap.Error(err))
		return false
	}

	ok := wonkacrypter.New().Verify(toVerify, msg.Signature, key)
	if !ok {
		metrics.Tagged(map[string]string{
			"error": "invalid_signature",
		}).Counter("disabled").Inc(1)
		return false
	}

	return true
}
