package analyze

import (
	"sync"
)

// ResolverTree is a struct for holding trees created for projects as versions are explored for fit.
// NodeList contains pointers to each node in the tree for efficient lookup of TreeNodes
type ResolverTree struct {
	NodeList    map[string]*TreeNode
	VersionTree *TreeNode
	LastError	*ResTreeSolveError
	mtx         sync.Mutex
}

// TreeNode is struct which holds the current project's name, a list of Versions tried,
// the selected version, and the project's dependencies.
type TreeNode struct {
	Name     string
	Versions []string
	Selected string
	Deps     []*TreeNode
}

type ResTreeSolveError struct {
	Pn string
	Fails []*ResTreeFailedVersion
}

type ResTreeFailedVersion struct {
	V string
	Err string
}

var resTree *ResolverTree

func InitializeResTree(rootName string) *ResolverTree {
	if resTree == nil {
		rootNode := newTreeNode(rootName)
		resTree = &ResolverTree{
			NodeList:    map[string]*TreeNode{rootName: rootNode},
			VersionTree: rootNode,
			LastError: &ResTreeSolveError{
				"",
				make([]*ResTreeFailedVersion, 0),
			},
		}
	}
	return resTree
}

// Adds a dependency node to a pre-existing project
func AddDep(depender string, depName string) {
	resTree.mtx.Lock()
	defer resTree.mtx.Unlock()
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
	resTree.mtx.Lock()
	defer resTree.mtx.Unlock()
	node := resTree.NodeList[nodeName]
	node.Versions = append(node.Versions, version)
}

func SelectVersion(nodeName string, version string) {
	resTree.mtx.Lock()
	defer resTree.mtx.Unlock()
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

func ResetLastFailure(projectName string) {
	resTree.mtx.Lock()
	defer resTree.mtx.Unlock()
	resTree.LastError = &ResTreeSolveError{
		Pn: projectName,
		Fails: make([]*ResTreeFailedVersion, 0),
	}
}

func CollectFails(version string, err error) {
	resTree.mtx.Lock()
	defer resTree.mtx.Unlock()
	failure := &ResTreeFailedVersion{
		V: version,
		Err: err.Error(),
	}
	resTree.LastError.Fails = append(resTree.LastError.Fails, failure)
}

func syncNodeList(rootNode *TreeNode) map[string]*TreeNode {
	syncQueue := []*TreeNode{rootNode}
	newNodeList := make(map[string]*TreeNode)

	for len(syncQueue) > 0 {
		curNode := syncQueue[0]
		syncQueue = append(syncQueue[1:])
		if newNodeList[curNode.Name] == nil {
			newNodeList[curNode.Name] = curNode

			for _, childNode := range curNode.Deps {
				syncQueue = append(syncQueue, childNode)
			}
		}
	}
	return newNodeList
}

func GenerateEncodedGraph(err error) {
	resTree.NodeList = syncNodeList(resTree.VersionTree)
	if err == nil {
		resTree.LastError = nil
	}
	writeToFile(resTree)
}

// For testing only
func GetResTree() *ResolverTree {
	return resTree
}

// For testing only
func ClearTree() {
	resTree = nil
}
