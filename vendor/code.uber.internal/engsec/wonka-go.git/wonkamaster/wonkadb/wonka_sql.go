package wonkadb

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/uber-go/tally"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"go.uber.org/zap"
)

// TODO(pmoody): investigate with auditors if we can get rid of the claims table.

// WonkaSQL implements the WonkaDB interface for interacting with percona.
type WonkaSQL struct {
	*sql.DB

	entities map[string]wonka.Entity
	entityMu *sync.RWMutex
	updated  time.Time

	claims   map[string]wonka.ClaimRequest
	claimsMu *sync.Mutex

	log     *zap.Logger
	metrics tally.Scope
}

const (
	gDBType = "mysql"
)

var (
	selectEntities = "SELECT entity_name,location,requires,created_on,expires_on,version,algo,pubkey,ecckey,sigtype,entity_signature FROM wonkamaster.entities"
)

// NewWonkaSQL creates a new sql-backed wonka db
func NewWonkaSQL(gDBC string, ms tally.Scope) (WonkaDB, error) {
	db, err := sql.Open(gDBType, gDBC)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(time.Minute)

	gDB := &WonkaSQL{
		DB:       db,
		entities: make(map[string]wonka.Entity, 1),
		entityMu: &sync.RWMutex{},
		claims:   make(map[string]wonka.ClaimRequest, 1),
		claimsMu: &sync.Mutex{},
		log:      zap.L(),
		metrics:  ms,
	}

	go gDB.periodicEntityUpdate(10 * time.Second)

	return WonkaDB(gDB), nil
}

// TODO(pmoody): investigate using something like cronos or redis for this.
func (w *WonkaSQL) periodicEntityUpdate(period time.Duration) {
	for {
		w.updateEntityCache()
		time.Sleep(period)
	}
}

func (w *WonkaSQL) updateEntityCache() {
	w.log.Debug("updating entity cache")

	stopwatch := w.metrics.Tagged(
		map[string]string{
			"action": "update_entity_cache",
		},
	).Timer("dbaccess").Start()
	rows, err := w.Query(selectEntities)
	stopwatch.Stop()
	if err != nil {
		w.log.Error("error refreshing entity cache", zap.Error(err))
		return
	}

	entities := make(map[string]wonka.Entity, 1)
	for rows.Next() {
		var e wonka.Entity
		err := rows.Scan(&e.EntityName, &e.Location, &e.Requires, &e.Ctime,
			&e.Etime, &e.Version, &e.Algo, &e.PublicKey, &e.ECCPublicKey,
			&e.SigType, &e.EntitySignature)
		if err != nil {
			w.log.Error("error scanning row", zap.Error(err))
			continue
		}
		entities[e.EntityName] = e
	}

	w.entityMu.Lock()
	w.entities = entities
	w.entityMu.Unlock()

	w.updated = time.Now()
	w.log.Debug("entity cache updated", zap.Int("len", len(w.entities)))
}

func dbCommit(tx *sql.Tx, err error) error {
	if err == nil {
		return tx.Commit()
	}
	return tx.Rollback()
}

// GetEntity returns a wonka.Entity associated with the name.
func (w *WonkaSQL) GetEntity(name string) *wonka.Entity {
	w.entityMu.RLock()
	e, ok := w.entities[name]
	w.entityMu.RUnlock()
	if !ok {
		return nil
	}
	return &e
}

func (w *WonkaSQL) getIdsForGroups(groupNames []string) (map[string]int, error) {
	groups := w.GetGroupsByName(groupNames)
	if len(groups) == 0 {
		return nil, fmt.Errorf("no such groups %s", groupNames)
	}
	ids := make(map[string]int, len(groupNames))
	for _, g := range groups {
		ids[g.GroupName] = int(g.ID)
	}
	return ids, nil
}

