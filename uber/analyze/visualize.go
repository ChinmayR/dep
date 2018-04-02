package analyze

import "github.com/golang/dep/internal/gps"

// ResolverTree is a struct for holding trees created for projects as versions are explored for fit. projectIdentifier
// represents the ProjectIdentifier of the root project.
type ResolverTree struct {
	ProjectIdentifier gps.ProjectIdentifier
	VersionTree       *TreeNode
}

// TreeNode is struct which holds the current project's name and a map with
// a string (major version) for the key and a slice containing gps.Version
// as values. This allows us to create a new TreeNode for each dependency we encounter,
// and a record of all attempted Versions for each TreeNode.  This will simplify the process of
// creating the real-time visualization feature later.
type TreeNode struct {
	Name     gps.ProjectRoot
	Versions map[string][]gps.Version
	Selected gps.Version
	Deps     []*TreeNode
}
