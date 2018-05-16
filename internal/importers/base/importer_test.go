// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package base

import (
	"fmt"
	"log"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/importers/importertest"
	"github.com/golang/dep/internal/test"
)

func TestBaseImporter_IsTag(t *testing.T) {
	testcases := map[string]struct {
		input     string
		wantIsTag bool
		wantTag   gps.Version
	}{
		"non-semver tag": {
			input:     importertest.Beta1Tag,
			wantIsTag: true,
			wantTag:   gps.NewVersion(importertest.Beta1Tag).Pair(importertest.Beta1Rev),
		},
		"semver-tag": {
			input:     importertest.V1PatchTag,
			wantIsTag: true,
			wantTag:   gps.NewVersion(importertest.V1PatchTag).Pair(importertest.V1PatchRev)},
		"untagged revision": {
			input:     importertest.UntaggedRev,
			wantIsTag: false,
		},
		"branch name": {
			input:     importertest.V2Branch,
			wantIsTag: false,
		},
		"empty": {
			input:     "",
			wantIsTag: false,
		},
	}

	pi := gps.ProjectIdentifier{ProjectRoot: importertest.Project}

	for name, tc := range testcases {
		name := name
		tc := tc
		t.Run(name, func(t *testing.T) {
			h := test.NewHelper(t)
			defer h.Cleanup()
			// Disable parallel tests until we can resolve this error on the Windows builds:
			// "remote repository at https://github.com/carolynvs/deptest-importers does not exist, or is inaccessible"
			//h.Parallel()

			ctx := importertest.NewTestContext(h)
			sm, err := ctx.SourceManager()
			h.Must(err)
			defer sm.Release()

			i := NewImporter(ctx.Err, ctx.Verbose, sm)
			gotIsTag, gotTag, err := i.isTag(pi, tc.input)
			h.Must(err)

			if tc.wantIsTag != gotIsTag {
				t.Fatalf("unexpected isTag result for %v: \n\t(GOT) %v \n\t(WNT) %v",
					tc.input, gotIsTag, tc.wantIsTag)
			}

			if tc.wantTag != gotTag {
				t.Fatalf("unexpected tag for %v: \n\t(GOT) %v \n\t(WNT) %v",
					tc.input, gotTag, tc.wantTag)
			}
		})
	}
}

func TestBaseImporter_LookupVersionForLockedProject(t *testing.T) {
	testcases := map[string]struct {
		revision    gps.Revision
		constraint  gps.Constraint
		wantVersion string
	}{
		"match revision to tag": {
			revision:    importertest.V1PatchRev,
			wantVersion: importertest.V1PatchTag,
		},
		"match revision with multiple tags using constraint": {
			revision:    importertest.MultiTaggedRev,
			constraint:  gps.NewVersion(importertest.MultiTaggedPlainTag),
			wantVersion: importertest.MultiTaggedPlainTag,
		},
		"revision with multiple tags with no constraint defaults to best match": {
			revision:    importertest.MultiTaggedRev,
			wantVersion: importertest.MultiTaggedSemverTag,
		},
		"revision with multiple tags with nonmatching constraint defaults to best match": {
			revision:    importertest.MultiTaggedRev,
			constraint:  gps.NewVersion("thismatchesnothing"),
			wantVersion: importertest.MultiTaggedSemverTag,
		},
		"untagged revision fallback to branch constraint": {
			revision:    importertest.UntaggedRev,
			constraint:  gps.NewBranch("master"),
			wantVersion: "master",
		},
		"fallback to revision": {
			revision:    importertest.UntaggedRev,
			wantVersion: importertest.UntaggedRev,
		},
	}

	pi := gps.ProjectIdentifier{ProjectRoot: importertest.Project}

	for name, tc := range testcases {
		name := name
		tc := tc
		t.Run(name, func(t *testing.T) {
			h := test.NewHelper(t)
			defer h.Cleanup()
			// Disable parallel tests until we can resolve this error on the Windows builds:
			// "remote repository at https://github.com/carolynvs/deptest-importers does not exist, or is inaccessible"
			//h.Parallel()

			ctx := importertest.NewTestContext(h)
			sm, err := ctx.SourceManager()
			h.Must(err)
			defer sm.Release()

			i := NewImporter(ctx.Err, ctx.Verbose, sm)
			v, err := i.lookupVersionForLockedProject(pi, tc.constraint, tc.revision)
			h.Must(err)

			gotVersion := v.String()
			if gotVersion != tc.wantVersion {
				t.Fatalf("unexpected locked version: \n\t(GOT) %v\n\t(WNT) %v", gotVersion, tc.wantVersion)
			}
		})
	}
}

