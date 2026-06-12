# Tree-Sitter Grammar Fix Guide

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date:** 2026-01-29  
**Status:** 5 languages need grammar integration  
**Files to modify:** `internal/symbols/parser.go`, `internal/symbols/types.go`, `go.mod`

---

## Step 1: Add Grammar Dependencies

Run these commands:

```bash
cd /path/to/mdemg

# Add tree-sitter grammar packages
go get github.com/smacker/go-tree-sitter/c
go get github.com/smacker/go-tree-sitter/cpp
go get github.com/smacker/go-tree-sitter/java
go get github.com/smacker/go-tree-sitter/rust

# Update go.sum
go mod tidy
```

**Note:** CUDA uses the C++ grammar since CUDA is a C++ superset.

---

## Step 2: Update `internal/symbols/parser.go`

### 2.1 Add Imports (lines 12-17)

```go
import (
 "context"
 "fmt"
 "log"
 "os"
 "path/filepath"
 "strings"
 "time"

 sitter "github.com/smacker/go-tree-sitter"
 "github.com/smacker/go-tree-sitter/c"       // ADD
 "github.com/smacker/go-tree-sitter/cpp"     // ADD
 "github.com/smacker/go-tree-sitter/golang"
 "github.com/smacker/go-tree-sitter/java"    // ADD
 "github.com/smacker/go-tree-sitter/javascript"
 "github.com/smacker/go-tree-sitter/python"
 "github.com/smacker/go-tree-sitter/rust"    // ADD
 "github.com/smacker/go-tree-sitter/typescript/typescript"
)
```

### 2.2 Register Grammars in NewParser() (lines 34-41)

```go
// Initialize language grammars
p.languages[LangTypeScript] = typescript.GetLanguage()
p.languages[LangJavaScript] = javascript.GetLanguage()
p.languages[LangGo] = golang.GetLanguage()
p.languages[LangPython] = python.GetLanguage()
p.languages[LangRust] = rust.GetLanguage()     // ADD
p.languages[LangC] = c.GetLanguage()           // ADD
p.languages[LangCPP] = cpp.GetLanguage()       // ADD
p.languages[LangCUDA] = cpp.GetLanguage()      // ADD (CUDA uses C++ grammar)
p.languages[LangJava] = java.GetLanguage()     // ADD
```

### 2.3 Add Extraction Cases in ParseContent() (lines 102-111)

```go
// Extract symbols based on language
var symbols []Symbol
switch lang {
case LangTypeScript, LangJavaScript:
 symbols = p.extractTypeScriptSymbols(tree.RootNode(), content, filePath)
case LangGo:
 symbols = p.extractGoSymbols(tree.RootNode(), content, filePath)
case LangPython:
 symbols = p.extractPythonSymbols(tree.RootNode(), content, filePath)
case LangRust:                                                              // ADD
 symbols = p.extractRustSymbols(tree.RootNode(), content, filePath)      // ADD
case LangC:                                                                 // ADD
 symbols = p.extractCSymbols(tree.RootNode(), content, filePath)         // ADD
case LangCPP, LangCUDA:                                                     // ADD
 symbols = p.extractCPPSymbols(tree.RootNode(), content, filePath)       // ADD
case LangJava:                                                              // ADD
 symbols = p.extractJavaSymbols(tree.RootNode(), content, filePath)      // ADD
default:
 return nil, fmt.Errorf("extraction not implemented for: %s", lang)
}
```

---

## Step 3: Update `internal/symbols/types.go`

Add Language constants and extension mappings:

```go
// Language constants
const (
 LangUnknown    Language = ""
 LangTypeScript Language = "typescript"
 LangJavaScript Language = "javascript"
 LangGo         Language = "go"
 LangPython     Language = "python"
 LangRust       Language = "rust"       // ADD
 LangC          Language = "c"          // ADD
 LangCPP        Language = "cpp"        // ADD
 LangCUDA       Language = "cuda"       // ADD
 LangJava       Language = "java"       // ADD
)

// LanguageFromExtension returns the language for a file extension.
func LanguageFromExtension(ext string) Language {
 switch strings.ToLower(ext) {
 case ".ts", ".tsx":
  return LangTypeScript
 case ".js", ".jsx", ".mjs":
  return LangJavaScript
 case ".go":
  return LangGo
 case ".py", ".pyw":
  return LangPython
 case ".rs":                          // ADD
  return LangRust                  // ADD
 case ".c", ".h":                     // ADD
  return LangC                     // ADD
 case ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx", ".inl":  // ADD
  return LangCPP                   // ADD
 case ".cu", ".cuh":                  // ADD
  return LangCUDA                  // ADD
 case ".java":                        // ADD
  return LangJava                  // ADD
 default:
  return LangUnknown
 }
}

// IsSupported returns true if the language has parser support.
func (l Language) IsSupported() bool {
 switch l {
 case LangTypeScript, LangJavaScript, LangGo, LangPython,
  LangRust, LangC, LangCPP, LangCUDA, LangJava:  // UPDATE
  return true
 default:
  return false
 }
}
```

