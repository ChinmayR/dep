// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package glide

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/internal/importers/base"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const glideYamlName = "glide.yaml"
const glideLockName = "glide.lock"

// Importer imports glide configuration into the dep configuration format.
type Importer struct {
	*base.Importer
	glideConfig glideYaml
	glideLock   glideLock
	lockFound   bool
}

// NewImporter for glide.
func NewImporter(logger *log.Logger, verbose bool, sm gps.SourceManager) *Importer {
	return &Importer{Importer: base.NewImporter(logger, verbose, sm)}
}

type glideYaml struct {
	Name        string         `yaml:"package"`
	Ignores     []string       `yaml:"ignore"`
	ExcludeDirs []string       `yaml:"excludeDirs"`
	Imports     []glidePackage `yaml:"import"`
	TestImports []glidePackage `yaml:"testImport"`
}

type glideLock struct {
	Imports     []glideLockedPackage `yaml:"imports"`
	TestImports []glideLockedPackage `yaml:"testImports"`
}

type glidePackage struct {
	Name       string `yaml:"package"`
	Reference  string `yaml:"version"` // could contain a semver, tag or branch
	Repository string `yaml:"repo"`

	// Unsupported fields that we will warn if used
	Subpackages []string `yaml:"subpackages"`
	OS          string   `yaml:"os"`
	Arch        string   `yaml:"arch"`
}

type glideLockedPackage struct {
	Name       string `yaml:"name"`
	Revision   string `yaml:"version"`
	Repository string `yaml:"repo"`
}

// Name of the importer.
func (g *Importer) Name() string {
	return "glide"
}

// HasDepMetadata checks if a directory contains config that the importer can handle.
func (g *Importer) HasDepMetadata(dir string, importCustomConfig bool) bool {
	// Only require glide.yaml or dep custom config, the lock is optional
	_, err1 := os.Stat(filepath.Join(dir, glideYamlName))
	_, err2 := os.Stat(filepath.Join(dir, base.CustomConfigName))
	// only does not have dep metadata if there is a glide manifest AND
	// custom config is meant to be imported (only in root repo) AND it has custom config file
	if err1 != nil && (importCustomConfig && err2 != nil) {
		return false
	}

	return true
}

// Import the config found in the directory.
func (g *Importer) Import(dir string, pr gps.ProjectRoot, importCustomConfig bool) (*dep.Manifest, *dep.Lock, error) {
	err := g.load(dir)
	if err != nil {
		return nil, nil, err
	}

	var impPkgs []base.ImportedPackage
	var customExcludeDirs []string
	if importCustomConfig {
		impPkgs, customExcludeDirs, err = base.ReadCustomConfig(dir)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to read custom configuration")
		}
	}

	m, l := g.convert(impPkgs, customExcludeDirs, pr)
	return m, l, nil
}

// load the glide configuration files. Failure to load `glide.yaml` is considered
// unrecoverable and an error is returned for it. But if there is any error while trying
// to load the lock file, only a warning is logged.
func (g *Importer) load(projectDir string) error {
	g.Logger.Println("Detected glide configuration files...")
	y := filepath.Join(projectDir, glideYamlName)
	if exists, _ := fs.IsRegular(y); exists {
		if g.Verbose {
			g.Logger.Printf("  Loading %s", y)
		}
		yb, err := ioutil.ReadFile(y)
		if err != nil {
			return errors.Wrapf(err, "unable to read %s", y)
		}
		err = yaml.Unmarshal(yb, &g.glideConfig)
		if err != nil {
			return errors.Wrapf(err, "unable to parse %s", y)
		}
	}

	l := filepath.Join(projectDir, glideLockName)
	if exists, _ := fs.IsRegular(l); exists {
		if g.Verbose {
			g.Logger.Printf("  Loading %s", l)
		}
		lb, err := ioutil.ReadFile(l)
		if err != nil {
			g.Logger.Printf("  Warning: Ignoring lock file. Unable to read %s: %s\n", l, err)
			return nil
		}
		lock := glideLock{}
		err = yaml.Unmarshal(lb, &lock)
		if err != nil {
			g.Logger.Printf("  Warning: Ignoring lock file. Unable to parse %s: %s\n", l, err)
			return nil
		}
		g.lockFound = true
		g.glideLock = lock
	}

	return nil
}

