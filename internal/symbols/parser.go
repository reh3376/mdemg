package symbols

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// Parser extracts symbols from source code files using tree-sitter.
// The tree-sitter C parser is not thread-safe, so all parse calls are serialized via mu.
type Parser struct {
	config      ParserConfig
	tsParser    *sitter.Parser
	mu          sync.Mutex
	languages   map[Language]*sitter.Language
	queryEngine *QueryEngine
}

// NewParser creates a new symbol parser with the given configuration.
func NewParser(config ParserConfig) (*Parser, error) {
	p := &Parser{
		config:    config,
		tsParser:  sitter.NewParser(),
		languages: make(map[Language]*sitter.Language),
	}

	// Initialize language grammars
	p.languages[LangTypeScript] = typescript.GetLanguage()
	p.languages[LangJavaScript] = javascript.GetLanguage()
	p.languages[LangGo] = golang.GetLanguage()
	p.languages[LangPython] = python.GetLanguage()
	p.languages[LangRust] = rust.GetLanguage()
	p.languages[LangC] = c.GetLanguage()
	p.languages[LangCPP] = cpp.GetLanguage()
	p.languages[LangCUDA] = cpp.GetLanguage() // CUDA uses C++ grammar
	p.languages[LangJava] = java.GetLanguage()

	// Initialize query engine for relationship extraction (Phase 75A)
	qe, err := NewQueryEngine(p.languages)
	if err != nil {
		slog.Warn("query engine init failed, relationship extraction disabled", "error", err)
	}
	p.queryEngine = qe

	return p, nil
}

// ParseFile parses a source file and extracts symbols.
func (p *Parser) ParseFile(ctx context.Context, filePath string) (*FileSymbols, error) {
	// Determine language from extension
	ext := filepath.Ext(filePath)
	lang := LanguageFromExtension(ext)

	if lang == LangUnknown {
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}

	// Check if language is enabled
	if len(p.config.Languages) > 0 {
		enabled := false
		for _, l := range p.config.Languages {
			if l == lang {
				enabled = true
				break
			}
		}
		if !enabled {
			return nil, fmt.Errorf("language %s is not enabled", lang)
		}
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return p.ParseContent(ctx, filePath, lang, content)
}

// ParseContent parses source code content and extracts symbols.
func (p *Parser) ParseContent(ctx context.Context, filePath string, lang Language, content []byte) (*FileSymbols, error) {
	result := &FileSymbols{
		FilePath: filePath,
		Language: lang,
		Symbols:  make([]Symbol, 0),
		ParsedAt: time.Now(),
	}

	// Get the tree-sitter language
	tsLang, ok := p.languages[lang]
	if !ok {
		return nil, fmt.Errorf("no grammar loaded for language: %s", lang)
	}

	// Parse the content (tree-sitter is not thread-safe, serialize access)
	p.mu.Lock()
	p.tsParser.SetLanguage(tsLang)
	tree, err := p.tsParser.ParseCtx(ctx, nil, content)
	p.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("failed to parse: %w", err)
	}
	defer tree.Close()

	// Extract symbols based on language
	var symbols []Symbol
	switch lang {
	case LangTypeScript, LangJavaScript:
		symbols = p.extractTypeScriptSymbols(tree.RootNode(), content, filePath)
	case LangGo:
		symbols = p.extractGoSymbols(tree.RootNode(), content, filePath)
	case LangPython:
		symbols = p.extractPythonSymbols(tree.RootNode(), content, filePath)
	case LangRust:
		symbols = p.extractRustSymbols(tree.RootNode(), content, filePath)
	case LangC:
		symbols = p.extractCSymbols(tree.RootNode(), content, filePath)
	case LangCPP, LangCUDA:
		symbols = p.extractCPPSymbols(tree.RootNode(), content, filePath)
	case LangJava:
		symbols = p.extractJavaSymbols(tree.RootNode(), content, filePath)
	default:
		return nil, fmt.Errorf("extraction not implemented for: %s", lang)
	}

	// Apply filters
	for _, sym := range symbols {
		// Skip short names
		if len(sym.Name) < p.config.MinNameLength {
			continue
		}

		// Skip private symbols unless configured
		if !p.config.IncludePrivate && !sym.Exported {
			continue
		}

		// Add language
		sym.Language = lang
		result.Symbols = append(result.Symbols, sym)

		// Enforce max symbols limit (0 means unlimited)
		if p.config.MaxSymbolsPerFile > 0 && len(result.Symbols) >= p.config.MaxSymbolsPerFile {
			result.ParseErrors = append(result.ParseErrors,
				fmt.Sprintf("truncated at %d symbols (max limit)", p.config.MaxSymbolsPerFile))
			break
		}
	}

	// Extract relationships using tree-sitter queries (Phase 75A)
	if p.queryEngine != nil {
		result.Relationships = p.queryEngine.ExtractRelationships(lang, tree.RootNode(), content, filePath)
	}

	return result, nil
}