---

## Step 4: Add Extraction Functions

Add these to `internal/symbols/parser.go` (after the Python extraction functions):

### 4.1 Rust Extraction

```go
// extractRustSymbols extracts symbols from Rust AST.
func (p *Parser) extractRustSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
 var symbols []Symbol
 lines := strings.Split(string(content), "\n")

 p.walkTree(root, func(node *sitter.Node) bool {
  nodeType := node.Type()

  switch nodeType {
  case "const_item":
   if sym := p.extractRustConst(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "static_item":
   if sym := p.extractRustStatic(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "function_item":
   if sym := p.extractRustFunction(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "struct_item":
   if sym := p.extractRustStruct(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "enum_item":
   if sym := p.extractRustEnum(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "trait_item":
   if sym := p.extractRustTrait(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "impl_item":
   // Extract methods from impl blocks
   symbols = append(symbols, p.extractRustImplMethods(node, content, lines, filePath)...)
  case "type_item":
   if sym := p.extractRustTypeAlias(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "mod_item":
   if sym := p.extractRustModule(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "macro_definition":
   if sym := p.extractRustMacro(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  }
  return true
 })

 return symbols
}

func (p *Parser) extractRustConst(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 name := nameNode.Content(content)
 
 // Check for pub visibility
 exported := false
 for i := 0; i < int(node.ChildCount()); i++ {
  if node.Child(i).Type() == "visibility_modifier" {
   exported = true
   break
  }
 }
 
 sym := Symbol{
  Name:     name,
  Type:     SymbolTypeConst,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  LineEnd:  int(node.EndPoint().Row) + 1,
  Exported: exported,
 }
 
 // Extract value
 valueNode := node.ChildByFieldName("value")
 if valueNode != nil {
  sym.RawValue = valueNode.Content(content)
  sym.Value = p.evaluateValue(sym.RawValue)
 }
 
 return &sym
}

func (p *Parser) extractRustFunction(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 name := nameNode.Content(content)
 
 exported := false
 for i := 0; i < int(node.ChildCount()); i++ {
  if node.Child(i).Type() == "visibility_modifier" {
   exported = true
   break
  }
 }
 
 sym := Symbol{
  Name:     name,
  Type:     SymbolTypeFunction,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  LineEnd:  int(node.EndPoint().Row) + 1,
  Exported: exported,
 }
 
 // Extract signature
 paramsNode := node.ChildByFieldName("parameters")
 returnNode := node.ChildByFieldName("return_type")
 if paramsNode != nil {
  sym.Signature = paramsNode.Content(content)
  if returnNode != nil {
   sym.Signature += " -> " + returnNode.Content(content)
  }
 }
 
 return &sym
}

func (p *Parser) extractRustStruct(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 name := nameNode.Content(content)
 
 exported := false
 for i := 0; i < int(node.ChildCount()); i++ {
  if node.Child(i).Type() == "visibility_modifier" {
   exported = true
   break
  }
 }
 
 return &Symbol{
  Name:     name,
  Type:     SymbolTypeStruct,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  LineEnd:  int(node.EndPoint().Row) + 1,
  Exported: exported,
 }
}

func (p *Parser) extractRustEnum(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 name := nameNode.Content(content)
 
 exported := false
 for i := 0; i < int(node.ChildCount()); i++ {
  if node.Child(i).Type() == "visibility_modifier" {
   exported = true
   break
  }
 }
 
 return &Symbol{
  Name:     name,
  Type:     SymbolTypeEnum,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  LineEnd:  int(node.EndPoint().Row) + 1,
  Exported: exported,
 }
}

func (p *Parser) extractRustTrait(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 name := nameNode.Content(content)
 
 exported := false
 for i := 0; i < int(node.ChildCount()); i++ {
  if node.Child(i).Type() == "visibility_modifier" {
   exported = true
   break
  }
 }
 
 return &Symbol{
  Name:     name,
  Type:     SymbolTypeTrait,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  LineEnd:  int(node.EndPoint().Row) + 1,
  Exported: exported,
 }
}

func (p *Parser) extractRustImplMethods(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
 var symbols []Symbol
 
 // Get the type this impl is for
 var parentName string
 typeNode := node.ChildByFieldName("type")
 if typeNode != nil {
  parentName = typeNode.Content(content)
 }
 
 // Walk children looking for function_item nodes
 for i := 0; i < int(node.NamedChildCount()); i++ {
  child := node.NamedChild(i)
  if child.Type() == "declaration_list" {
   for j := 0; j < int(child.NamedChildCount()); j++ {
    item := child.NamedChild(j)
    if item.Type() == "function_item" {
     if sym := p.extractRustFunction(item, content, lines, filePath); sym != nil {
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

func (p *Parser) extractRustTypeAlias(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 
 exported := false
 for i := 0; i < int(node.ChildCount()); i++ {
  if node.Child(i).Type() == "visibility_modifier" {
   exported = true
   break
  }
 }
 
 return &Symbol{
  Name:     nameNode.Content(content),
  Type:     SymbolTypeType,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  Exported: exported,
 }
}

func (p *Parser) extractRustModule(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 
 exported := false
 for i := 0; i < int(node.ChildCount()); i++ {
  if node.Child(i).Type() == "visibility_modifier" {
   exported = true
   break
  }
 }
 
 return &Symbol{
  Name:     nameNode.Content(content),
  Type:     SymbolTypeModule,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  Exported: exported,
 }
}

func (p *Parser) extractRustMacro(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 
 return &Symbol{
  Name:     nameNode.Content(content),
  Type:     SymbolTypeMacro,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  Exported: true, // macro_rules! are typically public
 }
}

func (p *Parser) extractRustStatic(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 
 exported := false
 for i := 0; i < int(node.ChildCount()); i++ {
  if node.Child(i).Type() == "visibility_modifier" {
   exported = true
   break
  }
 }
 
 return &Symbol{
  Name:     nameNode.Content(content),
  Type:     SymbolTypeConst,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  Exported: exported,
 }
}
```

