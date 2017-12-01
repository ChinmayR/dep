CREATE KEYSPACE IF NOT EXISTS wonkamaster WITH REPLICATION = {'class': 'NetworkTopologyStrategy', 'dca1': '3', 'sjc1': '3'} AND DURABLE_WRITES = true;

CREATE TABLE IF NOT EXISTS wonkamaster.entities (
    name text,
    requires text,
    public_key text,
    signature_type text,
    signature text,
    PRIMARY KEY (name)
);
