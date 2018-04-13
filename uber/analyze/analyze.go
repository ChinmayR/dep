package analyze

import (
	"errors"
)

// ResolverTree is a struct for holding trees created for projects as versions are explored for fit.

type ResolverTree struct {
	VersionTree *TreeNode
}

// TreeNode is struct which holds the current project's name and a map with
// a string (major version) for the key and a slice containing gps.Version
// as values.
type TreeNode struct {
	Name     string
	Versions []string
	Selected string
	Deps     []*TreeNode
}

// MakeTree creates resolver tree root and returns it.
func NewResolverTree(rootName string) *ResolverTree {
	rootNode := newTreeNode(rootName)
	resolverTree := &ResolverTree{
		VersionTree: rootNode,
	}

	return resolverTree
}

// Adds a dependency node to a pre-existing project
func (tn *TreeNode) AddDep(depName string) {
	node := newTreeNode(depName)
	tn.Deps = append(tn.Deps, node)
}

func (tn *TreeNode) RemoveVersion(version string) error {
	err := errors.New("visualization removal error: version not found")
	for i, removeVersion := range tn.Versions {
		if removeVersion == version {
			tn.Versions = append(tn.Versions[:i], tn.Versions[i+1:]...)
			return nil
		}
	}

	return err
}

func ReachedFailure() {
	//TODO: encode and return graph
}

func newTreeNode(projectName string) *TreeNode {
	node := &TreeNode{
		Name:     projectName,
		Versions: make([]string, 0),
		Deps:     make([]*TreeNode, 0),
	}

	return node
}