### 4.2 C Extraction

```go
// extractCSymbols extracts symbols from C AST.
func (p *Parser) extractCSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
 var symbols []Symbol
 lines := strings.Split(string(content), "\n")

 p.walkTree(root, func(node *sitter.Node) bool {
  nodeType := node.Type()

  switch nodeType {
  case "preproc_def":
   if sym := p.extractCMacro(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "preproc_function_def":
   if sym := p.extractCFunctionMacro(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "function_definition":
   if sym := p.extractCFunction(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "declaration":
   // Could be function declaration, variable, or typedef
   symbols = append(symbols, p.extractCDeclaration(node, content, lines, filePath)...)
  case "struct_specifier":
   if sym := p.extractCStruct(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "enum_specifier":
   if sym := p.extractCEnum(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "type_definition":
   if sym := p.extractCTypedef(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  }
  return true
 })

 return symbols
}

func (p *Parser) extractCMacro(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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
 
 valueNode := node.ChildByFieldName("value")
 if valueNode != nil {
  sym.RawValue = valueNode.Content(content)
  sym.Value = p.evaluateValue(sym.RawValue)
 }
 
 return &sym
}

func (p *Parser) extractCFunctionMacro(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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

func (p *Parser) extractCFunction(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 // Get declarator which contains the function name
 declNode := node.ChildByFieldName("declarator")
 if declNode == nil {
  return nil
 }
 
 // Function declarator contains the name
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
 
 // Check if static (not exported)
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

func (p *Parser) extractCDeclaration(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
 var symbols []Symbol
 
 // Check for function declarations (prototypes)
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

func (p *Parser) extractCStruct(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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

func (p *Parser) extractCEnum(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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

func (p *Parser) extractCTypedef(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 // Get the declarator which has the typedef name
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
```

### 4.3 C++ Extraction (also used for CUDA)

