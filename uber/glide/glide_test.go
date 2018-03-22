package glide

import (
	"reflect"
	"strings"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
)

func TestConvertDepToGlide(t *testing.T) {
	c, _ := gps.NewSemverConstraint("^0.12.0")
	depManifest := dep.Manifest{
		Constraints: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/golang/dep/foo"): {
				Constraint: gps.Any(),
				Source:     "dep/foo/source",
			},
			gps.ProjectRoot("github.com/golang/mock/bar"): {
				Constraint: c,
			},
			gps.ProjectRoot("github.com/golang/go/xyz"): {
				Constraint: gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb"),
			},
			gps.ProjectRoot("github.com/golang/fmt"): {
				Constraint: gps.NewBranch("dev"),
				Source:     "dep/foo/source",
			},
			gps.ProjectRoot("github.com/golang/fix/me"): {},
		},
		Ovr: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/golang/mock/bar"): {
				Constraint: gps.Any(),
			},
		},
		Ignored: []string{
			"ignore/this/package",
			"ignore/this/otherPackage",
		},
		Required: []string{
			"require/this/package",
			"require/this/otherPackage",
		},
	}

	wantGlide := glideYaml{
		Imports: []glidePackage{
			glidePackage{
				Name:       "github.com/golang/dep/foo",
				Reference:  "",
				Repository: "dep/foo/source",
			},
			glidePackage{
				Name:       "github.com/golang/fmt",
				Reference:  "dev",
				Repository: "dep/foo/source",
			},
			glidePackage{
				Name:      "github.com/golang/go/xyz",
				Reference: "d05d5aca9f895d19e9265839bffeadd74a2d2ecb",
			},
			glidePackage{
				Name:      "github.com/golang/mock/bar",
				Reference: "^0.12.0",
			},
			glidePackage{
				Name: "require/this/package",
			},
			glidePackage{
				Name: "require/this/otherPackage",
			},
		},
	}

	gotGlide, _ := convertDepToGlide(&depManifest)

	if !reflect.DeepEqual(gotGlide, wantGlide) {
		t.Error("Glide manifest is not as expected")
	}
}

func TestInvalidManifestType(t *testing.T) {
	depManifest := gps.SimpleManifest{}

	_, err := convertDepToGlide(&depManifest)
	if !strings.EqualFold(err.Error(), "depManifest is not of type manifest") {
		t.Error("Got no error when expected")
	}
}
