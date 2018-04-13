package analyze

import (
	"reflect"
	"testing"
)

type analyzeTestCase map[string]struct {
	givenRootProj, expRootProj, givenName, expName, givenVerType, expVerParent, givenVer1, givenSelected, expSelected string
	expDep, givenCurNode                                                                                              *TreeNode
	expTree                                                                                                           *ResolverTree
	givenVers, expVers                                                                                                []string
}

func TestUber_Analyze_NewResolverTree(t *testing.T) {
	cases := analyzeTestCase{
		"instantiates an empty resolver tree": {
			givenRootProj: RootProject,
			expRootProj:   RootProject,
			expTree:       ResolverTreeRoot,
		},
	}

	for name, tc := range cases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			resolverTree := NewResolverTree(tc.givenRootProj)
			gotVersionTree := resolverTree.VersionTree
			wantVersionTree := tc.expTree.VersionTree

			if reflect.TypeOf(gotVersionTree) != reflect.TypeOf(wantVersionTree) {
				t.Fatalf("unexpected type \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, reflect.TypeOf(gotVersionTree), reflect.TypeOf(wantVersionTree))
			}

			if gotVersionTree.Name != wantVersionTree.Name {
				t.Fatalf("unexpected name \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, gotVersionTree.Name, wantVersionTree.Name)
			}

			if gotVersionTree.Selected != wantVersionTree.Selected {
				t.Fatalf("unexpected selection \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, gotVersionTree.Selected, wantVersionTree.Selected)
			}

			if len(gotVersionTree.Deps) != len(wantVersionTree.Deps) {
				t.Fatalf("unexpected \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(gotVersionTree.Deps), len(wantVersionTree.Deps))
			}

			if len(gotVersionTree.Deps) != 0 {
				t.Fatalf("unexpected deps \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(gotVersionTree.Deps), len(wantVersionTree.Deps))
			}
		})
	}
}

func TestUber_Analyze_newTreeNode(t *testing.T) {
	cases := analyzeTestCase{
		"deps are created with a name and have otherwise empty attributes": {
			givenName: Project,
			expName:   Project,
			expDep:    DependerNode,
		},
	}

	for name, tc := range cases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			wantDep := tc.expDep
			gotDep := newTreeNode(tc.givenName)

			if gotDep.Name != wantDep.Name {
				t.Fatalf("unexpected VersionTree name \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, gotDep.Name, wantDep.Name)
			}

			if gotDep.Selected != wantDep.Selected {
				t.Fatalf("unexpected VersionTree selection \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, gotDep.Selected, wantDep.Selected)
			}

			if len(gotDep.Deps) != len(wantDep.Deps) {
				t.Fatalf("unexpected VersionTree Deps \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(gotDep.Deps), len(wantDep.Deps))
			}

			if len(gotDep.Deps) != 0 {
				t.Fatalf("unexpected VersionTree Deps \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(gotDep.Deps), len(wantDep.Deps))
			}
		})
	}
}

func TestUber_Analyze_AddDep(t *testing.T) {
	cases := analyzeTestCase{
		"adding a dependency to the current node increases the number of deps in that node": {
			givenName:    Project,
			givenCurNode: RootNode,
			expDep:       DependerNode,
		},

		"deps are appended to the given input tree": {
			givenName:    Project,
			expName:      Project,
			givenCurNode: RootNode,
			expDep:       DependerNode,
		},
	}

	for name, tc := range cases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			currentNode := tc.givenCurNode
			currentDepCount := len(currentNode.Deps)

			currentNode.AddDep(tc.givenName)
			newDepCount := len(currentNode.Deps)
			if newDepCount != currentDepCount+1 {
				t.Fatalf("unexpected Dep count \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, currentDepCount, newDepCount)
			}

			if currentNode.Deps[0].Name != tc.givenName {
				t.Fatalf("unexpected Dep name \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, currentNode.Deps[0].Name, tc.givenName)
			}
		})
	}
}

func TestUber_Analyze_RemoveVersion(t *testing.T) {
	cases := analyzeTestCase{
		"semver Versions are handled": {
			givenCurNode: NodeWithVersAndSel,
			givenVers:    []string{"v2.0.3", "2.0.4", "somerevision", "bc123314oi3241923h23sdafn"},
		},
		"unfound versions raise an error": {
			givenCurNode: NodeWithVersAndSel,
			givenVers:    []string{"unfound version"},
		},
		"removes last version in the slice without panic": {
			givenCurNode: NodeWithVersAndSel,
			givenVers:    []string{"2.0.4"},
		},
	}

	for name, tc := range cases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			currentNode := tc.givenCurNode
			prevLength := len(currentNode.Versions)
			for _, ver := range tc.givenVers {
				err := currentNode.RemoveVersion(ver)
				if err == nil {
					if len(currentNode.Versions) != prevLength-1 {
						t.Fatalf("unexpected number of Versions in tree %v \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(currentNode.Versions), prevLength-1)
					}
					prevLength -= 1
				} else {
					if len(currentNode.Versions) != prevLength {
						t.Fatalf("unexpected number of Versions in tree \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(currentNode.Versions), len(currentNode.Versions))
					}
				}
			}
		})
	}
}