```go
// extractCPPSymbols extracts symbols from C++ AST.
// Also handles CUDA files since CUDA is a C++ superset.
func (p *Parser) extractCPPSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
 var symbols []Symbol
 lines := strings.Split(string(content), "\n")
 
 // Track current class for method extraction
 var currentClass string

 p.walkTree(root, func(node *sitter.Node) bool {
  nodeType := node.Type()

  switch nodeType {
  case "preproc_def":
   if sym := p.extractCMacro(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "function_definition":
   if sym := p.extractCPPFunction(node, content, lines, filePath, currentClass); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "class_specifier":
   if sym := p.extractCPPClass(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
    currentClass = sym.Name
    // Extract methods
    symbols = append(symbols, p.extractCPPClassMethods(node, content, lines, filePath, sym.Name)...)
    currentClass = ""
   }
  case "struct_specifier":
   if sym := p.extractCStruct(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "enum_specifier":
   if sym := p.extractCEnum(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "namespace_definition":
   if sym := p.extractCPPNamespace(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "alias_declaration", "type_definition":
   if sym := p.extractCPPTypeAlias(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "template_declaration":
   // Process the templated item
   return true
  case "declaration":
   // Check for constexpr, const declarations
   symbols = append(symbols, p.extractCPPDeclaration(node, content, lines, filePath)...)
  }
  return true
 })

 return symbols
}

func (p *Parser) extractCPPClass(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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

func (p *Parser) extractCPPFunction(node *sitter.Node, content []byte, lines []string, filePath string, currentClass string) *Symbol {
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

func (p *Parser) extractCPPClassMethods(node *sitter.Node, content []byte, lines []string, filePath string, className string) []Symbol {
 var symbols []Symbol
 
 // Find field_declaration_list (class body)
 for i := 0; i < int(node.NamedChildCount()); i++ {
  child := node.NamedChild(i)
  if child.Type() == "field_declaration_list" {
   for j := 0; j < int(child.NamedChildCount()); j++ {
    item := child.NamedChild(j)
    switch item.Type() {
    case "function_definition":
     if sym := p.extractCPPFunction(item, content, lines, filePath, className); sym != nil {
      sym.Type = SymbolTypeMethod
      sym.Parent = className
      symbols = append(symbols, *sym)
     }
    case "declaration":
     // Method declaration without body
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

func (p *Parser) extractCPPNamespace(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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

func (p *Parser) extractCPPTypeAlias(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 // Handle "using X = ..." style
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
 
 // Fall back to typedef extraction
 return p.extractCTypedef(node, content, lines, filePath)
}

func (p *Parser) extractCPPDeclaration(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
 var symbols []Symbol
 
 // Check for constexpr
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
```

### 4.4 Java Extraction

```go
// extractJavaSymbols extracts symbols from Java AST.
func (p *Parser) extractJavaSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
 var symbols []Symbol
 lines := strings.Split(string(content), "\n")
 var currentClass string

 p.walkTree(root, func(node *sitter.Node) bool {
  nodeType := node.Type()

  switch nodeType {
  case "class_declaration":
   if sym := p.extractJavaClass(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
    currentClass = sym.Name
   }
  case "interface_declaration":
   if sym := p.extractJavaInterface(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "enum_declaration":
   if sym := p.extractJavaEnum(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "method_declaration":
   if sym := p.extractJavaMethod(node, content, lines, filePath, currentClass); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "constructor_declaration":
   if sym := p.extractJavaConstructor(node, content, lines, filePath, currentClass); sym != nil {
    symbols = append(symbols, *sym)
   }
  case "field_declaration":
   symbols = append(symbols, p.extractJavaField(node, content, lines, filePath, currentClass)...)
  case "record_declaration":
   if sym := p.extractJavaRecord(node, content, lines, filePath); sym != nil {
    symbols = append(symbols, *sym)
   }
  }
  return true
 })

 return symbols
}

func (p *Parser) extractJavaClass(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 
 // Check modifiers for public/abstract
 exported := false
 for i := 0; i < int(node.NamedChildCount()); i++ {
  child := node.NamedChild(i)
  if child.Type() == "modifiers" {
   modText := child.Content(content)
   if strings.Contains(modText, "public") {
    exported = true
   }
  }
 }
 
 return &Symbol{
  Name:     nameNode.Content(content),
  Type:     SymbolTypeClass,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  LineEnd:  int(node.EndPoint().Row) + 1,
  Exported: exported,
 }
}

func (p *Parser) extractJavaInterface(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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

func (p *Parser) extractJavaEnum(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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

func (p *Parser) extractJavaMethod(node *sitter.Node, content []byte, lines []string, filePath string, currentClass string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 
 // Check for static modifier
 isStatic := false
 exported := false
 for i := 0; i < int(node.NamedChildCount()); i++ {
  child := node.NamedChild(i)
  if child.Type() == "modifiers" {
   modText := child.Content(content)
   if strings.Contains(modText, "static") {
    isStatic = true
   }
   if strings.Contains(modText, "public") {
    exported = true
   }
  }
 }
 
 sym := Symbol{
  Name:     nameNode.Content(content),
  Type:     SymbolTypeMethod,
  Parent:   currentClass,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  LineEnd:  int(node.EndPoint().Row) + 1,
  Exported: exported,
 }
 
 if isStatic && currentClass != "" {
  // Could be marked as static method
 }
 
 return &sym
}

func (p *Parser) extractJavaConstructor(node *sitter.Node, content []byte, lines []string, filePath string, currentClass string) *Symbol {
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

func (p *Parser) extractJavaField(node *sitter.Node, content []byte, lines []string, filePath string, currentClass string) []Symbol {
 var symbols []Symbol
 
 // Check for static final (constants)
 isConstant := false
 exported := false
 for i := 0; i < int(node.NamedChildCount()); i++ {
  child := node.NamedChild(i)
  if child.Type() == "modifiers" {
   modText := child.Content(content)
   if strings.Contains(modText, "static") && strings.Contains(modText, "final") {
    isConstant = true
   }
   if strings.Contains(modText, "public") {
    exported = true
   }
  }
 }
 
 if !isConstant {
  return symbols
 }
 
 // Get declarators
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
    
    valueNode := child.ChildByFieldName("value")
    if valueNode != nil {
     sym.RawValue = valueNode.Content(content)
     sym.Value = p.evaluateValue(sym.RawValue)
    }
    
    symbols = append(symbols, sym)
   }
  }
 }
 
 return symbols
}

func (p *Parser) extractJavaRecord(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
 nameNode := node.ChildByFieldName("name")
 if nameNode == nil {
  return nil
 }
 
 return &Symbol{
  Name:     nameNode.Content(content),
  Type:     SymbolTypeClass, // Records are a kind of class
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  LineEnd:  int(node.EndPoint().Row) + 1,
  Exported: true,
 }
}
```

