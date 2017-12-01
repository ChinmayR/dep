// Package envfx provides information about the running service's environment.
package envfx

import (
	"bytes"
	"io/ioutil"
	"os"
	"strings"

	"go.uber.org/fx"
)

// Possible deployment environments.
const (
	EnvProduction  = "production"
	EnvStaging     = "staging"
	EnvTest        = "test"
	EnvDevelopment = "development"
)

const (
	// Version is the current package version.
	Version = "1.1.0"

	_environmentKey        = "UBER_ENVIRONMENT"
	_runtimeEnvironmentKey = "UBER_RUNTIME_ENVIRONMENT"
	_zoneKey               = "UBER_DATACENTER"
	_deploymentKey         = "UDEPLOY_DEPLOYMENT_NAME"
	_containerNameKey      = "MESOS_CONTAINER_NAME"
	_configDirKey          = "UBER_CONFIG_DIR"
	_configKeySeparator    = ":"
	_portSystemKey         = "UBER_PORT_SYSTEM"
	_appIDKey              = "UDEPLOY_APP_ID"
	_pipelineKey           = "UBER_PIPELINE"
	_clusterKey            = "UBER_CLUSTER"

	_environmentFile = "/etc/uber/environment"
	_pipelineFile    = "/etc/uber/pipeline"
	_zoneFile        = "/etc/uber/datacenter"
	_roleFile        = "/etc/uber/role"
	_podFile         = "/etc/uber/pod"

	_defaultConfigDir = "config"
)

// Module provides a Context, which describes the runtime context of the
// service. It's useful for other components to use when choosing a default
// configuration.
//
// It doesn't require any configuration.
var Module = fx.Provide(New)

// Result defines the objects that the envfx module provides.
type Result struct {
	fx.Out

	Environment Context
}

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New() Result {
	return Result{
		Environment: Context{
			Zone:               getZone(),
			Environment:        getEnvironment(),
			RuntimeEnvironment: getRuntimeEnvironment(),
			Hostname:           getHostname(),
			Deployment:         getDeployment(),
			ContainerName:      getContainerName(),
			SystemPort:         getSystemPort(),
			ApplicationID:      getAppID(),
			Pipeline:           getPipeline(),
			Cluster:            getCluster(),
			Pod:                getPod(),
			configDirs:         getConfigDirs(),
		},
	}
}

// Context describes the service's runtime environment, pulling information
// from environment variables and Puppet-managed files as necessary.
type Context struct {
	Environment        string // enum for host-level environment (development, test, production, staging)
	RuntimeEnvironment string // user-specified service runtime environment (t.uber.com/environments-for-compute)
	Zone               string // t.uber.com/panama-terms
	Hostname           string
	Deployment         string // t.uber.com/udeploy_env
	ContainerName      string // Mesos-only
	SystemPort         string // for health checks and introspection
	ApplicationID      string // uDeploy AppID (e.g., "populous-celery")
	Pipeline           string
	Cluster            string
	Pod                string

	configDirs []string
}

// ConfigDirs returns the directories to search for configuration files. This
// is typically just the service's config directory, but advanced users can
// specify multiple colon-separated paths:
//
//  export UBER_CONFIG_DIR="config:test_config:local_config"
func (c Context) ConfigDirs() []string {
	cp := make([]string, len(c.configDirs))
	copy(cp, c.configDirs)
	return cp
}

func getEnvironment() string {
	val, fromEnv := readValue(_environmentKey, _environmentFile)
	switch val {
	case EnvProduction:
		if !fromEnv {
			// Jenkins hosts are in production, but code running there shouldn't
			// have access to production secrets.
			if role, err := ioutil.ReadFile(_roleFile); err == nil {
				if bytes.Contains(role, []byte("jenkins")) {
					return EnvDevelopment
				}
			}
		}
		return val
	case EnvStaging:
		return EnvStaging
	case EnvTest:
		return EnvTest
	default:
		return EnvDevelopment
	}
}

func getHostname() string {
	if host, err := os.Hostname(); err == nil {
		return host
	}
	return ""
}

func getDeployment() string {
	return os.Getenv(_deploymentKey)
}

func getRuntimeEnvironment() string {
	return os.Getenv(_runtimeEnvironmentKey)
}

func getZone() string {
	val, _ := readValue(_zoneKey, _zoneFile)
	return val
}

func getContainerName() string {
	return os.Getenv(_containerNameKey)
}

func getConfigDirs() []string {
	// Allow overriding the directory config is loaded from, useful for tests
	// inside subdirectories when the config/ dir is in the top-level of a project.
	if configRoot := os.Getenv(_configDirKey); configRoot != "" {
		return strings.Split(configRoot, _configKeySeparator)
	}

	return []string{_defaultConfigDir}
}

// Read a value from the environment if possible, else fall back to a
// Puppet-managed file.
func readValue(envKey string, fileName string) (_ string, fromEnv bool) {
	if v, ok := os.LookupEnv(envKey); ok {
		return v, true
	}
	// N.B., these files don't have trailing newlines.
	if bs, err := ioutil.ReadFile(fileName); err == nil {
		return string(bs), false
	}
	return "", false
}

func getSystemPort() string {
	// Keep envfx minimal and let the consumer handle parsing.
	return os.Getenv(_portSystemKey)
}

func getAppID() string {
	return os.Getenv(_appIDKey)
}

func getPipeline() string {
	val, _ := readValue(_pipelineKey, _pipelineFile)
	return val
}

func getCluster() string {
	return os.Getenv(_clusterKey)
}

func getPod() string {
	line, err := ioutil.ReadFile(_podFile)
	if err != nil {
		return ""
	}

	return string(bytes.TrimSpace(line))
}
