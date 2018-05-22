package main

import (
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
)

type dbConfig struct {
	User     map[string]string `yaml:"auto_user_of_database"`
	Password map[string]string `yaml:"password_of_user"`
}

type masterKey struct {
	PrivatePem string `yaml:"private_dot_pem"`
}

// UserCA is the public key of the user cert signer
type UserCA struct {
	TrustedUserCa string `yaml:"trusted_user_ca"`
}

// HostCA is the public key of the host cert signer.
type HostCA struct {
	SSHKnownHosts string `yaml:"ssh_known_hosts"`
}

type appConfig struct {
	Verbose        bool
	Nemo           dbConfig                `yaml:"nemo"`
	WonkaMasterKey masterKey               `yaml:"wonkamasterkey"`
	Port           int                     `yaml:"port"`
	DBHost         string                  `yaml:"dbhost"`
	DBPort         int                     `yaml:"dbport"`
	Cassandra      wonkadb.CassandraConfig `yaml:"cassandra"`
	// PulloConfig is a static listing of group memberships. If this is set,
	// the real pullo is not consulted at all. This is probalby only useful
	// for testing wonkamaster locally.
	PulloConfig       map[string][]string        `yaml:"pulloconfig"`
	Impersonators     []string                   `yaml:"impersonators"`
	UsshHostSigner    HostCA                     `yaml:"ussh_host_signer"`
	UsshUserSigner    UserCA                     `yaml:"ussh_user_signer"`
	Derelicts         map[string]string          `yaml:"derelicts"`
	Launchers         map[string]common.Launcher `yaml:"launchers"`
	HoseCheckInterval int                        `yaml:"hose_check_interval"`
}
