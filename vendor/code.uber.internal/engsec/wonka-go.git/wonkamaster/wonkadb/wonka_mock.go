package wonkadb

import (
	"context"
	"encoding/json"
	"sync"

	wonka "code.uber.internal/engsec/wonka-go.git"
)

// NewMockEntityDB creates a mock EntityDB.
func NewMockEntityDB() EntityDB {
	return &mockEntityDB{
		store: make(map[string][]byte),
	}
}

type mockEntityDB struct {
	store map[string][]byte
	sync.RWMutex
}

func (m *mockEntityDB) Close() error { return nil }

func (m *mockEntityDB) Get(ctx context.Context, name string) (*wonka.Entity, error) {
	m.RLock()
	defer m.RUnlock()
	b, ok := m.store[wonka.CanonicalEntityName(name)]
	if !ok {
		return nil, ErrNotFound
	}
	return m.deserializeEntity(b)
}

func (m *mockEntityDB) Create(ctx context.Context, e *wonka.Entity) error {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.store[e.Name()]; ok {
		return ErrExists
	}
	b, err := m.serializeEntity(e)
	if err != nil {
		return err
	}
	m.store[e.Name()] = b
	return nil
}

func (m *mockEntityDB) Update(ctx context.Context, e *wonka.Entity) error {
	m.Lock()
	defer m.Unlock()
	b, ok := m.store[e.Name()]
	if !ok {
		return ErrNotFound
	}
	updateEntity, err := m.deserializeEntity(b)
	if err != nil {
		return err
	}
	updateEntity.Requires = e.Requires
	updateBytes, err := m.serializeEntity(updateEntity)
	if err != nil {
		return err
	}
	m.store[e.Name()] = updateBytes
	return nil
}

func (m *mockEntityDB) Delete(ctx context.Context, name string) error {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.store[wonka.CanonicalEntityName(name)]; !ok {
		return ErrNotFound
	}
	delete(m.store, wonka.CanonicalEntityName(name))
	return nil
}

func (m *mockEntityDB) serializeEntity(e *wonka.Entity) ([]byte, error) {
	return json.Marshal(e)
}

func (m *mockEntityDB) deserializeEntity(b []byte) (*wonka.Entity, error) {
	e := new(wonka.Entity)
	if err := json.Unmarshal(b, e); err != nil {
		return nil, err
	}
	return e, nil
}
