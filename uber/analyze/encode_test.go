package analyze

import (
	"testing"
)

// the input will be a ResTree
// the output will be a list of 1:1 relationships

// first test should make sure that each project name gets its own unique hash.
type encodeTestCase map[string]struct {
	givenTree          *ResolverTree
	expNumUniqueHashes int
}

func TestUber_Analyze_HashResolverNodes(t *testing.T) {
	cases := encodeTestCase{
		"should hash properly when given tree root only": {
			givenTree:          ResolverTreeRoot,
			expNumUniqueHashes: 1,
		},
		"should have correct number of hashes for simple tree": {
			givenTree:          SimpleResolverTreeWithDeps,
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
				t.Fatalf("unexpected number of bytes \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(hashList), tc.expNumUniqueHashes)
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
