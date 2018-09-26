package conflict_resolution

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/golang/dep/gps"
	"github.com/pkg/errors"
)

var testCases = map[string]struct {
	OverName         string
	OverType         string
	Constraint       string
	OverSource       string
	ExpectedError    error
	WantErr          bool
	ExpectedOverride *gps.OverridePackage
}{
	"none constraint type gives no error": {
		OverName:      "testName",
		OverType:      "none",
		OverSource:    "testSource",
		ExpectedError: nil,
		ExpectedOverride: &gps.OverridePackage{
			Name:       "testName",
			Source:     "testSource",
			Constraint: gps.Any(),
		},
	},
	"branch constraint type gives no error": {
		OverName:      "testName",
		OverType:      "branch",
		Constraint:    "master",
		OverSource:    "testSource",
		ExpectedError: nil,
		ExpectedOverride: &gps.OverridePackage{
			Name:       "testName",
			Source:     "testSource",
			Constraint: gps.NewBranch("master"),
		},
	},
	"revision constraint type gives no error": {
		OverName:      "testName",
		OverType:      "revision",
		Constraint:    "testRev",
		OverSource:    "testSource",
		ExpectedError: nil,
		ExpectedOverride: &gps.OverridePackage{
			Name:       "testName",
			Source:     "testSource",
			Constraint: gps.Revision("testRev"),
		},
	},
	"semver constraint type gives no error": {
		OverName:      "testName",
		OverType:      "semver",
		Constraint:    "^2.7.0",
		OverSource:    "testSource",
		ExpectedError: nil,
		ExpectedOverride: &gps.OverridePackage{
			Name:       "testName",
			Source:     "testSource",
			Constraint: mkSemverConstraint("^2.7.0"),
		},
	},
	"semver constraint does not auto imply caret": {
		OverName:      "testName",
		OverType:      "semver",
		Constraint:    "2.7.0",
		OverSource:    "testSource",
		ExpectedError: nil,
		ExpectedOverride: &gps.OverridePackage{
			Name:       "testName",
			Source:     "testSource",
			Constraint: mkSemverConstraint("2.7.0"),
		},
	},
	"semver type but branch constraint gives error": {
		OverName:      "testName",
		OverType:      "semver",
		Constraint:    "master",
		OverSource:    "testSource",
		WantErr:       true,
		ExpectedError: errors.New("failed to create semver constraint: Malformed constraint: master"),
	},
	"branch type but semver constraint gives no error": {
		OverName:      "testName",
		OverType:      "branch",
		Constraint:    "^2.7.0",
		OverSource:    "testSource",
		ExpectedError: nil,
		ExpectedOverride: &gps.OverridePackage{
			Name:       "testName",
			Source:     "testSource",
			Constraint: gps.NewBranch("^2.7.0"),
		},
	},
}

func mkSemverConstraint(body string) gps.Constraint {
	c, err := gps.NewSemverConstraint(body)
	if err != nil {
		panic(fmt.Sprintf("Error creating semver constraint %s: %s", body, err.Error()))
	}
	return c
}

func TestParseUserInputOverride(t *testing.T) {
	for name, tc := range testCases {
		name := name
		t.Run(name, func(t *testing.T) {
			ovrPkg, err := parseUserInputOverride(tc.OverName, tc.OverType, tc.Constraint, tc.OverSource)

			if tc.WantErr && tc.ExpectedError.Error() != err.Error() {
				t.Fatalf("wanted error \n\"%v\"\n but got \n\"%v\"\n", tc.ExpectedError, err)
			}

			if !reflect.DeepEqual(tc.ExpectedOverride, ovrPkg) {
				t.Fatalf("wanted override \n%v\n but got \n%v\n", tc.ExpectedOverride, ovrPkg)
			}
		})
	}
}