// convert the glide configuration files into dep configuration files.
func (g *Importer) convert(impPkgs []base.ImportedPackage, customExcludeDirs []string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock) {
	projectName := string(pr)

	task := bytes.NewBufferString("Converting from glide.yaml")
	if g.lockFound {
		task.WriteString(" and glide.lock")
	}
	task.WriteString("...")
	g.Logger.Println(task)

	numPkgs := len(g.glideConfig.Imports) + len(g.glideConfig.TestImports) + len(g.glideLock.Imports) + len(g.glideLock.TestImports)
	packages := make([]base.ImportedPackage, 0, numPkgs)

	// Constraints
	for _, pkg := range append(g.glideConfig.Imports, g.glideConfig.TestImports...) {
		// Validate
		if pkg.Name == "" {
			g.Logger.Println(
				"  Warning: Skipping project. Invalid glide configuration, Name is required",
			)
			continue
		}

		if pkg.Name == string(pr) {
			g.Logger.Printf("  Warning: Skipping project %v. Invalid glide configuration, Name matches repo being imported", pkg.Name)
			continue
		}

		// Warn
		if g.Verbose {
			if pkg.OS != "" {
				g.Logger.Printf("  The %s package specified an os, but that isn't supported by dep yet, and will be ignored. See https://github.com/golang/dep/issues/291.\n", pkg.Name)
			}
			if pkg.Arch != "" {
				g.Logger.Printf("  The %s package specified an arch, but that isn't supported by dep yet, and will be ignored. See https://github.com/golang/dep/issues/291.\n", pkg.Name)
			}
		}

		ip := base.ImportedPackage{
			Name:           pkg.Name,
			Source:         pkg.Repository,
			ConstraintHint: pkg.Reference,
		}
		packages = append(packages, ip)
	}

	// Locks
	for _, pkg := range append(g.glideLock.Imports, g.glideLock.TestImports...) {
		// Validate
		if pkg.Name == "" {
			g.Logger.Println("  Warning: Skipping project. Invalid glide lock, Name is required")
			continue
		}

		if pkg.Name == string(pr) {
			g.Logger.Printf("  Warning: Skipping project %v. Invalid glide configuration, Name matches repo being imported", pkg.Name)
			continue
		}

		ip := base.ImportedPackage{
			Name:     pkg.Name,
			Source:   pkg.Repository,
			LockHint: pkg.Revision,
		}
		packages = append(packages, ip)
	}

	packages = append(packages, impPkgs...)

	g.ImportPackages(packages, false)

	// Ignores
	g.Manifest.Ignored = append(g.Manifest.Ignored, g.glideConfig.Ignores...)
	if len(g.glideConfig.ExcludeDirs) > 0 {
		if g.glideConfig.Name != "" && g.glideConfig.Name != projectName {
			g.Logger.Printf("  Glide thinks the package is '%s' but dep thinks it is '%s', using dep's value.\n", g.glideConfig.Name, projectName)
		}

		for _, dir := range g.glideConfig.ExcludeDirs {
			pkg := path.Join(projectName, dir)
			g.Manifest.Ignored = append(g.Manifest.Ignored, pkg)
		}
	}
	for _, customIgnore := range customExcludeDirs {
		pkg := path.Join(projectName, customIgnore)
		alreadyExists := false
		for _, existingIgnore := range g.Manifest.Ignored {
			if strings.EqualFold(pkg, existingIgnore) {
				alreadyExists = true
				break
			}
		}
		if !alreadyExists {
			g.Manifest.Ignored = append(g.Manifest.Ignored, pkg)
		}
	}

	return g.Manifest, g.Lock
}
