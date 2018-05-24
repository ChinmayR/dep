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

func mkRelationships(currentLevel []*TreeNode, hashedValues map[string]uint32, rels map[uint32][]uint32) map[uint32][]uint32 {
	nextLevel := make([]*TreeNode, 0)

	for _, parentNode := range currentLevel {
		rels := setupRelationship(parentNode.Name, hashedValues, rels)
		rels, nextLevel = addBytesToRelMap(parentNode, hashedValues, rels, nextLevel)
	}

	if len(nextLevel) > 0 {
		return mkRelationships(nextLevel, hashedValues, rels)
	}

	return rels
}

func setupRelationship(name string, hashedValues map[string]uint32, rels map[uint32][]uint32) map[uint32][]uint32 {
	parentBytes := hashedValues[name]
	if rels[parentBytes] == nil {
		rels[parentBytes] = make([]uint32, 0)
	}

	return rels
}

func addBytesToRelMap(parentNode *TreeNode, hashedValues map[string]uint32, rels map[uint32][]uint32, nextLevel []*TreeNode) (map[uint32][]uint32, []*TreeNode) {
	parentBytes := hashedValues[parentNode.Name]

	// check dups to avoid adding the same dep to the map twice
	checkDups := make(map[uint32]bool)
	for _, childNode := range parentNode.Deps {
		childHash := hashedValues[childNode.Name]
		if !checkDups[childHash] {
			checkDups[childHash] = true
			rels[parentBytes] = append(rels[parentBytes], childHash)
		}
		// avoid stack overflow by checking against current relationship list before adding to nextLevel queue
		if rels[childHash] == nil {
			nextLevel = append(nextLevel, childNode)
		}
	}
	return rels, nextLevel
}
