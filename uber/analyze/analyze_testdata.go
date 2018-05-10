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
		Name:     Project,
		Versions: []string{"bc123314oi3241923h23sdafn", "somerevision", "v2.0.3", "v2.0.3", "2.0.4"},
		Selected: "v2.0.3",
		Deps:     []*TreeNode{},
	}

	Deps = []*TreeNode{RootNode, DependerNode, NodeWithVersAndSel}
)

const (
	RootProject = "code.uber.internal/devexp/test-root-repo.git"

	Project = "code.uber.internal/devexp/test-repo.git"
)