---

## Step 5: Add Missing Symbol Types

In `internal/symbols/types.go`, ensure these symbol types exist:

```go
const (
 SymbolTypeConst     SymbolType = "constant"
 SymbolTypeVar       SymbolType = "variable"
 SymbolTypeFunction  SymbolType = "function"
 SymbolTypeClass     SymbolType = "class"
 SymbolTypeStruct    SymbolType = "struct"
 SymbolTypeInterface SymbolType = "interface"
 SymbolTypeEnum      SymbolType = "enum"
 SymbolTypeMethod    SymbolType = "method"
 SymbolTypeType      SymbolType = "type"
 SymbolTypeTrait     SymbolType = "trait"      // ADD for Rust
 SymbolTypeMacro     SymbolType = "macro"      // ADD for C/Rust
 SymbolTypeModule    SymbolType = "module"     // ADD for Rust
 SymbolTypeNamespace SymbolType = "namespace"  // ADD for C++
 SymbolTypeKernel    SymbolType = "kernel"     // ADD for CUDA
)
```

---

## Step 6: Build and Test

```bash
# Build
cd /path/to/mdemg
go build -o bin/extract-symbols ./cmd/extract-symbols

# Test each language
./bin/extract-symbols --json upts/fixtures/rust_test_fixture.rs
./bin/extract-symbols --json upts/fixtures/c_test_fixture.c
./bin/extract-symbols --json upts/fixtures/cpp_test_fixture.cpp
./bin/extract-symbols --json upts/fixtures/cuda_test_fixture.cu
./bin/extract-symbols --json upts/fixtures/java_test_fixture.java

# Run UPTS validation
for lang in rust c cpp cuda java; do
  echo "=== Testing $lang ==="
  python3 upts/runners/upts_runner.py validate \
    --spec="upts/specs/${lang}.upts.json" \
    --parser="./bin/extract-symbols --json"
done
```

---

## Expected Results

After implementation:

```
┌────────────┬──────────────┬────────┐
│  Language  │   Matched    │ Status │
├────────────┼──────────────┼────────┤
│ Rust       │ 27/27 (100%) │ PASS   │
│ C          │ 24/24 (100%) │ PASS   │
│ C++        │ 30/30 (100%) │ PASS   │
│ CUDA       │ 25/25 (100%) │ PASS   │
│ Java       │ 32/32 (100%) │ PASS   │
└────────────┴──────────────┴────────┘

Total: 16/16 languages passing (100%)
```

---

## Summary of Changes

| File | Changes |
|------|---------|
| `go.mod` | Add c, cpp, java, rust grammar dependencies |
| `internal/symbols/types.go` | Add language constants, extension mappings, symbol types |
| `internal/symbols/parser.go` | Add imports, grammar registration, extraction functions |
| `cmd/extract-symbols/main.go` | No changes needed (already has fallback support) |
