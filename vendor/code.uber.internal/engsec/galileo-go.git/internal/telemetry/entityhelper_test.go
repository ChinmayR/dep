package telemetry_test

import (
	"testing"

	"code.uber.internal/engsec/galileo-go.git/internal/telemetry"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeEntityName(t *testing.T) {

	var testVars = []struct {
		descr    string // describes the test case
		original string // original name
		expected string //expected sanitized name
	}{
		{"noop", "foo", "foo"},
		{"screaming", "FOO", "foo"},
		{"hyphens", "foo-Bar", "foo-bar"},
		{"snake", "foO_baR", "foo_bar"},
		{"inception", "personnel_entity_redacted", "personnel_entity_redacted"},
		{"email address", "example@uber.com", "personnel_entity_redacted"},
		{"email like", "ex@ub", "personnel_entity_redacted"},
		{"@", "@", "personnel_entity_redacted"},
		{"namespaced", "wonkaSample:test", "wonkasample.test"},
		{"multiple colons", "m3:hates::colons", "m3.hates.colons"},
		{"empty", "", ""},
	}

	for _, m := range testVars {
		t.Run(m.descr, func(t *testing.T) {
			assert.Equal(t, m.expected, telemetry.SanitizeEntityName(m.original))
		})
	}
}
