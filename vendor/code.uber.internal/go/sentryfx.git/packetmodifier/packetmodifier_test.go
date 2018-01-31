package packetmodifier

import (
	"testing"

	"github.com/getsentry/raven-go"
	"github.com/stretchr/testify/assert"
)

func TestChain(t *testing.T) {
	m1 := func(packet *raven.Packet) {
		packet.Extra["m1"] = true
	}
	m2 := func(packet *raven.Packet) {
		packet.Extra["m2"] = true
	}

	m := Chain(m1, m2)
	packet := &raven.Packet{Extra: map[string]interface{}{}}
	m(packet)

	assert.Equal(t, true, packet.Extra["m1"])
	assert.Equal(t, true, packet.Extra["m2"])
}

func TestFingerprint(t *testing.T) {
	tests := []struct {
		fingerprint         interface{}
		expectedFingerprint interface{}
		msg                 string
	}{
		{
			nil,
			nil,
			"Packet with no fingerprint extra field has no fingerprint",
		},
		{
			1,
			nil,
			"Packet with fingerprint extra field of wrong type has no fingerprint",
		},
		{
			[]interface{}{1, 2},
			nil,
			"Packet with fingerprint extra field of wrong slice type has no fingerprint",
		},
		{
			"foo",
			[]string{"foo"},
			"Packet with string fingerprint extra field has fingerprint",
		},
		{
			[]interface{}{"{{ default }}", "other"},
			[]string{"{{ default }}", "other"},
			"Packet with string slice fingerprint extra field has fingerprint",
		},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			packet := &raven.Packet{Extra: map[string]interface{}{
				"fingerprint": tt.fingerprint,
			}}
			Fingerprint()(packet)
			if tt.expectedFingerprint == nil {
				assert.Nil(t, packet.Fingerprint)
			} else {
				assert.Equal(t, tt.expectedFingerprint, packet.Fingerprint)
			}
		})
	}
}
