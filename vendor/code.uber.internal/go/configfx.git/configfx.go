// Package configfx loads Uber's typical YAML configuration files and makes
// their merged output available to the application.
//
// configfx will inspect environment variables in order to determine the config files loaded.
// The main environment variables are:
//	$UBER_ENVIRONMENT
//	$UBER_RUNTIME_ENVIRONMENT
//	$UDEPLOY_DEPLOYMENT_NAME
//	$UBER_DATACENTER
//
// configfx first inspects $UBER_ENVIRONMENT. If it isn't set, it assumes it is running on a
// laptop and will load base.yaml and development.yaml. If you are using go-build, make test will
// set $UBER_ENVIRONMENT to test, and so base.yaml and test.yaml are run. If you are using plain
// go test ./..., you must set $UBER_ENVIRONMENT yourself.
//
// In production, $UBER_ENVIRONMENT is set to production and the inference can get a little
// more sophisticated. First configfx looks at $UBER_RUNTIME_ENVIRONMENT and if it is
// production it will load base.yaml, production.yaml and inspect $UBER_DATACENTER and
// load production-<datacenter>.yaml. If $UBER_RUNTIME_ENVIRONMENT is set to staging, it will
// load base.yaml, staging.yaml, and inspect $UBER_DATACENTER and load staging-<datacenter>.yaml
//
// Examples:
//	Laptop: base.yaml and development.yaml are loaded
//	Tests: base.yaml and test.yaml are loaded
//	Staging in SJC1: base.yaml, staging.yaml, and staging-sjc1.yaml are loaded
//	Production in SJC1: base.yaml, production.yaml and production-sjc1.yaml are loaded
//	Production in DCA1: base.yaml, production.yaml and production-dca1.yaml are loaded
//
// Note on monorepos (or poly-service repos, depending how you look at it).
// When a repository contains multiple services, the configuration loading process
// gets a little more complicated because each individual service needs to be
// associated with a particular set of configuration. The current recommendation
// is to modify the `go:command:` directive of associated `pinocchio.yaml` files.
//
// For example, given a repository with services `one` and `two`, the overall setup should
// look like this:
//		├── config
//	 	│   ├── one
//	 	│   │   └── base.yaml
//	 	│   └── two
//	 	│       └── base.yaml
//	 	├── one
//	 	│   └── main.go
//	 	├── two
//	 	│   └── main.go
//	 	└── udeploy
//	 	    └── pinnochio
//	 	        ├── one.yaml
//	 	        └── two.yaml
//
// To properly wire up service `one` in it's pinocchio file `udeploy/pinocchio/one.yaml`
// would contain:
//		service_name: one
//		network_protocol: http_system
//		service_type: go
//		go:
//			command: 'UBER_CONFIG_DIR="config/one" one/one'
//
// If you want to customize which files are loaded, create a meta.yaml file containing the
// files to load instead of the default files, highest priority last.
// An example meta.yaml could be:
//   files:
//     - base.yaml
//     - ${UBER_RUNTIME_ENVIRONMENT:"development"}.yaml
//     - ${UBER_RUNTIME_ENVIRONMENT:"development"}_${UBER_DATACENTER:"local"}.yaml
//     - secrets.yaml
// Note: go.uber.org/config is picky about whitespace, list entries must begin with 2 spaces,
// then a dash, one space, and the file name to read from.
package configfx // import "code.uber.internal/go/configfx.git"

import (
	"fmt"
	"strings"

	"code.uber.internal/go/configfx.git/load"

	ierr "code.uber.internal/go/configfx.git/internal/err"
	envfx "code.uber.internal/go/envfx.git"
	"go.uber.org/config"
	"go.uber.org/fx"
)

const (
	// Version is the current package version.
	Version = "1.3.0"

	_baseFile     = "base"
	_secretsFile  = "secrets"
	_deployPrefix = "deployment"
	_mesosPrefix  = "mesos"
)

var _digitRemover = strings.NewReplacer(
	"0", "",
	"1", "",
	"2", "",
	"3", "",
	"4", "",
	"5", "",
	"6", "",
	"7", "",
	"8", "",
	"9", "",
)

// Params defines the dependencies of the configfx module.
type Params struct {
	fx.In

	Environment envfx.Context
}

// Result defines the objects that the configfx module provides.
type Result struct {
	fx.Out

	Provider config.Provider
}

// stripDigits removes all numeric digits from a string
func stripDigits(s string) string {
	return _digitRemover.Replace(s)
}

// Appends zone to the last candidate, preserving the interpolation flag.
func appendZoneToLast(candidates []load.FileInfo, zone string) []load.FileInfo {
	if zone != "" {
		last := candidates[len(candidates)-1]
		last.Name += "-" + zone
		candidates = append(candidates, last)
	}

	return candidates
}

// Appends .yaml extension to the file list
func appendYAMLExtension(candidates []load.FileInfo) {
	for i := range candidates {
		candidates[i].Name += ".yaml"
	}
}

// defaultFiles lists the possible configuration files to load based on environment context.
func defaultFiles(context envfx.Context) []load.FileInfo {
	environment := context.RuntimeEnvironment
	if environment == "" {
		environment = context.Environment
	}
	candidates := []load.FileInfo{
		{Name: fmt.Sprintf("%s", _baseFile), Interpolate: true},
		{Name: fmt.Sprintf("%s", environment), Interpolate: true},
	}

	candidates = appendZoneToLast(candidates, context.Zone)
	if context.Deployment != "" {
		candidates = append(
			candidates,
			load.FileInfo{Name: fmt.Sprintf("%s-%s", _deployPrefix, stripDigits(context.Deployment)), Interpolate: true},
		)

		candidates = appendZoneToLast(candidates, context.Zone)
	}

	if context.ContainerName != "" {
		candidates = append(
			candidates,
			load.FileInfo{Name: fmt.Sprintf("%s", _mesosPrefix), Interpolate: true},
			load.FileInfo{Name: fmt.Sprintf("%s-%s", _mesosPrefix, environment), Interpolate: true},
		)

		candidates = appendZoneToLast(candidates, context.Zone)
	}

	candidates = append(
		candidates,
		load.FileInfo{Name: fmt.Sprintf("%s", _secretsFile), Interpolate: false},
	)

	candidates = appendZoneToLast(candidates, context.Zone)
	appendYAMLExtension(candidates)
	return candidates
}

func filesToFileinfo(files []string) []load.FileInfo {
	fileset := make([]load.FileInfo, 0, len(files))
	for _, file := range files {
		// Always interpolate unless the file is a secrets file.
		interpolate := !strings.Contains(file, _secretsFile)
		fileset = append(fileset, load.FileInfo{Name: file, Interpolate: interpolate})
	}
	return fileset
}

// IsNoFilesFoundErr returns true if it represents an error saying no config files were found.
func IsNoFilesFoundErr(err error) bool {
	_, ok := err.(ierr.NoConfig)
	return ok
}

// Module load config.Provider based on the environment context.
var Module = fx.Provide(New)

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	files := defaultFiles(p.Environment)
	meta, err := metaCfg(p.Environment)
	if err != nil {
		return Result{}, fmt.Errorf("error reading meta.yaml: %v", err)
	}
	if len(meta.Fileset) > 0 {
		files = filesToFileinfo(meta.Fileset)
	}

	cfg, err := load.FromFiles(
		p.Environment.ConfigDirs(),
		files,
		p.Environment.LookupEnv,
	)

	if err != nil {
		return Result{}, err
	}

	return Result{Provider: cfg}, nil
}
