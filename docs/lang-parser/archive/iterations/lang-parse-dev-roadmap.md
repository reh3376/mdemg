# Language Parser Development Roadmap

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


## Overview

Language parsers are **critical infrastructure** for MDEMG. The quality of symbol extraction directly determines:

1. Evidence quality in retrieval results (file:line references)
2. Learning edge formation between related code elements
3. Semantic understanding of codebase structure

**Current State:** 9 language parsers implemented, but symbol extraction depth varies significantly.

---

## Current Parser Inventory

| Language | Parser File | Symbol Types Extracted | Status |
|----------|-------------|----------------------|--------|
| TypeScript/JS | `typescript_parser.go` | classes, interfaces, types, enums, methods, functions, constants | **Enhanced 2026-01-28** |
| Go | `go_parser.go` | structs, interfaces, types, methods, functions, constants | **Enhanced 2026-01-28** |
| Python | `python_parser.go` | classes, enums, interfaces, types, methods, functions, constants | **Enhanced 2026-01-28** |
| Rust | `rust_parser.go` | functions, structs, enums, traits | Functional |
| Java | `java_parser.go` | classes, interfaces, methods, enums | Functional |
| C | `c_parser.go` | functions, structs, macros | Basic |
| C++ | `cpp_parser.go` | classes, functions, namespaces | Basic |
| CUDA | `cuda_parser.go` | kernels, device functions | Basic |
| SQL | `sql_parser.go` | tables, columns, procedures | Basic |
| JSON | `json_parser.go` | config keys/values | Basic |
| XML | `xml_parser.go` | elements, attributes | Basic |
| Markdown | `markdown_parser.go` | headings, sections | Documentation only |

---

## TypeScript Parser Enhancement (2026-01-28)

### Problem Identified

The original TypeScript parser only extracted:

- Constants (UPPER_CASE only)
- Top-level functions
- Arrow functions

This missed ~90% of meaningful code symbols in NestJS/React codebases:

- Classes (the backbone of NestJS)
- Interfaces/Types
- Methods (class members)
- Enums
- Decorators

### Solution Implemented

Enhanced `extractSymbols()` to capture:

```go
// New patterns added:
classPattern     - export class ClassName extends/implements
interfacePattern - export interface InterfaceName
typePattern      - export type TypeName =
enumPattern      - export enum EnumName
methodPattern    - methodName(args): Type (inside classes)
decoratorPattern - @DecoratorName( tracking
```

### Symbols Now Extracted

| Symbol Type | Example | Line Number |
|-------------|---------|-------------|
| `class` | `class UserService` | Yes |
| `interface` | `interface UserDto` | Yes |
| `type` | `type UserId = string` | Yes |
| `enum` | `enum UserStatus` | Yes |
| `method` | `UserService.findById()` | Yes |
| `function` | `function validateUser()` | Yes |
| `constant` | `const MAX_USERS` | Yes |

### Impact on Benchmark

Before: 6.7% of file refs had real line numbers
After: TBD (requires re-ingestion)

---

## Priority Improvements Needed

### P0: Critical (Blocking Evidence Quality)

#### 1. TypeScript: Decorator Argument Extraction

NestJS relies heavily on decorator metadata:

```typescript
@Controller('users')  // Extract 'users' as route prefix
@Injectable({ scope: Scope.REQUEST })  // Extract scope config
@Query(() => User)  // Extract GraphQL type
```

**Implementation:** Parse decorator arguments and store as `DocComment` or `Value`

#### 2. TypeScript: Property/Field Extraction

Class properties with decorators are critical for DTOs:

```typescript
@Field(() => ID)
id!: string;  // Extract as field with type annotation
```

**Implementation:** Add `fieldPattern` to capture class fields

#### 3. Python: Dataclass/Pydantic Field Extraction

```python
@dataclass
class User:
    id: int  # Extract field with type
    name: str = "default"  # Extract with default value
```

### P1: High Priority

#### 4. Go: Struct Field Extraction

```go
type User struct {
    ID   int    `json:"id"`  // Extract field with tag
    Name string `json:"name"`
}
```

#### 5. SQL: Index and Constraint Extraction

```sql
CREATE INDEX idx_user_email ON users(email);
ALTER TABLE users ADD CONSTRAINT fk_org FOREIGN KEY...
```

#### 6. TypeScript: Import/Export Graph

Track which symbols are imported from where:

```typescript
import { UserService } from './user.service';
export { UserDto } from './dto';
```

### P2: Medium Priority

#### 7. Generic AST Parser

Use tree-sitter for more accurate parsing instead of regex:

- Handles nested structures correctly
- Language-agnostic foundation
- Better multiline support

#### 8. JSDoc/TSDoc Comment Extraction

```typescript
/**
 * Finds a user by ID
 * @param id - The user ID
 * @returns The user or null
 */
async findById(id: string): Promise<User | null>
```

#### 9. React Hook Detection

```typescript
const [users, setUsers] = useState<User[]>([]);
const { data } = useQuery(GET_USERS);
```

---

## Testing Requirements

### Unit Tests per Parser

Each parser should have tests covering:

1. Basic symbol extraction
2. Line number accuracy
3. Nested structure handling
4. Edge cases (multiline, unicode, etc.)

### Integration Test: Re-ingestion Comparison

```bash
# Before changes
./ingest-codebase --path $REPO --space-id test-before --dry-run | grep "symbols extracted"

# After changes
./ingest-codebase --path $REPO --space-id test-after --dry-run | grep "symbols extracted"

# Compare symbol counts
```

### Benchmark Regression Test

Run 20-question benchmark before/after parser changes:

```bash
python3 docs/tests/whk-wms/run_benchmark_v4_agents.py --questions 20
```

---

## Architecture Notes

### Symbol Flow

```
Parser.extractSymbols()
  → CodeElement.Symbols[]
    → BatchIngestItem.Symbols[]
      → API /v1/memory/ingest/batch
        → symbols.Store.SaveSymbols()
          → Neo4j SymbolNode + DEFINES_SYMBOL edge
            → Retrieval GetSymbolsForMemoryNode()
              → Response.Evidence[]
```

### Key Files

- Parser implementations: `cmd/ingest-codebase/languages/*_parser.go`
- Parser interface: `cmd/ingest-codebase/languages/interface.go`
- Symbol storage: `internal/symbols/store.go`
- Evidence fetching: `internal/api/handlers.go` (handleRetrieve)

---

## Metrics to Track

| Metric | Target | Current |
|--------|--------|---------|
| Symbols per file (avg) | >10 | ~3 |
| Files with symbols (%) | >80% | ~40% |
| Line number accuracy | 100% | ~95% |
| Evidence coverage in retrieval | >70% | ~10% |

---

## Next Steps

1. Re-ingest whk-wms with enhanced TypeScript parser
2. Run 20-question benchmark to measure improvement
3. Prioritize P0 items based on benchmark gaps
4. Add parser unit tests for each enhancement
