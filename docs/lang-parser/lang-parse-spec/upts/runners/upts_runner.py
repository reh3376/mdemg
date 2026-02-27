#!/usr/bin/env python3
"""
Universal Parser Test Specification (UPTS) Test Runner

Validates parser implementations against UPTS specifications.
Language-agnostic - works with any parser that outputs standardized Symbol JSON.

NEW in v1.1.0: SHA256 hash verification for fixture integrity

Usage:
    # Validate single spec
    python upts_runner.py validate --spec specs/typescript.upts.json --parser ./parse
    
    # Validate all specs
    python upts_runner.py validate-all --spec-dir specs/ --parser ./parse
    
    # Skip hash verification
    python upts_runner.py validate --spec specs/rust.upts.json --parser ./parse --skip-hash
    
    # Add hashes to existing specs
    python upts_runner.py add-hashes --spec-dir specs/
    
    # Generate JSON report
    python upts_runner.py validate-all --spec-dir specs/ --parser ./parse --report report.json
    
    # Convert old expected.json to UPTS
    python upts_runner.py convert --input old.json --language python --fixture path/to/fixture.py

Parser Output Format:
    Your parser must output JSON with this structure:
    {
        "symbols": [
            {
                "name": "SymbolName",
                "type": "function|class|method|constant|interface|enum|type|struct",
                "line": 42,              // or "line_number"
                "exported": true,
                "parent": "ParentClass", // optional, for methods
                "signature": "...",      // optional
                "value": "...",          // optional, for constants
                "doc_comment": "..."     // optional
            }
        ]
    }

Author: reh3376
Version: 1.1.0
"""

from __future__ import annotations

import json
import shlex
import subprocess
import sys
import re
import hashlib
from pathlib import Path
from dataclasses import dataclass, field
from typing import Optional, List, Dict, Any, Tuple, Set
from enum import Enum
from datetime import datetime
import argparse


# ============================================================
# CONSTANTS
# ============================================================

UPTS_VERSION = "1.1.0"

# Type compatibility groups - semantic equivalences across languages
TYPE_COMPAT = {
    "class": {"class", "struct"},
    "struct": {"class", "struct"},
    "interface": {"interface", "trait", "protocol"},
    "trait": {"interface", "trait", "protocol"},
    "protocol": {"interface", "trait", "protocol"},
}


# ============================================================
# HASH UTILITIES
# ============================================================

