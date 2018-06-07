package gps

import (
	"github.com/golang/dep/gps/pkgtree"
	"github.com/golang/dep/uber/analyze"
)

// basicFixtureWIthResolverTree contains all the same fields as basicFixtureWithResolver
// and an additional field, tree, which gives the expected analyzer tree, which allows us
// to test that the resolver tree output matches the expected output.
type BasicFixtureWithResolverTree struct {
	// depspecs. always treat first as root
	ds []depspec
	// results; map of name/atom pairs
	r map[ProjectIdentifier]LockedProject
	// max attempts the solver should need to find solution. 0 means no limit
	maxAttempts int
	// Use downgrade instead of default upgrade sorter
	downgrade bool
	// lock file simulator, if one's to be used at all
	l fixLock
	// solve failure expected, if any
	fail error
	// overrides, if any
	ovr ProjectConstraints
	// request up/downgrade to all projects
	changeall bool
	// individual projects to change
	changelist []ProjectRoot
	// if the fixture is currently broken/expected to fail, this has a message
	// recording why
	broken string
	// anticipated properties of the resolution's visualization tree
	tree analyze.ResolverTree
}

func (f BasicFixtureWithResolverTree) rootmanifest() RootManifest {
	return simpleRootManifest{
		c:   pcSliceToMap(f.ds[0].deps),
		ovr: f.ovr,
	}
}

func (f BasicFixtureWithResolverTree) rootTree() pkgtree.PackageTree {
	var imp []string
	for _, dep := range f.ds[0].deps {
		imp = append(imp, string(dep.Ident.ProjectRoot))
	}

	n := string(f.ds[0].n)
	pt := pkgtree.PackageTree{
		ImportRoot: n,
		Packages: map[string]pkgtree.PackageOrErr{
			string(n): {
				P: pkgtree.Package{
					ImportPath: n,
					Name:       n,
					Imports:    imp,
				},
			},
		},
	}
	return pt
}

func mkResTree(testData []depspec, selections []interface{}) analyze.ResolverTree {
	rootNode := &analyze.TreeNode{
		Name:     string(testData[0].n),
		Versions: make([]string, 0),
		Selected: testData[0].v.String(),
		Deps:     make([]*analyze.TreeNode, 0),
	}

	resTree := analyze.ResolverTree{
		NodeList:    map[string]*analyze.TreeNode{rootNode.Name: rootNode},
		VersionTree: rootNode,
	}

	return mkDepNodes(testData, selectDeps(resTree, selections))
}

func selectDeps(resTree analyze.ResolverTree, selections []interface{}) analyze.ResolverTree {

	for _, selectedDep := range selections {
		dep := mkDepspec(selectedDep.(string))
		node := &analyze.TreeNode{
			Name:     string(dep.n),
			Versions: make([]string, 0),
			Deps:     make([]*analyze.TreeNode, 0),
			Selected: dep.v.String(),
		}
		resTree.NodeList[node.Name] = node
	}
	return resTree
}

func mkDepNodes(testData []depspec, resolverTree analyze.ResolverTree) analyze.ResolverTree {

	for _, spec := range testData {

		depInTree := resolverTree.NodeList[string(spec.n)]
		version := spec.v.String()
		if depInTree != nil {
			if depInTree.Selected == version {
				for _, dep := range spec.deps {
					depNode := resolverTree.NodeList[string(dep.Ident.ProjectRoot)]
					depInTree.Deps = append(depInTree.Deps, depNode)
				}
			}

			if version != "0.0.0" && depInTree.Name != "root" {
				depInTree.Versions = append(depInTree.Versions, version)
			}
		}
	}
	return resolverTree
}

func mkBasicFixtureWithResolverTree(data resolverData) BasicFixtureWithResolverTree {

	basicFixturesWithResolverTree := BasicFixtureWithResolverTree{
		data.ds,
		mksolution(data.selections...),
		data.maxAttempts,
		data.downgrade,
		data.l,
		data.fail,
		data.ovr,
		data.changeall,
		data.changelist,
		data.broken,
		mkResTree(data.treeSpecs, data.selections),
	}

	return basicFixturesWithResolverTree
}

