package helpers

// import (
// 	"fmt"
// 	"path/filepath"
// 	"regexp"
// 	"strings"

// 	sitter "github.com/smacker/go-tree-sitter"
// )

// // Assumes you have this map in the same package (from earlier)
// // var treeSitterParsers = map[string]*sitter.Language{
// // 	".go":   tsGo.GetLanguage(),
// // 	".py":   tsPython.GetLanguage(),
// // 	".js":   tsJS.GetLanguage(),
// // 	".ts":   tsTS.GetLanguage(),
// // 	".tsx":  tsTSX.GetLanguage(),
// // 	".cs":   tsCSharp.GetLanguage(),
// // 	".java": tsJava.GetLanguage(),
// // 	".php":  tsPHP.GetLanguage(),
// // }

// // ------------------------- Utilities -------------------------

// func atoiSafe(s string) int {
// 	if s == "" {
// 		return 0
// 	}
// 	var v int
// 	fmt.Sscanf(s, "%d", &v)
// 	return v
// }

// // parseAddedLineNumbersFromDiff returns a set (map[int]bool) of 1-based line numbers in the NEW file that are added in the diff.
// func parseAddedLineNumbersFromDiff(diff string) map[int]bool {
// 	added := map[int]bool{}
// 	hunkRe := regexp.MustCompile(`^@@\s+-(\d+)(?:,\d+)?\s+\+(\d+)(?:,(\d+))?\s+@@`)
// 	lines := strings.Split(diff, "\n")
// 	i := 0
// 	for i < len(lines) {
// 		line := lines[i]
// 		if m := hunkRe.FindStringSubmatch(line); m != nil {
// 			newStart := atoiSafe(m[2])
// 			newLine := newStart
// 			i++
// 			for i < len(lines) {
// 				l := lines[i]
// 				if strings.HasPrefix(l, "@@") {
// 					break
// 				}
// 				if strings.HasPrefix(l, "+") {
// 					// Exclude file header lines like "+++ b/file" by trusting hunks: those are not inside hunks
// 					added[newLine] = true
// 					newLine++
// 				} else if strings.HasPrefix(l, "-") {
// 					// removal: do not increment newLine
// 				} else {
// 					// context
// 					newLine++
// 				}
// 				i++
// 			}
// 			continue
// 		}
// 		i++
// 	}
// 	return added
// }

// // isCommentNodeType: primary comment node type. Extend if you discover language-specific names.
// func isCommentNodeType(t string) bool {
// 	switch t {
// 	case "comment":
// 		return true
// 	default:
// 		return false
// 	}
// }

// // isFunctionNode: make sure it includes arrow_function, function_declaration, method_declaration, function_definition, method_definition, etc.
// // Extend this if your grammars use other names for function-like nodes.
// // func isFunctionNode(node *sitter.Node) bool {
// // 	if node == nil {
// // 		return false
// // 	}
// // 	switch node.Type() {
// // 	case "function_declaration",
// // 		"function_definition",
// // 		"method_declaration",
// // 		"method_definition",
// // 		"arrow_function",
// // 		"function":
// // 		return true
// // 	default:
// // 		return false
// // 	}
// // }

// // getFunctionName: reuse your logic (looks for identifier child or variable declarator parent for arrow functions)
// // func getFunctionName(node *sitter.Node, content string) string {
// // 	if node == nil {
// // 		return ""
// // 	}
// // 	// Direct identifier child
// // 	for i := 0; i < int(node.ChildCount()); i++ {
// // 		child := node.Child(i)
// // 		if child == nil {
// // 			continue
// // 		}
// // 		if child.Type() == "identifier" || child.Type() == "name" {
// // 			return content[child.StartByte():child.EndByte()]
// // 		}
// // 	}
// // 	// Arrow function assigned to variable: check parent declarator
// // 	if node.Type() == "arrow_function" {
// // 		parent := node.Parent()
// // 		if parent != nil && (parent.Type() == "lexical_declarator" || parent.Type() == "variable_declarator") {
// // 			// try field "name"
// // 			if ident := parent.ChildByFieldName("name"); ident != nil {
// // 				return content[ident.StartByte():ident.EndByte()]
// // 			}
// // 			// fallback: scan children for identifier
// // 			for i := 0; i < int(parent.ChildCount()); i++ {
// // 				c := parent.Child(i)
// // 				if c != nil && (c.Type() == "identifier" || c.Type() == "name") {
// // 					return content[c.StartByte():c.EndByte()]
// // 				}
// // 			}
// // 		}
// // 	}
// // 	// Some languages (Python) may put function name in different node types; try a common fallback:
// // 	// look upward for a node with identifier/name sibling
// // 	if parent := node.Parent(); parent != nil {
// // 		for i := 0; i < int(parent.ChildCount()); i++ {
// // 			c := parent.Child(i)
// // 			if c != nil && (c.Type() == "identifier" || c.Type() == "name") {
// // 				return content[c.StartByte():c.EndByte()]
// // 			}
// // 		}
// // 	}
// // 	return ""
// // }

