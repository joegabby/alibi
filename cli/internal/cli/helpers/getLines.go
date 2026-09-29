package helpers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

type DiffHunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []string
}

func parseGitDiff(diff string) []DiffHunk {
	lines := strings.Split(diff, "\n")
	var hunks []DiffHunk

	hunkHeader := regexp.MustCompile(`@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@`)
	var current *DiffHunk

	for _, l := range lines {
		if matches := hunkHeader.FindStringSubmatch(l); len(matches) > 0 {
			if current != nil {
				hunks = append(hunks, *current)
			}
			current = &DiffHunk{
				OldStart: atoi(matches[1]),
				OldLines: atoiDefault(matches[2], 1),
				NewStart: atoi(matches[3]),
				NewLines: atoiDefault(matches[4], 1),
				Lines:    []string{},
			}
		} else if current != nil {
			current.Lines = append(current.Lines, l)
		}
	}

	if current != nil {
		hunks = append(hunks, *current)
	}

	return hunks
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	return atoi(s)
}

func ExtractFunctionDiffUsingTreeSitter(commitHash, path, functionName string) []string {
	// 1️⃣ Load file at this commit
	lang := getLanguageForPath(path)

	fileContent, err := RunGit("show", commitHash+":"+path)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	// 2️⃣ Parse diff
	diffOutput, err := RunGit("show", commitHash, "--", path)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	hunks := parseGitDiff(diffOutput)

	// 3️⃣ Index file line → byte offset
	lines := strings.Split(fileContent, "\n")
	lineByteOffset := make([]int, len(lines))
	offset := 0
	for i, ln := range lines {
		lineByteOffset[i] = offset
		offset += len(ln) + 1 // include newline
	}

	// 4️⃣ Use Tree-sitter to locate function range
	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree := parser.Parse(nil, []byte(fileContent))
	root := tree.RootNode()

	var funcStart, funcEnd uint32

	var find func(node *sitter.Node)
	find = func(node *sitter.Node) {
		if node == nil {
			return
		}
		if isFunctionNode(node) {
			name := getFunctionName(node, fileContent)
			if name == functionName {
				funcStart = node.StartByte()
				funcEnd = node.EndByte()
				return
			}
		}
		for i := 0; i < int(node.ChildCount()); i++ {
			find(node.Child(i))
		}
	}

	find(root)
	if funcStart == 0 && funcEnd == 0 {
		return []string{"+0", "-0"}
	}

	// 5️⃣ Walk diff hunks + map to byte ranges
	added := 0
	removed := 0

	for _, h := range hunks {
		newLine := h.NewStart
		oldLine := h.OldStart

		for _, l := range h.Lines {
			if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
				lineIndex := newLine - 1
				if lineIndex >= 0 && lineIndex < len(lineByteOffset) {
					bytePos := lineByteOffset[lineIndex]
					if uint32(bytePos) >= funcStart && uint32(bytePos) <= funcEnd {
						added++
					}
				}
				newLine++
			} else if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
				lineIndex := oldLine - 1
				if lineIndex >= 0 && lineIndex < len(lineByteOffset) {
					bytePos := lineByteOffset[lineIndex]
					if uint32(bytePos) >= funcStart && uint32(bytePos) <= funcEnd {
						removed++
					}
				}
				oldLine++
			} else {
				// context line
				newLine++
				oldLine++
			}
		}
	}

	return []string{
		fmt.Sprintf("+%d", added),
		fmt.Sprintf("-%d", removed),
	}
}
