package helpers

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	
	sitter "github.com/smacker/go-tree-sitter"
	tsCSharp "github.com/smacker/go-tree-sitter/csharp"
	tsGo "github.com/smacker/go-tree-sitter/golang"
	tsJava "github.com/smacker/go-tree-sitter/java"
	tsJS "github.com/smacker/go-tree-sitter/javascript"
	tsPHP "github.com/smacker/go-tree-sitter/php"
	tsPython "github.com/smacker/go-tree-sitter/python"
	tsTSX "github.com/smacker/go-tree-sitter/typescript/tsx"
	tsTS "github.com/smacker/go-tree-sitter/typescript/typescript"
)

var treeSitterParsers = map[string]*sitter.Language{
	".go":   tsGo.GetLanguage(),
	".py":   tsPython.GetLanguage(),
	".js":   tsJS.GetLanguage(),
	".ts":   tsTS.GetLanguage(),
	".tsx":  tsTSX.GetLanguage(),
	".cs":   tsCSharp.GetLanguage(),
	".java": tsJava.GetLanguage(),
	".php":  tsPHP.GetLanguage(),
}

func getLanguageForPath(path string) *sitter.Language {
	ext := strings.ToLower(filepath.Ext(path))
	if lang, ok := treeSitterParsers[ext]; ok {
		return lang
	}
	return nil
}

func ExtractFunctions(ext, content string) map[string]string {
	if lang, ok := treeSitterParsers[ext]; ok {
		// fmt.Println("✅ Using Tree-sitter for", ext)
		return extractTreeSitter(lang, content)
	}
	fmt.Println("⚠️ Unhandled extention:", ext, ". Accuracy may reduce")
	return extractGenericFunctionsRegex(content)
}

func extractTreeSitter(lang *sitter.Language, content string) map[string]string {
	parser := sitter.NewParser()
	parser.SetLanguage(lang)

	tree := parser.Parse(nil, []byte(content))
	root := tree.RootNode()

	functions := make(map[string]string)

	var walk func(node *sitter.Node)
	walk = func(node *sitter.Node) {
		// Handle normal function declarations
		if isFunctionNode(node) {
			name := getFunctionName(node, content)

			// Special handling for arrow functions
			if node.Type() == "arrow_function" && name == "" {
				// Get parent variable declarator name
				parent := node.Parent()
				if parent != nil && (parent.Type() == "lexical_declarator" || parent.Type() == "variable_declarator") {
					ident := parent.ChildByFieldName("name")
					if ident != nil {
						name = content[ident.StartByte():ident.EndByte()]
					}
				}
			}

			if name != "" {
				body := content[node.StartByte():node.EndByte()]
				functions[name] = body
			}
		}

		// Recursive walk
		for i := 0; i < int(node.ChildCount()); i++ {
			walk(node.Child(i))
		}
	}

	walk(root)
	return functions
}

// ----------- Fallback Regex Detection --------------
func extractGenericFunctionsRegex(content string) map[string]string {
	result := map[string]string{}
	if strings.TrimSpace(content) == "" {
		return result
	}

	patterns := []string{
		`(?m)^\s*func\s*(?:\([^)]+\)\s*)?([A-Za-z_]\w*)`,
		`(?m)^\s*fn\s+([A-Za-z_]\w*)`,
		`(?m)(?:^\s*@\w+\s*\n\s*)*^\s*def\s+([A-Za-z_]\w*)`,
		`(?ms)^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_]\w*)\s*(?:<[^<>]*>)?\s*\([^)]*\)`,
		`(?ms)^\s*(?:const|let|var)\s+([A-Za-z_]\w*)\s*=\s*(?:async\s+)?\(?[^\)]*\)?\s*=>`,
		`(?m)^\s*(?:public|private|protected|internal|static|async|override|final|open)?\s*([A-Za-z_]\w*)\s*\([^)]*\)\s*\{`,
	}

	for _, p := range patterns {
		r := regexp.MustCompile(p)
		matches := r.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if len(m) > 1 {
				result[m[1]] = m[0]
			}
		}
	}

	return result
}

// Detect function nodes based on language grammar
func isFunctionNode(node *sitter.Node) bool {
	functionNodes := map[string]bool{
		"function_declaration":           true,  // function foo() {}
		"function":                       true,  // function() {}
		"function_expression":            true,  // const foo = function() {}
		"generator_function":             true,  // function* foo() {}
		"generator_function_declaration": true,  // export function* foo() {}
		"arrow_function":                 true,  // () => {}
		"method_definition":              true,  // class A { foo() {} }
		"method_signature":               true,  // TS method signatures
		"public_field_definition":        true,  // foo = () => {} (class fields)
		"lexical_declaration":            false, // used for name extraction
	}
	return functionNodes[node.Type()]
}

// Extract function name from node
func getFunctionName(node *sitter.Node, content string) string {
	// --- Helper: check if inside class field ---
	isInsideClassField := func(n *sitter.Node) bool {
		for p := n.Parent(); p != nil; p = p.Parent() {
			if p.Type() == "public_field_definition" || p.Type() == "field_definition" {
				return true
			}
		}
		return false
	}

	// 1. Normal function declarations: function foo() {}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" && !isInsideClassField(child) {
			return content[child.StartByte():child.EndByte()]
		}
	}

	// 2. Class methods: method_definition name retrieved via field "name"
	if prop := node.ChildByFieldName("name"); prop != nil && !isInsideClassField(prop) {
		return content[prop.StartByte():prop.EndByte()]
	}

	// 3. Variable-assigned functions: const foo = () => {} OR const foo = function() {}
	parent := node.Parent()
	if parent != nil && (parent.Type() == "variable_declarator" || parent.Type() == "lexical_declarator") {
		if nameNode := parent.ChildByFieldName("name"); nameNode != nil {
			return content[nameNode.StartByte():nameNode.EndByte()]
		}
	}

	// 4. Object methods: const obj = { foo() {} }
	if parent != nil && parent.Type() == "pair" {
		if keyNode := parent.ChildByFieldName("key"); keyNode != nil {
			return content[keyNode.StartByte():keyNode.EndByte()]
		}
	}

	// 5. Default: anonymous
	return ""
}