// extractTypeScriptSymbols extracts symbols from TypeScript/JavaScript AST.
func (p *Parser) extractTypeScriptSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	// Walk the AST
	p.walkTree(root, func(node *sitter.Node) bool {
		nodeType := node.Type()

		switch nodeType {
		case "lexical_declaration", "variable_declaration":
			// const X = ... or let X = ... or var X = ...
			symbols = append(symbols, p.extractTSVariableDeclaration(node, content, lines, filePath)...)

		case "function_declaration", "function":
			// function X() { }
			if sym := p.extractTSFunction(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "class_declaration", "abstract_class_declaration":
			// class X { } or abstract class X { } - extract class and its methods
			if sym := p.extractTSClass(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
			// Extract methods from the class
			symbols = append(symbols, p.extractTSClassMethods(node, content, lines, filePath)...)

		case "interface_declaration":
			// interface X { }
			if sym := p.extractTSInterface(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "type_alias_declaration":
			// type X = ...
			if sym := p.extractTSTypeAlias(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "enum_declaration":
			// enum X { }
			symbols = append(symbols, p.extractTSEnum(node, content, lines, filePath)...)

		case "public_field_definition":
			// Class static/instance fields: static DEFAULT_VALUE = 42
			if sym := p.extractTSClassField(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "export_statement":
			// Check for export default or export { }
			// Child nodes will be processed recursively
			return true
		}

		return true // continue walking
	})

	return symbols
}

// extractTSVariableDeclaration extracts const/let/var declarations.
// Detects arrow functions assigned to const as functions.
func (p *Parser) extractTSVariableDeclaration(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	// Determine if const, let, or var
	declKind := SymbolTypeVar
	isConst := false
	if node.Type() == "lexical_declaration" {
		kindNode := node.ChildByFieldName("kind")
		if kindNode != nil && kindNode.Content(content) == "const" {
			declKind = SymbolTypeConst
			isConst = true
		}
	}

	// Check if exported (parent is export_statement)
	exported := p.isExported(node)

	// Find variable declarators
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "variable_declarator" {
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")

			if nameNode == nil {
				continue
			}

			name := nameNode.Content(content)
			symType := declKind

			// Check if value is an arrow function
			if valueNode != nil && isConst {
				valueType := valueNode.Type()
				if valueType == "arrow_function" || valueType == "function" {
					symType = SymbolTypeFunction
				}
			}

			sym := Symbol{
				Name:       name,
				Type:       symType,
				FilePath:   filePath,
				Line:       int(nameNode.StartPoint().Row) + 1,
				LineEnd:    int(node.EndPoint().Row) + 1,
				Column:     int(nameNode.StartPoint().Column),
				Exported:   exported,
			}

			if valueNode != nil {
				sym.RawValue = valueNode.Content(content)
				sym.Value = p.evaluateValue(sym.RawValue)

				// Extract signature from arrow function (parameters + return type)
				if symType == SymbolTypeFunction {
					paramsNode := valueNode.ChildByFieldName("parameters")
					returnNode := valueNode.ChildByFieldName("return_type")
					if paramsNode != nil {
						sym.Signature = paramsNode.Content(content)
						if returnNode != nil {
							sym.Signature += returnNode.Content(content)
							sym.TypeAnnotation = returnNode.Content(content)
						}
					}
				}
			}

			// Extract type annotation
			typeNode := child.ChildByFieldName("type")
			if typeNode != nil {
				sym.TypeAnnotation = typeNode.Content(content)
			}

			// Extract doc comment
			if p.config.IncludeDocComments {
				sym.DocComment = p.extractPrecedingComment(node, content)
			}

			// Add snippet
			sym.Snippet = p.extractSnippet(lines, sym.Line, sym.LineEnd)

			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// extractTSFunction extracts function declarations.
func (p *Parser) extractTSFunction(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := p.isExported(node)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeFunction,
		FilePath:   filePath,
		Line: int(nameNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	// Extract signature (parameters + return type)
	paramsNode := node.ChildByFieldName("parameters")
	returnTypeNode := node.ChildByFieldName("return_type")

	if paramsNode != nil {
		sym.Signature = paramsNode.Content(content)
		if returnTypeNode != nil {
			sym.Signature += returnTypeNode.Content(content)
			sym.TypeAnnotation = returnTypeNode.Content(content)
		}
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.Line, sym.LineEnd)

	return &sym
}

// extractTSClass extracts class declarations.
func (p *Parser) extractTSClass(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := p.isExported(node)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeClass,
		FilePath:   filePath,
		Line:       int(nameNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.Line, sym.Line+5) // First few lines

	return &sym
}

// extractTSClassMethods extracts methods from a class declaration.
func (p *Parser) extractTSClassMethods(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return symbols
	}

	className := nameNode.Content(content)
	classExported := p.isExported(node)

	// Find class body
	bodyNode := node.ChildByFieldName("body")
	if bodyNode == nil {
		return symbols
	}

	// Walk body looking for method definitions
	for i := 0; i < int(bodyNode.NamedChildCount()); i++ {
		member := bodyNode.NamedChild(i)
		memberType := member.Type()

		// Handle method_definition
		if memberType == "method_definition" {
			if sym := p.extractTSMethod(member, content, lines, filePath, className, classExported); sym != nil {
				symbols = append(symbols, *sym)
			}
		}
	}

	return symbols
}

// extractTSMethod extracts a single method from a class.
func (p *Parser) extractTSMethod(node *sitter.Node, content []byte, lines []string, filePath string, className string, classExported bool) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)

	// Skip private methods (start with #)
	if strings.HasPrefix(name, "#") {
		return nil
	}

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeMethod,
		Parent:     className,
		FilePath:   filePath,
		Line:       int(nameNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   classExported, // Methods inherit class export status
	}

	// Extract signature (parameters + return type)
	paramsNode := node.ChildByFieldName("parameters")
	returnNode := node.ChildByFieldName("return_type")

	if paramsNode != nil {
		sym.Signature = paramsNode.Content(content)
		if returnNode != nil {
			sym.Signature += returnNode.Content(content)
			sym.TypeAnnotation = returnNode.Content(content)
		}
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.Line, sym.Line+5)

	return &sym
}

// extractTSClassField extracts class static/readonly fields as constants.
// Handles patterns like: static DEFAULT_TIMEOUT = 1000
func (p *Parser) extractTSClassField(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	// Check for static modifier - only extract static fields as constants
	hasStatic := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "static" {
			hasStatic = true
			break
		}
	}

	// Only extract static fields (they're effectively constants)
	if !hasStatic {
		return nil
	}

	// Get field name - look for property_identifier child
	var nameNode *sitter.Node
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "property_identifier" {
			nameNode = child
			break
		}
	}
	if nameNode == nil {
		return nil
	}
	name := nameNode.Content(content)

	// Skip private fields (usually start with _ or #)
	if strings.HasPrefix(name, "#") {
		return nil
	}

	// Get value - find the value after the = sign
	var value string
	foundEquals := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "=" {
			foundEquals = true
			continue
		}
		if foundEquals && child.Type() != ";" {
			value = child.Content(content)
			// Evaluate simple numeric expressions
			value = p.evaluateValue(value)
			break
		}
	}

	// Get the class name for context (parent is class_body, grandparent is class_declaration)
	// Also check if the class is exported - if so, the static field is effectively exported
	var className string
	var classExported bool
	if parent := node.Parent(); parent != nil && parent.Type() == "class_body" {
		if grandparent := parent.Parent(); grandparent != nil {
			if classNameNode := grandparent.ChildByFieldName("name"); classNameNode != nil {
				className = classNameNode.Content(content)
			}
			classExported = p.isExported(grandparent)
		}
	}

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeConst, // Treat static fields as constants
		FilePath:   filePath,
		Line: int(nameNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   classExported, // Inherit export status from class
		Value:      value,
		RawValue:   value,
	}

	// Add class context to snippet
	if className != "" {
		sym.Snippet = className + "." + name
		if value != "" {
			sym.Snippet += " = " + value
		}
	} else {
		sym.Snippet = p.extractSnippet(lines, sym.Line, sym.Line)
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	return &sym
}

// extractTSInterface extracts interface declarations.
func (p *Parser) extractTSInterface(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := p.isExported(node)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeInterface,
		FilePath:   filePath,
		Line: int(nameNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.Line, sym.Line+5)

	return &sym
}

// extractTSTypeAlias extracts type alias declarations.
func (p *Parser) extractTSTypeAlias(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := p.isExported(node)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeType,
		FilePath:   filePath,
		Line: int(nameNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	// Get the type value
	valueNode := node.ChildByFieldName("value")
	if valueNode != nil {
		sym.RawValue = valueNode.Content(content)
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.Line, sym.LineEnd)

	return &sym
}

// extractTSEnum extracts enum declarations and their values.
func (p *Parser) extractTSEnum(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return symbols
	}

	enumName := nameNode.Content(content)
	exported := p.isExported(node)

	// Add the enum itself
	enumSym := Symbol{
		Name:       enumName,
		Type:       SymbolTypeEnum,
		FilePath:   filePath,
		Line: int(nameNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	if p.config.IncludeDocComments {
		enumSym.DocComment = p.extractPrecedingComment(node, content)
	}

	enumSym.Snippet = p.extractSnippet(lines, enumSym.Line, enumSym.LineEnd)
	symbols = append(symbols, enumSym)

	// Extract enum members
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		for i := 0; i < int(bodyNode.NamedChildCount()); i++ {
			member := bodyNode.NamedChild(i)
			if member.Type() == "enum_assignment" || member.Type() == "property_identifier" {
				var memberName, memberValue string

				if member.Type() == "enum_assignment" {
					memberNameNode := member.ChildByFieldName("name")
					memberValueNode := member.ChildByFieldName("value")
					if memberNameNode != nil {
						memberName = memberNameNode.Content(content)
					}
					if memberValueNode != nil {
						memberValue = memberValueNode.Content(content)
					}
				} else {
					memberName = member.Content(content)
				}

				if memberName != "" {
					memberSym := Symbol{
						Name:       enumName + "." + memberName,
						Type:       SymbolTypeEnumValue,
						Value:      memberValue,
						FilePath:   filePath,
						Line: int(member.StartPoint().Row) + 1,
						LineEnd:    int(member.EndPoint().Row) + 1,
						Column:     int(member.StartPoint().Column),
						Exported:   exported,
						Parent:     enumName,
					}
					symbols = append(symbols, memberSym)
				}
			}
		}
	}

	return symbols
}

// extractGoSymbols extracts symbols from Go AST.
func (p *Parser) extractGoSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	p.walkTree(root, func(node *sitter.Node) bool {
		nodeType := node.Type()

		switch nodeType {
		case "const_declaration":
			symbols = append(symbols, p.extractGoConst(node, content, lines, filePath)...)

		case "var_declaration":
			symbols = append(symbols, p.extractGoVar(node, content, lines, filePath)...)

		case "function_declaration":
			if sym := p.extractGoFunction(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "method_declaration":
			if sym := p.extractGoMethod(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "type_declaration":
			symbols = append(symbols, p.extractGoType(node, content, lines, filePath)...)
		}

		return true
	})

	return symbols
}

// extractGoConst extracts Go const declarations.
func (p *Parser) extractGoConst(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "const_spec" {
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")

			if nameNode == nil {
				continue
			}

			name := nameNode.Content(content)
			exported := isGoExported(name)

			sym := Symbol{
				Name:       name,
				Type:       SymbolTypeConst,
				FilePath:   filePath,
				Line: int(nameNode.StartPoint().Row) + 1,
				LineEnd:    int(child.EndPoint().Row) + 1,
				Column:     int(nameNode.StartPoint().Column),
				Exported:   exported,
			}

			if valueNode != nil {
				sym.RawValue = valueNode.Content(content)
				sym.Value = p.evaluateValue(sym.RawValue)
			}

			// Type annotation
			typeNode := child.ChildByFieldName("type")
			if typeNode != nil {
				sym.TypeAnnotation = typeNode.Content(content)
			}

			if p.config.IncludeDocComments {
				sym.DocComment = p.extractPrecedingComment(node, content)
			}

			sym.Snippet = p.extractSnippet(lines, sym.Line, sym.LineEnd)
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// extractGoVar extracts Go var declarations.
func (p *Parser) extractGoVar(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "var_spec" {
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")

			if nameNode == nil {
				continue
			}

			name := nameNode.Content(content)
			exported := isGoExported(name)

			sym := Symbol{
				Name:       name,
				Type:       SymbolTypeVar,
				FilePath:   filePath,
				Line: int(nameNode.StartPoint().Row) + 1,
				LineEnd:    int(child.EndPoint().Row) + 1,
				Column:     int(nameNode.StartPoint().Column),
				Exported:   exported,
			}

			if valueNode != nil {
				sym.RawValue = valueNode.Content(content)
				sym.Value = p.evaluateValue(sym.RawValue)
			}

			typeNode := child.ChildByFieldName("type")
			if typeNode != nil {
				sym.TypeAnnotation = typeNode.Content(content)
			}

			if p.config.IncludeDocComments {
				sym.DocComment = p.extractPrecedingComment(node, content)
			}

			sym.Snippet = p.extractSnippet(lines, sym.Line, sym.LineEnd)
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// extractGoFunction extracts Go function declarations.
func (p *Parser) extractGoFunction(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := isGoExported(name)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeFunction,
		FilePath:   filePath,
		Line: int(nameNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	// Extract signature (parameters + return type)
	paramsNode := node.ChildByFieldName("parameters")
	resultNode := node.ChildByFieldName("result")

	if paramsNode != nil {
		sym.Signature = paramsNode.Content(content)
		if resultNode != nil {
			sym.Signature += " " + resultNode.Content(content)
		}
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.Line, sym.Line+5)

	return &sym
}

// extractGoMethod extracts Go method declarations.
func (p *Parser) extractGoMethod(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := isGoExported(name)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeMethod,
		FilePath:   filePath,
		Line: int(nameNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	// Extract receiver type as parent
	receiverNode := node.ChildByFieldName("receiver")
	if receiverNode != nil {
		// Find the type identifier in the receiver
		for i := 0; i < int(receiverNode.NamedChildCount()); i++ {
			param := receiverNode.NamedChild(i)
			typeNode := param.ChildByFieldName("type")
			if typeNode != nil {
				typeName := typeNode.Content(content)
				// Remove pointer prefix if present
				typeName = strings.TrimPrefix(typeName, "*")
				sym.Parent = typeName
				break
			}
		}
	}

	paramsNode := node.ChildByFieldName("parameters")
	resultNode := node.ChildByFieldName("result")

	if paramsNode != nil {
		sym.Signature = paramsNode.Content(content)
		if resultNode != nil {
			sym.Signature += " " + resultNode.Content(content)
		}
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.Line, sym.Line+5)

	return &sym
}

// extractGoType extracts Go type declarations (struct, interface, type alias).
func (p *Parser) extractGoType(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "type_spec" {
			nameNode := child.ChildByFieldName("name")
			typeNode := child.ChildByFieldName("type")

			if nameNode == nil {
				continue
			}

			name := nameNode.Content(content)
			exported := isGoExported(name)

			sym := Symbol{
				Name:       name,
				FilePath:   filePath,
				Line: int(nameNode.StartPoint().Row) + 1,
				LineEnd:    int(child.EndPoint().Row) + 1,
				Column:     int(nameNode.StartPoint().Column),
				Exported:   exported,
			}

			// Determine type kind
			if typeNode != nil {
				switch typeNode.Type() {
				case "struct_type":
					sym.Type = SymbolTypeStruct
				case "interface_type":
					sym.Type = SymbolTypeInterface
				default:
					sym.Type = SymbolTypeType
					sym.RawValue = typeNode.Content(content)
				}
			}

			if p.config.IncludeDocComments {
				sym.DocComment = p.extractPrecedingComment(node, content)
			}

			sym.Snippet = p.extractSnippet(lines, sym.Line, sym.Line+10)
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// extractPythonSymbols extracts symbols from Python AST.
func (p *Parser) extractPythonSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	// Track class context for method detection
	var currentClass string
	var classEndLine int

	p.walkTree(root, func(node *sitter.Node) bool {
		nodeType := node.Type()

		// Update class context
		if currentClass != "" {
			nodeLine := int(node.StartPoint().Row) + 1
			if nodeLine > classEndLine {
				currentClass = ""
			}
		}

		switch nodeType {
		case "assignment":
			// CONSTANT = value or TypeAlias = Type
			symbols = append(symbols, p.extractPythonAssignment(node, content, lines, filePath)...)

		case "function_definition":
			if sym := p.extractPythonFunction(node, content, lines, filePath, currentClass); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "class_definition":
			if sym := p.extractPythonClass(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
				// Set class context for method detection
				currentClass = sym.Name
				classEndLine = sym.LineEnd
			}
		}

		return true
	})

	return symbols
}

// extractPythonAssignment extracts Python assignments (constants, type aliases, variables).
func (p *Parser) extractPythonAssignment(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	// Get the left side (name)
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")

	if leftNode == nil || leftNode.Type() != "identifier" {
		return symbols
	}

	name := leftNode.Content(content)
	exported := !strings.HasPrefix(name, "_")

	// Determine symbol type
	symType := SymbolTypeVar

	// Check if it's a constant (UPPER_CASE convention)
	if isUpperCase(name) {
		symType = SymbolTypeConst
	} else if rightNode != nil {
		// Check if it's a type alias (CamelCase = type expression)
		rightContent := rightNode.Content(content)
		if isPythonTypeAlias(name, rightContent) {
			symType = SymbolTypeType
		}
	}

	sym := Symbol{
		Name:       name,
		Type:       symType,
		FilePath:   filePath,
		Line:       int(leftNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(leftNode.StartPoint().Column),
		Exported:   exported,
	}

	if rightNode != nil {
		sym.RawValue = rightNode.Content(content)
		sym.Value = p.evaluateValue(sym.RawValue)
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.Line, sym.LineEnd)
	symbols = append(symbols, sym)

	return symbols
}

// isPythonTypeAlias checks if an assignment looks like a type alias.
// Type aliases are typically: CamelCase = str|int|List[...]|Dict[...]|Optional[...]|Union[...]
func isPythonTypeAlias(name, value string) bool {
	// Name must be CamelCase (starts with uppercase, has lowercase)
	if len(name) == 0 {
		return false
	}
	first := rune(name[0])
	if first < 'A' || first > 'Z' {
		return false
	}
	hasLower := false
	for _, r := range name[1:] {
		if r >= 'a' && r <= 'z' {
			hasLower = true
			break
		}
	}
	if !hasLower {
		return false // All caps = constant, not type alias
	}

	// Value must look like a type expression
	value = strings.TrimSpace(value)
	typePatterns := []string{
		"str", "int", "float", "bool", "bytes", "None",
		"List[", "Dict[", "Set[", "Tuple[", "Optional[", "Union[",
		"Callable[", "Sequence[", "Mapping[", "Iterable[", "Iterator[",
		"Type[", "Any", "TypeVar(",
	}
	for _, pattern := range typePatterns {
		if strings.HasPrefix(value, pattern) || value == pattern {
			return true
		}
	}

	return false
}

// extractPythonFunction extracts Python function/method definitions.
func (p *Parser) extractPythonFunction(node *sitter.Node, content []byte, lines []string, filePath string, currentClass string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := !strings.HasPrefix(name, "_")

	// Determine if this is a method (inside a class) or function
	symType := SymbolTypeFunction
	parent := ""
	if currentClass != "" {
		symType = SymbolTypeMethod
		parent = currentClass
	}

	sym := Symbol{
		Name:       name,
		Type:       symType,
		Parent:     parent,
		FilePath:   filePath,
		Line:       int(nameNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	// Extract signature (parameters + return type)
	paramsNode := node.ChildByFieldName("parameters")
	returnTypeNode := node.ChildByFieldName("return_type")

	if paramsNode != nil {
		sym.Signature = paramsNode.Content(content)
		if returnTypeNode != nil {
			sym.Signature += " -> " + returnTypeNode.Content(content)
			sym.TypeAnnotation = returnTypeNode.Content(content)
		}
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPythonDocstring(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.Line, sym.Line+5)

	return &sym
}

// extractPythonClass extracts Python class definitions.
func (p *Parser) extractPythonClass(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := !strings.HasPrefix(name, "_")

	// Determine symbol type based on base classes
	symType := SymbolTypeClass
	var parent string

	// Check for base classes (superclasses)
	superclassNode := node.ChildByFieldName("superclasses")
	if superclassNode != nil {
		bases := superclassNode.Content(content)
		if strings.Contains(bases, "Enum") || strings.Contains(bases, "IntEnum") || strings.Contains(bases, "StrEnum") {
			symType = SymbolTypeEnum
			parent = "Enum"
		} else if strings.Contains(bases, "Protocol") {
			symType = SymbolTypeInterface
			parent = "Protocol"
		} else {
			// Store base class for inheritance
			// Strip parentheses and take first base class
			parent = strings.TrimPrefix(bases, "(")
			parent = strings.TrimSuffix(parent, ")")
			// Handle multiple base classes - take first one
			if idx := strings.Index(parent, ","); idx != -1 {
				parent = strings.TrimSpace(parent[:idx])
			}
		}
	}

	sym := Symbol{
		Name:       name,
		Type:       symType,
		Parent:     parent,
		FilePath:   filePath,
		Line:       int(nameNode.StartPoint().Row) + 1,
		LineEnd:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPythonDocstring(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.Line, sym.Line+10)

	return &sym
}

// Helper functions

// walkTree walks the AST calling fn for each node.
func (p *Parser) walkTree(node *sitter.Node, fn func(*sitter.Node) bool) {
	if !fn(node) {
		return
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		p.walkTree(node.Child(i), fn)
	}
}

// isExported checks if a TypeScript/JavaScript node is exported.
func (p *Parser) isExported(node *sitter.Node) bool {
	// Check if parent is export_statement
	parent := node.Parent()
	if parent != nil && parent.Type() == "export_statement" {
		return true
	}

	// Check for export keyword in declaration
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "export" {
			return true
		}
	}

	return false
}

// isGoExported checks if a Go identifier is exported (starts with uppercase).
func isGoExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	first := rune(name[0])
	return first >= 'A' && first <= 'Z'
}

// isUpperCase checks if a string is all uppercase (Python constant convention).
func isUpperCase(s string) bool {
	hasLetter := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return false
		}
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
		}
	}
	return hasLetter
}

// extractPrecedingComment extracts comment above a node.
func (p *Parser) extractPrecedingComment(node *sitter.Node, content []byte) string {
	// Look for comment node before this one
	if node.PrevSibling() != nil {
		prev := node.PrevSibling()
		if prev.Type() == "comment" {
			return strings.TrimSpace(prev.Content(content))
		}
	}
	return ""
}

// extractPythonDocstring extracts docstring from Python function/class.
func (p *Parser) extractPythonDocstring(node *sitter.Node, content []byte) string {
	// Look for expression_statement with string as first child in body
	bodyNode := node.ChildByFieldName("body")
	if bodyNode == nil {
		return ""
	}

	if bodyNode.NamedChildCount() > 0 {
		first := bodyNode.NamedChild(0)
		if first.Type() == "expression_statement" && first.NamedChildCount() > 0 {
			expr := first.NamedChild(0)
			if expr.Type() == "string" {
				docstring := expr.Content(content)
				// Remove quotes
				docstring = strings.Trim(docstring, "\"'")
				docstring = strings.TrimPrefix(docstring, "\"\"")
				docstring = strings.TrimSuffix(docstring, "\"\"")
				return strings.TrimSpace(docstring)
			}
		}
	}

	return ""
}

// extractSnippet extracts source lines with context.
func (p *Parser) extractSnippet(lines []string, startLine, endLine int) string {
	// Convert to 0-indexed
	start := startLine - 1 - p.config.ContextLines
	end := endLine - 1 + p.config.ContextLines

	if start < 0 {
		start = 0
	}
	if end >= len(lines) {
		end = len(lines) - 1
	}

	// Limit snippet size
	maxLines := 15
	if end-start > maxLines {
		end = start + maxLines
	}

	return strings.Join(lines[start:end+1], "\n")
}

// evaluateValue attempts to evaluate simple constant expressions.
func (p *Parser) evaluateValue(raw string) string {
	if !p.config.EvaluateConstants {
		return raw
	}

	// Try to evaluate simple arithmetic
	raw = strings.TrimSpace(raw)

	// Handle simple multiplication like "60 * 1000"
	if strings.Contains(raw, "*") {
		parts := strings.Split(raw, "*")
		if len(parts) == 2 {
			a := parseNumber(strings.TrimSpace(parts[0]))
			b := parseNumber(strings.TrimSpace(parts[1]))
			if a != 0 && b != 0 {
				return fmt.Sprintf("%d", a*b)
			}
		}
	}

	// Handle simple addition
	if strings.Contains(raw, "+") && !strings.Contains(raw, "\"") {
		parts := strings.Split(raw, "+")
		if len(parts) == 2 {
			a := parseNumber(strings.TrimSpace(parts[0]))
			b := parseNumber(strings.TrimSpace(parts[1]))
			if a != 0 || b != 0 {
				return fmt.Sprintf("%d", a+b)
			}
		}
	}

	return raw
}

// parseNumber parses a simple integer from string.
func parseNumber(s string) int64 {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0
	}
	return n
}

// Close releases parser resources.
func (p *Parser) Close() {
	if p.tsParser != nil {
		p.tsParser.Close()
	}
}

// ParseDirectory parses all supported files in a directory.
func (p *Parser) ParseDirectory(ctx context.Context, dir string) ([]*FileSymbols, error) {
	var results []*FileSymbols
	var parseErrors []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// Skip common non-code directories
			name := info.Name()
			if name == "node_modules" || name == ".git" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file is supported
		ext := filepath.Ext(path)
		lang := LanguageFromExtension(ext)
		if !lang.IsSupported() {
			return nil
		}

		// Parse file
		relPath, _ := filepath.Rel(dir, path)
		fileSymbols, err := p.ParseFile(ctx, path)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("%s: %v", relPath, err))
			slog.Warn("failed to parse file", "path", relPath, "error", err)
			return nil // Continue with other files
		}

		// Update path to relative
		fileSymbols.FilePath = relPath
		for i := range fileSymbols.Symbols {
			fileSymbols.Symbols[i].FilePath = relPath
		}

		results = append(results, fileSymbols)
		return nil
	})

	if err != nil {
		return results, fmt.Errorf("directory walk failed: %w", err)
	}

	return results, nil
}

// extractRustSymbols extracts symbols from Rust AST.
func (p *Parser) extractRustSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol

	p.walkTree(root, func(node *sitter.Node) bool {
		nodeType := node.Type()

		switch nodeType {
		case "const_item", "static_item":
			if sym := p.extractRustConst(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "function_item":
			if sym := p.extractRustFunction(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "struct_item":
			if sym := p.extractRustStruct(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "enum_item":
			if sym := p.extractRustEnum(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "trait_item":
			if sym := p.extractRustTrait(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "impl_item":
			symbols = append(symbols, p.extractRustImplMethods(node, content, filePath)...)
		case "type_item":
			if sym := p.extractRustTypeAlias(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "mod_item":
			if sym := p.extractRustModule(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "macro_definition":
			if sym := p.extractRustMacro(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		}
		return true
	})

	return symbols
}

func (p *Parser) extractRustConst(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	exported := p.hasRustVisibility(node)
	sym := Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeConst,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: exported,
	}
	if valueNode := node.ChildByFieldName("value"); valueNode != nil {
		sym.RawValue = valueNode.Content(content)
		sym.Value = p.evaluateValue(sym.RawValue)
	}
	return &sym
}

func (p *Parser) extractRustFunction(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	sym := Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeFunction,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: p.hasRustVisibility(node),
	}
	if paramsNode := node.ChildByFieldName("parameters"); paramsNode != nil {
		sym.Signature = paramsNode.Content(content)
		if returnNode := node.ChildByFieldName("return_type"); returnNode != nil {
			sym.Signature += " -> " + returnNode.Content(content)
		}
	}
	return &sym
}

func (p *Parser) extractRustStruct(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeStruct,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: p.hasRustVisibility(node),
	}
}

func (p *Parser) extractRustEnum(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeEnum,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: p.hasRustVisibility(node),
	}
}

func (p *Parser) extractRustTrait(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeTrait,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: p.hasRustVisibility(node),
	}
}

func (p *Parser) extractRustImplMethods(node *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol
	var parentName string
	if typeNode := node.ChildByFieldName("type"); typeNode != nil {
		parentName = typeNode.Content(content)
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "declaration_list" {
			for j := 0; j < int(child.NamedChildCount()); j++ {
				item := child.NamedChild(j)
				if item.Type() == "function_item" {
					if sym := p.extractRustFunction(item, content, filePath); sym != nil {
						sym.Type = SymbolTypeMethod
						sym.Parent = parentName
						symbols = append(symbols, *sym)
					}
				}
			}
		}
	}
	return symbols
}

func (p *Parser) extractRustTypeAlias(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeType,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		Exported: p.hasRustVisibility(node),
	}
}

func (p *Parser) extractRustModule(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeModule,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		Exported: p.hasRustVisibility(node),
	}
}

func (p *Parser) extractRustMacro(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeMacro,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		Exported: true,
	}
}

func (p *Parser) hasRustVisibility(node *sitter.Node) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		if node.Child(i).Type() == "visibility_modifier" {
			return true
		}
	}
	return false
}

// extractCSymbols extracts symbols from C AST.
func (p *Parser) extractCSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol

	p.walkTree(root, func(node *sitter.Node) bool {
		nodeType := node.Type()

		switch nodeType {
		case "preproc_def":
			if sym := p.extractCMacro(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "preproc_function_def":
			if sym := p.extractCFunctionMacro(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "function_definition":
			if sym := p.extractCFunction(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "declaration":
			symbols = append(symbols, p.extractCDeclaration(node, content, filePath)...)
		case "struct_specifier":
			if sym := p.extractCStruct(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "enum_specifier":
			if sym := p.extractCEnum(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "type_definition":
			if sym := p.extractCTypedef(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		}
		return true
	})

	return symbols
}

func (p *Parser) extractCMacro(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	sym := Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeMacro,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		Exported: true,
	}
	if valueNode := node.ChildByFieldName("value"); valueNode != nil {
		sym.RawValue = valueNode.Content(content)
		sym.Value = p.evaluateValue(sym.RawValue)
	}
	return &sym
}

func (p *Parser) extractCFunctionMacro(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeMacro,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		Exported: true,
	}
}

func (p *Parser) extractCFunction(node *sitter.Node, content []byte, filePath string) *Symbol {
	declNode := node.ChildByFieldName("declarator")
	if declNode == nil {
		return nil
	}
	var name string
	p.walkTree(declNode, func(n *sitter.Node) bool {
		if n.Type() == "identifier" {
			name = n.Content(content)
			return false
		}
		return true
	})
	if name == "" {
		return nil
	}
	exported := true
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "storage_class_specifier" && child.Content(content) == "static" {
			exported = false
			break
		}
	}
	return &Symbol{
		Name:     name,
		Type:     SymbolTypeFunction,
		FilePath: filePath,
		Line:     int(node.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: exported,
	}
}

func (p *Parser) extractCDeclaration(node *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol
	declNode := node.ChildByFieldName("declarator")
	if declNode != nil && declNode.Type() == "function_declarator" {
		var name string
		p.walkTree(declNode, func(n *sitter.Node) bool {
			if n.Type() == "identifier" {
				name = n.Content(content)
				return false
			}
			return true
		})
		if name != "" {
			symbols = append(symbols, Symbol{
				Name:     name,
				Type:     SymbolTypeFunction,
				FilePath: filePath,
				Line:     int(node.StartPoint().Row) + 1,
				Exported: true,
			})
		}
	}
	return symbols
}

func (p *Parser) extractCStruct(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeStruct,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: true,
	}
}

func (p *Parser) extractCEnum(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeEnum,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: true,
	}
}

func (p *Parser) extractCTypedef(node *sitter.Node, content []byte, filePath string) *Symbol {
	declNode := node.ChildByFieldName("declarator")
	if declNode == nil {
		return nil
	}
	var name string
	if declNode.Type() == "type_identifier" {
		name = declNode.Content(content)
	} else {
		p.walkTree(declNode, func(n *sitter.Node) bool {
			if n.Type() == "type_identifier" {
				name = n.Content(content)
				return false
			}
			return true
		})
	}
	if name == "" {
		return nil
	}
	return &Symbol{
		Name:     name,
		Type:     SymbolTypeType,
		FilePath: filePath,
		Line:     int(node.StartPoint().Row) + 1,
		Exported: true,
	}
}

// extractCPPSymbols extracts symbols from C++ AST (also handles CUDA).
func (p *Parser) extractCPPSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol
	var currentClass string

	p.walkTree(root, func(node *sitter.Node) bool {
		nodeType := node.Type()

		switch nodeType {
		case "preproc_def":
			if sym := p.extractCMacro(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "function_definition":
			if sym := p.extractCPPFunction(node, content, filePath, currentClass); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "class_specifier":
			if sym := p.extractCPPClass(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
				oldClass := currentClass
				currentClass = sym.Name
				symbols = append(symbols, p.extractCPPClassMethods(node, content, filePath, sym.Name)...)
				currentClass = oldClass
			}
		case "struct_specifier":
			if sym := p.extractCStruct(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "enum_specifier":
			if sym := p.extractCEnum(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "namespace_definition":
			if sym := p.extractCPPNamespace(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "alias_declaration", "type_definition":
			if sym := p.extractCPPTypeAlias(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "declaration":
			symbols = append(symbols, p.extractCPPDeclaration(node, content, filePath)...)
		}
		return true
	})

	return symbols
}

func (p *Parser) extractCPPClass(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeClass,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: true,
	}
}

func (p *Parser) extractCPPFunction(node *sitter.Node, content []byte, filePath string, currentClass string) *Symbol {
	declNode := node.ChildByFieldName("declarator")
	if declNode == nil {
		return nil
	}
	var name string
	p.walkTree(declNode, func(n *sitter.Node) bool {
		if n.Type() == "identifier" || n.Type() == "field_identifier" {
			name = n.Content(content)
			return false
		}
		return true
	})
	if name == "" {
		return nil
	}
	symType := SymbolTypeFunction
	if currentClass != "" {
		symType = SymbolTypeMethod
	}
	return &Symbol{
		Name:     name,
		Type:     symType,
		Parent:   currentClass,
		FilePath: filePath,
		Line:     int(node.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: true,
	}
}

func (p *Parser) extractCPPClassMethods(node *sitter.Node, content []byte, filePath string, className string) []Symbol {
	var symbols []Symbol
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "field_declaration_list" {
			for j := 0; j < int(child.NamedChildCount()); j++ {
				item := child.NamedChild(j)
				switch item.Type() {
				case "function_definition":
					if sym := p.extractCPPFunction(item, content, filePath, className); sym != nil {
						sym.Type = SymbolTypeMethod
						sym.Parent = className
						symbols = append(symbols, *sym)
					}
				case "declaration":
					declNode := item.ChildByFieldName("declarator")
					if declNode != nil && declNode.Type() == "function_declarator" {
						var name string
						p.walkTree(declNode, func(n *sitter.Node) bool {
							if n.Type() == "identifier" || n.Type() == "field_identifier" {
								name = n.Content(content)
								return false
							}
							return true
						})
						if name != "" {
							symbols = append(symbols, Symbol{
								Name:     name,
								Type:     SymbolTypeMethod,
								Parent:   className,
								FilePath: filePath,
								Line:     int(item.StartPoint().Row) + 1,
								Exported: true,
							})
						}
					}
				}
			}
		}
	}
	return symbols
}

func (p *Parser) extractCPPNamespace(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeNamespace,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		Exported: true,
	}
}

func (p *Parser) extractCPPTypeAlias(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		return &Symbol{
			Name:     nameNode.Content(content),
			Type:     SymbolTypeType,
			FilePath: filePath,
			Line:     int(nameNode.StartPoint().Row) + 1,
			Exported: true,
		}
	}
	return p.extractCTypedef(node, content, filePath)
}

func (p *Parser) extractCPPDeclaration(node *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol
	isConstexpr := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_qualifier" && child.Content(content) == "constexpr" {
			isConstexpr = true
			break
		}
	}
	if isConstexpr {
		declNode := node.ChildByFieldName("declarator")
		if declNode != nil {
			var name string
			p.walkTree(declNode, func(n *sitter.Node) bool {
				if n.Type() == "identifier" {
					name = n.Content(content)
					return false
				}
				return true
			})
			if name != "" {
				symbols = append(symbols, Symbol{
					Name:     name,
					Type:     SymbolTypeConst,
					FilePath: filePath,
					Line:     int(node.StartPoint().Row) + 1,
					Exported: true,
				})
			}
		}
	}
	return symbols
}

// extractJavaSymbols extracts symbols from Java AST.
func (p *Parser) extractJavaSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol
	var currentClass string

	p.walkTree(root, func(node *sitter.Node) bool {
		nodeType := node.Type()

		switch nodeType {
		case "class_declaration":
			if sym := p.extractJavaClass(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
				currentClass = sym.Name
			}
		case "interface_declaration":
			if sym := p.extractJavaInterface(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "enum_declaration":
			if sym := p.extractJavaEnum(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "method_declaration":
			if sym := p.extractJavaMethod(node, content, filePath, currentClass); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "constructor_declaration":
			if sym := p.extractJavaConstructor(node, content, filePath, currentClass); sym != nil {
				symbols = append(symbols, *sym)
			}
		case "field_declaration":
			symbols = append(symbols, p.extractJavaField(node, content, filePath, currentClass)...)
		case "record_declaration":
			if sym := p.extractJavaRecord(node, content, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		}
		return true
	})

	return symbols
}

func (p *Parser) extractJavaClass(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	exported := p.hasJavaModifier(node, content, "public")
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeClass,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: exported,
	}
}

func (p *Parser) extractJavaInterface(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeInterface,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: true,
	}
}

func (p *Parser) extractJavaEnum(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeEnum,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: true,
	}
}

func (p *Parser) extractJavaMethod(node *sitter.Node, content []byte, filePath string, currentClass string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	exported := p.hasJavaModifier(node, content, "public")
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeMethod,
		Parent:   currentClass,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: exported,
	}
}

func (p *Parser) extractJavaConstructor(node *sitter.Node, content []byte, filePath string, currentClass string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeMethod,
		Parent:   currentClass,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: true,
	}
}

