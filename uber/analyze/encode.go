package analyze

import (
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"strings"

	"github.com/golang/dep/uber"
)

func makeEncodedString(name string) string {
	hash := fnv.New32()
	hash.Write([]byte(name))
	return strconv.FormatInt(int64(hash.Sum32()), 10)
}

// uses the ResolverTree to encode Nodes. It returns two maps. The first is a map of encodedRelationships,
// which is a map with key parentNode and value of slice of childNodes (given as strings of encoded values)
// The second return is a map with key of nodeName and value of nodeEncodedVal, both as strings.
func mkRelationships(resTree *TreeNode) (map[string][]string, map[string]string) {
	curLevel := []*TreeNode{resTree}
	encodedRelationships := make(map[string][]string)
	nameAndEncodedVals := make(map[string]string)

	for len(curLevel) > 0 {
		nextLevel := make([]*TreeNode, 0)
		for _, node := range curLevel {
			checkDups := make(map[string]bool)

			if nameAndEncodedVals[node.Name] == "" {
				nameAndEncodedVals[node.Name] = makeEncodedString(node.Name)
			}
			nodeEncoded := nameAndEncodedVals[node.Name]

			// if there are already values in the the hash, we've already visited this node.
			if encodedRelationships[nodeEncoded] == nil {
				encodedRelationships[nodeEncoded] = make([]string, 0)
				for _, child := range node.Deps {
					if nameAndEncodedVals[child.Name] == "" {
						nameAndEncodedVals[child.Name] = makeEncodedString(child.Name)
					}
					encodedChild := nameAndEncodedVals[child.Name]
					if !checkDups[encodedChild] {
						// append the hashed child value to the deppers list for the parent node
						encodedRelationships[nodeEncoded] = append(encodedRelationships[nodeEncoded], encodedChild)
						// put it in the duplicate map so we don't add it more than once
						checkDups[encodedChild] = true
						// add that child to the nextLevel so that we can get its children next
						nextLevel = append(nextLevel, child)
					}
				}
			}
		}
		curLevel = nextLevel
	}

	return encodedRelationships, nameAndEncodedVals
}

func buildGraphVizOutput(tree *ResolverTree) []string {
	relationships, namesToEncodedVals := mkRelationships(tree.VersionTree)
	fileInput := make([]string, 0)
	// write all of the encoded values and their project details
	for projectName, hashedValue := range namesToEncodedVals {
		color := "black"
		curNode := tree.NodeList[projectName]
		versions := curNode.Versions

		if curNode.Selected == "" && curNode.Name != tree.VersionTree.Name && len(curNode.Versions) != 0 {
			color = "red"
		}

		var lineOutput []string
		if curNode.Name == tree.VersionTree.Name {
			lineOutput = []string{"\n ", hashedValue, " [label=\" ROOT: \\n", projectName, "\\n\" color=", color, "]; "}
		} else {
			lineOutput = []string{"\n ", hashedValue, " [label=\"", projectName, "\\n", "tried: ", strings.Join(versions, ", "), "\\n", "selected: ", tree.NodeList[projectName].Selected, "\" color=", color, "];"}
		}

		concatString := strings.Join(lineOutput, "")
		fileInput = append(fileInput, concatString)
	}

	// Create the graphviz relationships between nodes
	for parent, childList := range relationships {
		for _, child := range childList {
			lineOutput := []string{"\n ", parent, "->", child, ";"}
			concatString := strings.Join(lineOutput, "")
			fileInput = append(fileInput, concatString)
		}
	}
	return fileInput
}

func writeToFile(tree *ResolverTree) {
	file := "./resolutionGraph-" + uber.RunId + ".txt"
	header := "digraph G{\n"

	writeFile, err := os.Create(file)
	if err != nil {
		fmt.Println(err)
	}

	writeFile, err = os.OpenFile(file, os.O_WRONLY, 0666)
	writeFile.WriteString(header)
	if err != nil {
		fmt.Println(err)
	}

	writeFile.WriteString(strings.Join(buildGraphVizOutput(tree), ""))

	footer := "\n }"
	writeFile.WriteString(footer)
	writeFile.Close()

	fmt.Printf("Find the graph of your dependencies in %v\n", file)
}
