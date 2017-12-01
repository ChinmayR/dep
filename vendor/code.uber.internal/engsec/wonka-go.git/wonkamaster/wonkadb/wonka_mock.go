package wonkadb

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"

	wonka "code.uber.internal/engsec/wonka-go.git"
)

// the members entry is a little odd here. it's a map whose key is the group id
// and the value is another map. the second map has a key of the entity name and
// the value of an empty struct. this allows for O(1) searching/deleting the
// members table.
type mockdb struct {
	entities map[string]wonka.Entity
	groups   map[string]WONKAGroup
	members  map[int]map[string]struct{}
}

// Connected always returns true for the mockdb
func (mockdb) IsConnected() bool { return true }

// DeleteEntity deletes an entity from the mockdb
func (m mockdb) DeleteEntity(e wonka.Entity) bool {
	delete(m.entities, e.EntityName)
	for _, g := range m.members {
		delete(g, e.EntityName)
	}
	return true
}

// GetEntity returns an entity from the mockdb.
func (m mockdb) GetEntity(name string) *wonka.Entity {
	if name == "" {
		return nil
	}

	e, ok := m.entities[name]
	if !ok {
		return nil
	}
	return &e
}

// GetGroupsByName returns list the of WONKAGroup's from the mockdb for the list
// groupNames provided.
func (m mockdb) GetGroupsByName(groupNames []string) []WONKAGroup {
	var groups []WONKAGroup
	for _, group := range groupNames {
		if g, ok := m.groups[group]; ok {
			groups = append(groups, g)
		}
	}
	return groups
}

// CreateEntity does nothing at this point.
func (m mockdb) CreateEntity(e wonka.Entity) bool {
	m.entities[e.EntityName] = e
	return true
}

// AddGroups does nothing at this point.
func (m mockdb) AddGroups(newGroups []string) error {
	for _, g := range newGroups {
		if _, ok := m.groups[g]; !ok {
			gid, e := rand.Int(rand.Reader, big.NewInt(2048))
			if e != nil {
				return e
			}
			m.groups[g] = WONKAGroup{ID: int64(gid.Uint64()), GroupName: g}
		}
	}
	return nil
}

// SetMembershipsForEntity explicitly sets the memberships for the entity named by entity.
func (m mockdb) SetMembershipsForEntity(entity string, gids []int) error {
	_, ok := m.entities[entity]
	if !ok {
		return fmt.Errorf("no such entity")
	}

	for _, gMembers := range m.members {
		delete(gMembers, entity)
	}

	for _, gid := range gids {
		if _, ok := m.members[gid]; !ok {
			m.members[gid] = make(map[string]struct{})
		}
		m.members[gid][entity] = struct{}{}
	}

	return nil
}

// UpdateEntity does nothing at this point.
func (m mockdb) UpdateEntity(e wonka.Entity) bool {
	m.entities[e.EntityName] = e
	return true
}

// LookupMemberInGroup returns the member id number if the entity named by entityName
// is a member of the group named by groupName.
func (m mockdb) LookupMemberInGroup(groupName, entityName string) (int, error) {
	g, ok := m.groups[groupName]
	if !ok {
		return 0, fmt.Errorf("no such group")
	}
	gid := int(g.ID)

	group, ok := m.members[gid]
	if !ok {
		return 0, fmt.Errorf("group has no members")
	}

	if _, ok := group[entityName]; ok {
		return gid, nil
	}

	return 0, fmt.Errorf("no such membership")
}

func (m mockdb) SetMembershipGroupNamesForEntity(entity string, groups []string) error {
	gids, err := m.GetIdsForGroups(groups)
	if err != nil || gids == nil || len(gids) == 0 {
		return err
	}

	var toAdd []string
	for _, g := range groups {
		if _, ok := gids[g]; !ok {
			toAdd = append(toAdd, g)
		}
	}

	if len(toAdd) > 0 {
		if err := m.AddGroups(toAdd); err != nil {
			return fmt.Errorf("adding %d groups: %v", len(toAdd), err)
		}
	}

	groupNames, err := m.GetIdsForGroups(groups)
	if err != nil {
		return fmt.Errorf("updated gids: %v", err)
	}

	var groupIds []int
	for _, v := range groupNames {
		groupIds = append(groupIds, v)
	}
	return m.SetMembershipsForEntity(entity, groupIds)
}

func (m mockdb) GetIdsForGroups(groupNames []string) (map[string]int, error) {
	groups := m.GetGroupsByName(groupNames)
	if len(groups) == 0 {
		return nil, fmt.Errorf("no such groups %s", groupNames)
	}
	ids := make(map[string]int, len(groupNames))
	for _, g := range groups {
		ids[g.GroupName] = int(g.ID)
	}
	return ids, nil
}

func membersFromEntities(uids []wonka.Entity) map[string]struct{} {
	m := make(map[string]struct{})
	for _, uid := range uids {
		m[uid.EntityName] = struct{}{}
	}
	return m
}

// CreateWonkaDB creates a mock db
func CreateWonkaDB() WonkaDB {
	e1 := wonka.Entity{EntityName: "e1"}
	e2 := wonka.Entity{EntityName: "e2"}
	e3 := wonka.Entity{EntityName: "admin"}

	g1 := WONKAGroup{ID: 1, GroupName: "g1"}
	g2 := WONKAGroup{ID: 2, GroupName: "g2"}
	g3 := WONKAGroup{ID: 3, GroupName: "WONKA_ADMINS"}
	g4 := WONKAGroup{ID: 4, GroupName: "EVERYONE"}

	m := mockdb{
		entities: map[string]wonka.Entity{
			e1.EntityName: e1,
			e2.EntityName: e2,
			e3.EntityName: e3},
		groups: map[string]WONKAGroup{
			g1.GroupName: g1,
			g2.GroupName: g2,
			g3.GroupName: g3,
			g4.GroupName: g4},
		members: map[int]map[string]struct{}{
			1: membersFromEntities([]wonka.Entity{e1, e2, e3}),
			2: membersFromEntities([]wonka.Entity{e1, e2, e3}),
			3: membersFromEntities([]wonka.Entity{e3})},
	}

	return WonkaDB(m)
}

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
