# Language Parser Test Framework

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


## Purpose

Standardized approach to test each language parser module ensuring:

1. All expected symbol types are extracted
2. Line numbers are accurate
3. Nested structures are handled correctly
4. Edge cases don't cause crashes

---

## Test Structure per Language

### 1. Test Fixture File

Create a comprehensive test file that exercises all parser capabilities:

```
cmd/ingest-codebase/languages/testdata/
├── typescript_test_fixture.ts
├── go_test_fixture.go
├── python_test_fixture.py
├── java_test_fixture.java
├── rust_test_fixture.rs
└── ...
```

### 2. Expected Symbols JSON

Define expected extraction results:

```
cmd/ingest-codebase/languages/testdata/
├── typescript_expected.json
├── go_expected.json
└── ...
```

### 3. Test Runner

`cmd/ingest-codebase/languages/parser_test.go`

---

## TypeScript Test Fixture Template

```typescript
// Line 1: Test all TypeScript symbol extraction
// testdata/typescript_test_fixture.ts

// Line 4: Constant (UPPER_CASE)
export const MAX_RETRIES = 3;

// Line 7: Type alias
export type UserId = string;

// Line 10: Interface
export interface UserDto {
  id: UserId;
  name: string;
  email?: string;
}

// Line 17: Enum
export enum UserStatus {
  ACTIVE = 'active',
  INACTIVE = 'inactive',
  PENDING = 'pending'
}

// Line 24: Decorated class
@Injectable()
export class UserService {
  // Line 27: Class property
  private readonly logger: Logger;

  // Line 30: Constructor
  constructor(private userRepo: UserRepository) {
    this.logger = new Logger(UserService.name);
  }

  // Line 35: Async method with decorator
  @Transactional()
  async findById(id: UserId): Promise<UserDto | null> {
    return this.userRepo.findOne(id);
  }

  // Line 41: Method with multiple params
  async create(data: CreateUserInput): Promise<UserDto> {
    return this.userRepo.create(data);
  }

  // Line 46: Static method
  static validateEmail(email: string): boolean {
    return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
  }
}

// Line 52: Top-level function
export function formatUser(user: UserDto): string {
  return `${user.name} <${user.email}>`;
}

// Line 57: Arrow function
export const validateId = (id: string): boolean => {
  return id.length > 0;
};

// Line 62: Abstract class
export abstract class BaseEntity {
  abstract getId(): string;
}
```

---

## Expected Symbols JSON Format

```json
{
  "parser": "typescript",
  "fixture": "typescript_test_fixture.ts",
  "expected_symbols": [
    {"name": "MAX_RETRIES", "type": "constant", "line": 5, "exported": true},
    {"name": "UserId", "type": "type", "line": 8, "exported": true},
    {"name": "UserDto", "type": "interface", "line": 11, "exported": true},
    {"name": "UserStatus", "type": "enum", "line": 18, "exported": true},
    {"name": "UserService", "type": "class", "line": 26, "exported": true, "decorators": ["Injectable"]},
    {"name": "findById", "type": "method", "line": 37, "parent": "UserService", "decorators": ["Transactional"]},
    {"name": "create", "type": "method", "line": 42, "parent": "UserService"},
    {"name": "validateEmail", "type": "method", "line": 47, "parent": "UserService"},
    {"name": "formatUser", "type": "function", "line": 53, "exported": true},
    {"name": "validateId", "type": "function", "line": 58, "exported": true},
    {"name": "BaseEntity", "type": "class", "line": 63, "exported": true}
  ],
  "expected_count": 11,
  "line_tolerance": 1
}
```

---

## Test Runner Implementation

