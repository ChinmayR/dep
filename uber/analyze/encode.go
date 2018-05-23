package analyze

import (
	"hash/fnv"
)

//keep track of encoded values alongside node names for graphviz mapping
func hashResolverNodes(tree *ResolverTree) map[string]uint32 {
	hashList := make(map[string]uint32, 0)
	for name, _ := range tree.NodeList {
		hash := fnv.New32()
		hash.Write([]byte(name))
		hashList[name] = hash.Sum32()
	}
	return hashList
}
