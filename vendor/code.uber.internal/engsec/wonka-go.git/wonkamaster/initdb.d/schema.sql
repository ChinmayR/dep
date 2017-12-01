drop table if exists entities;
CREATE TABLE entities (
  entity_name VARCHAR(127) NOT NULL PRIMARY KEY UNIQUE,
  location VARCHAR(256) NOT NULL,
  requires VARCHAR(256) NOT NULL,
  is_locked tinyint(1) NOT NULL,
  created_on BIGINT(1) NOT NULL,
  expires_on BIGINT(1) NOT NULL,
  version VARCHAR(32) NOT NULL,
  algo VARCHAR(32) NOT NULL,
  keybits INT NOT NULL,
  pubkey VARCHAR(4096) NOT NULL,
  ecckey VARCHAR(128) NOT NULL,
  sigtype VARCHAR(32) NOT NULL,
  entity_signature VARCHAR(1024) NOT NULL
)

#DROP TABLE IF EXISTS groups;
#CREATE TABLE groups (
#  id BIGINT(1) NOT NULL PRIMARY KEY AUTO_INCREMENT,
#  group_name VARCHAR(256) NOT NULL,
#  description VARCHAR(512) NOT NULL,
#  owner_id VARCHAR(256) NOT NULL,
#  created_on BIGINT(1) NOT NULL,
#  expires_on BIGINT(1) NOT NULL,
#  is_enabled TINYINT(1) NOT NULL
#)
#
#DROP TABLE IF EXISTS claims;
#CREATE TABLE claims (
#  id BIGINT(1) NOT NULL PRIMARY KEY AUTO_INCREMENT,
#  claim_group_id BIGINT(1) NOT NULL,
#  claim_member_id BIGINT(1) NOT NULL,
#  created_at DATETIME NOT NULL,
#  valid_until DATETIME NOT NULL,
#  claim_token VARCHAR(256) NOT NULL
#)
#
