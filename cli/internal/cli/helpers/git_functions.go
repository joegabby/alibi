package helpers

// import (
// 	"bytes"
// 	"fmt"
// 	"os/exec"
// 	"path/filepath"
// 	"regexp"
// 	"strings"

// 	sitter "github.com/smacker/go-tree-sitter"
// )

// // --------------------------- Types & constants ---------------------------

// // ChangeExtractorConfig can be extended later (e.g., ignore patterns)
// type ChangeExtractorConfig struct {
// 	IgnoreImportLines bool
// }

// // --------------------------- Parser helpers ---------------------------

// // getLanguageForPath returns the Tree-sitter language for a file path extension or nil.
// func getLanguageForPath(path string) *sitter.Language {
// 	ext := strings.ToLower(filepath.Ext(path))
// 	if lang, ok := treeSitterParsers[ext]; ok {
// 		return lang
// 	}
// 	return nil
// }

// // --------------------------- Diff helpers ---------------------------

// // isStringNode returns true for common string node types (used for Python docstrings)
// func isStringNode(t string) bool {
// 	switch t {
// 	case "string", "string_literal", "raw_string", "byte_string", "quoted_string":
// 		return true
// 	default:
// 		return false
// 	}
// }

// // --------------------------- Part 2/3: extraction logic ---------------------------

// // isImportLine heuristics per extension (basic)
// func isImportLine(ext, line string) bool {
// 	t := strings.TrimSpace(line)
// 	if t == "" {
// 		return false
// 	}
// 	switch ext {
// 	case ".py":
// 		// python imports: import x, from x import y
// 		return strings.HasPrefix(t, "import ") || strings.HasPrefix(t, "from ")
// 	case ".js", ".ts", ".tsx", ".jsx":
// 		// JS/TS imports and require
// 		return strings.HasPrefix(t, "import ") || strings.HasPrefix(t, "export ") || strings.Contains(t, "require(")
// 	case ".go":
// 		// package and import
// 		return strings.HasPrefix(t, "package ") || strings.HasPrefix(t, "import ")
// 	case ".java":
// 		return strings.HasPrefix(t, "import ") || strings.HasPrefix(t, "package ")
// 	case ".cs":
// 		return strings.HasPrefix(t, "using ")
// 	case ".php":
// 		return strings.HasPrefix(t, "use ") || strings.HasPrefix(t, "<?php")
// 	default:
// 		// generic: treat lines that begin with 'import' or 'package' as imports
// 		return strings.HasPrefix(t, "import ") || strings.HasPrefix(t, "package ")
// 	}
// }

// // runGit convenience wrapper (uses your existing runGit if available; fallback here if not)
// func runGitCmd(args ...string) (string, error) {
// 	// if you already have runGit implemented in your codebase, comment out this and call runGit directly.
// 	cmd := exec.Command("git", args...)
// 	var out bytes.Buffer
// 	cmd.Stdout = &out
// 	cmd.Stderr = &out
// 	err := cmd.Run()
// 	return out.String(), err
// }

// // getChildFunctionRanges returns a list of [startLine,endLine] (1-based new-file lines)
// // for nested function nodes inside `node`. We use StartPoint().Row and EndPoint().Row (0-based) and convert.
// func getChildFunctionRanges(node *sitter.Node) [][2]int {
// 	var ranges [][2]int
// 	var rec func(n *sitter.Node)
// 	rec = func(n *sitter.Node) {
// 		if n == nil {
// 			return
// 		}
// 		// skip the root node itself at first call; we only want nested function nodes
// 		for i := 0; i < int(n.ChildCount()); i++ {
// 			c := n.Child(i)
// 			if c == nil {
// 				continue
// 			}
// 			if isFunctionNode(c) {
// 				// record its line range
// 				start := int(c.StartPoint().Row) + 1
// 				end := int(c.EndPoint().Row) + 1
// 				ranges = append(ranges, [2]int{start, end})
// 				// do not descend into this child (we recorded it)
// 				continue
// 			}
// 			// descend into children to find deeper nested functions
// 			rec(c)
// 		}
// 	}
// 	// start recursion from node itself but skip directly recording node; we want nested children only
// 	rec(node)
// 	return ranges
// }

// // lineInRanges checks if a 1-based line is inside any of ranges
// func lineInRanges(line int, ranges [][2]int) bool {
// 	for _, r := range ranges {
// 		if line >= r[0] && line <= r[1] {
// 			return true
// 		}
// 	}
// 	return false
// }

// // ExtractFunctionChanges returns raw added lines (+) from commitHash for the specified function.
// // Returns empty slice if none found or on non-fatal errors.
// func ExtractFunctionChanges(commitHash, path, functionName string) []string {
// 	// 1) get diff patch for commit & file
// 	diff, err := RunGit("show", commitHash, "--", path)
// 	if err != nil {
// 		// fallback: try local runGitCmd if runGit not present
// 		if out, e := runGitCmd("show", commitHash, "--", path); e == nil {
// 			diff = out
// 		} else {
// 			// unable to get diff -> return empty slice
// 			return []string{}
// 		}
// 	}
// 	addedLinesSet := parseAddedLineNumbersFromDiff(diff) // 1-based line numbers

