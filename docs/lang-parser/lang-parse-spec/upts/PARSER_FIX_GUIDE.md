# Parser Gap Fixes - Implementation Guide

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


This document provides Go code patterns to fix the gaps identified in PARSER_GAPS.md.

---

## Python Parser Fixes

### 1. Protocol Detection → `interface`

**Current behavior:** `class UserRepository(Protocol)` → `{"type": "class"}`  
**Expected:** `{"type": "interface"}`

```go
// In python_parser.go

// Pattern to detect Protocol classes
var protocolPattern = regexp.MustCompile(`(?m)^class\s+(\w+)\s*\(\s*Protocol\s*\)`)

func (p *PythonParser) extractSymbols(content string) []Symbol {
    var symbols []Symbol
    
    // Check for Protocol-based classes
    for _, match := range protocolPattern.FindAllStringSubmatch(content, -1) {
        symbols = append(symbols, Symbol{
            Name:     match[1],
            Type:     "interface",  // NOT "class"
            Exported: !strings.HasPrefix(match[1], "_"),
            // ...
        })
    }
    
    // ... rest of extraction
}
```

### 2. Enum Detection → `enum`

**Current behavior:** `class Status(Enum)` → `{"type": "class"}`  
**Expected:** `{"type": "enum"}`

```go
// Pattern to detect Enum classes
var enumPattern = regexp.MustCompile(`(?m)^class\s+(\w+)\s*\(\s*(?:Enum|IntEnum|StrEnum)\s*\)`)

func classifyPythonClass(line string, bases string) string {
    switch {
    case strings.Contains(bases, "Protocol"):
        return "interface"
    case strings.Contains(bases, "Enum"), 
         strings.Contains(bases, "IntEnum"),
         strings.Contains(bases, "StrEnum"):
        return "enum"
    default:
        return "class"
    }
}
```

### 3. Method Extraction with Parent

**Current behavior:** Methods extracted as standalone `function`  
**Expected:** `{"type": "method", "parent": "ClassName"}`

