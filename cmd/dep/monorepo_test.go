package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestMergeMetadata(t *testing.T) {
	tests := []struct {
		summary          string
		sourceManifest   map[gps.ProjectRoot]gps.ProjectProperties
		sourceLock       []gps.LockedProject
		targetManifest   map[gps.ProjectRoot]gps.ProjectProperties
		targetLock       []gps.LockedProject
		expectedManifest map[gps.ProjectRoot]gps.ProjectProperties
		expectedLock     []gps.LockedProject
		expectedError    error
	}{
		{"Newer version locked in the source that is allowed by target constraint.",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.1"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.1"), []string{"common"}),
			},
			nil,
		},
		{"Multiple packages added at the same time.",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
				"github.com/user/baz": {Constraint: semverConstraint("^2.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common"}),
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/baz"}, gps.NewVersion("2.0"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/foo": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/foo"}, gps.NewVersion("1.0"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/foo": {Constraint: semverConstraint("^1.0")},
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
				"github.com/user/baz": {Constraint: semverConstraint("^2.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/foo"}, gps.NewVersion("1.0"), []string{"common"}),
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common"}),
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/baz"}, gps.NewVersion("2.0"), []string{"common"}),
			},
			nil,
		},
		{"Mismatch on the master branch can be ignored.",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: gps.NewBranch("master")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("7f5c9916f5d3154551d2d53a6cfaace8f75e0a07"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: gps.NewBranch("master")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("937bbe32f715802505cbe12e19429b2d04146fc4"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: gps.NewBranch("master")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("7f5c9916f5d3154551d2d53a6cfaace8f75e0a07"), []string{"common"}),
			},
			nil,
		},
		{"Source contains new import paths.",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"tools"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common", "tools"}),
			},
			nil,
		},
		{"Import is not allowed by target constraint",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.1"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common"}),
			},
			nil, nil, errors.New("failed to merge metadata."),
		},
		{"Merge manifest constraints.",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.1")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.1"), []string{"tools"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.1")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.1"), []string{"common", "tools"}),
			},
			nil,
		},
		{"Source manifest is empty, keep target constraint.",
			map[gps.ProjectRoot]gps.ProjectProperties{
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.1"), []string{"tools"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.1"), []string{"common", "tools"}),
			},
			nil,
		},
		{"Target manifest didn't have a constraint.",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"tools"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common", "tools"}),
			},
			nil,
		},
		{"Both manifests have no constraints.",
			map[gps.ProjectRoot]gps.ProjectProperties{
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"tools"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common", "tools"}),
			},
			nil,
		},
		{"Manifest constraint intersection is invalid.",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^2.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("2.0"), []string{"tools"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.0"), []string{"common"}),
			},
			nil, nil, errors.New("failed to merge metadata."),
		},
	}

	for idx, test := range tests {
		fmt.Printf("Running test case #%v: %v\n", idx, test.summary)
		source := newProject(test.sourceManifest, test.sourceLock)
		target := newProject(test.targetManifest, test.targetLock)

		ctx := new(dep.Ctx)
		logger := log.New(os.Stdout, "", 0)
		ctx.Out = logger
		ctx.Err = logger

		err := mergeMetadata(source, target, ctx)
		if test.expectedError != nil {
			assert.Equal(t, test.expectedError.Error(), err.Error())
			continue // No further verification is needed.
		}
		assert.Nil(t, err)
		assert.Equal(t, len(test.expectedManifest), len(target.Manifest.Constraints))
		for root, constraint := range target.Manifest.Constraints {
			assert.Equal(t, test.expectedManifest[root], constraint)
		}
		assert.Equal(t, len(test.expectedLock), len(target.Lock.P))
		for _, lock := range target.Lock.P {
			found := false
			for _, expectedLock := range test.expectedLock {
				if lock.Ident().ProjectRoot == expectedLock.Ident().ProjectRoot && lock.Ident().Source == expectedLock.Ident().Source {
					found = true
					assert.Equal(t, expectedLock.Version(), lock.Version())
					assert.Equal(t, expectedLock.Packages(), lock.Packages())
				}
			}
			assert.True(t, found, fmt.Sprintf("unexpected dependency on %v", lock.Ident()))
		}
	}
}

