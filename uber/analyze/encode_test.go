package analyze

import (
	"testing"
)

type encodeTestCase map[string]struct {
	givenTree           *ResolverTree
	expNumUniqueHashes  int
	givenHashedMap      map[string]string
	expRelationships    map[string][]string
	expFileOutput       []string
}

func TestUber_Analyze_mkRelationships(t *testing.T) {
	cases := encodeTestCase{
		"should correctly create mkRelationships for node without dependencies": {
			givenTree:      TreeWithRootOnly,
			givenHashedMap: encodedNodeValues,
			expNumUniqueHashes: 1,
			expRelationships: map[string][]string{
				encodedNodeValues[TreeWithRootOnly.VersionTree.Name]: {},
			},
		},
		"should correctly create mkRelationships for simple dependency tree": {
			givenTree:           TreeWithSimpleDeps,
			expNumUniqueHashes:  4,
			givenHashedMap:      encodedNodeValues,
			expRelationships:    encodedRelationshipsForNodeWithDeps,
		},
		"should return unique mkRelationships for duplicated dependencies": {
			givenTree:        TreeWithTwoToOneDepperToDep,
			givenHashedMap:   encodedNodeValues,
			expNumUniqueHashes: 5,
			expRelationships: encodeTestCaseFromBase(encodedRelationshipsForNodeWithDeps, encodedDepsOnRedundantDeps),
		},
		"should return no mkRelationships for unreferenced nodes": {
			givenTree:        TreeWithUnreferencedNode,
			givenHashedMap:   encodedNodeValues,
			expNumUniqueHashes: 4,
			expRelationships: encodedRelationshipsForNodeWithDeps,
		},
	}

	for name, tc := range cases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			relationships, hashList := mkRelationships(tc.givenTree.VersionTree)

			if len(hashList) != tc.expNumUniqueHashes {
				t.Fatalf("unexpected number of hashes \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(hashList), tc.expNumUniqueHashes)
			}

			isUnique := map[string]bool{}
			for nodeName, _ := range tc.givenTree.NodeList {
				hashVal := hashList[nodeName]
				if isUnique[hashVal] == true {
					t.Fatalf("unexpected duplicate hash value \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, hashVal, "")
				} else {
					isUnique[hashVal] = true
				}
			}

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

func TestUber_Analyze_buildGraphVizOutput(t *testing.T) {
	cases := encodeTestCase{
		"produces the expected graph": {
			givenTree:     TreeToGraph,
			expFileOutput: TreeToGraphOutput,
		},
	}

	for name, tc := range cases {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			gotFileOutput := buildGraphVizOutput(tc.givenTree)

			if len(gotFileOutput) != len(tc.expFileOutput) {
				t.Fatalf("Unexpected output length \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, len(gotFileOutput), len(tc.expFileOutput))
			}

			for _, gotLine := range gotFileOutput {
				found := false
				for _, wantLine := range tc.expFileOutput {
					if wantLine == gotLine {
						found = true
					}
				}
				if !found {
					t.Fatalf("Got unexpected line \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, gotLine, tc.expFileOutput)
				}

			}

			for _, wantLine := range tc.expFileOutput {
				found := false
				for _, gotLine := range gotFileOutput {
					if wantLine == gotLine {
						found = true
					}
				}
				if !found {
					t.Fatalf("Missing expected output line \n\t(CASE) %v \n\t(GOT) %v\n\t(WNT) %v", name, "", tc.expFileOutput)
				}
			}
		})
	}
}