func TestBaseImporter_ImportProjects(t *testing.T) {
	testcases := map[string]struct {
		importertest.TestCase
		projects []ImportedPackage
	}{
		"tag constraints are skipped": {
			importertest.TestCase{
				WantVersion:  importertest.Beta1Tag,
				WantRevision: importertest.Beta1Rev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.Beta1Rev,
					ConstraintHint: importertest.Beta1Tag,
				},
			},
		},
		"tag lock hints Lock to tagged revision": {
			importertest.TestCase{
				WantVersion:  importertest.Beta1Tag,
				WantRevision: importertest.Beta1Rev,
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project,
					LockHint: importertest.Beta1Tag,
				},
			},
		},
		"untagged revision ignores range constraint": {
			importertest.TestCase{
				WantRevision: importertest.UntaggedRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.UntaggedRev,
					ConstraintHint: importertest.V1Constraint,
				},
			},
		},
		"untagged revision keeps branch constraint": {
			importertest.TestCase{
				WantConstraint: "master",
				WantVersion:    "master",
				WantRevision:   importertest.UntaggedRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.UntaggedRev,
					ConstraintHint: "master",
				},
			},
		},
		"HEAD revisions default constraint to the matching branch": {
			importertest.TestCase{
				DefaultConstraintFromLock: true,
				WantConstraint:            importertest.V2Branch,
				WantVersion:               importertest.V2Branch,
				WantRevision:              importertest.V2Rev,
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project,
					LockHint: importertest.V2Rev,
				},
			},
		},
		"Semver tagged revisions default to ^VERSION": {
			importertest.TestCase{
				DefaultConstraintFromLock: true,
				WantConstraint:            importertest.V1Constraint,
				WantVersion:               importertest.V1Tag,
				WantRevision:              importertest.V1Rev,
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project,
					LockHint: importertest.V1Rev,
				},
			},
		},
		"Semver lock hint defaults constraint to ^VERSION": {
			importertest.TestCase{
				DefaultConstraintFromLock: true,
				WantConstraint:            importertest.V1Constraint,
				WantVersion:               importertest.V1Tag,
				WantRevision:              importertest.V1Rev,
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project,
					LockHint: importertest.V1Tag,
				},
			},
		},
		"Semver constraint hint": {
			importertest.TestCase{
				WantConstraint: importertest.V1Constraint,
				WantVersion:    importertest.V1PatchTag,
				WantRevision:   importertest.V1PatchRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.V1PatchRev,
					ConstraintHint: importertest.V1Constraint,
				},
			},
		},
		"Semver prerelease lock hint": {
			importertest.TestCase{
				WantConstraint: importertest.V2Branch,
				WantVersion:    importertest.V2PatchTag,
				WantRevision:   importertest.V2PatchRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.V2PatchRev,
					ConstraintHint: importertest.V2Branch,
				},
			},
		},
		"Revision constraints are skipped": {
			importertest.TestCase{
				WantVersion:  importertest.V1Tag,
				WantRevision: importertest.V1Rev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.V1Rev,
					ConstraintHint: importertest.V1Rev,
				},
			},
		},
		"Branch constraint hint": {
			importertest.TestCase{
				WantConstraint: "master",
				WantVersion:    importertest.V1Tag,
				WantRevision:   importertest.V1Rev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.V1Rev,
					ConstraintHint: "master",
				},
			},
		},
		"Non-matching semver constraint is skipped": {
			importertest.TestCase{
				WantVersion:  importertest.V1Tag,
				WantRevision: importertest.V1Rev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.V1Rev,
					ConstraintHint: "^2.0.0",
				},
			},
		},
		"git describe constraint is skipped": {
			importertest.TestCase{
				WantRevision: importertest.UntaggedRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.UntaggedRev,
					ConstraintHint: importertest.UntaggedRevAbbrv,
				},
			},
		},
		"consolidate subpackages under root": {
			importertest.TestCase{
				WantConstraint: "master",
				WantVersion:    "master",
				WantRevision:   importertest.UntaggedRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project + "/subpkA",
					ConstraintHint: "master",
				},
				{
					Name:     importertest.Project,
					LockHint: importertest.UntaggedRev,
				},
			},
		},
		"skip duplicate packages": {
			importertest.TestCase{
				WantRevision: importertest.UntaggedRev,
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project + "/subpkgA",
					LockHint: importertest.UntaggedRev, // first wins
				},
				{
					Name:     importertest.Project + "/subpkgB",
					LockHint: importertest.V1Rev,
				},
			},
		},
		"skip empty lock hints": {
			importertest.TestCase{
				WantRevision: "",
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project,
					LockHint: "",
				},
			},
		},
		"alternate source": {
			importertest.TestCase{
				WantConstraint: "*",
				WantSourceRepo: importertest.ProjectSrc,
			},
			[]ImportedPackage{
				{
					Name:   importertest.Project,
					Source: importertest.ProjectSrc,
				},
			},
		},
		"skip default source": {
			importertest.TestCase{
				WantSourceRepo: "",
			},
			[]ImportedPackage{
				{
					Name:   importertest.Project,
					Source: "https://" + importertest.Project,
				},
			},
		},
		"override only not in lock": {
			importertest.TestCase{
				WantOverride: "master",
				WantVersion:  "",
				WantRevision: "",
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       "",
					ConstraintHint: "master",
					IsOverride:     true,
				},
			},
		},
		"matching constraint and override result in override": {
			importertest.TestCase{
				WantOverride: importertest.V2Branch,
				WantVersion:  importertest.V2Branch,
				WantRevision: "",
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       "",
					ConstraintHint: importertest.V2Branch,
					IsOverride:     true,
				},
				{
					Name:           importertest.Project,
					LockHint:       "",
					ConstraintHint: importertest.V2Branch,
					IsOverride:     false,
				},
			},
		},
		"mismatch constraint and override result in taking the override": {
			importertest.TestCase{
				WantOverride: "master",
				WantVersion:  "master",
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       "",
					ConstraintHint: "master",
					IsOverride:     true,
				},
				{
					Name:           importertest.Project,
					LockHint:       "",
					ConstraintHint: importertest.V2Branch,
					IsOverride:     false,
				},
			},
		},
		"override and lock result in override constraint and no error": {
			importertest.TestCase{
				WantOverride: importertest.V2Branch,
				WantVersion:  importertest.V2Branch,
				WantRevision: "",
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.UntaggedRev,
					ConstraintHint: "",
					IsOverride:     false,
				},
				{
					Name:           importertest.Project,
					LockHint:       "",
					ConstraintHint: importertest.V2Branch,
					IsOverride:     true,
				},
			},
		},
		"override source clobbers lock source": {
			importertest.TestCase{
				WantOverride:   "master",
				WantVersion:    "master",
				WantSourceRepo: importertest.ProjectSrc,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.UntaggedRev,
					ConstraintHint: "",
					Source:         importertest.ProjectSrcInvalid,
					IsOverride:     false,
				},
				{
					Name:           importertest.Project,
					LockHint:       "",
					ConstraintHint: "master",
					Source:         importertest.ProjectSrc,
					IsOverride:     true,
				},
			},
		},
		"explicitly no overrides shows up as constraint": {
			importertest.TestCase{
				WantConstraint: "master",
				WantVersion:    "master",
				WantRevision:   "",
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       "",
					ConstraintHint: "master",
					IsOverride:     false,
				},
			},
		},
		"conflicting sources overriden by override source": {
			importertest.TestCase{
				WantOverride:   "master",
				WantVersion:    "master",
				WantRevision:   "",
				WantSourceRepo: importertest.ProjectSrc,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       "",
					Source:         importertest.ProjectSrcInvalid,
					ConstraintHint: importertest.V2Branch,
					IsOverride:     false,
				},
				{
					Name:           importertest.Project,
					LockHint:       "",
					Source:         importertest.ProjectSrcInvalid,
					ConstraintHint: importertest.V2Branch,
					IsOverride:     false,
				},
				{
					Name:           importertest.Project,
					LockHint:       "",
					Source:         importertest.ProjectSrc,
					ConstraintHint: "master",
					IsOverride:     true,
				},
			},
		},
		"source override should not clobber version constraint": {
			importertest.TestCase{
				WantOverride:   importertest.V2Branch,
				WantVersion:    importertest.V2Branch,
				WantRevision:   "",
				WantSourceRepo: importertest.ProjectSrc,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       "",
					Source:         importertest.ProjectSrcInvalid,
					ConstraintHint: importertest.V2Branch,
					IsOverride:     false,
				},
				{
					Name:       importertest.Project,
					LockHint:   "",
					Source:     importertest.ProjectSrc,
					IsOverride: true,
				},
			},
		},
		"apache thrift sources should be ignored": {
			importertest.TestCase{
				WantConstraint: importertest.V2Branch,
				WantSourceRepo: "",
			},
			[]ImportedPackage{
				{
					Name:   importertest.Project,
					Source: "git://git.apache.org/thrift.git",
					//set the constraint hint so the project does not get filtered out when the source is removed
					ConstraintHint: importertest.V2Branch,
				},
			},
		},
		"git wip us apache thrift sources should be ignored": {
			importertest.TestCase{
				WantConstraint: importertest.V2Branch,
				WantSourceRepo: "",
			},
			[]ImportedPackage{
				{
					Name:   importertest.Project,
					Source: "https://git-wip-us.apache.org/repos/asf/thrift.git",
					//set the constraint hint so the project does not get filtered out when the source is removed
					ConstraintHint: importertest.V2Branch,
				},
			},
		},
		"skip vendored source": {
			importertest.TestCase{
				WantSourceRepo: "",
				WantWarning:    "vendored sources aren't supported",
			},
			[]ImportedPackage{
				{
					Name:   importertest.Project,
					Source: "example.com/vendor/" + importertest.Project,
				},
			},
		},
		"invalid project root": {
			importertest.TestCase{
				WantSourceRepo: "",
				WantWarning:    "Warning: Skipping project. Cannot determine the project root for invalid-project",
			},
			[]ImportedPackage{
				{
					Name: "invalid-project",
				},
			},
		},
		"nonexistent project": {
			importertest.TestCase{
				WantSourceRepo: "",
				WantWarning: fmt.Sprintf(
					"Warning: Skipping project. Unable to import lock %q for %s",
					importertest.V1Tag, importertest.NonexistentPrj,
				),
			},
			[]ImportedPackage{
				{
					Name:     importertest.NonexistentPrj,
					LockHint: importertest.V1Tag,
				},
			},
		},
	}

	for name, tc := range testcases {
		name := name
		tc := tc
		t.Run(name, func(t *testing.T) {
			err := tc.Execute(t, func(logger *log.Logger, sm gps.SourceManager) (*dep.Manifest, *dep.Lock) {
				i := NewImporter(logger, true, sm)
				i.ImportPackages(tc.projects, tc.DefaultConstraintFromLock)
				return i.Manifest, i.Lock
			})
			if err != nil {
				t.Fatalf("%#v", err)
			}
		})
	}
}