// Allows us to optionally pass arguments to makeBasicFixtureWithResolverTree
type resolverData struct {
	// ds represents the solver's input for a run.
	ds []depspec
	// selections are the final selected deps and their versions
	selections []interface{}
	// treeSpecs specify the projects and their versions, in the order they are tried.
	treeSpecs   []depspec
	maxAttempts int
	downgrade   bool
	l           fixLock
	fail        error
	ovr         ProjectConstraints
	changeall   bool
	changelist  []ProjectRoot
	broken      string
}

// these tests are based on already-passing solver tests and leverage the solver's test methods.
// To test that the output tree is as expected in each of the solver's runs,
// the order of the treeSpecs is determined by examining the log output.
var basicFixturesWithResolverTree = map[string]BasicFixtureWithResolverTree{
	"no dependencies": mkBasicFixtureWithResolverTree(
		resolverData{
			ds:         []depspec{mkDepspec("root 0.0.0")},
			selections: []interface{}{},
			treeSpecs:  []depspec{mkDepspec("root 0.0.0")},
		},
	),
	"simple dependency tree": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "a 1.0.0", "b 1.0.0"),
				mkDepspec("a 1.0.0", "aa 1.0.0", "ab 1.0.0"),
				mkDepspec("aa 1.0.0"),
				mkDepspec("ab 1.0.0"),
				mkDepspec("b 1.0.0", "ba 1.0.0", "bb 1.0.0"),
				mkDepspec("ba 1.0.0"),
				mkDepspec("bb 1.0.0"),
			},
			selections: []interface{}{"a 1.0.0",
				"aa 1.0.0",
				"ab 1.0.0",
				"b 1.0.0",
				"ba 1.0.0",
				"bb 1.0.0"},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "a 1.0.0", "b 1.0.0"),
				mkDepspec("a 1.0.0", "aa 1.0.0", "ab 1.0.0"),
				mkDepspec("aa 1.0.0"),
				mkDepspec("ab 1.0.0"),
				mkDepspec("b 1.0.0", "ba 1.0.0", "bb 1.0.0"),
				mkDepspec("ba 1.0.0"),
				mkDepspec("bb 1.0.0"),
			},
		},
	),
	"shared dependency with overlapping constraints": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "a 1.0.0", "b 1.0.0"),
				mkDepspec("a 1.0.0", "shared >=2.0.0, <4.0.0"),
				mkDepspec("b 1.0.0", "shared >=3.0.0, <5.0.0"),
				mkDepspec("shared 2.0.0"),
				mkDepspec("shared 3.0.0"),
				mkDepspec("shared 3.6.9"),
				mkDepspec("shared 4.0.0"),
				mkDepspec("shared 5.0.0"),
			},
			selections: []interface{}{
				"a 1.0.0",
				"b 1.0.0",
				"shared 3.6.9"},

			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "a 1.0.0", "b 1.0.0"),
				mkDepspec("a 1.0.0", "shared >=2.0.0, <4.0.0"),
				mkDepspec("b 1.0.0", "shared >=3.0.0, <5.0.0"),
				mkDepspec("shared 5.0.0"),
				mkDepspec("shared 4.0.0"),
				mkDepspec("shared 3.6.9"),
			},
		},
	),
	"downgrade on overlapping constraints": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "a 1.0.0", "b 1.0.0"),
				mkDepspec("a 1.0.0", "shared >=2.0.0, <=4.0.0"),
				mkDepspec("b 1.0.0", "shared >=3.0.0, <5.0.0"),
				mkDepspec("shared 2.0.0"),
				mkDepspec("shared 3.0.0"),
				mkDepspec("shared 3.6.9"),
				mkDepspec("shared 4.0.0"),
				mkDepspec("shared 5.0.0"),
			},
			selections: []interface{}{
				"a 1.0.0",
				"b 1.0.0",
				"shared 3.0.0",
			},
			downgrade: true,
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "a 1.0.0", "b 1.0.0"),
				mkDepspec("a 1.0.0", "shared >=2.0.0, <=4.0.0"),
				mkDepspec("b 1.0.0", "shared >=3.0.0, <5.0.0"),
				mkDepspec("shared 2.0.0"),
				mkDepspec("shared 3.0.0"),
			},
		},
	),
	"shared dependency where dependent version in turn affects other dependencies": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo <=1.0.2", "bar 1.0.0"),
				mkDepspec("foo 1.0.0"),
				mkDepspec("foo 1.0.1", "bang 1.0.0"),
				mkDepspec("foo 1.0.2", "whoop 1.0.0"),
				mkDepspec("foo 1.0.3", "zoop 1.0.0"),
				mkDepspec("bar 1.0.0", "foo <=1.0.1"),
				mkDepspec("bang 1.0.0"),
				mkDepspec("whoop 1.0.0"),
				mkDepspec("zoop 1.0.0"),
			},
			selections: []interface{}{"foo 1.0.1",
				"bar 1.0.0",
				"bang 1.0.0"},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo <=1.0.2", "bar 1.0.0"),
				mkDepspec("foo 1.0.3", "zoop 1.0.0"),
				mkDepspec("foo 1.0.2", "whoop 1.0.0"),
				mkDepspec("foo 1.0.1", "bang 1.0.0"),
				mkDepspec("bar 1.0.0", "foo <=1.0.1"),
				mkDepspec("bang 1.0.0"),
				mkDepspec("whoop 1.0.0"),
				mkDepspec("zoop 1.0.0"),
			},
		},
	),
	"removed dependency": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 1.0.0", "foo 1.0.0", "bar *"),
				mkDepspec("foo 2.0.0"),
				mkDepspec("foo 1.0.0"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 2.0.0", "baz 1.0.0"),
				mkDepspec("baz 1.0.0", "foo 2.0.0"),
			},
			selections: []interface{}{"foo 1.0.0",
				"bar 1.0.0"},
			maxAttempts: 2,
			treeSpecs: []depspec{
				mkDepspec("root 1.0.0", "foo 1.0.0", "bar *"),
				mkDepspec("foo 2.0.0"),
				mkDepspec("foo 1.0.0"),
				mkDepspec("bar 2.0.0", "baz 1.0.0"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("baz 1.0.0", "foo 2.0.0"),
			},
		},
	),
	"with compatible locked dependency": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.1", "bar 1.0.1"),
				mkDepspec("foo 1.0.2", "bar 1.0.2"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 1.0.1"),
				mkDepspec("bar 1.0.2"),
			},
			l: mklock(
				"foo 1.0.1",
			),
			selections: []interface{}{
				"foo 1.0.1",
				"bar 1.0.1",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.1", "bar 1.0.1"),
				mkDepspec("bar 1.0.2"),
				mkDepspec("bar 1.0.1"),
			},
		},
	),
	"upgrade through lock": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.1", "bar 1.0.1"),
				mkDepspec("foo 1.0.2", "bar 1.0.2"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 1.0.1"),
				mkDepspec("bar 1.0.2"),
			},
			l: mklock(
				"foo 1.0.1",
			),
			changeall: true,
			selections: []interface{}{
				"foo 1.0.2",
				"bar 1.0.2",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.2", "bar 1.0.2"),
				mkDepspec("bar 1.0.2"),
			},
		},
	),
	"downgrade through lock": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.1", "bar 1.0.1"),
				mkDepspec("foo 1.0.2", "bar 1.0.2"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 1.0.1"),
				mkDepspec("bar 1.0.2"),
			},
			l: mklock(
				"foo 1.0.1",
			),
			selections: []interface{}{
				"foo 1.0.0",
				"bar 1.0.0",
			},
			changeall: true,
			downgrade: true,
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.0", "bar 1.0.0"),
				mkDepspec("bar 1.0.0"),
			},
		},
	),
	"update one with only one": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.0"),
				mkDepspec("foo 1.0.1"),
				mkDepspec("foo 1.0.2"),
			},
			l: mklock(
				"foo 1.0.1",
			),
			selections: []interface{}{
				"foo 1.0.2",
			},
			changelist: []ProjectRoot{"foo"},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.2"),
			},
		},
	),
	"update one of multi": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *", "bar *"),
				mkDepspec("foo 1.0.0"),
				mkDepspec("foo 1.0.1"),
				mkDepspec("foo 1.0.2"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 1.0.1"),
				mkDepspec("bar 1.0.2"),
			},
			l: mklock(
				"foo 1.0.1",
				"bar 1.0.1",
			),
			selections: []interface{}{
				"foo 1.0.2",
				"bar 1.0.1",
			},
			changelist: []ProjectRoot{"foo"},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *", "bar *"),
				mkDepspec("foo 1.0.2"),
				mkDepspec("bar 1.0.1"),
			},
		},
	),
	"update both of multi": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *", "bar *"),
				mkDepspec("foo 1.0.0"),
				mkDepspec("foo 1.0.1"),
				mkDepspec("foo 1.0.2"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 1.0.1"),
				mkDepspec("bar 1.0.2"),
			},
			l: mklock(
				"foo 1.0.1",
				"bar 1.0.1",
			),
			selections: []interface{}{
				"foo 1.0.2",
				"bar 1.0.2",
			},
			changelist: []ProjectRoot{"foo", "bar"},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *", "bar *"),
				mkDepspec("foo 1.0.2"),
				mkDepspec("bar 1.0.2"),
			},
		},
	),
	"break other lock with targeted update": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *", "baz *"),
				mkDepspec("foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.1", "bar 1.0.1"),
				mkDepspec("foo 1.0.2", "bar 1.0.2"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 1.0.1"),
				mkDepspec("bar 1.0.2"),
				mkDepspec("baz 1.0.0"),
				mkDepspec("baz 1.0.1"),
				mkDepspec("baz 1.0.2"),
			},
			l: mklock(
				"foo 1.0.1",
				"bar 1.0.1",
				"baz 1.0.1",
			),
			selections: []interface{}{
				"foo 1.0.2",
				"bar 1.0.2",
				"baz 1.0.1"},

			changelist: []ProjectRoot{"foo", "bar"},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *", "baz *"),
				mkDepspec("foo 1.0.2", "bar 1.0.2"),
				mkDepspec("bar 1.0.2"),
				mkDepspec("baz 1.0.1"),
			},
		},
	),
	"with incompatible locked dependency": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo >1.0.1"),
				mkDepspec("foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.1", "bar 1.0.1"),
				mkDepspec("foo 1.0.2", "bar 1.0.2"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 1.0.1"),
				mkDepspec("bar 1.0.2"),
			},
			l: mklock(
				"foo 1.0.1",
			),
			selections: []interface{}{
				"foo 1.0.2",
				"bar 1.0.2",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo >1.0.1"),
				mkDepspec("foo 1.0.1", "bar 1.0.1"),
				mkDepspec("foo 1.0.2", "bar 1.0.2"),
				mkDepspec("bar 1.0.2"),
			},
		},
	),
	"with unrelated locked dependency": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.1", "bar 1.0.1"),
				mkDepspec("foo 1.0.2", "bar 1.0.2"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 1.0.1"),
				mkDepspec("bar 1.0.2"),
				mkDepspec("baz 1.0.0 bazrev"),
			},
			l: mklock(
				"baz 1.0.0 bazrev",
			),
			selections: []interface{}{
				"foo 1.0.2",
				"bar 1.0.2",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.2", "bar 1.0.2"),
				mkDepspec("bar 1.0.2"),
			},
		},
	),
	"break lock when only the deps necessitate it": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *", "bar *"),
				mkDepspec("foo 1.0.0 foorev", "bar <2.0.0"),
				mkDepspec("foo 2.0.0", "bar <3.0.0"),
				mkDepspec("bar 2.0.0", "baz <3.0.0"),
				mkDepspec("baz 2.0.0", "foo >1.0.0"),
			},
			l: mklock(
				"foo 1.0.0 foorev",
			),
			selections: []interface{}{
				"foo 2.0.0",
				"bar 2.0.0",
				"baz 2.0.0",
			},
			maxAttempts: 4,
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *", "bar *"),
				mkDepspec("foo 1.0.0 foorev", "bar <2.0.0"),
				mkDepspec("foo 2.0.0", "bar <3.0.0"),
				mkDepspec("bar 2.0.0", "baz <3.0.0"),
				mkDepspec("bar 2.0.0"),
				mkDepspec("baz 2.0.0", "foo >1.0.0"),
			},
		},
	),
	"locked atoms are matched on both local and net name": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.0 foorev"),
				mkDepspec("foo 2.0.0 foorev2"),
			},
			l: mklock(
				"foo from baz 1.0.0 foorev",
			),
			selections: []interface{}{
				"foo 2.0.0 foorev2",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.0 foorev"),
				mkDepspec("foo 2.0.0 foorev2"),
			},
		},
	),
	"pairs bare revs in lock with versions": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo ~1.0.1"),
				mkDepspec("foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.1 foorev", "bar 1.0.1"),
				mkDepspec("foo 1.0.2", "bar 1.0.2"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 1.0.1"),
				mkDepspec("bar 1.0.2"),
			},
			l: mkrevlock(
				"foo 1.0.1 foorev",
			),
			selections: []interface{}{
				"foo 1.0.1 foorev",
				"bar 1.0.1",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo ~1.0.1"),
				mkDepspec("foo 1.0.1 foorev", "bar 1.0.1"),
				mkDepspec("bar 1.0.2"),
				mkDepspec("bar 1.0.1"),
			},
		}),
	"lock to now-moved tag on old rev keeps old rev": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo ptaggerino"),
				mkDepspec("foo ptaggerino newrev"),
			},
			l: mklock(
				"foo ptaggerino oldrev",
			),
			selections: []interface{}{
				"foo ptaggerino oldrev",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo ptaggerino"),
				mkDepspec("foo ptaggerino newrev"),
			},
		},
	),

	"no version that matches requirement": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo ^1.0.0"),
				mkDepspec("foo 2.0.0"),
				mkDepspec("foo 2.1.3"),
			},
			fail: &noVersionError{
				pn: mkPI("foo"),
				fails: []failedVersion{
					{
						v: NewVersion("2.1.3"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("foo 2.1.3"),
							failparent: []dependency{mkDep("root", "foo ^1.0.0", "foo")},
							c:          mkSVC("^1.0.0"),
						},
					},
					{
						v: NewVersion("2.0.0"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("foo 2.0.0"),
							failparent: []dependency{mkDep("root", "foo ^1.0.0", "foo")},
							c:          mkSVC("^1.0.0"),
						},
					},
				},
			},
			selections: []interface{}{
				"foo 0.0.0",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo ^1.0.0"),
				mkDepspec("foo 2.1.3"),
				mkDepspec("foo 2.0.0"),
			},
		},
	),
	"no version that matches combined constraint": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.0", "shared >=2.0.0, <3.0.0"),
				mkDepspec("bar 1.0.0", "shared >=2.9.0, <4.0.0"),
				mkDepspec("shared 2.5.0"),
				mkDepspec("shared 3.5.0"),
			},
			fail: &noVersionError{
				pn: mkPI("shared"),
				fails: []failedVersion{
					{
						v: NewVersion("3.5.0"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("shared 3.5.0"),
							failparent: []dependency{mkDep("foo 1.0.0", "shared >=2.0.0, <3.0.0", "shared")},
							c:          mkSVC(">=2.9.0, <3.0.0"),
						},
					},
					{
						v: NewVersion("2.5.0"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("shared 2.5.0"),
							failparent: []dependency{mkDep("bar 1.0.0", "shared >=2.9.0, <4.0.0", "shared")},
							c:          mkSVC(">=2.9.0, <3.0.0"),
						},
					},
				},
			},
			selections: []interface{}{
				"foo 1.0.0",
				"bar 1.0.0",
				"shared 0.0.0",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.0", "shared >=2.0.0, <3.0.0"),
				mkDepspec("bar 1.0.0", "shared >=2.9.0, <4.0.0"),
				mkDepspec("shared 3.5.0"),
				mkDepspec("shared 2.5.0"),
			},
		},
	),
	"no valid solution": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "a *", "b *"),
				mkDepspec("a 1.0.0", "b 1.0.0"),
				mkDepspec("a 2.0.0", "b 2.0.0"),
				mkDepspec("b 1.0.0", "a 2.0.0"),
				mkDepspec("b 2.0.0", "a 1.0.0"),
			},
			fail: &noVersionError{
				pn: mkPI("b"),
				fails: []failedVersion{
					{
						v: NewVersion("2.0.0"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("b 2.0.0"),
							failparent: []dependency{mkDep("a 1.0.0", "b 1.0.0", "b")},
							c:          mkSVC("1.0.0"),
						},
					},
					{
						v: NewVersion("1.0.0"),
						f: &constraintNotAllowedFailure{
							goal: mkDep("b 1.0.0", "a 2.0.0", "a"),
							v:    NewVersion("1.0.0"),
						},
					},
				},
			},
			selections: []interface{}{
				"a 1.0.0",
				"b 0.0.0",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "a *", "b *"),
				mkDepspec("a 2.0.0", "b 2.0.0"),
				mkDepspec("a 1.0.0", "b 1.0.0"),
				mkDepspec("b 2.0.0", "a 1.0.0"),
				mkDepspec("b 1.0.0", "a 2.0.0"),
				mkDepspec("b 2.0.0"),
				mkDepspec("b 1.0.0"),
			},
		},
	),
	"search real failer": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "a *", "b *"),
				mkDepspec("a 1.0.0", "c 1.0.0"),
				mkDepspec("a 2.0.0", "c 2.0.0"),
				mkDepspec("b 1.0.0"),
				mkDepspec("b 2.0.0"),
				mkDepspec("b 3.0.0"),
				mkDepspec("c 1.0.0"),
			},
			selections: []interface{}{
				"a 1.0.0",
				"b 3.0.0",
				"c 1.0.0",
			},
			maxAttempts: 2,
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "a *", "b *"),
				mkDepspec("a 2.0.0", "c 2.0.0"),
				mkDepspec("a 1.0.0", "c 1.0.0"),
				mkDepspec("b 3.0.0"),
				mkDepspec("c 1.0.0"),
				mkDepspec("c 1.0.0"),
			},
		},
	),
	"root constraints pre-eliminate versions": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *", "bar *"),
				mkDepspec("foo 1.0.0", "none 2.0.0"),
				mkDepspec("foo 2.0.0", "none 2.0.0"),
				mkDepspec("foo 3.0.0", "none 2.0.0"),
				mkDepspec("foo 4.0.0", "none 2.0.0"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 2.0.0"),
				mkDepspec("bar 3.0.0"),
				mkDepspec("none 1.0.0"),
			},
			fail: &noVersionError{
				pn: mkPI("none"),
				fails: []failedVersion{
					{
						v: NewVersion("1.0.0"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("none 1.0.0"),
							failparent: []dependency{mkDep("foo 1.0.0", "none 2.0.0", "none")},
							c:          mkSVC("2.0.0"),
						},
					},
				},
			},
			selections: []interface{}{
				"foo 1.0.0", "bar 3.0.0", "none 0.0.0",
			},
			treeSpecs: []depspec{mkDepspec("root 0.0.0", "foo *", "bar *"),
				mkDepspec("foo 4.0.0", "none 2.0.0"),
				mkDepspec("foo 3.0.0", "none 2.0.0"),
				mkDepspec("foo 2.0.0", "none 2.0.0"),
				mkDepspec("foo 1.0.0", "none 2.0.0"),
				mkDepspec("bar 3.0.0"),
				mkDepspec("none 1.0.0"),
				mkDepspec("none 1.0.0"),
				mkDepspec("none 1.0.0"),
				mkDepspec("none 1.0.0"),
			},
		},
	),
	"backjump past failed package on disjoint constraint": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "a *", "foo *"),
				mkDepspec("a 1.0.0", "foo *"),
				mkDepspec("a 2.0.0", "foo <1.0.0"),
				mkDepspec("foo 2.0.0"),
				mkDepspec("foo 2.0.1"),
				mkDepspec("foo 2.0.2"),
				mkDepspec("foo 2.0.3"),
				mkDepspec("foo 2.0.4"),
				mkDepspec("none 1.0.0"),
			},
			selections: []interface{}{
				"a 1.0.0",
				"foo 2.0.4",
			},
			maxAttempts: 2,
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "a *", "foo *"),
				mkDepspec("a 2.0.0", "foo <1.0.0"),
				mkDepspec("a 1.0.0", "foo *"),
				mkDepspec("foo 2.0.4"),
				mkDepspec("foo 2.0.3"),
				mkDepspec("foo 2.0.2"),
				mkDepspec("foo 2.0.1"),
				mkDepspec("foo 2.0.0"),
				mkDepspec("foo 2.0.4"),
				mkDepspec("none 1.0.0"),
			},
		},
	),
	"revision injected into vqueue": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo r123abc"),
				mkDepspec("foo r123abc"),
				mkDepspec("foo 1.0.0 foorev"),
				mkDepspec("foo 2.0.0 foorev2"),
			},
			selections: []interface{}{
				"foo r123abc",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo r123abc"),
				mkDepspec("foo r123abc"),
			},
		},
	),
	"override root's own constraint": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "a *", "b *"),
				mkDepspec("a 1.0.0", "b 1.0.0"),
				mkDepspec("a 2.0.0", "b 1.0.0"),
				mkDepspec("b 1.0.0"),
			},
			ovr: ProjectConstraints{
				ProjectRoot("a"): ProjectProperties{
					Constraint: NewVersion("1.0.0"),
				},
			},
			selections: []interface{}{
				"a 1.0.0",
				"b 1.0.0",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "a *", "b *"),
				mkDepspec("a 2.0.0", "b 1.0.0"),
				mkDepspec("a 1.0.0", "b 1.0.0"),
				mkDepspec("b 1.0.0"),
			},
		},
	),
	"override dep's constraint": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "a *"),
				mkDepspec("a 1.0.0", "b 1.0.0"),
				mkDepspec("a 2.0.0", "b 1.0.0"),
				mkDepspec("b 1.0.0"),
				mkDepspec("b 2.0.0"),
			},
			ovr: ProjectConstraints{
				ProjectRoot("b"): ProjectProperties{
					Constraint: NewVersion("2.0.0"),
				},
			},
			selections: []interface{}{
				"a 2.0.0",
				"b 2.0.0",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "a *"),
				mkDepspec("a 2.0.0", "b 1.0.0"),
				mkDepspec("b 2.0.0"),
			},
		},
	),
	"overridden mismatched net addrs, alt in dep, back to default": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 1.0.0", "foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.0", "bar from baz 1.0.0"),
				mkDepspec("bar 1.0.0"),
			},
			ovr: ProjectConstraints{
				ProjectRoot("bar"): ProjectProperties{
					Source: "bar",
				},
			},
			selections: []interface{}{
				"foo 1.0.0",
				"bar from bar 1.0.0",
			},
			treeSpecs: []depspec{
				mkDepspec("root 1.0.0", "foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.0", "bar from baz 1.0.0"),
				mkDepspec("bar 1.0.0"),
			},
		},
	),
	"disjoint constraints": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.0", "shared <=2.0.0"),
				mkDepspec("bar 1.0.0", "shared >3.0.0"),
				mkDepspec("shared 2.0.0"),
				mkDepspec("shared 4.0.0"),
			},
			fail: &noVersionError{
				pn: mkPI("foo"),
				fails: []failedVersion{
					{
						v: NewVersion("1.0.0"),
						f: &disjointConstraintFailure{
							goal:      mkDep("foo 1.0.0", "shared <=2.0.0", "shared"),
							failsib:   []dependency{mkDep("bar 1.0.0", "shared >3.0.0", "shared")},
							nofailsib: nil,
							c:         mkSVC(">3.0.0"),
						},
					},
				},
			},
			selections: []interface{}{
				"bar 1.0.0",
				"foo 0.0.0",
				"shared 0.0.0",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 1.0.0", "shared <=2.0.0"),
				mkDepspec("bar 1.0.0", "shared >2.0.0"),
			},
		},
	),
	"no version that matches while backtracking": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "a *", "b >1.0.0"),
				mkDepspec("a 1.0.0"),
				mkDepspec("b 1.0.0"),
			},
			fail: &noVersionError{
				pn: mkPI("b"),
				fails: []failedVersion{
					{
						v: NewVersion("1.0.0"),
						f: &versionNotAllowedFailure{
							goal:       mkAtom("b 1.0.0"),
							failparent: []dependency{mkDep("root", "b >1.0.0", "b")},
							c:          mkSVC(">1.0.0"),
						},
					},
				},
			},
			selections: []interface{}{
				"a 1.0.0",
				"b 0.0.0",
			},
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "a *", "b >1.0.0"),
				mkDepspec("a 1.0.0"),
				mkDepspec("b 1.0.0"),
			},
		},
	),
	"unlocks dependencies if necessary to ensure that a new dependency is satisfied": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *", "newdep *"),
				mkDepspec("foo 1.0.0 foorev", "bar <2.0.0"),
				mkDepspec("bar 1.0.0 barrev", "baz <2.0.0"),
				mkDepspec("baz 1.0.0 bazrev", "qux <2.0.0"),
				mkDepspec("qux 1.0.0 quxrev"),
				mkDepspec("foo 2.0.0", "bar <3.0.0"),
				mkDepspec("bar 2.0.0", "baz <3.0.0"),
				mkDepspec("baz 2.0.0", "qux <3.0.0"),
				mkDepspec("qux 2.0.0"),
				mkDepspec("newdep 2.0.0", "baz >=1.5.0"),
			},
			l: mklock(
				"foo 1.0.0 foorev",
				"bar 1.0.0 barrev",
				"baz 1.0.0 bazrev",
				"qux 1.0.0 quxrev",
			),
			selections: []interface{}{
				"foo 2.0.0",
				"bar 2.0.0",
				"baz 2.0.0",
				"qux 1.0.0 quxrev",
				"newdep 2.0.0",
			},
			maxAttempts: 4,
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *", "newdep *"),
				mkDepspec("foo 1.0.0 foorev", "bar <2.0.0"),
				mkDepspec("foo 2.0.0", "bar <3.0.0"),
				mkDepspec("bar 1.0.0 barrev", "baz <2.0.0"),
				mkDepspec("bar 2.0.0", "baz <3.0.0"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 2.0.0"),
				mkDepspec("baz 1.0.0 bazrev", "qux <2.0.0"),
				mkDepspec("qux 1.0.0 quxrev"),
				mkDepspec("qux 1.0.0"),
				mkDepspec("qux 1.0.0"),
				mkDepspec("qux 1.0.0"),
				mkDepspec("baz 2.0.0", "qux <3.0.0"),
				mkDepspec("baz 1.0.0"),
				mkDepspec("baz 2.0.0"),
				mkDepspec("baz 1.0.0"),
				mkDepspec("baz 2.0.0"),
				mkDepspec("newdep 2.0.0", "baz >=1.5.0"),
				mkDepspec("newdep 2.0.0"),
				mkDepspec("newdep 2.0.0"),
				mkDepspec("newdep 2.0.0"),
			},
		},
	),
	"mutual downgrading": mkBasicFixtureWithResolverTree(
		resolverData{
			ds: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 1.0.0", "bar 1.0.0"),
				mkDepspec("foo 2.0.0", "bar 2.0.0"),
				mkDepspec("foo 3.0.0", "bar 3.0.0"),
				mkDepspec("bar 1.0.0", "baz *"),
				mkDepspec("bar 2.0.0", "baz 2.0.0"),
				mkDepspec("bar 3.0.0", "baz 3.0.0"),
				mkDepspec("baz 1.0.0"),
			},
			selections: []interface{}{
				"foo 1.0.0",
				"bar 1.0.0",
				"baz 1.0.0",
			},
			maxAttempts: 3,
			treeSpecs: []depspec{
				mkDepspec("root 0.0.0", "foo *"),
				mkDepspec("foo 3.0.0", "bar 3.0.0"),
				mkDepspec("foo 2.0.0", "bar 2.0.0"),
				mkDepspec("foo 1.0.0", "bar 1.0.0"),
				mkDepspec("bar 3.0.0", "baz 3.0.0"),
				mkDepspec("bar 2.0.0", "baz 2.0.0"),
				mkDepspec("bar 1.0.0", "baz *"),
				mkDepspec("bar 3.0.0"),
				mkDepspec("bar 2.0.0"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("bar 3.0.0"),
				mkDepspec("bar 2.0.0"),
				mkDepspec("bar 1.0.0"),
				mkDepspec("baz 1.0.0"),
				mkDepspec("baz 1.0.0"),
				mkDepspec("baz 1.0.0"),
			},
		},
	),
}