func TestDeleteFromRootLockAndManifest(t *testing.T) {
	tests := []struct {
		summary          string
		importRoot       string
		manifest         map[gps.ProjectRoot]gps.ProjectProperties
		lock             []gps.LockedProject
		expectedManifest map[gps.ProjectRoot]gps.ProjectProperties
		expectedLock     []gps.LockedProject
	}{
		{"Remove from lock and manifest.",
			"github.com/user/bar",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/foo": {Constraint: semverConstraint("^1.0")},
				"github.com/user/bar": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/foo"}, gps.NewVersion("1.1"), []string{"common"}),
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.1"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/foo": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/foo"}, gps.NewVersion("1.1"), []string{"common"}),
			},
		},
		{"Remove from lock and manifest with .git suffixes.",
			"github.com/user/bar",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/foo.git": {Constraint: semverConstraint("^1.0")},
				"github.com/user/bar.git": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/foo.git"}, gps.NewVersion("1.1"), []string{"common"}),
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar.git"}, gps.NewVersion("1.1"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/foo.git": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/foo.git"}, gps.NewVersion("1.1"), []string{"common"}),
			},
		},

		{"Remove from lock only.",
			"github.com/user/bar",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/foo": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/foo"}, gps.NewVersion("1.1"), []string{"common"}),
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/bar"}, gps.NewVersion("1.1"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/foo": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/foo"}, gps.NewVersion("1.1"), []string{"common"}),
			},
		},
		{"Package didn't exist in manifest and lock.",
			"github.com/user/bar",
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/foo": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/foo"}, gps.NewVersion("1.1"), []string{"common"}),
			},
			map[gps.ProjectRoot]gps.ProjectProperties{
				"github.com/user/foo": {Constraint: semverConstraint("^1.0")},
			},
			[]gps.LockedProject{
				gps.NewLockedProject(gps.ProjectIdentifier{ProjectRoot: "github.com/user/foo"}, gps.NewVersion("1.1"), []string{"common"}),
			},
		},
	}
	for idx, test := range tests {
		fmt.Printf("Running test case #%v: %v\n", idx, test.summary)
		rootProj := newProject(test.manifest, test.lock)
		proj := new(dep.Project)
		proj.ImportRoot = gps.ProjectRoot(test.importRoot)
		deleteFromRootLockAndManifest(rootProj, proj)

		assert.Equal(t, len(test.expectedManifest), len(rootProj.Manifest.Constraints))
		for root, constraint := range rootProj.Manifest.Constraints {
			assert.Equal(t, test.expectedManifest[root], constraint)
		}
		assert.Equal(t, len(test.expectedLock), len(rootProj.Lock.P))
		for _, lock := range rootProj.Lock.P {
			found := false
			for _, expectedLock := range test.expectedLock {
				if lock.Ident().ProjectRoot == expectedLock.Ident().ProjectRoot && lock.Ident().Source == expectedLock.Ident().Source {
					found = true
					assert.Equal(t, expectedLock.Version(), lock.Version())
					assert.Equal(t, expectedLock.Packages(), lock.Packages())
				}
			}
			assert.True(t, found, fmt.Sprintf("unexpected dependency on %v", lock.Ident()))
		}
	}
}

func TestMonorepoCommandHidden(t *testing.T) {
	command := new(monorepoCommand)
	assert.True(t, command.Hidden())
}

func TestVerifyExists(t *testing.T) {
	defer func() { existsOnDisk = exists }()
	tests := []struct {
		summary           string
		imp               string
		src               string
		vendor            string
		pkg               pkgtree.Package
		existingLocations []string
		resultUnresolved  bool
		resultAmbiguous   bool
	}{
		{
			"Location exists in source.",
			"github.com/foo/bar/baz",
			"/go/src",
			"/go/src/vendor",
			pkgtree.Package{Name: "test-package", ImportPath: "code.uber.internal/devexp/test-package"},
			[]string{"/go/src/github.com/foo/bar/baz"},
			false, false,
		},
		{
			"Location exists in vendor.",
			"github.com/foo/bar/baz",
			"/go/src",
			"/go/src/vendor",
			pkgtree.Package{Name: "test-package", ImportPath: "code.uber.internal/devexp/test-package"},
			[]string{"/go/src/vendor/github.com/foo/bar/baz"},
			false, false,
		},
		{
			"Location exists in both src and vendor.",
			"github.com/foo/bar/baz",
			"/go/src",
			"/go/src/vendor",
			pkgtree.Package{Name: "test-package", ImportPath: "code.uber.internal/devexp/test-package"},
			[]string{"/go/src/github.com/foo/bar/baz", "/go/src/vendor/github.com/foo/bar/baz"},
			false, true,
		},
		{
			"Location exists in both src and vendor.",
			"github.com/foo/bar/baz",
			"/go/src",
			"/go/src/vendor",
			pkgtree.Package{Name: "test-package", ImportPath: "code.uber.internal/devexp/test-package"},
			[]string{"/go/src/github.com/boo/goo/zoo"},
			true, false,
		},
		{
			"Standard import path.",
			"fmt",
			"/go/src",
			"/go/src/vendor",
			pkgtree.Package{Name: "test-package", ImportPath: "code.uber.internal/devexp/test-package"},
			[]string{"/go/src/github.com/boo/goo/zoo"},
			false, false,
		},
	}
	for _, test := range tests {
		existsOnDisk = func(path string) bool {
			for _, l := range test.existingLocations {
				if l == path {
					return true
				}
			}
			return false
		}
		unresolved := make(map[string][]string)
		ambiguous := make(map[string][]string)
		verifyExists(test.imp, test.src, test.vendor, unresolved, ambiguous, test.pkg)
		assert.Equal(t, test.resultUnresolved, len(unresolved) != 0, test.summary)
		assert.Equal(t, test.resultAmbiguous, len(ambiguous) != 0, test.summary)
		pkgPath := filepath.Join(test.pkg.ImportPath, test.pkg.Name)
		if len(unresolved) > 0 {
			assert.Equal(t, []string{test.imp}, unresolved[pkgPath], test.summary)
		}
		if len(ambiguous) > 0 {
			assert.Equal(t, []string{test.imp}, ambiguous[pkgPath], test.summary)
		}
	}

}

func newProject(manifestConstraints map[gps.ProjectRoot]gps.ProjectProperties, lockedProjects []gps.LockedProject) *dep.Project {
	proj := new(dep.Project)
	proj.Manifest = dep.NewManifest()
	proj.Manifest.Constraints = manifestConstraints
	proj.Lock = new(dep.Lock)
	proj.Lock.P = lockedProjects
	return proj
}

func semverConstraint(version string) gps.Constraint {
	constraint, e := gps.NewSemverConstraint(version)
	if e != nil {
		panic("Unable to create a constraint for " + version)
	}
	return constraint
}
