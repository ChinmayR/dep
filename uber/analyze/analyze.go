package analyze

// ResolverTree is a struct for holding trees created for projects as versions are explored for fit.
// NodeList contains pointers to each node in the tree for efficient lookup of TreeNodes
type ResolverTree struct {
	NodeList    map[string]*TreeNode
	VersionTree *TreeNode
}

// TreeNode is struct which holds the current project's name, a list of Versions tried,
// the selected version, and the project's dependencies.
type TreeNode struct {
	Name     string
	Versions []string
	Selected string
	Deps     []*TreeNode
}

var resTree *ResolverTree

func InitializeResTree(rootName string) *ResolverTree {
	if resTree == nil {
		rootNode := newTreeNode(rootName)
		resTree = &ResolverTree{
			map[string]*TreeNode{rootName: rootNode},
			rootNode,
		}
	}
	return resTree
}

// Adds a dependency node to a pre-existing project
func AddDep(depender string, depName string) {
	dependerNode := resTree.NodeList[depender]
	var dep *TreeNode
	if resTree.NodeList[depName] == nil {
		dep = newTreeNode(depName)
		resTree.NodeList[dep.Name] = dep
	} else {
		dep = resTree.NodeList[depName]
	}

	dependerNode.Deps = append(dependerNode.Deps, dep)
}

func AddVersion(nodeName string, version string) {
	node := resTree.NodeList[nodeName]
	node.Versions = append(node.Versions, version)
}

func SelectVersion(nodeName string, version string) {
	node := resTree.NodeList[nodeName]
	node.Deps = make([]*TreeNode, 0)
	node.Selected = version
}

func newTreeNode(projectName string) *TreeNode {
	node := &TreeNode{
		Name:     projectName,
		Versions: make([]string, 0),
		Deps:     make([]*TreeNode, 0),
	}

	return node
}

// For testing only
func GetResTree() *ResolverTree {
	return resTree
}

// For testing only
func ClearTree() {
	resTree = nil
}
