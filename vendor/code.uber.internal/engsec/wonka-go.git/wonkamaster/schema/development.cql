CREATE KEYSPACE IF NOT EXISTS development WITH REPLICATION = {'class': 'SimpleStrategy', 'replication_factor': '1'} AND DURABLE_WRITES = true;
CREATE KEYSPACE IF NOT EXISTS test WITH REPLICATION = {'class': 'SimpleStrategy', 'replication_factor': '1'} AND DURABLE_WRITES = true;

CREATE TABLE IF NOT EXISTS development.entities (
    name text,
    requires text,
    public_key text,
    signature_type text,
    signature text,
    rsa_public_key_old text,
    created_at bigint,
    PRIMARY KEY (name)
);

CREATE TABLE IF NOT EXISTS test.entities (
    name text,
    requires text,
    public_key text,
    signature_type text,
    signature text,
    rsa_public_key_old text,
    created_at bigint,
    PRIMARY KEY (name)
);