```go
// cmd/ingest-codebase/languages/parser_test.go

package languages

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
)

type ExpectedSymbol struct {
    Name       string   `json:"name"`
    Type       string   `json:"type"`
    Line       int      `json:"line"`
    Exported   bool     `json:"exported"`
    Parent     string   `json:"parent,omitempty"`
    Decorators []string `json:"decorators,omitempty"`
}

type ExpectedResult struct {
    Parser          string           `json:"parser"`
    Fixture         string           `json:"fixture"`
    ExpectedSymbols []ExpectedSymbol `json:"expected_symbols"`
    ExpectedCount   int              `json:"expected_count"`
    LineTolerance   int              `json:"line_tolerance"`
}

func TestTypeScriptParser(t *testing.T) {
    testParserWithFixture(t, &TypeScriptParser{}, "typescript")
}

func TestGoParser(t *testing.T) {
    testParserWithFixture(t, &GoParser{}, "go")
}

func TestPythonParser(t *testing.T) {
    testParserWithFixture(t, &PythonParser{}, "python")
}

func testParserWithFixture(t *testing.T, parser LanguageParser, language string) {
    // Load expected results
    expectedPath := filepath.Join("testdata", language+"_expected.json")
    expectedData, err := os.ReadFile(expectedPath)
    if err != nil {
        t.Skipf("No expected file for %s: %v", language, err)
        return
    }

    var expected ExpectedResult
    if err := json.Unmarshal(expectedData, &expected); err != nil {
        t.Fatalf("Failed to parse expected JSON: %v", err)
    }

    // Parse fixture
    fixturePath := filepath.Join("testdata", expected.Fixture)
    elements, err := parser.ParseFile("testdata", fixturePath, true)
    if err != nil {
        t.Fatalf("Parser failed: %v", err)
    }

    // Collect all symbols
    var symbols []Symbol
    for _, elem := range elements {
        symbols = append(symbols, elem.Symbols...)
    }

    // Check count
    if len(symbols) < expected.ExpectedCount {
        t.Errorf("Expected at least %d symbols, got %d", expected.ExpectedCount, len(symbols))
    }

    // Verify each expected symbol
    for _, exp := range expected.ExpectedSymbols {
        found := false
        for _, sym := range symbols {
            if sym.Name == exp.Name && sym.Type == exp.Type {
                found = true
                // Check line number within tolerance
                lineDiff := abs(sym.LineNumber - exp.Line)
                if lineDiff > expected.LineTolerance {
                    t.Errorf("Symbol %s: expected line %d, got %d (tolerance %d)",
                        exp.Name, exp.Line, sym.LineNumber, expected.LineTolerance)
                }
                // Check parent for methods
                if exp.Parent != "" && sym.Parent != exp.Parent {
                    t.Errorf("Symbol %s: expected parent %s, got %s",
                        exp.Name, exp.Parent, sym.Parent)
                }
                break
            }
        }
        if !found {
            t.Errorf("Expected symbol not found: %s (type: %s)", exp.Name, exp.Type)
        }
    }
}

func abs(x int) int {
    if x < 0 {
        return -x
    }
    return x
}
```

---

## Running Tests

```bash
# Run all parser tests
go test ./cmd/ingest-codebase/languages/... -v

# Run specific language test
go test ./cmd/ingest-codebase/languages/... -v -run TestTypeScriptParser

# With coverage
go test ./cmd/ingest-codebase/languages/... -v -cover
```

---

## Integration Test: Full Pipeline

```bash
#!/bin/bash
# scripts/test-parser-pipeline.sh

LANGUAGE=$1
FIXTURE_DIR="cmd/ingest-codebase/languages/testdata"
SPACE_ID="parser-test-${LANGUAGE}"

echo "Testing ${LANGUAGE} parser pipeline..."

# 1. Clear test space
curl -s -X DELETE "http://localhost:9999/v1/memory/space/${SPACE_ID}"

# 2. Ingest fixture file
go run ./cmd/ingest-codebase \
  --path "${FIXTURE_DIR}/${LANGUAGE}_test_fixture.*" \
  --space-id "${SPACE_ID}" \
  --extract-symbols \
  --verbose

# 3. Query symbols
echo "Symbols stored:"
curl -s "http://localhost:9999/v1/memory/symbols?space_id=${SPACE_ID}&limit=50" | jq '.symbols[] | {name, type, line_number}'

# 4. Test retrieval with evidence
echo "Retrieval test:"
curl -s -X POST "http://localhost:9999/v1/memory/retrieve" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\": \"${SPACE_ID}\", \"query_text\": \"findById method\", \"top_k\": 3}" | jq '.results[] | {path, evidence}'
```

---

## Checklist for New Parser Development

- [ ] Create test fixture with all symbol types for language
- [ ] Create expected.json with line numbers
- [ ] Run unit test to verify extraction
- [ ] Run integration test to verify storage
- [ ] Run benchmark comparison (before/after)
- [ ] Update roadmap with findings
- [ ] Document any edge cases discovered

---

## Metrics Dashboard (Future)

Track parser quality over time:

```
| Language   | Symbol Types | Avg/File | Line Accuracy | Last Updated |
|------------|--------------|----------|---------------|--------------|
| TypeScript | 7            | 12.3     | 98%           | 2026-01-28   |
| Go         | 5            | 8.7      | 99%           | 2026-01-15   |
| Python     | 4            | 6.2      | 95%           | 2026-01-15   |
```
