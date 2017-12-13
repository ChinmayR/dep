package base

import (
	"io/ioutil"
	"path/filepath"

	"os"

	"github.com/go-yaml/yaml"
	"github.com/pkg/errors"
)

const CustomConfigName = "DepConfig.yaml"

type CustomConfig struct {
	Overrides []overridePackage `yaml:"override"`
}

type overridePackage struct {
	Name      string `yaml:"package"`
	Reference string `yaml:"version"`
	Source    string `yaml:"source"`
}

func (i *Importer) ReadCustomConfig(dir string) ([]ImportedPackage, error) {
	y := filepath.Join(dir, CustomConfigName)
	if _, err := os.Stat(y); err != nil {
		i.Logger.Println("Did not detect custom configuration files...")
		return nil, nil
	}

	i.Logger.Println("Detected custom configuration files...")
	if i.Verbose {
		i.Logger.Printf("  Loading %s", y)
	}
	yb, err := ioutil.ReadFile(y)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read %s", y)
	}
	i.Logger.Println(string(yb))
	customConfig := CustomConfig{}
	err = yaml.Unmarshal(yb, &customConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to parse %s", y)
	}

	return ParseConfig(customConfig)
}

func ParseConfig(config CustomConfig) ([]ImportedPackage, error) {

	var impPkgs []ImportedPackage
	pkgSeen := make(map[string]bool)

	for _, pkg := range config.Overrides {
		if val, ok := pkgSeen[pkg.Name]; ok && val {
			return nil, errors.Errorf("found multiple entries for %s in custom config", pkg.Name)
		}
		impPkgs = append(impPkgs, ImportedPackage{
			Name:           pkg.Name,
			ConstraintHint: pkg.Reference,
			Source:         pkg.Source,
			IsOverride:     true,
		})
		pkgSeen[pkg.Name] = true
	}

	return impPkgs, nil
}