// // normalizeCommentLine: remove leading/trailing comment markers and whitespace
// func normalizeCommentLine(s string) string {
// 	s = strings.TrimSpace(s)
// 	if strings.HasPrefix(s, "//") {
// 		s = strings.TrimPrefix(s, "//")
// 	} else if strings.HasPrefix(s, "#") {
// 		s = strings.TrimPrefix(s, "#")
// 	} else if strings.HasPrefix(s, "/*") {
// 		s = strings.TrimPrefix(s, "/*")
// 		s = strings.TrimSuffix(s, "*/")
// 	} else if strings.HasPrefix(s, "*") {
// 		s = strings.TrimPrefix(s, "*")
// 	} else if strings.HasPrefix(s, `"""`) || strings.HasPrefix(s, `'''`) {
// 		s = strings.TrimPrefix(s, `"""`)
// 		s = strings.TrimPrefix(s, `'''`)
// 		s = strings.TrimSuffix(s, `"""`)
// 		s = strings.TrimSuffix(s, `'''`)
// 	}
// 	s = strings.Trim(s, " \t\"'")
// 	return s
// }

// func uniqueTrimmed(inp []string) []string {
// 	seen := map[string]bool{}
// 	out := []string{}
// 	for _, s := range inp {
// 		t := strings.TrimSpace(s)
// 		if t == "" {
// 			continue
// 		}
// 		if !seen[t] {
// 			seen[t] = true
// 			out = append(out, t)
// 		}
// 	}
// 	return out
// }

// // ------------------ AST comment collectors ------------------

// // collectCommentsDirectChildren walks only the immediate body of `node` and collects comment nodes
// // but **skips** nested function nodes so child function comments are not assigned to parent.
// func collectCommentsDirectChildren(node *sitter.Node, content string) []string {
// 	if node == nil {
// 		return nil
// 	}
// 	var out []string
// 	// Depending on grammar, the function body may be a child node (e.g., "body", "block", "compound_statement")
// 	// We'll iterate through node's descendants but stop recursion at nested function nodes.
// 	var rec func(n *sitter.Node)
// 	rec = func(n *sitter.Node) {
// 		if n == nil {
// 			return
// 		}
// 		// If this node itself is a nested function (but not the root function node we were called with),
// 		// do not descend into it (skip).
// 		if n != node && isFunctionNode(n) {
// 			// skip nested function entirely
// 			return
// 		}
// 		// collect comment nodes that are direct or nested under non-function nodes
// 		if isCommentNodeType(n.Type()) {
// 			txt := strings.TrimSpace(content[n.StartByte():n.EndByte()])
// 			if txt != "" {
// 				out = append(out, txt)
// 			}
// 		} else {
// 			// python triple-quoted docstrings inside function bodies often appear as string nodes
// 			if n.Type() == "string" || n.Type() == "string_literal" || n.Type() == "raw_string" {
// 				txt := strings.TrimSpace(content[n.StartByte():n.EndByte()])
// 				if strings.HasPrefix(txt, `"""`) || strings.HasPrefix(txt, `'''`) {
// 					out = append(out, txt)
// 				}
// 			}
// 		}
// 		// recurse into children unless they are function nodes (handled above)
// 		for i := 0; i < int(n.ChildCount()); i++ {
// 			rec(n.Child(i))
// 		}
// 	}
// 	// Start recursion at the function node's body child if available, else from node itself.
// 	// Prefer body-like fields if present
// 	if body := node.ChildByFieldName("body"); body != nil {
// 		rec(body)
// 	} else if body := node.ChildByFieldName("body_block"); body != nil {
// 		rec(body)
// 	} else {
// 		// fallback: recurse children but skip nested functions as implemented
// 		for i := 0; i < int(node.ChildCount()); i++ {
// 			rec(node.Child(i))
// 		}
// 	}
// 	return uniqueTrimmed(out)
// }

// // getDocCommentsAbove returns contiguous doc/comment lines immediately above the node start (normalized)
// func getDocCommentsAbove(node *sitter.Node, content string) []string {
// 	if node == nil {
// 		return nil
// 	}
// 	lines := strings.Split(content, "\n")
// 	startLine := int(node.StartPoint().Row) // zero-based
// 	var out []string
// 	for i := startLine - 1; i >= 0; i-- {
// 		raw := lines[i]
// 		trim := strings.TrimSpace(raw)
// 		if trim == "" {
// 			// allow blank lines between comment lines (continue scanning)
// 			continue
// 		}
// 		// treat common doc/comment prefixes
// 		if strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "#") ||
// 			strings.HasPrefix(trim, "/*") || strings.HasPrefix(trim, "*") ||
// 			strings.HasPrefix(trim, `"""`) || strings.HasPrefix(trim, `'''`) {
// 			out = append([]string{normalizeCommentLine(trim)}, out...)
// 			continue
// 		}
// 		break
// 	}
// 	return out
// }

