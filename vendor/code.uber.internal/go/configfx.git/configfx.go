// Package configfx loads Uber's typical YAML configuration files and makes
// their merged output available to the application.
package configfx

import (
	"fmt"
	"os"
	"strings"

	"code.uber.internal/go/configfx.git/load"

	envfx "code.uber.internal/go/envfx.git"
	"go.uber.org/config"
	"go.uber.org/fx"
)

const (
	// Version is the current package version.
	Version = "1.1.0"

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

// getFiles lists the possible configuration files to load based on environment context.
func getFiles(context envfx.Context) []load.FileInfo {
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

// Module load config.Provider based on the environment context.
var Module = fx.Provide(New)

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	cfg, err := load.FromFiles(
		p.Environment.ConfigDirs(),
		getFiles(p.Environment),
		os.LookupEnv)

	if err != nil {
		return Result{}, err
	}

	return Result{Provider: cfg}, nil
}