func TestFilteringApacheThrift(t *testing.T) {
	testcases := map[string]struct {
		source         string
		expectedSource string
	}{
		"apache thrift source is ignored": {
			source:         "git://git.apache.org/thrift.git",
			expectedSource: "",
		},
		"git wip us apache thrift source is ignored": {
			source:         "https://git-wip-us.apache.org/repos/asf/thrift.git",
			expectedSource: "",
		},
		"apache thriftrw source is not ignored": {
			source:         "git://git.apache.org/thriftrw.git",
			expectedSource: "git://git.apache.org/thriftrw.git",
		},
		"apache thriftrw mirror source is not ignored": {
			source:         "ssh://gitolite@code.uber.internal/github/apache/thrift.git",
			expectedSource: "ssh://gitolite@code.uber.internal/github/apache/thrift.git",
		},
	}
	for name, tc := range testcases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			gotSource := filterApacheThriftSource(tc.source)
			if gotSource != tc.expectedSource {
				t.Fatal("expected source did not meet source received")
			}
		})
	}
}

func TestBaseImporter_FilterGitoliteUrl(t *testing.T) {
	testcases := map[string]struct {
		gitoliteProject     gps.ProjectRoot
		gitoliteURL         string
		wantGitoliteProject gps.ProjectRoot
		wantGitoliteURL     string
	}{
		"cloned gitolite urls are filtered": {
			gitoliteProject:     importertest.GitoliteProject,
			gitoliteURL:         importertest.GitoliteSrc,
			wantGitoliteURL:     importertest.FilteredGitoliteUrl,
			wantGitoliteProject: importertest.GitoliteProject,
		},
		"cloned gitolite urls with matching suffix are filtered": {
			gitoliteProject:     importertest.GitoliteProject + ".git",
			gitoliteURL:         importertest.GitoliteSrc + ".git",
			wantGitoliteURL:     importertest.FilteredGitoliteUrl + ".git",
			wantGitoliteProject: importertest.GitoliteProject + ".git",
		},
		"removes .git suffix from Project Root to match source": {
			gitoliteProject:     importertest.GitoliteProject + ".git",
			gitoliteURL:         importertest.GitoliteSrc,
			wantGitoliteURL:     importertest.FilteredGitoliteUrl,
			wantGitoliteProject: importertest.GitoliteProject,
		},
		"removes .git suffix from source to match project root": {
			gitoliteProject:     importertest.GitoliteProject,
			gitoliteURL:         importertest.GitoliteSrc + ".git",
			wantGitoliteURL:     importertest.FilteredGitoliteUrl,
			wantGitoliteProject: importertest.GitoliteProject,
		},
		"filters properly with port": {
			gitoliteProject:     importertest.GitoliteProject,
			gitoliteURL:         "gitolite@code.uber.internal:12345/personal/cwest1/depTest.git",
			wantGitoliteURL:     importertest.FilteredGitoliteUrl,
			wantGitoliteProject: importertest.GitoliteProject,
		},
		"handles unambiguously formed link": {
			gitoliteProject:     importertest.GitoliteProject + ".git",
			gitoliteURL:         "gitolite@" + importertest.GitoliteProject + ".git",
			wantGitoliteURL:     importertest.FilteredGitoliteUrl + ".git",
			wantGitoliteProject: importertest.GitoliteProject + ".git",
		},
		"returns code.uber.internal links without gitolite prefix unfiltered": {
			gitoliteProject:     importertest.GitoliteProject,
			gitoliteURL:         importertest.GitoliteProject,
			wantGitoliteURL:     importertest.GitoliteProject,
			wantGitoliteProject: importertest.GitoliteProject,
		},
		"returns non-gitolite links unfiltered": {
			gitoliteProject:     importertest.GitoliteProject,
			gitoliteURL:         importertest.ProjectSrc,
			wantGitoliteURL:     importertest.ProjectSrc,
			wantGitoliteProject: importertest.GitoliteProject,
		},
		"suffix characters that do not introduce ambiguity are filtered normally": {
			gitoliteProject:     importertest.GitoliteProject,
			gitoliteURL:         "gitolite@code.uber.internal:hello#at$gmail/s.git",
			wantGitoliteURL:     "ssh://gitolite@code.uber.internal/hello#at$gmail/s",
			wantGitoliteProject: importertest.GitoliteProject,
		},
		"urls with more than one : are not filtered": {
			gitoliteProject:     importertest.GitoliteProject,
			gitoliteURL:         "gitolite@code.uber.internal:hello#at$gmail:/s",
			wantGitoliteURL:     "gitolite@code.uber.internal:hello#at$gmail:/s",
			wantGitoliteProject: importertest.GitoliteProject,
		},
		"urls with shorter paths are filtered normally": {
			gitoliteProject:     "code.uber.internal/foo.git",
			gitoliteURL:         "gitolite@code.uber.internal/foo.git",
			wantGitoliteURL:     "ssh://gitolite@code.uber.internal/foo.git",
			wantGitoliteProject: "code.uber.internal/foo.git",
		},
		"urls with longer paths are filtered normally": {
			gitoliteProject:     "code.uber.internal/foo/bar/baz/qux.git",
			gitoliteURL:         "gitolite@code.uber.internal/foo/bar/baz/qux.git",
			wantGitoliteURL:     "ssh://gitolite@code.uber.internal/foo/bar/baz/qux.git",
			wantGitoliteProject: "code.uber.internal/foo/bar/baz/qux.git",
		},
	}

	for name, tc := range testcases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			gotGitoliteURL, gotGitoliteProjectRoot := NormalizeGitoliteURL(tc.gitoliteURL, tc.gitoliteProject)
			if gotGitoliteURL != tc.wantGitoliteURL {
				t.Fatalf("unexpected gitolite url: \nt(GOT) %v\n\t(WNT) %v", gotGitoliteURL, tc.wantGitoliteURL)
			}

			if tc.wantGitoliteProject != "" {
				if gotGitoliteProjectRoot != tc.wantGitoliteProject {
					t.Fatalf("unexpected project root: \nt(GOT) %v\n\t(WNT) %v", gotGitoliteProjectRoot, tc.wantGitoliteProject)
				}
			}
		})
	}
}
