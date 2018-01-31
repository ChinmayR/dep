// Package servicefx provides static metadata about the running service.
package servicefx // import "code.uber.internal/go/servicefx.git"

import (
	"errors"
	"fmt"
	"time"

	"code.uber.internal/go/version.git"
	"go.uber.org/config"
	"go.uber.org/fx"
)

const (
	// Version is the current package version.
	Version = "1.1.0"
	// ConfigurationKey is the portion of the service configuration that this
	// package reads.
	ConfigurationKey = "service"
)

// Module provides Metadata, which describes the running service. It merges
// information found in configuration and information supplied through
// go-build's linker flags.
//
// At minimum, the service's name and owner email must be specified in
// configuration under the "service" key. Typical YAML configuration looks
// like this:
//
//   service:
//     name: ${UDEPLOY_APP_ID:my-service-name}
var Module = fx.Provide(New)

// Params defines the dependencies of the servicefx module.
type Params struct {
	fx.In

	Config config.Provider
}

// Result defines the objects that the servicefx module provides.
type Result struct {
	fx.Out

	Metadata Metadata
}

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(params Params) (Result, error) {
	var meta Metadata
	if err := params.Config.Get(ConfigurationKey).Populate(&meta); err != nil {
		return Result{}, fmt.Errorf("couldn't read service metadata from config: %v", err)
	}
	if meta.Name == "" {
		return Result{}, errors.New("no service name (key service.name) found in config")
	}
	meta.BuildTime = version.BuildTime
	meta.BuildUserHost = version.BuildUserHost
	meta.BuildHash = version.BuildHash
	return Result{Metadata: meta}, nil
}

// Metadata describes the running service, merging information found in
// configuration and build-time linker flags.
type Metadata struct {
	// Static data must come from configuration.
	Name string `yaml:"name"`

	// Build information, gathered from linker flags specified by go-build.
	BuildTime     time.Time `yaml:"-"`
	BuildUserHost string    `yaml:"-"`
	BuildHash     string    `yaml:"-"`
}
