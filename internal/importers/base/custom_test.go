package base

import (
	"testing"

	"github.com/golang/dep/internal/importers/importertest"
)

func TestCustomConfig_Parse(t *testing.T) {
	testCases := map[string]struct {
		config  CustomConfig
		impPkgs []ImportedPackage
		wantErr bool
	}{
		"read single override": {
			config: CustomConfig{
				Overrides: []overridePackage{
					{
						Name:      importertest.Project,
						Reference: importertest.V1Constraint,
					},
				},
			},
			impPkgs: []ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       "",
					ConstraintHint: importertest.V1Constraint,
					IsOverride:     true,
				},
			},
			wantErr: false,
		},
		"read multiple overrides": {
			config: CustomConfig{
				Overrides: []overridePackage{
					{
						Name:      importertest.Project,
						Reference: importertest.V1Constraint,
					},
					{
						Name:      "github.com/ChinmayR/testproject",
						Reference: "master",
					},
				},
			},
			impPkgs: []ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       "",
					ConstraintHint: importertest.V1Constraint,
					IsOverride:     true,
				},
				{
					Name:           "github.com/ChinmayR/testproject",
					LockHint:       "",
					ConstraintHint: "master",
					IsOverride:     true,
				},
			},
			wantErr: false,
		},
		"duplicate overrides results in error": {
			config: CustomConfig{
				Overrides: []overridePackage{
					{
						Name:      importertest.Project,
						Reference: importertest.V1Constraint,
					},
					{
						Name:      importertest.Project,
						Reference: importertest.V1Constraint,
					},
				},
			},
			impPkgs: nil,
			wantErr: true,
		},
	}

	for name, testCase := range testCases {
		name := name
		t.Run(name, func(t *testing.T) {
			impPkgs, err := ParseConfig(testCase.config)
			if testCase.wantErr && err == nil {
				t.Fatalf("wanted error but got none")
			}
			if !equalImpPkgs(impPkgs, testCase.impPkgs) {
				t.Fatal("imported packages did not match")
			}
		})
	}
}

func equalImpPkgs(a, b []ImportedPackage) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	for _, i := range a {
		found := false
		for _, j := range b {
			if i.Name == j.Name && i.ConstraintHint == j.ConstraintHint && i.Source == j.Source &&
				i.LockHint == j.LockHint && i.IsOverride == j.IsOverride {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
