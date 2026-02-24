# Canonical Test Patterns for Language Parsers

## Purpose

Define standardized code patterns that exist in ALL programming languages. Each language parser must correctly extract these patterns to pass validation.

---

## Universal Code Patterns

### Pattern 1: Constant/Configuration Values

Every language has named constant values.

| Language | Syntax Example | Symbol Type |
|----------|---------------|-------------|
| TypeScript | `export const MAX_RETRIES = 3;` | constant |
| Go | `const MaxRetries = 3` | constant |
| Python | `MAX_RETRIES = 3` | constant |
| Java | `public static final int MAX_RETRIES = 3;` | constant |
| Rust | `const MAX_RETRIES: i32 = 3;` | constant |
| C/C++ | `#define MAX_RETRIES 3` or `const int MAX_RETRIES = 3;` | constant |

**Expected Extraction:**

- Name: `MAX_RETRIES` (or language equivalent)
- Type: `constant`
- Value: `3`
- Line number: accurate

---

### Pattern 2: Function/Method Definition

Standalone functions with parameters and return types.

| Language | Syntax Example |
|----------|---------------|
| TypeScript | `export function calculateTotal(items: Item[]): number { }` |
| Go | `func CalculateTotal(items []Item) int { }` |
| Python | `def calculate_total(items: List[Item]) -> int:` |
| Java | `public int calculateTotal(List<Item> items) { }` |
| Rust | `pub fn calculate_total(items: Vec<Item>) -> i32 { }` |
| C/C++ | `int calculate_total(Item* items, int count) { }` |

**Expected Extraction:**

- Name: function name
- Type: `function`
- Signature: full signature with params and return type
- Line number: accurate
- Exported: true/false

---

### Pattern 3: Class/Struct Definition

Object-oriented or structured data type.

| Language | Syntax Example |
|----------|---------------|
| TypeScript | `export class UserService { }` |
| Go | `type UserService struct { }` |
| Python | `class UserService:` |
| Java | `public class UserService { }` |
| Rust | `pub struct UserService { }` |
| C++ | `class UserService { };` |

**Expected Extraction:**

- Name: class/struct name
- Type: `class` or `struct`
- Line number: accurate
- Exported: true/false
- Extends/Implements: if applicable

---

### Pattern 4: Interface/Trait/Protocol

Abstract type definitions.

| Language | Syntax Example |
|----------|---------------|
| TypeScript | `export interface UserRepository { }` |
| Go | `type UserRepository interface { }` |
| Python | `class UserRepository(Protocol):` |
| Java | `public interface UserRepository { }` |
| Rust | `pub trait UserRepository { }` |
| C++ | `class UserRepository { virtual void method() = 0; };` (abstract) |

**Expected Extraction:**

- Name: interface/trait name
- Type: `interface` or `trait`
- Line number: accurate

---

### Pattern 5: Enumeration

Named set of values.

| Language | Syntax Example |
|----------|---------------|
| TypeScript | `export enum Status { ACTIVE, INACTIVE }` |
| Go | `type Status int; const (ACTIVE Status = iota; INACTIVE)` |
| Python | `class Status(Enum): ACTIVE = 1; INACTIVE = 2` |
| Java | `public enum Status { ACTIVE, INACTIVE }` |
| Rust | `pub enum Status { Active, Inactive }` |
| C/C++ | `enum Status { ACTIVE, INACTIVE };` |

**Expected Extraction:**

- Name: enum name
- Type: `enum`
- Values: enum members (optional but valuable)
- Line number: accurate

---

### Pattern 6: Method (Inside Class)

Member function of a class.

| Language | Syntax Example |
|----------|---------------|
| TypeScript | `async findById(id: string): Promise<User>` |
| Go | `func (s *UserService) FindById(id string) (*User, error)` |
| Python | `def find_by_id(self, id: str) -> User:` |
| Java | `public User findById(String id) { }` |
| Rust | `pub fn find_by_id(&self, id: &str) -> User { }` |
| C++ | `User* findById(std::string id);` |