// 	// 2) get new file content at commit
// 	newContent, err := RunGit("show", fmt.Sprintf("%s:%s", commitHash, path))
// 	if err != nil {
// 		if out, e := runGitCmd("show", fmt.Sprintf("%s:%s", commitHash, path)); e == nil {
// 			newContent = out
// 		} else {
// 			return []string{}
// 		}
// 	}

// 	ext := strings.ToLower(filepath.Ext(path))
// 	lang := getLanguageForPath(path)
// 	lines := strings.Split(newContent, "\n") // 0-based slice, but addedLinesSet is 1-based

// 	// If no Tree-sitter grammar available, fallback: scan added lines for functionName context (best-effort)
// 	if lang == nil {
// 		return fallbackExtractFunctionChanges(addedLinesSet, lines, ext, functionName)
// 	}

// 	// parse with Tree-sitter
// 	parser := sitter.NewParser()
// 	parser.SetLanguage(lang)
// 	tree := parser.Parse(nil, []byte(newContent))
// 	root := tree.RootNode()

// 	// find target function node
// 	fnNode := findFunctionNodeByName(root, functionName, newContent)
// 	if fnNode == nil {
// 		// not found -> return empty slice
// 		return []string{}
// 	}

// 	// compute parent function start/end lines
// 	fnStart := int(fnNode.StartPoint().Row) + 1
// 	fnEnd := int(fnNode.EndPoint().Row) + 1

// 	// compute nested child function ranges so we can exclude changes inside them
// 	childRanges := getChildFunctionRanges(fnNode)

// 	// collect added lines that are:
// 	// - within fnStart..fnEnd (inclusive), OR are doc comment lines immediately above function (we handle separately)
// 	// - not within any childRanges
// 	var out []string

// 	// 3) include doc comments above function if the doc/comment line was added
// 	// scan upwards for contiguous comment lines; include those that are in addedLinesSet
// 	for i := fnStart - 1; i >= 1; i-- {
// 		// i is 1-based; index in lines is i-1
// 		raw := lines[i-1]
// 		trim := strings.TrimSpace(raw)
// 		if trim == "" {
// 			// allow blank lines between doc comments
// 			continue
// 		}
// 		// doc/comment prefixes
// 		if strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "#") ||
// 			strings.HasPrefix(trim, "/*") || strings.HasPrefix(trim, "*") ||
// 			strings.HasPrefix(trim, `"""`) || strings.HasPrefix(trim, `'''`) {
// 			if addedLinesSet[i] {
// 				out = append(out, raw) // keep raw formatting
// 			}
// 			continue
// 		}
// 		// non-comment line -> stop scanning upward
// 		break
// 	}

// 	// 4) include added lines in function body, excluding child function ranges and ignoring import lines
// 	for ln := fnStart; ln <= fnEnd; ln++ {
// 		if !addedLinesSet[ln] {
// 			continue
// 		}
// 		// skip lines that belong to nested child functions
// 		if lineInRanges(ln, childRanges) {
// 			continue
// 		}
// 		raw := lines[ln-1]
// 		// ignore import lines if configured
// 		if isImportLine(ext, raw) {
// 			continue
// 		}
// 		out = append(out, raw)
// 	}

// 	// preserve order as in file: doc-comments above are added first (we scanned upward and preprended in order),
// 	// and then body lines in increasing line order
// 	if len(out) == 0 {
// 		return []string{}
// 	}
// 	return out
// }

// // fallbackExtractFunctionChanges: best-effort when no Tree-sitter grammar available
// func fallbackExtractFunctionChanges(addedLines map[int]bool, lines []string, ext, functionName string) []string {
// 	var out []string
// 	// Find the first occurrence in the new content that looks like a function declaration/assignment
// 	funcLineIndex := -1
// 	funcRe := regexp.MustCompile(fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(functionName)))
// 	for i, l := range lines {
// 		if funcRe.MatchString(l) && (strings.Contains(l, "func") || strings.Contains(l, "def") || strings.Contains(l, "function") || strings.Contains(l, "=>") || strings.Contains(l, "(")) {
// 			funcLineIndex = i
// 			break
// 		}
// 	}
// 	if funcLineIndex == -1 {
// 		return []string{}
// 	}
// 	// doc above
// 	for j := funcLineIndex - 1; j >= 0; j-- {
// 		t := strings.TrimSpace(lines[j])
// 		if t == "" {
// 			continue
// 		}
// 		if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "/*") || strings.HasPrefix(t, `"""`) || strings.HasPrefix(t, `'''`) {
// 			if addedLines[j+1] {
// 				out = append([]string{lines[j]}, out...)
// 			}
// 			continue
// 		}
// 		break
// 	}
// 	// inside: include added lines forward until a closing brace or next top-level blank line as heuristic
// 	for k := funcLineIndex; k < len(lines); k++ {
// 		if addedLines[k+1] {
// 			ln := lines[k]
// 			if isImportLine(ext, ln) {
// 				continue
// 			}
// 			out = append(out, ln)
// 		}
// 		// break heuristics: stop at top-level blank line or closing brace
// 		trim := strings.TrimSpace(lines[k])
// 		if trim == "" {
// 			break
// 		}
// 		if strings.Contains(lines[k], "}") {
// 			// still continue one more line in case closing brace was added
// 			// but then break
// 			// (no-op)
// 		}
// 	}
// 	return out
// }
// // func ExtractFunctionChanges(commitHash, path, functionName string) []string {
// //     return ExtractFunctionChangesInternal(commitHash, path, functionName)
// // }