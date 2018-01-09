package base

import (
	"io/ioutil"
	"path/filepath"

	"os"

	"github.com/go-yaml/yaml"
	"github.com/pkg/errors"
	"fmt"
	"log"
	"strings"
)

const CustomConfigName = "DepConfig.yaml"

type CustomConfig struct {
	Overrides []overridePackage `yaml:"override"`
	ExcludeDirs []string `yaml:"excludeDirs"`
}

type overridePackage struct {
	Name      string `yaml:"package"`
	Reference string `yaml:"version"`
	Source    string `yaml:"source"`
}

func ReadCustomConfig(dir string) ([]ImportedPackage, []string, error) {
	y := filepath.Join(dir, CustomConfigName)
	if _, err := os.Stat(y); err != nil {
		fmt.Println("Did not detect custom configuration files...")
		return nil, nil, nil
	}

	fmt.Println("Detected custom configuration files...")
	fmt.Printf("  Loading %s", y)
	yb, err := ioutil.ReadFile(y)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to read %s", y)
	}
	fmt.Println(string(yb))
	customConfig := CustomConfig{}
	err = yaml.Unmarshal(yb, &customConfig)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to parse %s", y)
	}

	return ParseConfig(customConfig)
}

func WriteCustomConfig(dir string, impPkgs []ImportedPackage, excludeDirs []string, overwrite bool, out *log.Logger) error {
	y := filepath.Join(dir, CustomConfigName)
	if _, err := os.Stat(y); err == nil && overwrite == false {
		return errors.Errorf("custom config exists and cannot overwrite")
	}

	out.Println("Overwriting custom configuration files...")
	yb, err := yaml.Marshal(CustomConfig{
			Overrides: convertImpPkgToOveridePkg(impPkgs),
			ExcludeDirs: excludeDirs,
		})
	if err != nil {
		return errors.Wrap(err, "unable to marshall imported packages")
	}
	out.Println(string(yb))
	out.Printf("  Writing %s", y)
	err = ioutil.WriteFile(y, yb, 0644)
	if err != nil {
		return errors.Wrap(err, "error writing config file")
	}
	return nil
}

func convertImpPkgToOveridePkg(impPkgs []ImportedPackage) []overridePackage {
	var overidePkgs []overridePackage
	for _, impPkg := range impPkgs {
		overidePkgs = append(overidePkgs, overridePackage{
			Name: impPkg.Name,
			Reference: impPkg.ConstraintHint,
			Source: impPkg.Source,
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
var basicOverrides = []overridePackage {
	{
		Name: "golang.org/x/net",
		Source: "golang.org/x/net",
	},
	{
		Name: "golang.org/x/sys",
		Source: "golang.org/x/sys",
	},
	{
		Name: "golang.org/x/tools",
		Source: "golang.org/x/tools",
	},
}

/*
These are the basic directories that could be auto generated and
we want to ignore while scanning for imports in dep. Bootstrap
the dep config with these preset.
 */
var basicExcludeDirs = []string {
	".gen",
	".tmp",
	"_templates",
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
						if subPkg.ConstraintHint != "" {
							return nil, errors.Errorf("reference override for %s already exists in current config",
								subPkg.Name)
						} else {
							subPkg.ConstraintHint = pkg.Reference
						}
					}
					// overwrite source if not empty otherwise return error (don't clobber)
					if pkg.Source != "" {
						if subPkg.Source != "" {
							return nil, errors.Errorf("source override for %s already exists in current config",
								subPkg.Name)
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