// CreateEntity stores the wonka.Entity in the database.
func (w *WonkaSQL) CreateEntity(e wonka.Entity) bool {
	// Prepare the SQL statement
	stmt, err := w.Prepare("INSERT INTO wonkamaster.entities SET entity_name=?," +
		"location=?,requires=?,is_locked=?,created_on=?,expires_on=?,version=?,algo=?," +
		"keybits=?,pubkey=?,ecckey=?,sigtype=?,entity_signature=?")
	if err != nil {
		w.log.Error("error preparing sql statment", zap.Error(err))
		return false
	}
	defer stmt.Close()

	// Execute INSERT
	stopwatch := w.metrics.Tagged(
		map[string]string{
			"action": "create_entity",
		},
	).Timer("dbaccess").Start()
	if _, err = stmt.Exec(e.EntityName, e.Location, e.Requires, e.IsLocked, e.Ctime,
		e.Etime, e.Version, e.Algo, e.KeyBits, e.PublicKey, e.ECCPublicKey, e.SigType,
		e.EntitySignature); err != nil {
		w.log.Error("error executing stmt", zap.Error(err))
		stopwatch.Stop()
		return false
	}
	stopwatch.Stop()

	w.updateEntityCache()

	gid, err := w.getIdsForGroups([]string{wonka.EveryEntity})
	if err != nil {
		return true
	}

	if len(gid) != 1 {
		w.log.Error("wrong number of ids for EVERYONE group", zap.Int("id_count", len(gid)))
		// return here?
	}

	// everyone is a member of everyone
	// TODO(pmoody): make an insert group method?
	stopwatch = w.metrics.Tagged(
		map[string]string{
			"action": "create_entity_group_insert",
		},
	).Timer("dbaccess").Start()
	defer stopwatch.Stop()
	_, err = w.Query("INSERT INTO wonkamaster.members (group_id, entity_name, created_on, expires_on, is_enabled) "+
		"VALUES (?, ?, ?, ?, 1)", gid[wonka.EveryEntity], e.EntityName, time.Now().Unix(), e.ExpireTime)
	if err != nil {
		w.log.Error("error adding entity to EVERYONE group", zap.Error(err), zap.Any("entity", e.EntityName))
	}

	return true
}

// LookupMemberInGroup returns true if the named user is a member of the named group.
func (w *WonkaSQL) LookupMemberInGroup(groupName, entityName string) (int, error) {
	// TODO: Add a check for local/cached entries before going out to the DB (PERF)
	// Query group row entry by name
	g := w.GetGroupsByName([]string{groupName})
	if g == nil {
		return 0, fmt.Errorf("no such groups")
	}
	// Query member row entry by group_id + entity_name
	stopwatch := w.metrics.Tagged(
		map[string]string{
			"action": "lookup_member_in_group",
		},
	).Timer("dbaccess").Start()
	row := w.QueryRow("SELECT id FROM wonkamaster.members WHERE group_id=? and entity_name=?",
		g[0].ID, entityName)
	stopwatch.Stop()
	if row == nil {
		return 0, fmt.Errorf("no such membership")
	}

	// Scan at least one matched row ID or fail
	var ID int64
	err := row.Scan(&ID)
	return int(ID), err
}

// DeleteEntity deletes an entity from the entity database.
func (w *WonkaSQL) DeleteEntity(e wonka.Entity) bool {
	w.log.Info("deleting entity", zap.Any("entity", e.EntityName))
	stmt, err := w.Prepare("DELETE FROM wonkamaster.entities WHERE entity_name=?")
	if err != nil {
		w.log.Error("error preparing delete", zap.Error(err))
		return false
	}
	defer stmt.Close()
	stopwatch := w.metrics.Tagged(
		map[string]string{
			"action": "delete_entity",
		},
	).Timer("dbaccess").Start()
	defer stopwatch.Stop()
	if _, err := stmt.Exec(e.EntityName); err != nil {
		w.log.Error("error executing delete", zap.Error(err))
		return false
	}
	return true
}

// UpdateEntity updates the information stored for a particular entity.
// It currently only allows updating the location and the requires fields.
func (w *WonkaSQL) UpdateEntity(e wonka.Entity) bool {
	stmt, err := w.Prepare("UPDATE wonkamaster.entities " +
		"SET location=?,requires=? " +
		"WHERE entity_name=?")
	if err != nil {
		w.log.Error("error preparing sql statement", zap.Error(err))
		return false
	}
	defer stmt.Close()
	stopwatch := w.metrics.Tagged(
		map[string]string{
			"action": "update_entity",
		},
	).Timer("dbaccess").Start()
	defer stopwatch.Stop()
	if _, err = stmt.Exec(e.Location, e.Requires, e.EntityName); err != nil {
		w.log.Error("error running sql statement", zap.Error(err))
		return false
	}

	go w.updateEntityCache()
	return true
}

