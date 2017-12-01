package wonkadb

import (
	"context"
	"errors"

	wonka "code.uber.internal/engsec/wonka-go.git"
	_ "github.com/go-sql-driver/mysql" // this is a comment justifying a stupid lint error
)

var (
	// ErrNotFound indicates an entity doesn't exist by that name.
	ErrNotFound = errors.New("entity not found")
	// ErrExists indicates an entity already exists by that name.
	ErrExists = errors.New("entity exists")
)

// WONKAGroup - Group DB access helper
type WONKAGroup struct {
	ID          int64
	GroupName   string
	Description string
	Owner       string
	CreatedOn   int64
	ExpiresOn   int64
	IsEnabled   int8
}

// WONKAMember - Group Membership DB access helper
type WONKAMember struct {
	ID         int64
	GroupID    int64
	EntityName string
	CreatedOn  int64
	ExpiresOn  int64
	IsEnabled  int8
	Scope      string
}

// WONKARULE - Describes a single ACL used for claim granting
// (ALLOW RULES / DEFAULT: DENY)
type WONKARULE struct {
	ID          int64
	RuleType    int8
	OwnerID     int64
	IsEnabled   int8
	CreatedOn   int64
	ValidAfter  int64
	ExpiresOn   int64
	Source      string
	Destination string
}

// WonkaDB is an interface for mocking out the connection to the sql database.
type WonkaDB interface {
	// Connected returns err if the connection has been closed.
	IsConnected() bool

	// Entity related functions.
	// GetEntity returns the WONKAEntity for the given name.
	GetEntity(name string) *wonka.Entity
	// CreateEntity adds the WONKAEntity to the database.
	CreateEntity(e wonka.Entity) bool
	UpdateEntity(e wonka.Entity) bool
	DeleteEntity(e wonka.Entity) bool

	// Group realted functions.
	// GetGroupsByName returns the WONKAGroup's associated with the provided groupNames
	GetIdsForGroups(groupNames []string) (map[string]int, error)
	GetGroupsByName(groupNames []string) []WONKAGroup
	AddGroups(newGroups []string) error
	LookupMemberInGroup(groupName, entityName string) (int, error)
	SetMembershipsForEntity(entity string, gids []int) error
	SetMembershipGroupNamesForEntity(entity string, groupNames []string) error
}

// Global Entity Cache
var gEntityDB map[string]wonka.Entity

// EntityDB provides basic CRUD operations for Wonka entities.
type EntityDB interface {
	// Close cleans up the underlying database connection.
	Close() error

	// Get retrieves an Entity by its name.
	Get(ctx context.Context, name string) (*wonka.Entity, error)

	// Create creates a new Entity.
	Create(ctx context.Context, e *wonka.Entity) error

	// Update updates an existing Entity.
	Update(ctx context.Context, e *wonka.Entity) error

	// Delete removes an Entity.
	Delete(ctx context.Context, name string) error
}
