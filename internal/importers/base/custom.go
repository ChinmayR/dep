package base

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/dep/uber"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

const (
	CustomConfigName = "DepConfig.toml"
)

type ReferenceOverrideAlreadyExistsForBasic struct {
	subPkg string
}

type SourceOverrideAlreadyExistsForBasic struct {
	subPkg string
}

func (e ReferenceOverrideAlreadyExistsForBasic) Error() string {
	return fmt.Sprintf("reference override for %s already exists in current config", e.subPkg)
}

func (e SourceOverrideAlreadyExistsForBasic) Error() string {
	return fmt.Sprintf("source override for %s already exists in current config", e.subPkg)
}

type CustomConfig struct {
	Overrides   []overridePackage `toml:"override"`
	ExcludeDirs []string          `toml:"excludeDirs"`
}

type overridePackage struct {
	Name      string `toml:"package"`
	Reference string `toml:"version"`
	Source    string `toml:"source"`
}

func ReadCustomConfig(dir string) ([]ImportedPackage, []string, error) {
	t := filepath.Join(dir, CustomConfigName)
	if _, err := os.Stat(t); err != nil {
		uber.UberLogger.Printf("Did not detect custom configuration files at %s\n", dir)
		return nil, nil, nil
	}

	uber.UberLogger.Println("Detected custom configuration files...")
	uber.UberLogger.Printf("Loading %s\n", t)
	yb, err := ioutil.ReadFile(t)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to read %s", t)
	}
	uber.UberLogger.Println(string(yb))
	customConfig := CustomConfig{}
	err = toml.Unmarshal(yb, &customConfig)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to parse %s", t)
	}

	return ParseConfig(customConfig)
}

func WriteCustomConfig(dir string, impPkgs []ImportedPackage, excludeDirs []string, overwrite bool, out *log.Logger) error {
	t := filepath.Join(dir, CustomConfigName)
	if _, err := os.Stat(t); err == nil && overwrite == false {
		return errors.Errorf("custom config exists and cannot overwrite")
	}

	tb := new(bytes.Buffer)
	err := toml.NewEncoder(tb).Encode(CustomConfig{
		Overrides:   convertImpPkgToOveridePkg(impPkgs),
		ExcludeDirs: excludeDirs,
	})
	if err != nil {
		return errors.Wrap(err, "unable to marshall imported packages")
	}
	out.Printf("Writing %s\n", t)
	out.Println(tb.String())
	err = ioutil.WriteFile(t, tb.Bytes(), 0644)
	if err != nil {
		return errors.Wrap(err, "error writing config file")
	}
	return nil
}

func convertImpPkgToOveridePkg(impPkgs []ImportedPackage) []overridePackage {
	var overidePkgs []overridePackage
	for _, impPkg := range impPkgs {
		overidePkgs = append(overidePkgs, overridePackage{
			Name:      impPkg.Name,
			Reference: impPkg.ConstraintHint,
			Source:    impPkg.Source,
		})
	}
	return overidePkgs
}

func ParseConfig(config CustomConfig) ([]ImportedPackage, []string, error) {
	var impPkgs []ImportedPackage
	pkgSeen := make(map[string]bool)

	for _, pkg := range config.Overrides {
		if val, ok := pkgSeen[pkg.Name]; ok && val {
			return nil, nil, errors.Errorf("found multiple entries for %s in custom config", pkg.Name)
		}
		impPkgs = append(impPkgs, ImportedPackage{
			Name:           pkg.Name,
			ConstraintHint: pkg.Reference,
			Source:         pkg.Source,
			IsOverride:     true,
		})
		pkgSeen[pkg.Name] = true
	}

	return impPkgs, config.ExcludeDirs, nil
}

/*
These are basic uber specific overrides that help avoid conflicts
and speed up resolution for uber repos. These were derived through
testing and data collected from resolve failures.
*/
var basicOverrides = []overridePackage{
	{
		Name:   "golang.org/x/net",
		Source: "golang.org/x/net",
	},
	{
		Name:   "golang.org/x/sys",
		Source: "golang.org/x/sys",
	},
	{
		Name:   "golang.org/x/tools",
		Source: "golang.org/x/tools",
	},
}

/*
These are the basic directories that could be auto generated and
we want to ignore while scanning for imports in dep. Bootstrap
the dep config with these preset.
*/
var basicExcludeDirs = []string{
	".tmp",
}

func AppendBasicExcludeDirs(currentExcludeDirs []string) []string {
	for _, basicIgnore := range basicExcludeDirs {
		alreadyExists := false
		for _, existingIgnore := range currentExcludeDirs {
			if strings.EqualFold(basicIgnore, existingIgnore) {
				alreadyExists = true
				break
			}
		}
		if !alreadyExists {
			currentExcludeDirs = append(currentExcludeDirs, basicIgnore)
		}
	}
	return currentExcludeDirs
}

func AppendBasicOverrides(impPkgs []ImportedPackage, pkgSeen map[string]bool) ([]ImportedPackage, error) {

	for _, pkg := range basicOverrides {
		if val, ok := pkgSeen[pkg.Name]; ok && val {
			pkgFound := false
			for idx := range impPkgs {
				subPkg := &impPkgs[idx]
				if subPkg.Name == pkg.Name {
					pkgFound = true
					// overwrite reference if not empty otherwise return error (don't clobber)
					if pkg.Reference != "" {
						if subPkg.ConstraintHint != "" && pkg.Reference != subPkg.ConstraintHint {
							return nil, ReferenceOverrideAlreadyExistsForBasic{subPkg: subPkg.Name}
						} else {
							subPkg.ConstraintHint = pkg.Reference
						}
					}
					// overwrite source if not empty otherwise return error (don't clobber)
					if pkg.Source != "" {
						if subPkg.Source != "" && pkg.Source != subPkg.Source {
							return nil, SourceOverrideAlreadyExistsForBasic{subPkg: subPkg.Name}
						} else {
							subPkg.Source = pkg.Source
						}
					}
				}
			}
			if !pkgFound {
				return nil, errors.Errorf("could not find package %s in list", pkg.Name)
			}
		} else {
			impPkgs = append(impPkgs, ImportedPackage{
				Name:           pkg.Name,
				ConstraintHint: pkg.Reference,
				Source:         pkg.Source,
				IsOverride:     true,
			})
			pkgSeen[pkg.Name] = true
		}
	}

	return impPkgs, nil
}

func AddOverrideToConfig(name, constraint, source, workingDir string, logOut *log.Logger) error {
	curPkgs, basicExcludeDirs, err := ReadCustomConfig(workingDir)
	if err != nil {
		return err
	}

	curPkgs = append(curPkgs, ImportedPackage{
		Name:           name,
		ConstraintHint: constraint,
		Source:         source,
		IsOverride:     true,
	})

	return WriteCustomConfig(workingDir, curPkgs, basicExcludeDirs, true, logOut)
}