def compute_sha256(filepath: Path) -> str:
    """Compute SHA256 hash of a file."""
    sha256_hash = hashlib.sha256()
    with open(filepath, "rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            sha256_hash.update(chunk)
    return sha256_hash.hexdigest()


def verify_fixture_hash(fixture_path: Path, expected_hash: Optional[str]) -> Tuple[bool, str]:
    """
    Verify fixture file integrity via SHA256 hash.
    
    Returns: (is_valid, message)
        - (True, "") if no hash specified or hash matches
        - (False, error_message) if hash mismatch
    """
    if not expected_hash:
        return True, ""  # No hash specified, skip verification
    
    if not fixture_path.exists():
        return False, f"Fixture not found: {fixture_path}"
    
    actual_hash = compute_sha256(fixture_path)
    
    if actual_hash != expected_hash:
        return False, (
            f"HASH MISMATCH: Fixture has been modified since spec generation\n"
            f"  Expected: {expected_hash}\n"
            f"  Actual:   {actual_hash}\n"
            f"  File:     {fixture_path}\n"
            f"  Action:   Regenerate spec from parser output, or run with --skip-hash"
        )
    
    return True, ""


# ============================================================
# DATA MODELS
# ============================================================

class Status(str, Enum):
    PASS = "pass"
    FAIL = "fail"
    WARN = "warn"
    SKIP = "skip"
    ERROR = "error"


@dataclass
class SymbolResult:
    """Result of validating one expected symbol"""
    name: str
    sym_type: str
    expected_line: int
    status: Status
    actual_line: Optional[int] = None
    actual_type: Optional[str] = None
    actual_parent: Optional[str] = None
    issues: List[str] = field(default_factory=list)
    pattern: Optional[str] = None
    tags: List[str] = field(default_factory=list)


@dataclass
class SpecResult:
    """Result of validating a UPTS spec"""
    spec_path: str
    language: str
    status: Status
    total_expected: int = 0
    matched: int = 0
    failed: int = 0
    warnings: int = 0
    skipped: int = 0
    symbol_results: List[SymbolResult] = field(default_factory=list)
    excluded_found: List[str] = field(default_factory=list)
    extra_symbols: List[Dict] = field(default_factory=list)
    error_message: Optional[str] = None
    duration_ms: float = 0.0
    hash_verified: bool = False
    hash_skipped: bool = False
    
    @property
    def pass_rate(self) -> float:
        return (self.matched / self.total_expected * 100) if self.total_expected else 0.0
    
    def to_dict(self) -> Dict:
        return {
            "spec_path": self.spec_path,
            "language": self.language,
            "status": self.status.value,
            "total_expected": self.total_expected,
            "matched": self.matched,
            "failed": self.failed,
            "pass_rate": round(self.pass_rate, 2),
            "duration_ms": round(self.duration_ms, 2),
            "hash_verified": self.hash_verified,
            "hash_skipped": self.hash_skipped,
            "failures": [
                {"name": r.name, "type": r.sym_type, "expected_line": r.expected_line,
                 "actual_line": r.actual_line, "issues": r.issues, "pattern": r.pattern}
                for r in self.symbol_results if r.status == Status.FAIL
            ],
            "excluded_found": self.excluded_found,
            "extra_symbol_count": len(self.extra_symbols),
            "error": self.error_message
        }


@dataclass
class TestReport:
    """Full test report"""
    timestamp: str
    total_specs: int
    passed: int
    failed: int
    errors: int
    results: List[SpecResult] = field(default_factory=list)
    
    @property
    def pass_rate(self) -> float:
        return (self.passed / self.total_specs * 100) if self.total_specs else 0.0
    
    def to_dict(self) -> Dict:
        return {
            "timestamp": self.timestamp,
            "upts_version": UPTS_VERSION,
            "summary": {
                "total_specs": self.total_specs,
                "passed": self.passed,
                "failed": self.failed,
                "errors": self.errors,
                "pass_rate": round(self.pass_rate, 2)
            },
            "results": [r.to_dict() for r in self.results]
        }


# ============================================================
# SPEC LOADER
# ============================================================

class UPTSLoader:
    """Load and validate UPTS specification files"""
    
    def __init__(self, spec_path: Path):
        self.spec_path = spec_path
        self.spec: Dict = {}
        self.errors: List[str] = []
    
    def load(self) -> bool:
        """Load spec file and validate structure"""
        try:
            self.spec = json.loads(self.spec_path.read_text(encoding='utf-8'))
        except json.JSONDecodeError as e:
            self.errors.append(f"Invalid JSON: {e}")
            return False
        except Exception as e:
            self.errors.append(f"Load error: {e}")
            return False
        
        # Validate required fields
        for field in ["upts_version", "language", "fixture", "expected"]:
            if field not in self.spec:
                self.errors.append(f"Missing required field: {field}")
        
        # Validate fixture
        fixture = self.spec.get("fixture", {})
        ftype = fixture.get("type")
        if ftype == "file" and "path" not in fixture:
            self.errors.append("Fixture type 'file' requires 'path'")
        elif ftype == "inline" and "content" not in fixture:
            self.errors.append("Fixture type 'inline' requires 'content'")
        
        # Validate symbols have required fields
        for i, sym in enumerate(self.spec.get("expected", {}).get("symbols", [])):
            for req in ["name", "type", "line"]:
                if req not in sym:
                    self.errors.append(f"Symbol {i} missing '{req}'")
        
        return len(self.errors) == 0
    
    @property
    def language(self) -> str:
        return self.spec.get("language", "unknown")
    
    @property
    def config(self) -> Dict:
        return self.spec.get("config", {})
    
    @property
    def fixture_path(self) -> Optional[Path]:
        fixture = self.spec.get("fixture", {})
        if fixture.get("type") == "file":
            return self.spec_path.parent / fixture.get("path", "")
        return None
    
    @property
    def fixture_hash(self) -> Optional[str]:
        """Get the expected SHA256 hash of the fixture, if specified."""
        return self.spec.get("fixture", {}).get("sha256")
    
    @property
    def fixture_content(self) -> Optional[str]:
        fixture = self.spec.get("fixture", {})
        if fixture.get("type") == "inline":
            return fixture.get("content")
        path = self.fixture_path
        if path and path.exists():
            return path.read_text(encoding='utf-8')
        return None
    
    @property
    def expected_symbols(self) -> List[Dict]:
        return self.spec.get("expected", {}).get("symbols", [])
    
    @property
    def excluded_symbols(self) -> List[Dict]:
        return self.spec.get("expected", {}).get("excluded", [])
    
    def update_fixture_hash(self) -> bool:
        """
        Compute and update the fixture hash in the spec.
        Saves the updated spec to disk.
        
        Returns: True if hash was added/updated, False on error
        """
        fixture_path = self.fixture_path
        if not fixture_path or not fixture_path.exists():
            return False
        
        new_hash = compute_sha256(fixture_path)
        self.spec["fixture"]["sha256"] = new_hash
        
        # Save updated spec
        self.spec_path.write_text(json.dumps(self.spec, indent=2))
        return True


# ============================================================
# PARSER INTERFACE
# ============================================================

class ParserInterface:
    """Interface for calling parser implementations"""
    
    def __init__(self, parser_cmd: str):
        self.parser_cmd = parser_cmd
    
    def parse(self, file_path: Path) -> Tuple[List[Dict], Optional[str]]:
        """
        Run parser on file and return normalized symbols.
        
        Returns: (symbols_list, error_message)
        """
        try:
            result = subprocess.run(
                shlex.split(self.parser_cmd) + [str(file_path)],
                capture_output=True,
                text=True,
                timeout=60
            )
            
            if result.returncode != 0:
                return [], f"Parser exit {result.returncode}: {result.stderr[:500]}"
            
            output = json.loads(result.stdout)
            symbols = self._normalize(output.get("symbols", []))
            return symbols, None
            
        except subprocess.TimeoutExpired:
            return [], "Parser timeout (60s)"
        except json.JSONDecodeError as e:
            return [], f"Invalid JSON output: {e}"
        except FileNotFoundError:
            return [], f"Parser not found: {self.parser_cmd}"
        except Exception as e:
            return [], f"Parser error: {e}"
    
    def _normalize(self, symbols: List[Dict]) -> List[Dict]:
        """Normalize field names"""
        normalized = []
        for sym in symbols:
            normalized.append({
                "name": sym.get("name", ""),
                "type": sym.get("type", ""),
                "line": sym.get("line") or sym.get("line_number", 0),
                "exported": sym.get("exported", True),
                "parent": sym.get("parent", ""),
                "signature": sym.get("signature", ""),
                "value": sym.get("value", ""),
                "doc_comment": sym.get("doc_comment", ""),
            })
        return normalized


class MockParser(ParserInterface):
    """Mock parser for testing the runner"""
    
    def __init__(self, mock_output: List[Dict]):
        super().__init__("mock")
        self.mock_output = mock_output
    
    def parse(self, file_path: Path) -> Tuple[List[Dict], Optional[str]]:
        return self._normalize(self.mock_output), None


# ============================================================
# VALIDATOR
# ============================================================

class Validator:
    """Validate parser output against UPTS spec"""
    
    def __init__(self, spec: UPTSLoader, parser: ParserInterface, skip_hash: bool = False):
        self.spec = spec
        self.parser = parser
        self.skip_hash = skip_hash
    
    def validate(self) -> SpecResult:
        """Run validation"""
        import time
        start = time.time()
        
        result = SpecResult(
            spec_path=str(self.spec.spec_path),
            language=self.spec.language,
            status=Status.PASS,
            total_expected=len(self.spec.expected_symbols)
        )
        
        # Get fixture path
        fixture_path = self.spec.fixture_path
        if not fixture_path:
            result.status = Status.ERROR
            result.error_message = "No fixture path"
            return result
        
        if not fixture_path.exists():
            result.status = Status.ERROR
            result.error_message = f"Fixture not found: {fixture_path}"
            return result
        
        # Verify fixture hash (unless skipped)
        if self.skip_hash:
            result.hash_skipped = True
        else:
            hash_valid, hash_error = verify_fixture_hash(fixture_path, self.spec.fixture_hash)
            if not hash_valid:
                result.status = Status.ERROR
                result.error_message = hash_error
                result.duration_ms = (time.time() - start) * 1000
                return result
            if self.spec.fixture_hash:
                result.hash_verified = True
        
        # Parse
        actual_symbols, error = self.parser.parse(fixture_path)
        if error:
            result.status = Status.ERROR
            result.error_message = error
            result.duration_ms = (time.time() - start) * 1000
            return result
        
        # Config
        line_tolerance = self.spec.config.get("line_tolerance", 2)
        validate_signature = self.spec.config.get("validate_signature", False)
        validate_value = self.spec.config.get("validate_value", False)
        validate_parent = self.spec.config.get("validate_parent", True)

        # Index actual symbols by name
        actual_by_name: Dict[str, List[Dict]] = {}
        for sym in actual_symbols:
            name = sym["name"]
            actual_by_name.setdefault(name, []).append(sym)

        matched_ids: Set[str] = set()

        # Validate each expected symbol
        for expected in self.spec.expected_symbols:
            sym_result = self._validate_symbol(expected, actual_by_name, matched_ids, line_tolerance,
                                               validate_signature, validate_value, validate_parent)
            result.symbol_results.append(sym_result)
            
            if sym_result.status == Status.PASS:
                result.matched += 1
            elif sym_result.status == Status.FAIL:
                result.failed += 1
            elif sym_result.status == Status.WARN:
                result.warnings += 1
            elif sym_result.status == Status.SKIP:
                result.skipped += 1
        
        # Check excluded
        for excl in self.spec.excluded_symbols:
            name = excl.get("name", "")
            pattern = excl.get("name_pattern")
            
            if name and name in actual_by_name:
                result.excluded_found.append(name)
            elif pattern:
                regex = re.compile(pattern)
                for sym_name in actual_by_name:
                    if regex.match(sym_name):
                        result.excluded_found.append(sym_name)
        
        # Track extra symbols
        all_matched_names = {r.name for r in result.symbol_results if r.status == Status.PASS}
        for name, syms in actual_by_name.items():
            if name not in all_matched_names:
                result.extra_symbols.extend(syms)

        # --- Schema-runner parity: enforce require_all_symbols ---
        require_all = self.spec.config.get("require_all_symbols", True)
        if require_all and result.matched < result.total_expected:
            unmatched = result.total_expected - result.matched - result.skipped
            if unmatched > 0:
                result.failed += unmatched
                result.error_message = (
                    result.error_message or ""
                ) + f" require_all_symbols: {unmatched} expected symbol(s) unmatched."

        # --- Schema-runner parity: enforce allow_extra_symbols ---
        allow_extra = self.spec.config.get("allow_extra_symbols", True)
        if not allow_extra and result.extra_symbols:
            extra_names = [s["name"] for s in result.extra_symbols[:5]]
            result.failed += 1
            result.error_message = (
                result.error_message or ""
            ) + f" allow_extra_symbols=false: {len(result.extra_symbols)} extra symbol(s) found: {extra_names}."

        # --- Schema-runner parity: explicit warning for unimplemented fields ---
        # relationships: present in 4 specs but runner can't validate yet.
        # NOT silently ignored — reported as warning. Does not affect pass/fail
        # because relationship validation is supplementary to symbol matching.
        if self.spec.spec.get("expected", {}).get("relationships"):
            result.warnings += 1
            result.error_message = (
                result.error_message or ""
            ) + " UNIMPLEMENTED: 'relationships' field present but validation not yet supported by runner."

        # Determine final status
        if result.failed > 0:
            result.status = Status.FAIL
        elif result.excluded_found:
            result.status = Status.WARN
        
        result.duration_ms = (time.time() - start) * 1000
        return result
    
    def _validate_symbol(
        self,
        expected: Dict,
        actual_by_name: Dict[str, List[Dict]],
        matched_ids: Set[str],
        line_tolerance: int,
        validate_signature: bool = False,
        validate_value: bool = False,
        validate_parent: bool = True
    ) -> SymbolResult:
        """Validate a single expected symbol"""
        name = expected["name"]
        exp_type = expected["type"]
        exp_line = expected["line"]
        exp_parent = expected.get("parent", "")
        optional = expected.get("optional", False)
        pattern = expected.get("pattern", "")
        tags = expected.get("tags", [])
        
        sym_result = SymbolResult(
            name=name,
            sym_type=exp_type,
            expected_line=exp_line,
            status=Status.FAIL,
            pattern=pattern,
            tags=tags
        )
        
        # Find candidates
        candidates = actual_by_name.get(name, [])
        if not candidates:
            if optional:
                sym_result.status = Status.SKIP
                sym_result.issues.append("optional symbol not found")
            else:
                sym_result.issues.append(f"not found in parser output")
            return sym_result
        
        # Find best match
        best_match = None
        best_score = -1
        
        for actual in candidates:
            score = 0
            
            # Type match
            actual_type = actual["type"]
            if actual_type == exp_type:
                score += 10
            elif exp_type in TYPE_COMPAT and actual_type in TYPE_COMPAT.get(exp_type, set()):
                score += 5
            
            # Line proximity
            line_diff = abs(actual["line"] - exp_line)
            if line_diff <= line_tolerance:
                score += (line_tolerance - line_diff + 1) * 2
            
            # Parent match
            if validate_parent and exp_parent:
                actual_parent = actual.get("parent", "")
                if actual_parent == exp_parent:
                    score += 3
                elif exp_parent in actual_parent or actual_parent in exp_parent:
                    score += 1  # Partial match (handles generics)
            
            if score > best_score:
                best_score = score
                best_match = actual
        
        if not best_match:
            if optional:
                sym_result.status = Status.SKIP
                sym_result.issues.append("optional symbol not matched")
            else:
                sym_result.issues.append("no unmatched candidate found")
            return sym_result
        
        # Mark as matched
        matched_ids.add(f"{best_match['name']}:{best_match['line']}")
        sym_result.actual_line = best_match["line"]
        sym_result.actual_type = best_match["type"]
        sym_result.actual_parent = best_match.get("parent")
        
        # Collect issues
        issues = []
        
        # Type check
        actual_type = best_match["type"]
        if actual_type != exp_type:
            if exp_type in TYPE_COMPAT and actual_type in TYPE_COMPAT.get(exp_type, set()):
                pass  # Compatible
            else:
                issues.append(f"type mismatch: expected '{exp_type}', got '{actual_type}'")
        
        # Line check
        line_diff = abs(best_match["line"] - exp_line)
        if line_diff > line_tolerance:
            issues.append(f"line {best_match['line']} outside tolerance (expected {exp_line}±{line_tolerance})")
        
        # Parent validation (only if enabled in config)
        actual_parent = best_match.get("parent", "")
        if validate_parent and exp_parent and actual_parent != exp_parent:
            # Allow partial match for generics (UserService vs UserService<R>)
            if not (exp_parent in actual_parent or actual_parent in exp_parent):
                issues.append(f"parent mismatch: expected '{exp_parent}', got '{actual_parent}'")
        
        # Signature validation
        if validate_signature:
            sig_contains = expected.get("signature_contains", [])
            actual_sig = best_match.get("signature", "").lower()
            for substr in sig_contains:
                if substr.lower() not in actual_sig:
                    issues.append(f"signature missing '{substr}'")
        
        # Value validation
        if validate_value:
            exp_value = expected.get("value")
            actual_value = best_match.get("value", "")
            if exp_value and actual_value != exp_value:
                issues.append(f"value mismatch: expected '{exp_value}', got '{actual_value}'")
        
        sym_result.issues = issues
        sym_result.status = Status.FAIL if issues else Status.PASS
        
        return sym_result


# ============================================================
# CONVERTER
# ============================================================

class Converter:
    """Convert old parser_expected.json to UPTS format"""
    
    PATTERN_MAP = {
        "constant": "P1_CONSTANT",
        "function": "P2_FUNCTION",
        "class": "P3_CLASS_STRUCT",
        "struct": "P3_CLASS_STRUCT",
        "interface": "P4_INTERFACE_TRAIT",
        "trait": "P4_INTERFACE_TRAIT",
        "enum": "P5_ENUM",
        "method": "P6_METHOD",
        "type": "P7_TYPE_ALIAS",
    }
    
    VARIANTS = {
        "typescript": [".ts", ".tsx"],
        "javascript": [".js", ".jsx", ".mjs"],
        "python": [".py", ".pyw"],
        "go": [".go"],
        "rust": [".rs"],
        "java": [".java"],
        "c": [".c", ".h"],
        "cpp": [".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx"],
        "cuda": [".cu", ".cuh"],
    }
    
    @classmethod
    def convert(cls, old: Dict, language: str, fixture_path: str) -> Dict:
        """Convert old format to UPTS"""
        # Detect line number field
        sample = old.get("symbols", [{}])[0]
        line_field = "line_number" if "line_number" in sample else "line"
        
        symbols = []
        for s in old.get("symbols", []):
            sym = {
                "name": s["name"],
                "type": s["type"],
                "line": s.get(line_field, s.get("line", 0)),
            }
            
            for f in ["exported", "parent", "signature", "value", "doc_comment"]:
                if f in s:
                    sym[f] = s[f]
            
            pattern = cls.PATTERN_MAP.get(s["type"])
            if pattern:
                sym["pattern"] = pattern
            
            symbols.append(sym)
        
        upts = {
            "upts_version": UPTS_VERSION,
            "language": language,
            "variants": cls.VARIANTS.get(language, [f".{language}"]),
            "metadata": {
                "created": datetime.now().strftime("%Y-%m-%d"),
                "description": f"Converted from {old.get('parser', language)}_expected.json",
                "parser_status": old.get("parser_status", "unknown"),
            },
            "config": {
                "line_tolerance": old.get("line_tolerance", 2),
                "require_all_symbols": True,
                "allow_extra_symbols": True
            },
            "fixture": {
                "type": "file",
                "path": fixture_path
            },
            "expected": {
                "symbol_count": {"min": old.get("expected_count", len(symbols))},
                "symbols": symbols
            }
        }
        
        if "enhancements_made" in old:
            upts["metadata"]["enhancements"] = old["enhancements_made"]
        
        return upts


# ============================================================
# REPORTER
# ============================================================

class Reporter:
    """Generate test reports"""
    
    def __init__(self, color: bool = True):
        self.color = color
    
    def _c(self, text: str, color: str) -> str:
        if not self.color:
            return text
        codes = {"green": "\033[92m", "red": "\033[91m", "yellow": "\033[93m",
                 "blue": "\033[94m", "bold": "\033[1m", "reset": "\033[0m"}
        return f"{codes.get(color, '')}{text}{codes['reset']}"
    
    def print_result(self, r: SpecResult):
        """Print single result"""
        icons = {Status.PASS: self._c("✓ PASS", "green"),
                 Status.FAIL: self._c("✗ FAIL", "red"),
                 Status.WARN: self._c("⚠ WARN", "yellow"),
                 Status.ERROR: self._c("✗ ERROR", "red")}
        
        print(f"\n{'='*60}")
        print(f"{self._c(r.language.upper(), 'bold')} Parser Test")
        print(f"Spec: {r.spec_path}")
        print(f"Status: {icons.get(r.status, r.status.value)}")
        print(f"Matched: {r.matched}/{r.total_expected} ({r.pass_rate:.1f}%)")
        
        # Hash status
        if r.hash_verified:
            print(f"Fixture Hash: {self._c('✓ Verified', 'green')}")
        elif r.hash_skipped:
            print(f"Fixture Hash: {self._c('⊘ Skipped', 'yellow')}")
        else:
            print(f"Fixture Hash: {self._c('○ Not specified', 'yellow')}")
        
        print(f"Duration: {r.duration_ms:.1f}ms")
        
        if r.error_message:
            print(f"{self._c('Error:', 'red')} {r.error_message}")
        
        failures = [s for s in r.symbol_results if s.status == Status.FAIL]
        if failures:
            print(f"\n{self._c('Failures:', 'red')}")
            for f in failures[:10]:
                print(f"  - {f.name} ({f.sym_type}): {', '.join(f.issues)}")
            if len(failures) > 10:
                print(f"  ... and {len(failures) - 10} more")
        
        if r.excluded_found:
            print(f"\n{self._c('Warning:', 'yellow')} Excluded symbols found: {', '.join(r.excluded_found)}")
    
    def print_summary(self, report: TestReport):
        """Print summary"""
        print(f"\n{'='*60}")
        print(f"{self._c('UPTS Test Summary', 'bold')}")
        print(f"{'='*60}")
        print(f"Total: {report.total_specs}")
        print(f"Passed: {self._c(str(report.passed), 'green')}")
        print(f"Failed: {self._c(str(report.failed), 'red')}")
        print(f"Errors: {report.errors}")
        print(f"Pass Rate: {report.pass_rate:.1f}%")
    
    def save_report(self, report: TestReport, path: Path):
        """Save JSON report"""
        path.write_text(json.dumps(report.to_dict(), indent=2))
        print(f"\nReport saved: {path}")


# ============================================================
# RUNNER
# ============================================================

class Runner:
    """Main test runner"""
    
    def __init__(self, parser: ParserInterface, skip_hash: bool = False):
        self.parser = parser
        self.skip_hash = skip_hash
        self.reporter = Reporter()
    
    def run_spec(self, spec_path: Path) -> SpecResult:
        """Run single spec"""
        loader = UPTSLoader(spec_path)
        
        if not loader.load():
            return SpecResult(
                spec_path=str(spec_path),
                language="unknown",
                status=Status.ERROR,
                error_message=f"Spec errors: {'; '.join(loader.errors)}"
            )
        
        validator = Validator(loader, self.parser, skip_hash=self.skip_hash)
        return validator.validate()
    
    def run_all(self, spec_dir: Path, pattern: str = "*.upts.json") -> TestReport:
        """Run all specs in directory"""
        specs = sorted(spec_dir.glob(pattern))
        
        results = []
        passed = failed = errors = 0
        
        for spec in specs:
            result = self.run_spec(spec)
            self.reporter.print_result(result)
            results.append(result)
            
            if result.status == Status.PASS:
                passed += 1
            elif result.status == Status.ERROR:
                errors += 1
            else:
                failed += 1
        
        report = TestReport(
            timestamp=datetime.now().isoformat(),
            total_specs=len(specs),
            passed=passed,
            failed=failed,
            errors=errors,
            results=results
        )
        
        self.reporter.print_summary(report)
        return report


# ============================================================
# HASH MANAGEMENT COMMANDS
# ============================================================

def add_hashes_to_specs(spec_dir: Path, pattern: str = "*.upts.json") -> Tuple[int, int]:
    """
    Add SHA256 hashes to all specs in a directory.
    
    Returns: (updated_count, error_count)
    """
    updated = 0
    errors = 0
    
    for spec_path in sorted(spec_dir.glob(pattern)):
        loader = UPTSLoader(spec_path)
        if not loader.load():
            print(f"  ✗ {spec_path.name}: Load error")
            errors += 1
            continue
        
        fixture_path = loader.fixture_path
        if not fixture_path or not fixture_path.exists():
            print(f"  ✗ {spec_path.name}: Fixture not found")
            errors += 1
            continue
        
        # Compute hash
        new_hash = compute_sha256(fixture_path)
        old_hash = loader.fixture_hash
        
        if old_hash == new_hash:
            print(f"  ○ {spec_path.name}: Hash unchanged")
            continue
        
        # Update spec
        if loader.update_fixture_hash():
            if old_hash:
                print(f"  ✓ {spec_path.name}: Hash updated")
            else:
                print(f"  ✓ {spec_path.name}: Hash added")
            updated += 1
        else:
            print(f"  ✗ {spec_path.name}: Update failed")
            errors += 1
    
    return updated, errors


def verify_all_hashes(spec_dir: Path, pattern: str = "*.upts.json") -> Tuple[int, int, int]:
    """
    Verify hashes for all specs without running full validation.
    
    Returns: (verified_count, missing_count, mismatch_count)
    """
    verified = 0
    missing = 0
    mismatched = 0
    
    for spec_path in sorted(spec_dir.glob(pattern)):
        loader = UPTSLoader(spec_path)
        if not loader.load():
            continue
        
        fixture_path = loader.fixture_path
        expected_hash = loader.fixture_hash
        
        if not expected_hash:
            print(f"  ○ {spec_path.name}: No hash specified")
            missing += 1
            continue
        
        if not fixture_path or not fixture_path.exists():
            print(f"  ✗ {spec_path.name}: Fixture not found")
            mismatched += 1
            continue
        
        actual_hash = compute_sha256(fixture_path)
        if actual_hash == expected_hash:
            print(f"  ✓ {spec_path.name}: Hash verified")
            verified += 1
        else:
            print(f"  ✗ {spec_path.name}: HASH MISMATCH")
            print(f"      Expected: {expected_hash[:16]}...")
            print(f"      Actual:   {actual_hash[:16]}...")
            mismatched += 1
    
    return verified, missing, mismatched


# ============================================================
# CLI
# ============================================================

def main():
    parser = argparse.ArgumentParser(description="UPTS Test Runner v1.1.0")
    sub = parser.add_subparsers(dest="cmd", required=True)
    
    # validate
    val = sub.add_parser("validate", help="Validate single spec")
    val.add_argument("--spec", type=Path, required=True)
    val.add_argument("--parser", type=str, required=True)
    val.add_argument("--report", type=Path)
    val.add_argument("--skip-hash", action="store_true", help="Skip fixture hash verification")
    
    # validate-all
    val_all = sub.add_parser("validate-all", help="Validate all specs")
    val_all.add_argument("--spec-dir", type=Path, required=True)
    val_all.add_argument("--parser", type=str, required=True)
    val_all.add_argument("--pattern", type=str, default="*.upts.json")
    val_all.add_argument("--report", type=Path)
    val_all.add_argument("--skip-hash", action="store_true", help="Skip fixture hash verification")
    
    # add-hashes
    add_hash = sub.add_parser("add-hashes", help="Add SHA256 hashes to specs")
    add_hash.add_argument("--spec-dir", type=Path, required=True)
    add_hash.add_argument("--pattern", type=str, default="*.upts.json")
    
    # verify-hashes
    verify_hash = sub.add_parser("verify-hashes", help="Verify fixture hashes without full validation")
    verify_hash.add_argument("--spec-dir", type=Path, required=True)
    verify_hash.add_argument("--pattern", type=str, default="*.upts.json")
    
    # convert
    conv = sub.add_parser("convert", help="Convert old format to UPTS")
    conv.add_argument("--input", type=Path, required=True)
    conv.add_argument("--language", type=str, required=True)
    conv.add_argument("--fixture", type=str, required=True)
    conv.add_argument("--output", type=Path)
    
    args = parser.parse_args()
    
    if args.cmd == "validate":
        runner = Runner(ParserInterface(args.parser), skip_hash=args.skip_hash)
        result = runner.run_spec(args.spec)
        runner.reporter.print_result(result)
        
        if args.report:
            report = TestReport(datetime.now().isoformat(), 1,
                              1 if result.status == Status.PASS else 0,
                              0 if result.status in (Status.PASS, Status.ERROR) else 1,
                              1 if result.status == Status.ERROR else 0,
                              [result])
            runner.reporter.save_report(report, args.report)
        
        sys.exit(0 if result.status == Status.PASS else 1)
    
    elif args.cmd == "validate-all":
        runner = Runner(ParserInterface(args.parser), skip_hash=args.skip_hash)
        report = runner.run_all(args.spec_dir, args.pattern)
        
        if args.report:
            runner.reporter.save_report(report, args.report)
        
        sys.exit(0 if report.failed == 0 and report.errors == 0 else 1)
    
    elif args.cmd == "add-hashes":
        print(f"Adding SHA256 hashes to specs in {args.spec_dir}")
        updated, errors = add_hashes_to_specs(args.spec_dir, args.pattern)
        print(f"\nUpdated: {updated}, Errors: {errors}")
        sys.exit(0 if errors == 0 else 1)
    
    elif args.cmd == "verify-hashes":
        print(f"Verifying fixture hashes in {args.spec_dir}")
        verified, missing, mismatched = verify_all_hashes(args.spec_dir, args.pattern)
        print(f"\nVerified: {verified}, Missing: {missing}, Mismatched: {mismatched}")
        sys.exit(0 if mismatched == 0 else 1)
    
    elif args.cmd == "convert":
        old = json.loads(args.input.read_text())
        upts = Converter.convert(old, args.language, args.fixture)
        
        if args.output:
            args.output.write_text(json.dumps(upts, indent=2))
            print(f"Converted: {args.output}")
        else:
            print(json.dumps(upts, indent=2))


if __name__ == "__main__":
    main()
