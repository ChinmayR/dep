package base

import (
	"reflect"
	"testing"

	"github.com/golang/dep/internal/importers/importertest"
)

func TestCustomConfig_Parse(t *testing.T) {
	testCases := map[string]struct {
		config      CustomConfig
		impPkgs     []ImportedPackage
		excludeDirs []string
		wantErr     bool
	}{
		"read single override": {
			config: CustomConfig{
				Overrides: []overridePackage{
					{
						Name:      importertest.Project,
						Reference: importertest.V1Constraint,
					},
				},
				ExcludeDirs: AppendBasicExcludeDirs(nil),
			},
			impPkgs: []ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       "",
					ConstraintHint: importertest.V1Constraint,
					IsOverride:     true,
				},
			},
			excludeDirs: []string{
				".tmp",
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
		"source override": {
			config: CustomConfig{
				Overrides: []overridePackage{
					{
						Name:      importertest.Project,
						Reference: importertest.V1Constraint,
						Source:    "overrideSource",
					},
				},
			},
			impPkgs: []ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       "",
					Source:         "overrideSource",
					ConstraintHint: importertest.V1Constraint,
					IsOverride:     true,
				},
			},
			wantErr: false,
		},
	}

	for name, testCase := range testCases {
		name := name
		t.Run(name, func(t *testing.T) {
			impPkgs, excludeDirs, err := ParseConfig(testCase.config)
			if testCase.wantErr && err == nil {
				t.Fatalf("wanted error but got none")
			}
			if !reflect.DeepEqual(excludeDirs, testCase.excludeDirs) {
				t.Fatal("excludeDirs did not match")
			}
			if !equalImpPkgs(impPkgs, testCase.impPkgs) {
				t.Fatal("imported packages did not match")
			}
		})
	}
}

func TestCustomConfig_BasicExcludeDirs(t *testing.T) {
	testCases := map[string]struct {
		currentExcludeDirs  []string
		expectedExcludeDirs []string
		wantErr             bool
	}{
		"no overlapping exclude dirs": {
			currentExcludeDirs: []string{},
			expectedExcludeDirs: []string{
				".tmp",
			},
			wantErr: false,
		},
		"overlapping exclude dirs are not duplicated": {
			currentExcludeDirs: []string{
				".random",
				".tmp",
			},
			expectedExcludeDirs: []string{
				".random",
				".tmp",
			},
			wantErr: false,
		},
	}
	for name, testCase := range testCases {
		name := name
		t.Run(name, func(t *testing.T) {
			expectedExcludeDirs := AppendBasicExcludeDirs(testCase.currentExcludeDirs)
			if testCase.wantErr {
				t.Fatalf("wanted error but got none")
			}
			if !reflect.DeepEqual(expectedExcludeDirs, testCase.expectedExcludeDirs) {
				t.Fatal("expectedExcludeDirs did not match")
			}
		})
	}
}

func TestCustomConfig_BasicOverrides(t *testing.T) {
	testCases := map[string]struct {
		existPkgs []ImportedPackage
		pkgSeen   map[string]bool
		impPkgs   []ImportedPackage
		wantErr   bool
	}{
		"basic case with no existing config": {
			existPkgs: make([]ImportedPackage, 0),
			pkgSeen:   make(map[string]bool),
			impPkgs: []ImportedPackage{
				{
					Name:           "golang.org/x/net",
					LockHint:       "",
					Source:         "golang.org/x/net",
					ConstraintHint: "",
					IsOverride:     true,
				},
				{
					Name:           "golang.org/x/sys",
					LockHint:       "",
					Source:         "golang.org/x/sys",
					ConstraintHint: "",
					IsOverride:     true,
				},
				{
					Name:           "golang.org/x/tools",
					LockHint:       "",
					Source:         "golang.org/x/tools",
					ConstraintHint: "",
					IsOverride:     true,
				},
			},
			wantErr: false,
		},
		"overlapping override source returns error": {
			existPkgs: []ImportedPackage{
				{
					Name:           "golang.org/x/net",
					ConstraintHint: importertest.V1Constraint,
					Source:         "overrideSource",
				},
			},
			pkgSeen: map[string]bool{
				"golang.org/x/net": true,
			},
			impPkgs: nil,
			wantErr: true,
		},
		"overlapping override ref throws no error": {
			existPkgs: []ImportedPackage{
				{
					Name:           "golang.org/x/net",
					ConstraintHint: importertest.V1Constraint,
					Source:         "",
					IsOverride:     true,
				},
			},
			pkgSeen: map[string]bool{
				"golang.org/x/net": true,
			},
			impPkgs: []ImportedPackage{
				{
					Name:           "golang.org/x/net",
					LockHint:       "",
					Source:         "golang.org/x/net",
					ConstraintHint: importertest.V1Constraint,
					IsOverride:     true,
				},
				{
					Name:           "golang.org/x/sys",
					LockHint:       "",
					Source:         "golang.org/x/sys",
					ConstraintHint: "",
					IsOverride:     true,
				},
				{
					Name:           "golang.org/x/tools",
					LockHint:       "",
					Source:         "golang.org/x/tools",
					ConstraintHint: "",
					IsOverride:     true,
				},
			},
			wantErr: false,
		},
		"matching source don't error": {
			existPkgs: []ImportedPackage{
				{
					Name:           "golang.org/x/net",
					ConstraintHint: "",
					Source:         "golang.org/x/net",
					IsOverride:     true,
				},
				{
					Name:           "golang.org/x/sys",
					ConstraintHint: "",
					Source:         "golang.org/x/sys",
					IsOverride:     true,
				},
			},
			pkgSeen: map[string]bool{
				"golang.org/x/net": true,
				"golang.org/x/sys": true,
			},
			impPkgs: []ImportedPackage{
				{
					Name:           "golang.org/x/net",
					LockHint:       "",
					Source:         "golang.org/x/net",
					ConstraintHint: "",
					IsOverride:     true,
				},
				{
					Name:           "golang.org/x/sys",
					LockHint:       "",
					Source:         "golang.org/x/sys",
					ConstraintHint: "",
					IsOverride:     true,
				},
				{
					Name:           "golang.org/x/tools",
					LockHint:       "",
					Source:         "golang.org/x/tools",
					ConstraintHint: "",
					IsOverride:     true,
				},
			},
			wantErr: false,
		},
	}

	for name, testCase := range testCases {
		name := name
		t.Run(name, func(t *testing.T) {
			impPkgs, err := AppendBasicOverrides(testCase.existPkgs, testCase.pkgSeen)
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