```go
// Track current class context during parsing
type PythonParser struct {
    currentClass string
    indentLevel  int
}

var methodPattern = regexp.MustCompile(`(?m)^(\s+)(async\s+)?def\s+(\w+)\s*\(`)

func (p *PythonParser) extractMethods(content string, classInfo ClassInfo) []Symbol {
    var symbols []Symbol
    
    lines := strings.Split(content, "\n")
    inClass := false
    currentClass := ""
    classIndent := 0
    
    for lineNum, line := range lines {
        // Detect class start
        if classMatch := classPattern.FindStringSubmatch(line); classMatch != nil {
            inClass = true
            currentClass = classMatch[1]
            classIndent = len(line) - len(strings.TrimLeft(line, " \t"))
            continue
        }
        
        // Detect method inside class
        if inClass {
            currentIndent := len(line) - len(strings.TrimLeft(line, " \t"))
            
            // Check if we've exited the class (less indentation)
            if currentIndent <= classIndent && strings.TrimSpace(line) != "" {
                inClass = false
                currentClass = ""
            }
            
            // Extract method
            if methodMatch := methodPattern.FindStringSubmatch(line); methodMatch != nil {
                methodIndent := len(methodMatch[1])
                if methodIndent > classIndent {
                    symbols = append(symbols, Symbol{
                        Name:       methodMatch[3],
                        Type:       "method",
                        Parent:     currentClass,  // KEY: Set parent
                        LineNumber: lineNum + 1,
                        Exported:   !strings.HasPrefix(methodMatch[3], "_"),
                    })
                }
            }
        }
    }
    
    return symbols
}
```

### 4. Type Alias Detection → `type`

**Current behavior:** `UserId = str` → `{"type": "variable"}`  
**Expected:** `{"type": "type"}`

```go
// Pattern for type aliases
// CamelCase name = type expression (not a call)
var typeAliasPattern = regexp.MustCompile(`(?m)^([A-Z][a-zA-Z0-9]*)\s*(?::\s*TypeAlias\s*)?=\s*([A-Z][\w\[\], "\.]+|str|int|float|bool|None|List|Dict|Optional|Union|Tuple)`)

func isTypeAlias(name string, value string) bool {
    // CamelCase name suggests type alias
    if !unicode.IsUpper(rune(name[0])) {
        return false
    }
    
    // Value is a type expression, not a literal or call
    typePrefixes := []string{
        "str", "int", "float", "bool", "None",
        "List", "Dict", "Set", "Tuple", "Optional", "Union", "Callable",
    }
    
    for _, prefix := range typePrefixes {
        if strings.HasPrefix(value, prefix) {
            return true
        }
    }
    
    // Also check for quoted forward references
    if strings.Contains(value, `"`) && strings.Contains(value, "[") {
        return true
    }
    
    return false
}
```

---

## TypeScript Parser Fixes

### 1. Arrow Function Detection → `function`

**Current behavior:** `export const validateId = (id: string): boolean => {...}` → `{"type": "constant"}`  
**Expected:** `{"type": "function"}`

```go
// Pattern for arrow functions
var arrowFunctionPattern = regexp.MustCompile(`(?m)^export\s+const\s+(\w+)\s*=\s*(?:async\s+)?\([^)]*\)\s*(?::\s*[\w<>\[\]|]+)?\s*=>`)

func (p *TypeScriptParser) classifyConstant(line string) string {
    if arrowFunctionPattern.MatchString(line) {
        return "function"
    }
    return "constant"
}
```

### 2. Class Method Extraction

**Current behavior:** Methods inside classes not extracted  
**Expected:** Each method extracted with `parent`

```go
var tsMethodPattern = regexp.MustCompile(`(?m)^\s+(async\s+)?(?:static\s+)?(\w+)\s*\([^)]*\)\s*(?::\s*[\w<>\[\]|]+)?`)

func (p *TypeScriptParser) extractClassMethods(classContent string, className string, startLine int) []Symbol {
    var symbols []Symbol
    
    lines := strings.Split(classContent, "\n")
    braceDepth := 0
    
    for i, line := range lines {
        // Track brace depth to know when we're inside the class
        braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
        
        // Skip constructor, getters, setters for now
        if strings.Contains(line, "constructor") {
            continue
        }
        
        if match := tsMethodPattern.FindStringSubmatch(line); match != nil {
            methodName := match[2]
            
            // Skip if it looks like a property assignment
            if strings.Contains(line, "=") && !strings.Contains(line, "=>") {
                continue
            }
            
            symbols = append(symbols, Symbol{
                Name:       methodName,
                Type:       "method",
                Parent:     className,
                LineNumber: startLine + i,
                Exported:   true,  // Class methods inherit class export
            })
        }
    }
    
    return symbols
}
```

### 3. Abstract Class Detection

**Current behavior:** `export abstract class BaseEntity` → Not extracted  
**Expected:** `{"type": "class"}`

```go
// Update class pattern to handle abstract
var tsClassPattern = regexp.MustCompile(`(?m)^export\s+(?:abstract\s+)?class\s+(\w+)`)
```

---

## Testing Your Fixes

After implementing fixes:

```bash
# 1. Rebuild parser
go build -o parser ./cmd/ingest-codebase

# 2. Run against fixture
./parser fixtures/python_test_fixture.py > output.json

# 3. Validate with UPTS
python runners/upts_runner.py validate \
    --spec specs/python.upts.json \
    --parser ./parser

# 4. Check specific gap
cat output.json | jq '.symbols[] | select(.name == "UserRepository")'
# Should show: {"type": "interface", ...}
```

---

## Verification Checklist

After fixing each gap, verify:

- [ ] Protocol classes → `type: "interface"`
- [ ] Enum classes → `type: "enum"`
- [ ] Class methods → `type: "method"` with `parent`
- [ ] Type aliases → `type: "type"`
- [ ] Arrow functions → `type: "function"`
- [ ] All class methods extracted with parent
- [ ] Abstract classes extracted

Then remove the corresponding TYPE_COMPAT workaround from `upts_runner.py`.
