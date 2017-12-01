package wonkadb

import (
	"context"
	"fmt"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"github.com/gocql/gocql"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

const (
	// LOCAL_QUORUM provides strong consistency on writes within a datacenter.
	defaultConsistencyLevel = gocql.LocalQuorum
	// LOCAL_SERIAL achieves linearizable consistency for lightweight transactions,
	// but confined within a single data center.
	defaultSerialConsistencyLevel = gocql.LocalSerial
	// current CQL spec version used by production CaaS hosts
	defaultCQLVersion = "3.4.0"
	// default request timeout (a conservative value above the CaaS p99 100ms SLA)
	defaultRequestTimeout = 500 * time.Millisecond
	// default number of times to retry queries
	defaultNumRetries = 3
	// default initial sleep between retries
	defaultInitialRetrySleep = 100 * time.Millisecond

	readEntityStmt   = "SELECT name, requires, public_key, signature_type, signature, rsa_public_key_old, created_at FROM entities WHERE name = ?"
	createEntityStmt = "INSERT INTO entities (name, requires, public_key, signature_type, signature, rsa_public_key_old, created_at) VALUES (?, ?, ?, ?, ?, ?, ?) IF NOT EXISTS"
	updateEntityStmt = "UPDATE entities SET requires = ? WHERE name = ? IF EXISTS"
	deleteEntityStmt = "DELETE FROM entities WHERE name = ? IF EXISTS"
)

// default retry policy is to use exponential backoff
var defaultRetryPolicy = &gocql.ExponentialBackoffRetryPolicy{
	NumRetries: defaultNumRetries,
	Min:        defaultInitialRetrySleep,
}

// default pool config is to use a token aware policy
// this means we try to execute a query based on the host who owns the partition
// if no routing information is available, we use the round robin default
var defaultPoolConfig = gocql.PoolConfig{
	HostSelectionPolicy: gocql.TokenAwareHostPolicy(gocql.RoundRobinHostPolicy()),
}

// CassandraConfig allows for customization of the Cassandra database.
// TODO(tjulian): replace Hosts/Port with UNS path.
type CassandraConfig struct {
	// Hosts is a list of Cassandra nodes to connect to.
	Hosts []string `yaml:"hosts"`
	// Port is the node port to use when connecting.
	Port int `yaml:"port"`
	// Keyspace is the Cassandra keyspace to use for database operations.
	Keyspace string `yaml:"keyspace"`
	// Username authenticates the user.
	Username string `yaml:"username"`
	// Password authenticates the user.
	Password string `yaml:"password"`

	Logger  *zap.Logger `yaml:"-"`
	Metrics tally.Scope `yaml:"-"`
}

// NewCassandra returns a new EntityDB using C* as the underlying data source.
func NewCassandra(cfg CassandraConfig) (EntityDB, error) {
	cluster := gocql.NewCluster(cfg.Hosts...)
	cluster.Port = cfg.Port
	cluster.Keyspace = cfg.Keyspace
	cluster.Consistency = defaultConsistencyLevel
	cluster.SerialConsistency = defaultSerialConsistencyLevel
	cluster.CQLVersion = defaultCQLVersion
	cluster.RetryPolicy = defaultRetryPolicy
	cluster.Timeout = defaultRequestTimeout
	cluster.PoolConfig = defaultPoolConfig
	cluster.Authenticator = newAuthenticator(cfg.Username, cfg.Password)
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("error setting up cassandra: %v", err)
	}
	return &cassandra{
		session: session,
		log:     loggerOrDefault(cfg.Logger),
		metrics: metricsOrDefault(cfg.Metrics),
	}, nil
}

func newAuthenticator(username, password string) gocql.Authenticator {
	if username == "" || password == "" {
		return nil
	}
	return gocql.PasswordAuthenticator{
		Username: username,
		Password: password,
	}
}

func loggerOrDefault(log *zap.Logger) *zap.Logger {
	if log == nil {
		return zap.L()
	}
	return log
}

func metricsOrDefault(metrics tally.Scope) tally.Scope {
	if metrics == nil {
		return tally.NoopScope
	}
	return metrics.SubScope("entitydb")
}

type cassandra struct {
	session *gocql.Session
	log     *zap.Logger
	metrics tally.Scope
}

func (c *cassandra) Close() error {
	c.session.Close()
	return nil
}

func (c *cassandra) Get(ctx context.Context, rawName string) (*wonka.Entity, error) {
	defer c.instrument("get")()
	e := new(wonka.Entity)
	name := wonka.CanonicalEntityName(rawName)
	if err := c.session.Query(readEntityStmt, name).WithContext(ctx).Scan(&e.EntityName, &e.Requires, &e.ECCPublicKey, &e.SigType, &e.EntitySignature, &e.PublicKey, &e.Ctime); err != nil {
		if err == gocql.ErrNotFound {
			c.log.Warn("get entity", zap.Error(err), zap.String("rawName", rawName), zap.String("canonicalName", name))
			return nil, ErrNotFound
		}
		c.log.Error("get entity", zap.Error(err))
		return nil, err
	}
	// This fixes an issue where we return an entity with a different name than the one provided
	// e.g. db.Get("TyLeR") returns &wonka.Entity{EntityName: "tyler"}
	// TODO(tjulian): remove this when entity names are canonicalized across the entire project
	e.EntityName = rawName
	return e, nil
}

func (c *cassandra) instrument(id string) func() {
	c.metrics.Counter(id).Inc(1)
	start := time.Now()
	return func() {
		c.metrics.Timer(id).Record(time.Now().Sub(start))
	}
}

func (c *cassandra) Create(ctx context.Context, e *wonka.Entity) error {
	defer c.instrument("create")()
	applied, err := c.session.Query(createEntityStmt, e.Name(), e.Requires, e.ECCPublicKey, e.SigType, e.EntitySignature, e.PublicKey, e.Ctime).WithContext(ctx).ScanCAS()
	if !applied {
		err = ErrExists
	}
	if err != nil {
		c.log.Error("create entity", zap.Error(err))
		return err
	}
	return nil
}

func (c *cassandra) Update(ctx context.Context, e *wonka.Entity) error {
	defer c.instrument("update")()
	applied, err := c.session.Query(updateEntityStmt, e.Requires, e.Name()).WithContext(ctx).ScanCAS()
	if !applied {
		err = ErrNotFound
	}
	if err != nil {
		c.log.Error("update entity", zap.Error(err))
		return err
	}
	return nil
}

func (c *cassandra) Delete(ctx context.Context, rawName string) error {
	defer c.instrument("delete")()
	name := wonka.CanonicalEntityName(rawName)
	applied, err := c.session.Query(deleteEntityStmt, name).WithContext(ctx).ScanCAS()
	if !applied {
		err = ErrNotFound
	}
	if err != nil {
		c.log.Error("delete entity", zap.Error(err))
		return err
	}
	return nil
}
