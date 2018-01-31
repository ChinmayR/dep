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

func (e EntityType) String() string {
	switch e {
	case EntityTypeInvalid:
		return "Invalid"
	case EntityTypeService:
		return "Service"
	case EntityTypeUser:
		return "User"
	case EntityTypeHost:
		return "Host"
	}
	return "Undefined"
}
