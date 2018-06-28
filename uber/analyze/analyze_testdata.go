package analyze

import (
	"strings"
)

var (
	RootNode = newTreeNode(RootProject)

	DependerNode = newTreeNode(Project)

	UnreferencedNode = newTreeNode("Unreferenced")

	NodeWithVersAndSel = &TreeNode{
		Name:     "Node With Vers and Sel",
		Versions: []string{"bc123314oi3241923h23sdafn", "somerevision", "v2.0.3", "v2.0.3", "2.0.4"},
		Selected: "v2.0.3",
		Deps:     make([]*TreeNode, 0),
	}

	NodeWithDeps = &TreeNode{
		Name:     "Node With Deps",
		Versions: []string{"bc123314oi3241923h23sdafn", "somerevision", "v2.0.3", "v2.0.3", "2.0.4"},
		Selected: "v2.0.3",
		Deps:     Deps,
	}

	NodeWithRedundantDeps = &TreeNode{
		Name:     "Redudant Depper Node",
		Versions: make([]string, 0),
		Deps:     RedundantDeps,
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

	ParentOfNodeThatFailed = &TreeNode{
		Name: "ParentOfNodeThatFailed",
		Deps: []*TreeNode{
			RootNode,
			NodeThatFailed,
		},
	}

	RootOfVizTesterTree = &TreeNode{
		Name: "RootOfVizTesterTree",
		Deps: []*TreeNode{
			RootNode,
			DependerNode,
			DependerNode,
			NodeWithVersAndSel,
			ParentOfNodeThatFailed,
			NodeThatWasIntroducedButNeverTried,
			NodeWithDeps,
		},
	}

	NodeThatFailed = &TreeNode{
		Name:     "NodeThatFailed",
		Versions: []string{"Something was tried"},
	}

	NodeThatWasIntroducedButNeverTried = newTreeNode("NodeThatWasIntroducedButNeverTried")

	TreeToGraph = &ResolverTree{
		NodeList: map[string]*TreeNode{
			RootOfVizTesterTree.Name:                RootOfVizTesterTree,
			ParentOfNodeThatFailed.Name:             ParentOfNodeThatFailed,
			RootNode.Name:                           RootNode,
			DependerNode.Name:                       DependerNode,
			NodeWithVersAndSel.Name:                 NodeWithVersAndSel,
			NodeThatFailed.Name:                     NodeThatFailed,
			NodeThatWasIntroducedButNeverTried.Name: NodeThatWasIntroducedButNeverTried,
			NodeWithDeps.Name:                       NodeWithDeps,
			UnreferencedNode.Name:                   UnreferencedNode,
		},
		VersionTree: RootOfVizTesterTree,
		LastError: &ResTreeSolveError{
			Pn: "NodeThatFailed",
			Fails: []*ResTreeFailedVersion{
				&ResTreeFailedVersion{
					"2.3.0",
					"No Version met constraints",
				},
			},
		},
	}

	Deps = []*TreeNode{RootNode, DependerNode, NodeWithVersAndSel}
	// it should not be able to happen that a dep is added twice to a dependency list, but if it does,
	// it should only appear once in the graph.
	RedundantDeps = []*TreeNode{DependerNode, DependerNode, NodeWithVersAndSel, NodeWithVersAndSel, NodeWithDeps}

	//allows us to test that
	encodedNodeValues = map[string]string{
		RootNode.Name:              makeEncodedString(RootNode.Name),
		DependerNode.Name:          makeEncodedString(DependerNode.Name),
		NodeWithVersAndSel.Name:    makeEncodedString(NodeWithVersAndSel.Name),
		NodeWithDeps.Name:          makeEncodedString(NodeWithDeps.Name),
		UnreferencedNode.Name:      makeEncodedString(UnreferencedNode.Name),
		NodeWithRedundantDeps.Name: makeEncodedString(NodeWithRedundantDeps.Name),
	}

	TreeToGraphOutput = []string{
		"\n " + makeEncodedString(RootOfVizTesterTree.Name) + " [label=\" ROOT: \\n" + RootOfVizTesterTree.Name + "\\n\" color=black];",
		"\n " + makeEncodedString(RootNode.Name) + " [label=\"" + RootNode.Name + "\\n" + "tried: " + strings.Join(RootNode.Versions, ", ") + "\\n" + "selected: " + RootNode.Selected + "\" color=black];",
		"\n " + makeEncodedString(DependerNode.Name) + " [label=\"" + DependerNode.Name + "\\ntried: " + strings.Join(DependerNode.Versions, ", ") + "\\nselected: " + DependerNode.Selected + "\" color=black];",
		"\n " + makeEncodedString(NodeWithVersAndSel.Name) + " [label=\"" + NodeWithVersAndSel.Name + "\\ntried: " + strings.Join(NodeWithVersAndSel.Versions, ", ") + "\\nselected: " + NodeWithVersAndSel.Selected + "\" color=black];",
		"\n " + makeEncodedString(NodeThatFailed.Name) + " [label=\"" + NodeThatFailed.Name + "\\ntried: " + strings.Join(NodeThatFailed.Versions, ", ") + "\\nselected: " + NodeThatFailed.Selected + "\\n\\n ERROR: No Version met constraints\" color=red];",
		"\n " + makeEncodedString(NodeThatWasIntroducedButNeverTried.Name) + " [label=\"" + NodeThatWasIntroducedButNeverTried.Name + "\\ntried: " + strings.Join(NodeThatWasIntroducedButNeverTried.Versions, ", ") + "\\nselected: " + NodeThatWasIntroducedButNeverTried.Selected + "\" color=black];",
		"\n " + makeEncodedString(NodeWithDeps.Name) + " [label=\"" + NodeWithDeps.Name + "\\ntried: " + strings.Join(NodeWithDeps.Versions, ", ") + "\\nselected: " + NodeWithDeps.Selected + "\" color=black];",
		"\n " + makeEncodedString(ParentOfNodeThatFailed.Name) + " [label=\"" + ParentOfNodeThatFailed.Name + "\\ntried: " + strings.Join(ParentOfNodeThatFailed.Versions, ", ") + "\\nselected: " + ParentOfNodeThatFailed.Selected + "\" color=black];",
		"\n " + makeEncodedString(RootOfVizTesterTree.Name) + "->" + makeEncodedString(RootNode.Name) + ";",
		"\n " + makeEncodedString(RootOfVizTesterTree.Name) + "->" + makeEncodedString(DependerNode.Name) + ";",
		"\n " + makeEncodedString(RootOfVizTesterTree.Name) + "->" + makeEncodedString(NodeWithVersAndSel.Name) + ";",
		"\n " + makeEncodedString(RootOfVizTesterTree.Name) + "->" + makeEncodedString(ParentOfNodeThatFailed.Name) + ";",
		"\n " + makeEncodedString(RootOfVizTesterTree.Name) + "->" + makeEncodedString(NodeThatWasIntroducedButNeverTried.Name) + ";",
		"\n " + makeEncodedString(RootOfVizTesterTree.Name) + "->" + makeEncodedString(NodeWithDeps.Name) + ";",
		"\n " + makeEncodedString(NodeWithDeps.Name) + "->" + makeEncodedString(RootNode.Name) + ";",
		"\n " + makeEncodedString(NodeWithDeps.Name) + "->" + makeEncodedString(DependerNode.Name) + ";",
		"\n " + makeEncodedString(NodeWithDeps.Name) + "->" + makeEncodedString(NodeWithVersAndSel.Name) + ";",
		"\n " + makeEncodedString(ParentOfNodeThatFailed.Name) + "->" + makeEncodedString(NodeThatFailed.Name) + " [color=\"red\"];",
		"\n " + makeEncodedString(ParentOfNodeThatFailed.Name) + "->" + makeEncodedString(RootNode.Name) + ";",
	}

	//basic hashed map for a tree that has a root of NodeWithDeps
	encodedRelationshipsForNodeWithDeps = map[string][]string{
		encodedNodeValues[NodeWithDeps.Name]: {
			encodedNodeValues[RootNode.Name],
			encodedNodeValues[DependerNode.Name],
			encodedNodeValues[NodeWithVersAndSel.Name],
		},
		encodedNodeValues[RootNode.Name]:           {},
		encodedNodeValues[DependerNode.Name]:       {},
		encodedNodeValues[NodeWithVersAndSel.Name]: {},
	}

	// additional relationships needed to add to encodedRelationshipsForNodeWithDeps
	// to replicate expected encoded output of NodeWithRedundantDeps
	encodedDepsOnRedundantDeps = map[string][]string{
		encodedNodeValues[NodeWithRedundantDeps.Name]: {
			encodedNodeValues[DependerNode.Name],
			encodedNodeValues[NodeWithVersAndSel.Name],
			encodedNodeValues[NodeWithDeps.Name],
		},
	}
)

const (
	RootProject = "code.uber.internal/devexp/test-root-repo.git"

	Project = "code.uber.internal/devexp/test-repo.git"
)

// because test cases often have overlapping nodes, this allows us to use the "base case"
// encodedRelationshipsForNodeWithDeps map and append new cases.
func encodeTestCaseFromBase(prevEncodedRelationships map[string][]string, newEncodedDeps map[string][]string) map[string][]string {
	newEncodedRelationships := make(map[string][]string)

	for newParent, newDep := range newEncodedDeps {
		newEncodedRelationships[newParent] = newDep
	}

	for oldParent, oldDeps := range prevEncodedRelationships {
		newEncodedRelationships[oldParent] = oldDeps
	}

	return newEncodedRelationships
}
