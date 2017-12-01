CREATE TABLE entities (
  entity_name VARCHAR(127) NOT NULL PRIMARY KEY,
  location VARCHAR(256) NOT NULL,
  requires VARCHAR(256) NOT NULL,
  is_locked tinyint(1) NOT NULL,
  created_on BIGINT(1) NOT NULL,
  expires_on BIGINT(1) NOT NULL,
  version VARCHAR(32) NOT NULL,
  algo VARCHAR(32) NOT NULL,
  keybits INT(1) NOT NULL,
  pubkey VARCHAR(4096) NOT NULL,
  ecckey VARCHAR(128) NOT NULL,
  sigtype VARCHAR(32) NOT NULL,
  entity_signature VARCHAR(1024) NOT NULL
);

CREATE TABLE groups (
  id INT NOT NULL AUTO_INCREMENT,
  group_name VARCHAR(256) NOT NULL,
  description VARCHAR(512) NOT NULL,
  owner_id VARCHAR(256) NOT NULL,
  created_on BIGINT(1) NOT NULL,
  expires_on BIGINT(1) NOT NULL,
  is_enabled TINYINT(1) NOT NULL,
  PRIMARY KEY(id)
);

CREATE TABLE claims (
  id INT NOT NULL AUTO_INCREMENT,
  claim_group_id BIGINT(1) NOT NULL,
  claim_member_id BIGINT(1) NOT NULL,
  created_at DATETIME NOT NULL,
  valid_until DATETIME NOT NULL,
  claim_token VARCHAR(1024) NOT NULL,
  entity varchar(256),
  PRIMARY KEY(id)
);
