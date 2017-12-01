package wonkadb

import (
	"context"
	"os"
	"strconv"
	"testing"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var integrationTestMode = os.Getenv("WONKA_INTEGRATION_TEST") != ""

var testEntityFoo = &wonka.Entity{
	EntityName:      "wonkaSample:foo",
	Requires:        "EVERYONE",
	ECCPublicKey:    "foo.pub",
	SigType:         "SHA256",
	EntitySignature: "foo.sig",
	PublicKey:       "foo.pub.rsa",
	Ctime:           1500000000,
}
var testEntityFoo2 = &wonka.Entity{
	EntityName: "WONKASAMPLE:FOO",
}

var tCtx = context.TODO()

func TestInit(t *testing.T) {
	db := newTestCassandraDB(t)
	defer db.Close()
}

// set up a new test db
func newTestCassandraDB(t *testing.T) EntityDB {
	var err error
	db := NewMockEntityDB()
	if integrationTestMode {
		db, err = NewCassandra(CassandraConfig{
			Hosts:    []string{"127.0.0.1"},
			Port:     dockerCassandraPortOrDefault(),
			Keyspace: "test",
			Username: "cassandra",
			Password: "cassandra",
		})
	}
	require.NoError(t, err)
	cleanTestDB(t, db)
	return db
}

func dockerCassandraPortOrDefault() int {
	sPort := os.Getenv("DOCKER_CASSANDRA_PORT")
	if sPort == "" {
		return 9042
	}
	port, err := strconv.Atoi(sPort)
	if err != nil {
		return 9042
	}
	return port
}

// delete records from prior test runs
func cleanTestDB(t *testing.T, db EntityDB) {
	db.Delete(tCtx, testEntityFoo.Name())
	db.Delete(tCtx, testEntityFoo2.Name())
}

func TestCreate(t *testing.T) {
	db := newTestCassandraDB(t)
	defer db.Close()
	newCreatedTestEntity(t, db)
}

func TestCreateExists(t *testing.T) {
	db := newTestCassandraDB(t)
	defer db.Close()
	newCreatedTestEntity(t, db)
	err := db.Create(tCtx, testEntityFoo)
	assert.EqualError(t, err, ErrExists.Error())
}

func TestCreateExistsCaseInsensitive(t *testing.T) {
	db := newTestCassandraDB(t)
	defer db.Close()
	newCreatedTestEntity(t, db)
	err := db.Create(tCtx, testEntityFoo2)
	assert.EqualError(t, err, ErrExists.Error())
}

// create a new test entity
func newCreatedTestEntity(t *testing.T, db EntityDB) *wonka.Entity {
	err := db.Create(tCtx, testEntityFoo)
	require.NoError(t, err)
	return testEntityFoo
}

func TestGet(t *testing.T) {
	db := newTestCassandraDB(t)
	defer db.Close()
	e := newCreatedTestEntity(t, db)
	foo, err := db.Get(tCtx, e.EntityName)
	assert.NoError(t, err)
	assert.Equal(t, e.EntityName, foo.EntityName)
	assert.Equal(t, e.Requires, foo.Requires)
	assert.Equal(t, e.ECCPublicKey, foo.ECCPublicKey)
	assert.Equal(t, e.SigType, foo.SigType)
	assert.Equal(t, e.EntitySignature, foo.EntitySignature)
	assert.Equal(t, e.PublicKey, foo.PublicKey)
	assert.Equal(t, e.Ctime, foo.Ctime)
}

func TestGetNotFound(t *testing.T) {
	db := newTestCassandraDB(t)
	defer db.Close()
	foo, err := db.Get(tCtx, testEntityFoo.EntityName)
	assert.EqualError(t, err, ErrNotFound.Error())
	assert.Nil(t, foo)
}

func TestDelete(t *testing.T) {
	db := newTestCassandraDB(t)
	defer db.Close()
	e := newCreatedTestEntity(t, db)
	err := db.Delete(tCtx, e.EntityName)
	assert.NoError(t, err)
}

func TestDeleteNotFound(t *testing.T) {
	db := newTestCassandraDB(t)
	defer db.Close()
	err := db.Delete(tCtx, "foo")
	assert.EqualError(t, err, ErrNotFound.Error())
}

func TestUpdate(t *testing.T) {
	db := newTestCassandraDB(t)
	defer db.Close()
	e := newCreatedTestEntity(t, db)
	newUpdatedEntity := &wonka.Entity{
		EntityName: "wonkaSample:foo",
		Requires:   "no one",
	}
	err := db.Update(tCtx, newUpdatedEntity)
	assert.NoError(t, err)
	foo, err := db.Get(tCtx, e.EntityName)
	assert.NoError(t, err)
	assert.Equal(t, "no one", foo.Requires)
}

func TestUpdateNotFound(t *testing.T) {
	db := newTestCassandraDB(t)
	defer db.Close()
	err := db.Update(tCtx, testEntityFoo)
	assert.EqualError(t, err, ErrNotFound.Error())
}

func TestCantUpdatePublicKey(t *testing.T) {
	db := newTestCassandraDB(t)
	defer db.Close()
	e := newCreatedTestEntity(t, db)
	newUpdatedEntity := &wonka.Entity{
		EntityName:   "wonkaSample:foo",
		ECCPublicKey: "new.pub",
	}
	err := db.Update(tCtx, newUpdatedEntity)
	assert.NoError(t, err)
	foo, err := db.Get(tCtx, e.EntityName)
	assert.NoError(t, err)
	assert.NotEqual(t, "new.pub", foo.ECCPublicKey)
}