**Expected Extraction:**

- Name: method name
- Type: `method`
- Parent: owning class/struct name
- Signature: params and return type
- Line number: accurate

---

### Pattern 7: Type Alias

Named type definitions.

| Language | Syntax Example |
|----------|---------------|
| TypeScript | `export type UserId = string;` |
| Go | `type UserId string` |
| Python | `UserId = str` (or `UserId: TypeAlias = str`) |
| Java | N/A (use wrapper class) |
| Rust | `pub type UserId = String;` |
| C/C++ | `typedef char* UserId;` or `using UserId = std::string;` |

**Expected Extraction:**

- Name: type alias name
- Type: `type`
- Line number: accurate

---

## Canonical Test File Template

Each language parser test fixture should include ALL patterns above. Template structure:

```
// [Language] Parser Test Fixture
// Line numbers must be predictable for validation

// === Pattern 1: Constants ===
// Line 5-10
[constant definitions]

// === Pattern 2: Functions ===
// Line 12-20
[function definitions]

// === Pattern 3: Classes/Structs ===
// Line 22-50
[class with methods]

// === Pattern 4: Interfaces/Traits ===
// Line 52-60
[interface definitions]

// === Pattern 5: Enums ===
// Line 62-70
[enum definitions]

// === Pattern 6: Methods ===
// (inside class from Pattern 3)

// === Pattern 7: Type Aliases ===
// Line 72-75
[type aliases]
```

---

## Validation Matrix

| Pattern | TS | Go | Py | Java | Rust | C | C++ |
|---------|----|----|----|----|------|---|-----|
| Constants | ✓ | ✓ | ✓ | ? | ? | ? | ? |
| Functions | ✓ | ✓ | ✓ | ? | ? | ? | ? |
| Classes | ✓ | N/A | ✓ | ? | ? | N/A | ? |
| Interfaces | ✓ | ✓ | ✓ | ? | ? | N/A | ? |
| Enums | ✓ | ✓ | ✓ | ? | ? | ? | ? |
| Methods | ✓ | ✓ | ✓ | ? | ? | N/A | ? |
| Type Aliases | ✓ | ✓ | ✓ | ? | ? | ? | ? |

Legend: ✓ = Tested, ? = Not tested, N/A = Not applicable

**Enhanced parsers (2026-01-28):** TypeScript, Go, Python

---

## Test Automation Script

```bash
#!/bin/bash
# scripts/test-all-parsers.sh

LANGUAGES="typescript go python java rust c cpp"
RESULTS_FILE="parser-validation-results.json"

echo "[]" > "$RESULTS_FILE"

for lang in $LANGUAGES; do
    echo "Testing $lang parser..."
    
    FIXTURE="cmd/ingest-codebase/languages/testdata/${lang}_test_fixture.*"
    EXPECTED="cmd/ingest-codebase/languages/testdata/${lang}_expected.json"
    
    if [ ! -f "$EXPECTED" ]; then
        echo "  Skipping: no expected.json"
        continue
    fi
    
    # Run parser and capture output
    RESULT=$(go run ./cmd/ingest-codebase \
        --path "$(dirname $FIXTURE)" \
        --space-id "parser-test-${lang}" \
        --dry-run \
        --verbose \
        --extract-symbols 2>&1 | grep "symbols extracted")
    
    SYMBOL_COUNT=$(echo "$RESULT" | grep -oP '\d+(?= symbols)' | head -1)
    EXPECTED_COUNT=$(jq '.expected_count' "$EXPECTED")
    
    if [ "$SYMBOL_COUNT" -ge "$EXPECTED_COUNT" ]; then
        STATUS="PASS"
    else
        STATUS="FAIL"
    fi
    
    echo "  $STATUS: Found $SYMBOL_COUNT symbols (expected $EXPECTED_COUNT)"
done
```

---

## Next Steps

1. Create test fixtures for each language using this template
2. Implement expected.json for each fixture
3. Run validation matrix
4. Fix parsers that fail validation
5. Add CI integration for parser regression testing