// // collectAllCommentsForNode = doc above + comments inside (but skipping nested functions)
// func collectAllCommentsForNode(node *sitter.Node, content string) []string {
// 	if node == nil {
// 		return nil
// 	}
// 	above := getDocCommentsAbove(node, content)
// 	inside := collectCommentsDirectChildren(node, content)
// 	merged := append([]string{}, above...)
// 	merged = append(merged, inside...)
// 	return uniqueTrimmed(merged)
// }

// // collectCommentsFromAddedLines returns only comments (doc above or inside) that overlap addedLines set (1-based)
// func collectCommentsFromAddedLines(node *sitter.Node, content string, addedLines map[int]bool) []string {
// 	if node == nil {
// 		return nil
// 	}
// 	var out []string
// 	lines := strings.Split(content, "\n")

// 	// doc comments above: only include those lines that are added
// 	startLine := int(node.StartPoint().Row) // zero-based
// 	for i := startLine - 1; i >= 0; i-- {
// 		trim := strings.TrimSpace(lines[i])
// 		if trim == "" {
// 			continue
// 		}
// 		if strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "#") ||
// 			strings.HasPrefix(trim, "/*") || strings.HasPrefix(trim, "*") ||
// 			strings.HasPrefix(trim, `"""`) || strings.HasPrefix(trim, `'''`) {
// 			if addedLines[i+1] { // addedLines uses 1-based new-file lines
// 				out = append([]string{normalizeCommentLine(trim)}, out...)
// 			}
// 			continue
// 		}
// 		break
// 	}

// 	// inside comments: iterate comment nodes inside the function node (skip nested functions)
// 	var rec func(n *sitter.Node)
// 	rec = func(n *sitter.Node) {
// 		if n == nil {
// 			return
// 		}
// 		// skip nested functions
// 		if n != node && isFunctionNode(n) {
// 			return
// 		}
// 		if isCommentNodeType(n.Type()) {
// 			start := int(n.StartPoint().Row) + 1
// 			end := int(n.EndPoint().Row) + 1
// 			overlap := false
// 			for l := start; l <= end; l++ {
// 				if addedLines[l] {
// 					overlap = true
// 					break
// 				}
// 			}
// 			if overlap {
// 				txt := strings.TrimSpace(content[n.StartByte():n.EndByte()])
// 				if txt != "" {
// 					out = append(out, txt)
// 				}
// 			}
// 		} else {
// 			// python docstrings as string nodes
// 			if n.Type() == "string" || n.Type() == "string_literal" || n.Type() == "raw_string" {
// 				txt := strings.TrimSpace(content[n.StartByte():n.EndByte()])
// 				if strings.HasPrefix(txt, `"""`) || strings.HasPrefix(txt, `'''`) {
// 					start := int(n.StartPoint().Row) + 1
// 					end := int(n.EndPoint().Row) + 1
// 					overlap := false
// 					for l := start; l <= end; l++ {
// 						if addedLines[l] {
// 							overlap = true
// 							break
// 						}
// 					}
// 					if overlap {
// 						out = append(out, txt)
// 					}
// 				}
// 			}
// 		}
// 		for i := 0; i < int(n.ChildCount()); i++ {
// 			rec(n.Child(i))
// 		}
// 	}
// 	// start recursion at body field if exists
// 	if body := node.ChildByFieldName("body"); body != nil {
// 		rec(body)
// 	} else {
// 		for i := 0; i < int(node.ChildCount()); i++ {
// 			rec(node.Child(i))
// 		}
// 	}
// 	return uniqueTrimmed(out)
// }

// // findFunctionNodeByName returns the first function node matching functionName
// func findFunctionNodeByName(root *sitter.Node, functionName string, content string) *sitter.Node {
// 	var found *sitter.Node
// 	var walk func(n *sitter.Node)
// 	walk = func(n *sitter.Node) {
// 		if n == nil || found != nil {
// 			return
// 		}
// 		if isFunctionNode(n) {
// 			name := getFunctionName(n, content)
// 			if name == functionName {
// 				found = n
// 				return
// 			}
// 		}
// 		for i := 0; i < int(n.ChildCount()); i++ {
// 			walk(n.Child(i))
// 			if found != nil {
// 				return
// 			}
// 		}
// 	}
// 	walk(root)
// 	return found
// }

// // ------------------ Public functions ------------------

