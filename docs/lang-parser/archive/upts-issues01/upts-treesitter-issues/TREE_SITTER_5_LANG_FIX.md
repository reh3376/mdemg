# Tree-Sitter Grammar Integration: 5 Languages

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Status:** 11/16 passing → 16/16 target  
**Languages to add:** Rust, C, C++, CUDA, Java

---

## Quick Fix Commands

```bash
cd /path/to/mdemg

# Step 1: Add grammar dependencies
go get github.com/smacker/go-tree-sitter/c
go get github.com/smacker/go-tree-sitter/cpp  
go get github.com/smacker/go-tree-sitter/java
go get github.com/smacker/go-tree-sitter/rust

go mod tidy

# Step 2: Apply code changes (see below)

# Step 3: Build and test
go build -o bin/extract-symbols ./cmd/extract-symbols

# Step 4: Verify
for lang in rust c cpp cuda java; do
  echo "=== $lang ===" 
  python3 upts/runners/upts_runner.py validate \
    --spec="upts/specs/${lang}.upts.json" \
    --parser="./bin/extract-symbols --json" 2>&1 | grep -E "Status:|Matched:"
done
```

---

## Code Changes: internal/symbols/parser.go

### Change 1: Add Imports (lines 12-17)

**Before:**

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
 "github.com/smacker/go-tree-sitter/golang"
 "github.com/smacker/go-tree-sitter/javascript"
 "github.com/smacker/go-tree-sitter/python"
 "github.com/smacker/go-tree-sitter/typescript/typescript"
)
```

**After:**

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
 "github.com/smacker/go-tree-sitter/c"
 "github.com/smacker/go-tree-sitter/cpp"
 "github.com/smacker/go-tree-sitter/golang"
 "github.com/smacker/go-tree-sitter/java"
 "github.com/smacker/go-tree-sitter/javascript"
 "github.com/smacker/go-tree-sitter/python"
 "github.com/smacker/go-tree-sitter/rust"
 "github.com/smacker/go-tree-sitter/typescript/typescript"
)
```

---

### Change 2: Register Grammars in NewParser() (lines 34-41)

**Before:**

```go
 // Initialize language grammars
 p.languages[LangTypeScript] = typescript.GetLanguage()
 p.languages[LangJavaScript] = javascript.GetLanguage()
 p.languages[LangGo] = golang.GetLanguage()
 p.languages[LangPython] = python.GetLanguage()
 // Note: Rust grammar requires separate import if needed

 return p, nil
```

**After:**

```go
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

 return p, nil
```

---

### Change 3: Add Extraction Cases in ParseContent() (lines 100-111)

**Before:**

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
 default:
  return nil, fmt.Errorf("extraction not implemented for: %s", lang)
 }
```

**After:**

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
```

---

## Code Changes: internal/symbols/types.go

### Add Language Constants

Ensure these constants exist (add if missing):

```go
const (
 LangUnknown    Language = ""
 LangTypeScript Language = "typescript"
 LangJavaScript Language = "javascript"
 LangGo         Language = "go"
 LangPython     Language = "python"
 LangRust       Language = "rust"
 LangC          Language = "c"
 LangCPP        Language = "cpp"
 LangCUDA       Language = "cuda"
 LangJava       Language = "java"
)
```

### Add Extension Mappings

In `LanguageFromExtension()`:

```go
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
 case ".rs":
  return LangRust
 case ".c", ".h":
  return LangC
 case ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx":
  return LangCPP
 case ".cu", ".cuh":
  return LangCUDA
 case ".java":
  return LangJava
 default:
  return LangUnknown
 }
}
```

### Update IsSupported()

```go
func (l Language) IsSupported() bool {
 switch l {
 case LangTypeScript, LangJavaScript, LangGo, LangPython,
  LangRust, LangC, LangCPP, LangCUDA, LangJava:
  return true
 default:
  return false
 }
}
```

---

## Extraction Functions to Add

Add these to the end of `internal/symbols/parser.go` (before the `Close()` function):

```go
// extractRustSymbols extracts symbols from Rust AST.
func (p *Parser) extractRustSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
 var symbols []Symbol
 lines := strings.Split(string(content), "\n")

 p.walkTree(root, func(node *sitter.Node) bool {
  nodeType := node.Type()

  switch nodeType {
  case "const_item", "static_item":
   if sym := p.extractRustConst(node, content, lines, filePath); sym != nil {
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

func (p *Parser) extractRustFunction(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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

func (p *Parser) extractRustStruct(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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

func (p *Parser) extractRustEnum(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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

func (p *Parser) extractRustTrait(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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

func (p *Parser) extractRustImplMethods(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
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
 return &Symbol{
  Name:     nameNode.Content(content),
  Type:     SymbolTypeType,
  FilePath: filePath,
  Line:     int(nameNode.StartPoint().Row) + 1,
  Exported: p.hasRustVisibility(node),
 }
}

func (p *Parser) extractRustModule(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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
 if valueNode := node.ChildByFieldName("value"); valueNode != nil {
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

func (p *Parser) extractCDeclaration(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
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
 lines := strings.Split(string(content), "\n")
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
    oldClass := currentClass
    currentClass = sym.Name
    symbols = append(symbols, p.extractCPPClassMethods(node, content, lines, filePath, sym.Name)...)
    currentClass = oldClass
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
  case "declaration":
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
 return p.extractCTypedef(node, content, lines, filePath)
}

func (p *Parser) extractCPPDeclaration(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
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

func (p *Parser) extractJavaRecord(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
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
```

---

## Symbol Types to Add in types.go

Ensure these exist:

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
 SymbolTypeTrait     SymbolType = "trait"
 SymbolTypeMacro     SymbolType = "macro"
 SymbolTypeModule    SymbolType = "module"
 SymbolTypeNamespace SymbolType = "namespace"
 SymbolTypeKernel    SymbolType = "kernel"
)
```

---

## Expected Result

After changes:

```
=== rust ===
Status: PASS
Matched: 27/27 (100%)

=== c ===
Status: PASS
Matched: 24/24 (100%)

=== cpp ===
Status: PASS
Matched: 30/30 (100%)

=== cuda ===
Status: PASS
Matched: 25/25 (100%)

=== java ===
Status: PASS
Matched: 32/32 (100%)

Total: 16/16 languages (100%)
```
