// Package packetmodifier provides sentry packet modifiers that can modify Sentry packets before they
// are sent to Sentry. Sentryfx will automatically use these modifiers if they are provided.
package packetmodifier // import "code.uber.internal/go/sentryfx.git/packetmodifier"

import "github.com/getsentry/raven-go"

// Chain accepts zero or more Sentry packet modifiers and returns a new packet
// modifier that calls them in-order.
func Chain(ms ...func(*raven.Packet)) func(*raven.Packet) {
	return func(packet *raven.Packet) {
		for _, m := range ms {
			m(packet)
		}
	}
}

// Fingerprint returns a packet modifier that sets the packet fingerprint.
// This modifier will look for a logged field named "fingerprint" and will set it as the packet
// fingerprint if the value is either a string slice or a single string. This modifier will
// also remove "fingerprint" from the Extra map.
func Fingerprint() func(*raven.Packet) {
	return func(packet *raven.Packet) {
		if rawFingerprint, ok := packet.Extra["fingerprint"]; ok {
			packet.Fingerprint = buildFingerprint(rawFingerprint)
			if packet.Fingerprint != nil {
				delete(packet.Extra, "fingerprint")
			}
		}
	}
}

func buildFingerprint(rawFingerprint interface{}) []string {
	fingerprintArr, ok := rawFingerprint.([]interface{})
	if !ok {
		if f, ok := rawFingerprint.(string); ok {
			return []string{f}
		}
		return nil
	}

	fingerprint := make([]string, 0, len(fingerprintArr))
	for _, f := range fingerprintArr {
		if fstr, ok := f.(string); ok {
			fingerprint = append(fingerprint, fstr)
		}
	}
	if len(fingerprint) == 0 {
		return nil
	}
	return fingerprint
}