// // ExtractFunctionComments: for a commit and path, returns all comments for the function if new,
// // otherwise returns only comments added in the commit (doc above or inside).
// func ExtractFunctionComments(commitHash, path, functionName string) ([]string, error) {
// 	// get diff (patch) for the commit for that file
// 	diff, err := RunGit("show", commitHash, "--", path)
// 	if err != nil {
// 		return nil, fmt.Errorf("git show diff failed: %v", err)
// 	}
// 	addedLines := parseAddedLineNumbersFromDiff(diff)

// 	// get new-content at commit
// 	newContent, err := RunGit("show", fmt.Sprintf("%s:%s", commitHash, path))
// 	if err != nil {
// 		return nil, fmt.Errorf("git show file at commit failed: %v", err)
// 	}

// 	// try get previous content (parent commit)
// 	prevCommit := commitHash + "^"
// 	prevContent, _ := RunGit("show", fmt.Sprintf("%s:%s", prevCommit, path))
// 	// prevContent may be empty if file didn't exist in parent

// 	ext := strings.ToLower(filepath.Ext(path))
// 	lang := treeSitterParsers[ext]
// 	if lang == nil {
// 		// fallback: best-effort using line-scan of addedLines in newContent
// 		comments,_ :=extractCommentsViaLineScan(addedLines, newContent, functionName)
// 		return comments, nil
// 	}

// 	// parse new content
// 	parser := sitter.NewParser()
// 	parser.SetLanguage(lang)
// 	tree := parser.Parse(nil, []byte(newContent))
// 	root := tree.RootNode()

// 	// find function node in new content
// 	fnNode := findFunctionNodeByName(root, functionName, newContent)
// 	if fnNode == nil {
// 		return nil, fmt.Errorf("function %s not found in new content", functionName)
// 	}

// 	// determine existence in prev content
// 	existedBefore := false
// 	if strings.TrimSpace(prevContent) != "" {
// 		parserPrev := sitter.NewParser()
// 		parserPrev.SetLanguage(lang)
// 		treePrev := parserPrev.Parse(nil, []byte(prevContent))
// 		rootPrev := treePrev.RootNode()
// 		if findFunctionNodeByName(rootPrev, functionName, prevContent) != nil {
// 			existedBefore = true
// 		}
// 	}

// 	if !existedBefore {
// 		// new function => return all doc + inside comments
// 		all := collectAllCommentsForNode(fnNode, newContent)
// 		if len(all) == 0 {
// 			return nil, nil
// 		}
// 		return all, nil
// 	}

// 	// modified function => return only comments that overlap added lines in the diff
// 	added := collectCommentsFromAddedLines(fnNode, newContent, addedLines)
// 	if len(added) == 0 {
// 		return nil, nil
// 	}
// 	return added, nil
// }

// // ExtractCommentsInFunction: same semantics as ExtractFunctionComments (keeps your public API)
// func ExtractCommentsInFunction(commitHash, path, functionName string) ([]string, error) {
// 	return ExtractFunctionComments(commitHash, path, functionName)
// }

// // ------------------ Fallback (line-scan) ------------------
// // minimal fallback used when no Tree-sitter grammar available.
// // Best-effort: scans for functionName, uses addedLines set to return comment lines.
// func extractCommentsViaLineScan(addedLines map[int]bool, newContent, functionName string) ([]string, error) {
// 	if addedLines == nil {
// 		return nil, nil
// 	}
// 	lines := strings.Split(newContent, "\n")
// 	var out []string
// 	for i, line := range lines {
// 		// simple heuristics to find function start line containing functionName
// 		if strings.Contains(line, functionName) && (strings.Contains(line, "func") || strings.Contains(line, "def") || strings.Contains(line, "function") || strings.Contains(line, "=>") || strings.Contains(line, "(")) {
// 			// doc above
// 			for j := i - 1; j >= 0; j-- {
// 				t := strings.TrimSpace(lines[j])
// 				if t == "" {
// 					continue
// 				}
// 				if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "/*") || strings.HasPrefix(t, `"""`) || strings.HasPrefix(t, `'''`) {
// 					if addedLines[j+1] {
// 						out = append([]string{normalizeCommentLine(t)}, out...)
// 					}
// 					continue
// 				}
// 				break
// 			}
// 			// inside: scan forward for added comment lines until closing brace or blank heuristics
// 			for k := i; k < len(lines); k++ {
// 				if addedLines[k+1] {
// 					t := strings.TrimSpace(lines[k])
// 					if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") {
// 						out = append(out, normalizeCommentLine(t))
// 					}
// 				}
// 				// stop heuristics (stop at next top-level blank line or closing brace)
// 				if strings.Contains(lines[k], "}") || strings.Contains(lines[k], "end") {
// 					break
// 				}
// 			}
// 			break
// 		}
// 	}
// 	return uniqueTrimmed(out), nil
// }
