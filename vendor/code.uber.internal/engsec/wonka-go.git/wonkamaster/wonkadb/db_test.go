package wonkadb

import (
	"testing"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"github.com/stretchr/testify/require"
)

func TestLookup(t *testing.T) {
	e := wonka.Entity{}
	m := mockdb{
		entities: map[string]wonka.Entity{"test": e},
	}
	db := WonkaDB(m)

	entity := db.GetEntity("test")
	require.True(t, entity != nil)
}

func TestGetIds(t *testing.T) {
	db := CreateWonkaDB()

	_, e := db.GetIdsForGroups([]string{})
	require.Error(t, e, "empty search should error")
	require.Contains(t, e.Error(), "no such groups")

	groups, e := db.GetIdsForGroups([]string{"g1", "g2"})
	require.NoError(t, e, "getting groups shouldn't error")
	require.Equal(t, 2, len(groups), "should be two groups")
}

func TestSetMemberships(t *testing.T) {
	db := CreateWonkaDB()

	_, err := db.LookupMemberInGroup("g1", "e1")
	require.NoError(t, err, "e1 should be a memeber of g1")
	_, err = db.LookupMemberInGroup("g2", "e1")
	require.NoError(t, err, "e2 should be a member g1")
	_, err = db.LookupMemberInGroup("g3", "e1")
	require.Error(t, err, "e1 shouldn't be a member d3")

	e := db.SetMembershipGroupNamesForEntity("e1", []string{"g1"})
	require.NoError(t, e, "setmemberships shouldn't error")

	_, err = db.LookupMemberInGroup("g1", "e1")
	require.NoError(t, err, "e1 should be a memeber g1, round 2")
	_, err = db.LookupMemberInGroup("g2", "e1")
	require.Error(t, err, "e1 should be a member g2, round 2")
	_, err = db.LookupMemberInGroup("g3", "e1")
	require.Error(t, err, "e1 shouldn't be a member g3, round 2")
}
