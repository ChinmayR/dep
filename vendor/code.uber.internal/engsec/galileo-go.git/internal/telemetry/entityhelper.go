package telemetry

import (
	"regexp"
	"strings"

	"code.uber.internal/engsec/wonka-go.git"
)

// PersonnelEntityRedacted is the string used in M3 metrics instead of personnel
// entity names (email addresses) so we don't blow up cardinality.
const PersonnelEntityRedacted = "personnel_entity_redacted"

var _colonsRe = regexp.MustCompile(":+")

// SanitizeEntityName returns an entity name safe for use as a tag value in M3.
func SanitizeEntityName(e string) string {
	if strings.ContainsRune(e, '@') {
		return PersonnelEntityRedacted
	}

	// Colon is the namespace separator for Wonka Entity Names, and cannot be
	// used in M3 tag values. M3 will also coalesce multiple dots.
	// https://engdocs.uberinternal.com/m3_and_umonitor/intro/data_model.html
	e = _colonsRe.ReplaceAllLiteralString(e, ".")

	return wonka.CanonicalEntityName(e)
}