// GetGroupsByName returns a slice for WONKAGroup's for the given list of groupNames
func (w *WonkaSQL) GetGroupsByName(groupNames []string) []WONKAGroup {
	q := fmt.Sprintf("SELECT * FROM wonkamaster.groups WHERE group_name IN (?%s)",
		strings.Repeat(",?", len(groupNames)-1))

	// convert []string{} to []interface{}
	args := make([]interface{}, len(groupNames))
	for idx, val := range groupNames {
		args[idx] = val
	}

	stopwatch := w.metrics.Tagged(
		map[string]string{
			"action": "get_groups_by_name",
		},
	).Timer("dbaccess").Start()
	rows, err := w.Query(q, args...)
	stopwatch.Stop()
	if err != nil {
		return nil
	}
	defer rows.Close()

	var groups []WONKAGroup
	for rows.Next() {
		var g WONKAGroup
		// Scan the row fields into the output variable
		if err := rows.Scan(&g.ID, &g.GroupName, &g.Description, &g.Owner,
			&g.CreatedOn, &g.ExpiresOn, &g.IsEnabled); err != nil {
			w.log.Error("error scanning row", zap.Error(err))
			continue
		}
		groups = append(groups, g)
	}
	return groups
}

// IsConnected returns true if the connection is still up.
func (w *WonkaSQL) IsConnected() bool {
	return w.Ping() == nil
}

// AddGroups inserts new groups into the groups table.
func (w *WonkaSQL) AddGroups(newGroups []string) (err error) {
	stopwatch := w.metrics.Tagged(
		map[string]string{
			"action": "add_groups",
		},
	).Timer("dbaccess").Start()
	defer stopwatch.Stop()
	if len(newGroups) == 0 {
		err = nil
		return err
	}

	tx, err := w.Begin()
	if err != nil {
		return err
	}
	defer dbCommit(tx, err)

	insert := fmt.Sprintf(
		"INSERT INTO wonkamaster.groups (group_name, owner_id, created_on, is_enabled) VALUES (?,?,?,?)%s",
		strings.Repeat(",(?,?,?,?)", len(newGroups)-1))

	var args []interface{}
	now := time.Now().Unix()
	for _, g := range newGroups {
		args = append(args, []interface{}{g, "wonkamaster", now, 1}...)
	}

	if _, err := tx.Exec(insert, args...); err != nil {
		w.log.Error("exec", zap.Error(err))
		return err
	}
	return nil
}

// SetMembershipsForEntity explicitly sets the group memberships for an entity.
func (w *WonkaSQL) SetMembershipsForEntity(entity string, gids []int) (err error) {
	stopwatch := w.metrics.Tagged(
		map[string]string{
			"action": "set_memberships_for_entity_delete_memberships",
		},
	).Timer("dbaccess").Start()
	defer stopwatch.Stop()
	if entity == "" || len(gids) == 0 {
		return nil
	}
	// find what membership entries already exist.
	tx, err := w.Begin()
	if err != nil {
		w.log.Error("begin", zap.Error(err))
		return err
	}
	defer dbCommit(tx, err)

	del := "DELETE FROM wonkamaster.members WHERE entity_name = ?"
	_, err = tx.Exec(del, entity)
	if err != nil {
		return fmt.Errorf("removing old memberships: %v", err)
	}

	var args []interface{}
	now := time.Now().Unix()
	for _, gid := range gids {
		// Give autocreated groups infinite expiration by default
		args = append(args, []interface{}{gid, entity, now, int(0), 1}...)
	}

	newGids := fmt.Sprintf(
		"INSERT INTO wonkamaster.members (group_id, entity_name, created_on, expires_on, is_enabled) VALUES (?,?,?,?,?)%s",
		strings.Repeat(",(?,?,?,?,?)", len(gids)-1))
	_, err = tx.Exec(newGids, args...)
	if err != nil {
		return fmt.Errorf("adding new groups: %v", err)
	}
	return nil
}

// SetMembershipGroupNamesForEntity does
func (w *WonkaSQL) SetMembershipGroupNamesForEntity(entity string, groups []string) error {
	gids, err := w.GetIdsForGroups(groups)
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
		if err := w.AddGroups(toAdd); err != nil {
			return fmt.Errorf("adding %d groups: %v", len(toAdd), err)
		}
	}

	groupNames, err := w.GetIdsForGroups(groups)
	if err != nil {
		return fmt.Errorf("updated gids: %v", err)
	}

	var groupIds []int
	for _, v := range groupNames {
		groupIds = append(groupIds, v)
	}
	return w.SetMembershipsForEntity(entity, groupIds)
}

// GetIdsForGroups does
func (w *WonkaSQL) GetIdsForGroups(groupNames []string) (map[string]int, error) {
	groups := w.GetGroupsByName(groupNames)
	if len(groups) == 0 {
		return nil, fmt.Errorf("no such groups %s", groupNames)
	}
	ids := make(map[string]int, len(groupNames))
	for _, g := range groups {
		ids[g.GroupName] = int(g.ID)
	}
	return ids, nil
}
