package analyze

import (
	"reflect"
	"testing"
)

type analyzeTestCase map[string]struct {
	givenRootProj, expRootProj, givenName, expName, givenVer, expVer, givenSelected, prevSelected string
	expDep, givenCurNode                                                                          *TreeNode
	prevDeps                                                                                      []*TreeNode
	expTree                                                                                       *ResolverTree
	givenVers, expVers, prevVers, newVers                                                         []string
}

func TestUber_Analyze_InitializeResolverTree(t *testing.T) {
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
			resolverTree := InitializeResTree(tc.givenRootProj)
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

			if len(gotVersionTree.Versions) != len(wantVersionTree.Versions) {
				t.Fatalf("unexpected \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(gotVersionTree.Deps), len(wantVersionTree.Deps))
			}

			if len(gotVersionTree.Deps) != len(wantVersionTree.Deps) {
				t.Fatalf("unexpected deps \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(gotVersionTree.Deps), len(wantVersionTree.Deps))
			}

			if len(resolverTree.NodeList) != len(tc.expTree.NodeList) {
				t.Fatalf("unexpected number of nodes in the Node List \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(resolverTree.NodeList), len(tc.expTree.NodeList))
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

			if gotDep.Selected != "" {
				t.Fatalf("unexpected VersionTree selection \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, gotDep.Selected, wantDep.Selected)
			}

			if len(gotDep.Versions) != len(wantDep.Versions) {
				t.Fatalf("unexpected VersionTree Deps \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(gotDep.Deps), len(wantDep.Deps))
			}

			if len(gotDep.Deps) != len(wantDep.Deps) {
				t.Fatalf("unexpected VersionTree Deps \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(gotDep.Deps), len(wantDep.Deps))
			}
		})
	}
}

func TestUber_Analyze_AddDep(t *testing.T) {
	cases := analyzeTestCase{
		"adding a dependency to the current node increases the number of deps in that node": {
			givenRootProj: "root",
			givenName:     Project,
			givenCurNode:  RootNode,
			expDep:        DependerNode,
		},

		"deps are appended to the given input tree": {
			givenRootProj: "root",
			givenName:     Project,
			expName:       Project,
			givenCurNode:  RootNode,
			expDep:        DependerNode,
		},
	}

	for name, tc := range cases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			resTree := InitializeResTree(tc.givenRootProj)
			currentNode := resTree.NodeList[tc.givenRootProj]
			currentDepCount := len(currentNode.Deps)

			AddDep(currentNode.Name, tc.givenName)
			newDepCount := len(currentNode.Deps)
			if newDepCount != currentDepCount+1 {
				t.Fatalf("unexpected Dep count \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, currentDepCount, newDepCount)
			}

			if currentNode.Deps[len(currentNode.Deps)-1].Name != tc.givenName {
				t.Fatalf("unexpected Dep name \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, currentNode.Deps[len(currentNode.Deps)-1].Name, tc.givenName)
			}
		})
	}
}

func TestUber_Analyze_AddVersion(t *testing.T) {
	cases := analyzeTestCase{
		"adding a version to a node should increase node's version list length by 1": {
			givenRootProj: "root",
			givenName:     Project,
			givenCurNode:  RootNode,
			expDep:        DependerNode,
			givenVers:     []string{"1.0.0"},
			expVers:       []string{"1.0.0"},
		},

		"versions appear in the order that they are encountered": {
			givenRootProj: "root",
			givenName:     Project,
			expName:       Project,
			givenCurNode:  RootNode,
			givenVers:     []string{"1.0.0", "3.0.0", "2.0.0"},
			expVers:       []string{"1.0.0", "3.0.0", "2.0.0"},
		},
	}

	for name, tc := range cases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			ClearTree()
			resTree := InitializeResTree(tc.givenRootProj)
			currentNode := resTree.NodeList[tc.givenRootProj]
			currentVerCount := len(currentNode.Versions)

			for _, version := range tc.givenVers {
				AddVersion(currentNode.Name, version)
			}

			newVerCount := len(currentNode.Versions)
			if newVerCount != currentVerCount+len(tc.givenVers) {
				t.Fatalf("unexpected Dep count \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, currentVerCount, newVerCount)
			}

			for i := 0; i < len(currentNode.Versions); i++ {
				if tc.expVers[i] != currentNode.Versions[i] {
					t.Fatalf("version out of order \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, currentNode.Versions[i], tc.expVer[i])
				}
			}
		})
	}
}

func TestUber_Analyze_SelectVersion(t *testing.T) {
	cases := analyzeTestCase{
		"Should selected a version when no versions has been selected yet": {
			givenRootProj: "root",
			givenName:     Project,
			givenCurNode:  RootNode,
			expDep:        DependerNode,
			givenSelected: "1.0.0",
			prevSelected:  "",
		},

		"should over-write a previously selected version": {
			givenRootProj: "root",
			givenName:     Project,
			expName:       Project,
			givenCurNode:  RootNode,
			expDep:        DependerNode,
			givenSelected: "2.0.0",
			prevSelected:  "1.0.0",
		},

		"should maintain the old version list when a new dep is selected": {
			givenRootProj: "root",
			givenName:     Project,
			expName:       Project,
			givenCurNode:  RootNode,
			expDep:        DependerNode,
			givenSelected: "2.0.0",
			prevSelected:  "1.0.0",
			prevVers:      []string{"3.0.0", "2.0.0"},
		},

		"should clear all deps for an old version when a new verison is selected": {
			givenRootProj: "root",
			givenName:     Project,
			expName:       Project,
			givenCurNode:  RootNode,
			expDep:        DependerNode,
			givenSelected: "2.0.0",
			prevSelected:  "1.0.0",
			prevVers:      []string{"3.0.0", "2.0.0"},
			prevDeps:      Deps,
		},
	}
	for name, tc := range cases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			resTree := InitializeResTree(tc.givenRootProj)
			currentNode := resTree.NodeList[tc.givenRootProj]
			currentNode.Versions = tc.prevVers
			currentNode.Selected = tc.prevSelected
			currentNode.Deps = tc.prevDeps

			SelectVersion(currentNode.Name, tc.givenSelected)

			if currentNode.Selected != tc.givenSelected {
				t.Fatalf("unexpected Selected version \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, currentNode.Selected, tc.givenSelected)
			}

			//this function doesn't perform the actual adding of versions to the list, so that is tested separately
			if len(currentNode.Versions) != len(tc.prevVers) {
				t.Fatalf("unexpected change in version list \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, currentNode.Versions, tc.prevVers)
			}

			newDepCount := len(currentNode.Deps)
			if newDepCount != 0 {
				t.Fatalf("unexpected Dep count \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, newDepCount, 0)
			}
		})
	}
}
