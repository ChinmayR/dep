package gps

import (
	"sort"
	"testing"

	"github.com/golang/dep/uber"
	"github.com/golang/dep/uber/analyze"
)

// Recycled, with modifications, from solve_test.go TestBasicSolves(t ....)
func TestBasicResolverTreeSolves(t *testing.T) {
	defer uber.SetAndUnsetEnvVar(uber.UseNonDefaultVersionBranches, "yes")()
	names := make([]string, 0, len(basicFixturesWithResolverTree))
	for n := range basicFixturesWithResolverTree {
		names = append(names, n)
	}

	sort.Strings(names)
	for _, n := range names {
		n := n
		t.Run(n, func(t *testing.T) {
			testSolveHelper(basicFixturesWithResolverTree[n], t)
		})
	}
}

// Recycled with modifications from solveBasicsAndCheck();
// solver's version accepts basicFixture and returns another test function
// this one accepts BasicFixtureWithResolverTree and runs the tests.
func testSolveHelper(fix BasicFixtureWithResolverTree, t *testing.T) {
	sm := newdepspecSM(fix.ds, nil)
	if fix.broken != "" {
		t.Skip(fix.broken)
	}

	params := SolveParameters{
		RootDir:         string(fix.ds[0].n),
		RootPackageTree: fix.rootTree(),
		Manifest:        fix.rootmanifest(),
		Lock:            dummyLock{},
		Downgrade:       fix.downgrade,
		ChangeAll:       fix.changeall,
		ToChange:        fix.changelist,
		ProjectAnalyzer: naiveAnalyzer{},
	}

	if fix.l != nil {
		params.Lock = fix.l
	}

	analyze.ClearTree()
	fixSolve(params, sm, t)
	resolverTree := analyze.GetResTree()
	testResolverTree(fix.tree, resolverTree, t)
}

func testResolverTree(expTree analyze.ResolverTree, gotTree *analyze.ResolverTree, t *testing.T) {
	if gotTree == nil {
		t.Fatalf("no tree returned from solver")
	}

	if gotTree.VersionTree.Name != expTree.VersionTree.Name {
		t.Fatalf("Unexpected Root Project: \n\t(WNT) %v \n\t(GOT) %v", expTree.VersionTree.Name, gotTree.VersionTree.Name)
	}

	for expName, expNode := range expTree.NodeList {
		gotNode := gotTree.NodeList[expName]
		if gotNode == nil {
			t.Fatalf("Missing Node: \n\t(WNT) %v \n\t(GOT) %v", expNode, "")
		}

		for i := 0; i < len(expNode.Deps); i++ {
			expDep := expNode.Deps[i]
			found := false
			for j := 0; j < len(gotNode.Deps); j++ {
				gotDep := gotNode.Deps[j]
				if expDep.Name == gotDep.Name {
					if expDep.Selected == gotDep.Selected || gotDep.Selected == "" && expDep.Selected == "0.0.0" {
						found = true
					}
				}
			}
			if found == false {
				t.Fatalf("Missing an expected dep: \n\t(WNT) %v \n\t(GOT) %v", expDep.Name, "")
			}
		}

	OUTER_MISSING_VERSION:
		for expVersion := range expNode.Versions {
			for gotVersion := range gotNode.Versions {
				if gotVersion == expVersion {
					continue OUTER_MISSING_VERSION
				}
			}
			t.Fatalf("Missing a version: %v, \n\t(WNT) %v \n\t(GOT) %v", expNode.Name, expNode.Versions, gotNode.Versions)
		}

		if len(expNode.Versions) == len(gotNode.Versions) {
			for i := 0; i < len(expNode.Versions); i++ {
				if expNode.Versions[i] != gotNode.Versions[i] {
					t.Fatalf("Versions out of order: \n\t(WNT) %v \n\t(GOT) %v", expNode.Versions[i], gotNode.Versions[i])
				}
			}
		} else {
			t.Fatalf("Version lists don't match: %v \n\t(WNT) %v \n\t(GOT) %v", expNode.Name, expNode.Versions, gotNode.Versions)
		}

		if gotNode.Selected != "" {
			if expNode.Selected != gotTree.NodeList[expName].Selected {
				t.Fatalf("Unexpected Selected Version \n\t(WNT) %v \n\t(GOT) %v", expNode.Name, gotTree.NodeList[expName].Selected)
			}
		}

		if len(expNode.Deps) != len(gotNode.Deps) {
			t.Fatalf("Unexpected number of Dependencies \n\t(WNT) %v \n\t(GOT) %v", len(expNode.Deps), len(gotNode.Deps))
		}
	}
}
