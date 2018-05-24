package analyze

var (
	RootNode = &TreeNode{
		Name:     RootProject,
		Versions: []string{},
		Selected: "",
		Deps:     []*TreeNode{},
	}

	DependerNode = &TreeNode{
		Name:     Project,
		Versions: []string{},
		Selected: "",
		Deps:     []*TreeNode{},
	}

	NodeWithVersAndSel = &TreeNode{
		Name:     "Node With Vers and Sel",
		Versions: []string{"bc123314oi3241923h23sdafn", "somerevision", "v2.0.3", "v2.0.3", "2.0.4"},
		Selected: "v2.0.3",
		Deps:     []*TreeNode{},
	}

	NodeWithDeps = &TreeNode{
		Name:     "Node With Deps",
		Versions: []string{"bc123314oi3241923h23sdafn", "somerevision", "v2.0.3", "v2.0.3", "2.0.4"},
		Selected: "v2.0.3",
		Deps:     Deps,
	}

	UnreferencedNode = &TreeNode{
		Name: "Unreferenced",
	}

	NodeWithRedundantDeps = &TreeNode{
		Name: "Redudant Depper Node",
		Deps: RedundantDeps,
	}

	TreeWithRootOnly = &ResolverTree{
		NodeList: map[string]*TreeNode{
			RootNode.Name: RootNode,
		},
		VersionTree: RootNode,
	}

	TreeWithSimpleDeps = &ResolverTree{
		NodeList: map[string]*TreeNode{
			NodeWithDeps.Name:       NodeWithDeps,
			RootNode.Name:           RootNode,
			DependerNode.Name:       DependerNode,
			NodeWithVersAndSel.Name: NodeWithVersAndSel,
		},

		VersionTree: NodeWithDeps,
	}

	//to test for 1:1 ratio of nodes to hashed bytes when more than one reference exists
	TreeWithTwoToOneDepperToDep = &ResolverTree{
		NodeList: map[string]*TreeNode{
			NodeWithDeps.Name:          NodeWithDeps,
			RootNode.Name:              RootNode,
			DependerNode.Name:          DependerNode,
			NodeWithVersAndSel.Name:    NodeWithVersAndSel,
			NodeWithRedundantDeps.Name: NodeWithRedundantDeps,
		},

		VersionTree: NodeWithRedundantDeps,
	}

	// same as simple resolver tree, but with an unreferenced node to test for encoding
	TreeWithUnreferencedNode = &ResolverTree{
		NodeList: map[string]*TreeNode{
			NodeWithDeps.Name:       NodeWithDeps,
			RootNode.Name:           RootNode,
			DependerNode.Name:       DependerNode,
			NodeWithVersAndSel.Name: NodeWithVersAndSel,
			UnreferencedNode.Name:   UnreferencedNode,
		},

		VersionTree: NodeWithDeps,
	}

	Deps = []*TreeNode{RootNode, DependerNode, NodeWithVersAndSel}
	// it should not be able to happen that a dep is added twice to a dependency list, but if it does,
	// it should only appear once in the graph.
	RedundantDeps = []*TreeNode{DependerNode, DependerNode, NodeWithVersAndSel, NodeWithVersAndSel, NodeWithDeps}

	encodedNodeValues = map[string]uint32{
		RootNode.Name:              1,
		DependerNode.Name:          2,
		NodeWithVersAndSel.Name:    3,
		NodeWithDeps.Name:          4,
		UnreferencedNode.Name:      5,
		NodeWithRedundantDeps.Name: 6,
	}

	encodedRelationshipsForNodeWithDeps = map[uint32][]uint32{
		4: {1, 2, 3},
		1: {},
		2: {},
		3: {},
	}

	encodedDepsOnRedundantDeps = []uint32{2, 3, 4}
)

const (
	RootProject = "code.uber.internal/devexp/test-root-repo.git"

	Project = "code.uber.internal/devexp/test-repo.git"

	Project2 = "test project 2"
)

func encodeTestCaseFromBase(prevEncodedRelationships map[uint32][]uint32, newKey uint32, newVals []uint32) map[uint32][]uint32 {
	newEncodedRelationships := make(map[uint32][]uint32)
	newEncodedRelationships[newKey] = newVals
	for key, val := range prevEncodedRelationships {
		newEncodedRelationships[key] = val
	}

	return newEncodedRelationships
}
