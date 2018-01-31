package wonkadb

import (
	"context"
	"errors"

	wonka "code.uber.internal/engsec/wonka-go.git"
)

var (
	// ErrNotFound indicates an entity doesn't exist by that name.
	ErrNotFound = errors.New("entity not found")
	// ErrExists indicates an entity already exists by that name.
	ErrExists = errors.New("entity exists")
)

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
