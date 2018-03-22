package glide

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-yaml/yaml"
	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/uber"
	"github.com/pkg/errors"
)

var UberLogger = uber.UberLogger

const (
	GlideYamlName       = "glide.yaml"
	GlideLockName       = "glide.lock"
	modifiedPrefix      = ".old"
	glideManifestHeader = `# This glide manifest is generated and kept in sync by dep for backwards compatibility.
# It is not a full glide manifest and will not work with traditional glide commands.
`
)

type glideYaml struct {
	Imports []glidePackage `yaml:"import"`
}

type glidePackage struct {
	Name       string `yaml:"package"`
	Reference  string `yaml:"version"` // could contain a semver, tag or branch
	Repository string `yaml:"repo"`
}

// If the glide manifest/lock exists then we suffix them with .old and write a
// direct copy (from toml to yaml) of the dep manifest for backwards compatibility.
// Dep keeps the glide manifest in sync with the dep manifest
func UpdateGlideArtifacts(depManifest gps.Manifest, dir string) error {

	glideManifestPath := filepath.Join(dir, GlideYamlName)
	if _, err := os.Stat(glideManifestPath); err == nil {
		errInternal := os.Rename(glideManifestPath, glideManifestPath+modifiedPrefix)
		if errInternal != nil {
			UberLogger.Println("Found glide manifest but failed to modify the file name")
			return errInternal
		}
		UberLogger.Printf("Found glide manifest and modified it to %s\n", glideManifestPath+modifiedPrefix)
	} else if os.IsNotExist(err) {
		UberLogger.Println("Could not find glide manifest")
	} else {
		UberLogger.Printf("Error occured while finding glide manifest: %s\n", err)
	}

	glideLockPath := filepath.Join(dir, GlideLockName)
	if _, err := os.Stat(glideLockPath); err == nil {
		errInternal := os.Rename(glideLockPath, glideLockPath+modifiedPrefix)
		if errInternal != nil {
			UberLogger.Println("Found glide lock but failed to modify the file name")
			return errInternal
		}
		UberLogger.Printf("Found glide lock and modified it to %s\n", glideLockPath+modifiedPrefix)
	} else if os.IsNotExist(err) {
		UberLogger.Println("Could not find glide lock")
	} else {
		UberLogger.Printf("Error occured while finding glide lock: %s\n", err)
	}

	glideManifest, err := convertDepToGlide(depManifest)
	if err != nil {
		UberLogger.Println("Failed to convert dep manfiest to glide manifest")
		return err
	}

	err = WriteNewGlideManifest(dir, false, glideManifest)
	if err != nil {
		UberLogger.Println(err)
		return err
	}

	return nil
}

func convertDepToGlide(depManifest gps.Manifest) (glideYaml, error) {

	glideManifest := glideYaml{}
	if te, ok := depManifest.(*dep.Manifest); ok {
		rawManifest := te.ConvertToRaw()

		// These are the only fields we have to convert and write (imports field) because
		// that is the only thing that glide reads from its direct/transitive dependencies
		// NOTE: The dep manifest constraint can never have a name that does not point to a
		// project root. Also we can safely ignore the subpackages here since dep does not
		// include subpackages in its manifest. The way glide pulls in subpackages from dep
		// manifest is by normalizing the name, but since the name cannot be anything but the
		// project root, there is no way glide can find the subpackages from just the manifest.
		// The subpackages are optional in the glide manifest.
		for _, pkg := range rawManifest.Constraints {

			if pkg.Source == "" && pkg.Version == "" && pkg.Revision == "" && pkg.Branch == "" {
				continue
			}

			glidePkg := glidePackage{
				Name:       pkg.Name,
				Repository: pkg.Source,
			}
			if pkg.Version != "" {
				glidePkg.Reference = getVersionWithImpliedSemverMajorRange(pkg.Version)
			} else if pkg.Revision != "" {
				glidePkg.Reference = pkg.Revision
			} else if pkg.Branch != "" {
				glidePkg.Reference = pkg.Branch
			}
			glideManifest.Imports = append(glideManifest.Imports, glidePkg)
		}

		for _, reqStr := range rawManifest.Required {
			glidePkg := glidePackage{
				Name: reqStr,
			}
			glideManifest.Imports = append(glideManifest.Imports, glidePkg)
		}

		// Glide does not read anything other than the Imports field from direct/transient
		// dependencies so there is no need to populate the other fields

	} else {
		return glideManifest, errors.New("depManifest is not of type manifest")
	}

	return glideManifest, nil
}

func WriteNewGlideManifest(dir string, overwrite bool, glideManifest glideYaml) error {
	y := filepath.Join(dir, GlideYamlName)
	if _, err := os.Stat(y); err == nil && overwrite == false {
		return errors.Errorf("glide manifest exists and cannot overwrite")
	}

	yb, err := yaml.Marshal(glideManifest)
	if err != nil {
		return errors.Wrap(err, "unable to marshall imported packages")
	}
	UberLogger.Printf("Writing glide manifest at %s", y)
	err = ioutil.WriteFile(y, append([]byte(glideManifestHeader), yb...), 0644)
	if err != nil {
		return errors.Wrap(err, "error writing glide manifest")
	}
	return nil
}

// Dep implies a caret for semver versions that glide would
// treat as direct semver. So a caret needs to be added to
// versions that are read from dep by glide
func getVersionWithImpliedSemverMajorRange(version string) string {
	if _, err := strconv.Atoi(string(version[0])); err == nil {
		// first char is a number so there is an implied caret from dep
		return "^" + version
	}
	return version
}
