# Markdown Parser Test Fixture

Tests symbol extraction for Markdown documentation files.
Line numbers are predictable for UPTS validation.

## Overview

This document tests the Markdown parser's ability to extract:
- Headings at various levels
- Code blocks with language specifiers
- Links to external resources

### Nested Section

Content under a nested section to test parent tracking.

#### Deeply Nested

Even deeper nesting for hierarchy testing.

## Code Examples

Here are some code blocks:

```python
def hello():
    print("Hello, World!")
```

```go
func main() {
    fmt.Println("Hello, Go!")
}
```

```bash
echo "Hello from shell"
```

## External Links

[GitHub](https://github.com)
[Documentation](https://docs.example.com)
[API Reference](/api/reference)

## Configuration

### Environment Variables

Description of environment setup.

### Build Settings

Build configuration details.

## Summary

Final section with summary information.

### Subsection A

First subsection.

### Subsection B

Second subsection.
