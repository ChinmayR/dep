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

// uses the ResolverTree's root node to encode Nodes. It returns two maps. The first is a map of encodedRelationships,
// which is a map with key parentNode and value of slice of childNodes (given as strings of encoded values)
// The second return is a map with key of nodeName and value of nodeEncodedVal, both as strings.
func mkRelationships(rootNode *TreeNode) (map[string][]string, map[string]string) {
	encodeQueue := []*TreeNode{rootNode}
	encodedRelationships := make(map[string][]string)
	nameAndEncodedVals := make(map[string]string)

	for len(encodeQueue) > 0 {
		curNode := encodeQueue[0]
		encodeQueue = append(encodeQueue[1:])
		if nameAndEncodedVals[curNode.Name] == "" {
			encodedNodeVal := makeEncodedString(curNode.Name)
			nameAndEncodedVals[curNode.Name] = encodedNodeVal

			encodedRelationships[encodedNodeVal] = make([]string, 0)
			dups := make(map[string]bool)
			for _, childNode := range curNode.Deps {
				if !dups[childNode.Name]{
					encodeQueue = append(encodeQueue, childNode)
					dups[childNode.Name] = true
					encodedRelationships[encodedNodeVal] = append(encodedRelationships[encodedNodeVal], makeEncodedString(childNode.Name))
				}
			}
		}
	}
	return encodedRelationships, nameAndEncodedVals
}

func buildGraphVizOutput(tree *ResolverTree) []string {
	relationships, namesToEncodedVals := mkRelationships(tree.VersionTree)
	fileInput := make([]string, 0)
	// write all of the encoded values and their project details
	failedProject := ""
	errorOutput := make([]string, 0)
	if tree.LastError != nil {
		failedProject = tree.LastError.Pn
		for _, err := range tree.LastError.Fails {
			errorOutput = append(errorOutput, err.Err)
		}
	}

	for projectName, hashedValue := range namesToEncodedVals {
		var lineOutput []string

		curNode := tree.NodeList[projectName]
		nodeLabel := " [label=\""
		tried :=  "\\ntried: "
		versions := curNode.Versions
		allTriedVersions := strings.Join(versions, ", ")
		selectedLabel := "\\nselected: "
		selectedVersion := tree.NodeList[projectName].Selected
		colorLabel := "\" color="
		color := "black"
		errLabel := ""
		reportedError := ""

		if curNode.Name == tree.VersionTree.Name {
			nodeLabel = " [label=\" ROOT: \\n"
			tried = ""
			selectedLabel = "\\n"
			selectedVersion = ""
		}

		if curNode.Name == failedProject {
			color = "red"
			errLabel = "\\n\\n ERROR: "
			reportedError = strings.Join(errorOutput, "\\n")
		}

		lineOutput = []string{"\n ", hashedValue, nodeLabel, projectName, tried, allTriedVersions, selectedLabel, selectedVersion, errLabel, reportedError, colorLabel, color, "];"}
		concatString := strings.Join(lineOutput, "")
		fileInput = append(fileInput, concatString)
	}

	// Create the graphviz relationships between nodes
	failNodeEncoded := makeEncodedString(failedProject)
	for parent, childList := range relationships {
		highlightFailedParents := ""
		for _, child := range childList {
			if child == failNodeEncoded {
				highlightFailedParents = " [color=\"red\"]"
			}

			lineOutput := []string{"\n ", parent, "->", child, highlightFailedParents, ";"}
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
