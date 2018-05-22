package wonka

import "strings"

// Name returns the canonical representation of an Entity's name.
func (e *Entity) Name() string {
	return CanonicalEntityName(e.EntityName)
}

// CanonicalEntityName returns the canonical representation of an Entity's name.
func CanonicalEntityName(name string) string {
	return strings.ToLower(name)
}
