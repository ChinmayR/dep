package analyze

var (
	ResolverTreeRoot = &ResolverTree{
		NodeList: map[string]*TreeNode{
			RootNode.Name: RootNode,
		},
		VersionTree: RootNode,
	}

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

	NodeWithRedudantDeps = &TreeNode{
		Name: "Redudant Depper Node",
		Deps: RedundantDeps,
	}

	SimpleResolverTreeWithDeps = &ResolverTree{
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
			NodeWithDeps.Name:         NodeWithDeps,
			RootNode.Name:             RootNode,
			DependerNode.Name:         DependerNode,
			NodeWithVersAndSel.Name:   NodeWithVersAndSel,
			NodeWithRedudantDeps.Name: NodeWithRedudantDeps,
		},

		VersionTree: NodeWithDeps,
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

	Deps          = []*TreeNode{RootNode, DependerNode, NodeWithVersAndSel}
	RedundantDeps = []*TreeNode{DependerNode, NodeWithVersAndSel, NodeWithDeps}
)

const (
	RootProject = "code.uber.internal/devexp/test-root-repo.git"

	Project = "code.uber.internal/devexp/test-repo.git"

	Project2 = "test project 2"
)
