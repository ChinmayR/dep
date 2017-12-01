package wonka

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal"
	"code.uber.internal/engsec/wonka-go.git/internal/url"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	"go.uber.org/zap"
)

type keyInfo struct {
	key        *ecdsa.PublicKey
	recordName string
}

// IsGloballyDisabled returns true if wonka is disabled and false otherwise.
func IsGloballyDisabled(w Wonka) bool {
	uw, ok := w.(*uberWonka)
	if !ok {
		return false
	}
	return uw.isGloballyDisabled.Load()
}

// setupKeyInfo takes the list of wonkamster publickeys and turns them into a slice
// of keyInfo structs.
func (w *uberWonka) setupKeyInfo(masterKeys []*ecdsa.PublicKey) []keyInfo {
	kiSlice := make([]keyInfo, 0, len(masterKeys))

	for _, k := range masterKeys {
		record, err := marshallKeyToRecord(k)
		if err != nil {
			w.log.Warn("error marshalling key", zap.Error(err))
			continue
		}

		ki := keyInfo{
			key:        k,
			recordName: record,
		}

		kiSlice = append(kiSlice, ki)
	}

	return kiSlice
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

// checkGlobalDisableStatus looks in dns every `DisableCheckPeriod` for signed disable messsages.
// if we're not currently disabled, it checks to see if we should be disabled. If we're currently
// enabled, it checks to see if we should still be disabled.
func (w *uberWonka) checkGlobalDisableStatus(ctx context.Context, masterKeys []*ecdsa.PublicKey) {
	keys := w.setupKeyInfo(masterKeys)
	if len(keys) == 0 {
		w.log.Error("no valid keys")
		return
	}

	sentinel := keyInfo{}
	ticker := time.NewTicker(internal.DisableCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if isCurrentlyEnabled(sentinel) {
				if ki := w.shouldDisable(lookup(keys)); ki != nil {
					w.log.Warn("disabling wonka; may willie have mercy on our souls")
					// TODO(pmoody): we should emit metrics here.
					sentinel = *ki
				}
				continue
			}
			if ok := w.shouldReEnable(lookup([]keyInfo{sentinel})); ok {
				w.log.Info("re-enabling wonka")
				// TODO(pmoody): we should emit metrics here.
				sentinel = keyInfo{}
			}
		}
	}
}

// lookup returns the value of the first txt record that resovles from the given keyslice.
func lookup(keys []keyInfo) (string, *ecdsa.PublicKey) {
	for _, k := range keys {
		if r, err := net.LookupTXT(k.recordName); err == nil && len(r) > 0 {
			// there shouldn't be more than one record here, ignore anything that's not
			// the first record all the same.
			return r[0], k.key
		}
	}

	return "", nil
}

// isCurrentlyEnabled returns true if wonka is currently enabled.
func isCurrentlyEnabled(sentinel keyInfo) bool {
	return sentinel.key == nil
}

// shouldDisable returns the key and key in b32 if this record is a valid disable message.
func (w *uberWonka) shouldDisable(record string, key *ecdsa.PublicKey) *keyInfo {
	ok := w.isDisabled(record, key)
	w.isGloballyDisabled.Store(ok)
	// if this key signed a valid disable message, we store it so we can
	// monitor for when that disable message has been removed.
	if ok {
		record, _ := marshallKeyToRecord(key)
		return &keyInfo{key: key, recordName: record}
	}

	return nil
}

// shouldReEnable checks to make sure a previously signed disable message is still
// present in dns.
func (w *uberWonka) shouldReEnable(record string, key *ecdsa.PublicKey) bool {
	if record == "" {
		w.isGloballyDisabled.Store(false)
		return true
	}

	// make sure the disabled message is still valid.
	disabled := w.isDisabled(record, key)
	w.isGloballyDisabled.Store(disabled)
	return !disabled
}

// isDisabled checks the validity of a potential signed disable message.
func (w *uberWonka) isDisabled(record string, k *ecdsa.PublicKey) bool {
	if k == nil {
		return false
	}

	reply, err := base64.StdEncoding.DecodeString(record)
	if err != nil {
		w.metrics.Tagged(map[string]string{
			"error": "base64_decode_record",
		}).Counter("disabled").Inc(1)

		w.log.Warn("error base64 decoding",
			zap.String("record", record),
			zap.Error(err))
		return false
	}

	var msg DisableMessage
	if err := json.Unmarshal(reply, &msg); err != nil {
		w.metrics.Tagged(map[string]string{
			"error": "json_unmarshal",
		}).Counter("disabled").Inc(1)
		w.log.Warn("error unmarshalling disable txt record", zap.Error(err))
		return false
	}

	// TODO(pmoody): allow for clock skew.
	now := time.Now()
	cTime := time.Unix(msg.Ctime, 0)
	eTime := time.Unix(msg.Etime, 0)
	// check that it was created before now
	if cTime.After(now) {
		w.metrics.Tagged(map[string]string{
			"error": "not_yet_valid",
		}).Counter("disabled").Inc(1)
		w.log.Warn("disable message not yet valid", zap.Time("ctime", cTime))
		return false
	}

	// check that it hasn't expired
	if eTime.Before(now) {
		w.metrics.Tagged(map[string]string{
			"error": "expired",
		}).Counter("disabled").Inc(1)
		w.log.Error("disable message expired", zap.Time("etime", eTime))
		return false
	}

	// check that the disable message isn't good for more than 24 hours
	if !cTime.Add(internal.MaxDisableDuration).After(eTime) {
		w.metrics.Tagged(map[string]string{
			"error": "disable_too_long",
		}).Counter("disabled").Inc(1)
		w.log.Error("disable message is good for too long", zap.Time("etime", eTime))
		return false
	}

	verify := msg
	verify.Signature = nil
	toVerify, err := json.Marshal(verify)
	if err != nil {
		w.metrics.Tagged(map[string]string{
			"error": "json_marshal",
		}).Counter("disabled").Inc(1)
		w.log.Error("unable to marshal msg to verify", zap.Error(err))
		return false
	}

	ok := wonkacrypter.New().Verify(toVerify, msg.Signature, k)
	if !ok {
		w.metrics.Tagged(map[string]string{
			"error": "invalid_signature",
		}).Counter("disabled").Inc(1)
		return false
	}

	w.metrics.Tagged(map[string]string{
		"disabled": "true",
	}).Counter("disabled").Inc(1)

	// we're disabled
	return true
}