func (p *Parser) extractJavaField(node *sitter.Node, content []byte, filePath string, currentClass string) []Symbol {
	var symbols []Symbol
	isConstant := p.hasJavaModifier(node, content, "static") && p.hasJavaModifier(node, content, "final")
	if !isConstant {
		return symbols
	}
	exported := p.hasJavaModifier(node, content, "public")
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "variable_declarator" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				sym := Symbol{
					Name:     nameNode.Content(content),
					Type:     SymbolTypeConst,
					Parent:   currentClass,
					FilePath: filePath,
					Line:     int(nameNode.StartPoint().Row) + 1,
					Exported: exported,
				}
				if valueNode := child.ChildByFieldName("value"); valueNode != nil {
					sym.RawValue = valueNode.Content(content)
					sym.Value = p.evaluateValue(sym.RawValue)
				}
				symbols = append(symbols, sym)
			}
		}
	}
	return symbols
}

func (p *Parser) extractJavaRecord(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return &Symbol{
		Name:     nameNode.Content(content),
		Type:     SymbolTypeClass,
		FilePath: filePath,
		Line:     int(nameNode.StartPoint().Row) + 1,
		LineEnd:  int(node.EndPoint().Row) + 1,
		Exported: true,
	}
}

func (p *Parser) hasJavaModifier(node *sitter.Node, content []byte, modifier string) bool {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "modifiers" && strings.Contains(child.Content(content), modifier) {
			return true
		}
	}
	return false
}
