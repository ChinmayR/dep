package analyze

import (
	"testing"
)

// first test should make sure that each project name gets its own unique hash.
type encodeTestCase map[string]struct {
	givenTree          *ResolverTree
	expNumUniqueHashes int
	givenHashedMap     map[string]uint32
	expRelationships   map[uint32][]uint32
}

func TestUber_Analyze_HashResolverNodes(t *testing.T) {
	cases := encodeTestCase{
		"should hash properly when given tree root only": {
			givenTree:          TreeWithRootOnly,
			expNumUniqueHashes: 1,
		},
		"should have correct number of hashes for simple tree": {
			givenTree:          TreeWithSimpleDeps,
			expNumUniqueHashes: 4,
		},
		"should have correct number of hashed values when nodes have more than 1 depender": {
			givenTree:          TreeWithTwoToOneDepperToDep,
			expNumUniqueHashes: 5,
		},
		"should hash properly when unreferenced node remains in project list": {
			givenTree:          TreeWithUnreferencedNode,
			expNumUniqueHashes: 5,
		},
	}

	for name, tc := range cases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			hashList := hashResolverNodes(tc.givenTree)

			if len(hashList) != tc.expNumUniqueHashes {
				t.Fatalf("unexpected number of hashes \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(hashList), tc.expNumUniqueHashes)
			}

			isUnique := map[uint32]bool{}
			for nodeName, _ := range tc.givenTree.NodeList {
				hashVal := hashList[nodeName]
				if hashVal == 0 {
					t.Fatalf("missing node from hash list \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, nodeName, "")
				}

				if isUnique[hashVal] == true {
					t.Fatalf("unexpected duplicate hash value \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, hashVal, "")
				} else {
					isUnique[hashVal] = true
				}
			}
		})
	}
}

// when given a tree of hashed nodes, creates a map with key of parent proj and value of []projHashes
func TestUber_Analyze_HashRelationships(t *testing.T) {
	cases := encodeTestCase{
		"should correctly create mkRelationships for node without dependencies": {
			givenTree:      TreeWithRootOnly,
			givenHashedMap: encodedNodeValues,
			expRelationships: map[uint32][]uint32{
				1: {},
			},
		},
		"should correctly create mkRelationships for simple dependency tree": {
			givenTree:        TreeWithSimpleDeps,
			givenHashedMap:   encodedNodeValues,
			expRelationships: encodedRelationshipsForNodeWithDeps,
		},
		"should return unique mkRelationships for duplicated dependencies": {
			givenTree:        TreeWithTwoToOneDepperToDep,
			givenHashedMap:   encodedNodeValues,
			expRelationships: encodeTestCaseFromBase(encodedRelationshipsForNodeWithDeps, encodedNodeValues[NodeWithRedundantDeps.Name], encodedDepsOnRedundantDeps),
		},
		"should return no mkRelationships for unreferenced nodes": {
			givenTree:        TreeWithUnreferencedNode,
			givenHashedMap:   encodedNodeValues,
			expRelationships: encodedRelationshipsForNodeWithDeps,
		},
	}

	for name, tc := range cases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			curLevel := []*TreeNode{tc.givenTree.VersionTree}
			rels := make(map[uint32][]uint32)
			relationships := mkRelationships(curLevel, tc.givenHashedMap, rels)

			if len(relationships) != len(tc.expRelationships) {
				t.Fatalf("unexpected number of keys \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(relationships), len(tc.expRelationships))

			}

			for depender, deps := range tc.expRelationships {
				var found bool
				if len(relationships[depender]) != len(deps) {
					t.Fatalf("unexpected nodes represented in the hash map \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, relationships[depender], deps)
				}

				// order doesn't matter because it will not affect the way the graph is displayed.
				if len(relationships[depender]) > 0 {
					for _, dep := range deps {
						for _, child := range relationships[depender] {
							if child == dep {
								found = !found
							}
						}
					}
					if !found {
						t.Fatalf("missing an expected hashed value \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, deps, relationships[depender])
					}

				}
			}
		})
	}
}
