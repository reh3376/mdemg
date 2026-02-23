#!/usr/bin/env python3
"""
Universal API Test Schema (UATS) Test Runner

Validates API endpoints against UATS specifications.
Language-agnostic - works with any HTTP API.

Usage:
    # Validate single spec
    python uats_runner.py validate --spec specs/health.uats.json --base-url http://localhost:8082

    # Validate all specs
    python uats_runner.py validate-all --spec-dir specs/ --base-url http://localhost:8082

    # Skip hash verification
    python uats_runner.py validate --spec specs/health.uats.json --base-url http://localhost:8082 --skip-hash

    # Add SHA256 hashes to spec files
    python uats_runner.py add-hashes --spec-dir specs/

    # Verify SHA256 hashes of spec files
    python uats_runner.py verify-hashes --spec-dir specs/

    # Generate JSON report
    python uats_runner.py validate-all --spec-dir specs/ --base-url http://localhost:8082 --report report.json

    # Run with auth token
    python uats_runner.py validate-all --spec-dir specs/ --base-url http://localhost:8082 --token "$API_TOKEN"

Author: reh3376
Version: 1.1.0
"""

from __future__ import annotations

import json
import sys
import re
import hashlib
import os
import time
from pathlib import Path
from dataclasses import dataclass, field
from typing import Optional, List, Dict, Any, Tuple, Set, Union
from enum import Enum
from datetime import datetime
from urllib.parse import urljoin, urlencode
import argparse

try:
    import requests
except ImportError:
    print("ERROR: 'requests' library required. Install with: pip install requests")
    sys.exit(1)

try:
    import jsonpath_ng
    from jsonpath_ng.ext import parse as jsonpath_parse
    HAS_JSONPATH = True
except ImportError:
    HAS_JSONPATH = False
    print("WARNING: 'jsonpath-ng' not installed. JSONPath assertions disabled.")
    print("         Install with: pip install jsonpath-ng")


# ============================================================
# CONSTANTS
# ============================================================

UATS_VERSION = "1.1.0"  # Added spec file hash verification


# ============================================================
# HASH UTILITIES
# ============================================================

def compute_sha256(data: Union[str, bytes, dict]) -> str:
    """Compute SHA256 hash of data."""
    if isinstance(data, dict):
        data = json.dumps(data, sort_keys=True, separators=(',', ':'))
    if isinstance(data, str):
        data = data.encode('utf-8')
    return hashlib.sha256(data).hexdigest()


def compute_file_sha256(filepath: Path) -> str:
    """Compute SHA256 hash of a file."""
    sha256_hash = hashlib.sha256()
    with open(filepath, "rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            sha256_hash.update(chunk)
    return sha256_hash.hexdigest()


def compute_spec_hash(spec: Dict) -> str:
    """
    Compute SHA256 hash of spec content, excluding the sha256 field itself.
    This allows the hash to be stored within the spec file.
    """
    # Deep copy to avoid modifying original
    spec_copy = json.loads(json.dumps(spec))

    # Remove sha256 from config if present
    if "config" in spec_copy and "sha256" in spec_copy["config"]:
        del spec_copy["config"]["sha256"]

    # Compute hash of normalized JSON
    normalized = json.dumps(spec_copy, sort_keys=True, separators=(',', ':'))
    return hashlib.sha256(normalized.encode('utf-8')).hexdigest()


def add_hash_to_spec(spec_path: Path) -> Tuple[bool, str]:
    """
    Add or update SHA256 hash in a spec file.
    Returns (success, message).
    """
    try:
        content = spec_path.read_text(encoding='utf-8')
        spec = json.loads(content)

        # Compute hash (excluding existing hash)
        new_hash = compute_spec_hash(spec)

        # Check if hash already exists and matches
        existing_hash = spec.get("config", {}).get("sha256")
        if existing_hash == new_hash:
            return True, f"Hash unchanged: {new_hash[:12]}..."

        # Add/update hash in config
        if "config" not in spec:
            spec["config"] = {}
        spec["config"]["sha256"] = new_hash

        # Write back with nice formatting
        spec_path.write_text(json.dumps(spec, indent=2) + "\n", encoding='utf-8')

        action = "Updated" if existing_hash else "Added"
        return True, f"{action} hash: {new_hash[:12]}..."
    except Exception as e:
        return False, f"Error: {e}"


def verify_spec_hash(spec_path: Path) -> Tuple[bool, str, Optional[str], Optional[str]]:
    """
    Verify SHA256 hash of a spec file.
    Returns (valid, message, expected_hash, actual_hash).
    """
    try:
        spec = json.loads(spec_path.read_text(encoding='utf-8'))

        expected_hash = spec.get("config", {}).get("sha256")
        if not expected_hash:
            return False, "No hash in spec", None, None

        actual_hash = compute_spec_hash(spec)

        if expected_hash == actual_hash:
            return True, "Hash valid", expected_hash, actual_hash
        else:
            return False, "Hash mismatch", expected_hash, actual_hash
    except Exception as e:
        return False, f"Error: {e}", None, None


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
class AssertionResult:
    """Result of a single assertion."""
    name: str
    path: str
    status: Status
    expected: Any = None
    actual: Any = None
    message: str = ""


@dataclass
class SpecResult:
    """Result of validating a UATS spec."""
    spec_path: str
    api_name: str
    method: str
    endpoint: str
    status: Status
    status_code: int = 0
    expected_status: Any = None
    response_time_ms: float = 0.0
    assertions: List[AssertionResult] = field(default_factory=list)
    passed: int = 0
    failed: int = 0
    warnings: int = 0
    skipped: int = 0
    error_message: Optional[str] = None
    hash_verified: bool = False
    hash_skipped: bool = False
    variant_name: Optional[str] = None
    
    @property
    def total_assertions(self) -> int:
        return self.passed + self.failed + self.warnings + self.skipped
    
    @property
    def pass_rate(self) -> float:
        total = self.passed + self.failed
        return (self.passed / total * 100) if total else 100.0
    
    def to_dict(self) -> Dict:
        return {
            "spec_path": self.spec_path,
            "api_name": self.api_name,
            "method": self.method,
            "endpoint": self.endpoint,
            "variant": self.variant_name,
            "status": self.status.value,
            "status_code": self.status_code,
            "expected_status": self.expected_status,
            "response_time_ms": round(self.response_time_ms, 2),
            "assertions": {
                "total": self.total_assertions,
                "passed": self.passed,
                "failed": self.failed,
                "warnings": self.warnings
            },
            "pass_rate": round(self.pass_rate, 2),
            "hash_verified": self.hash_verified,
            "failures": [
                {"name": a.name, "path": a.path, "expected": a.expected, 
                 "actual": a.actual, "message": a.message}
                for a in self.assertions if a.status == Status.FAIL
            ],
            "error": self.error_message
        }


@dataclass
class TestReport:
    """Full test report."""
    timestamp: str
    base_url: str
    total_specs: int
    total_variants: int
    passed: int
    failed: int
    errors: int
    results: List[SpecResult] = field(default_factory=list)
    
    @property
    def pass_rate(self) -> float:
        total = self.passed + self.failed
        return (self.passed / total * 100) if total else 0.0
    
    def to_dict(self) -> Dict:
        return {
            "timestamp": self.timestamp,
            "uats_version": UATS_VERSION,
            "base_url": self.base_url,
            "summary": {
                "total_specs": self.total_specs,
                "total_variants": self.total_variants,
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

class UATSLoader:
    """Load and validate UATS specification files."""
    
    def __init__(self, spec_path: Path):
        self.spec_path = spec_path
        self.spec: Dict = {}
        self.errors: List[str] = []
    
    def load(self) -> bool:
        """Load spec file and validate structure."""
        try:
            self.spec = json.loads(self.spec_path.read_text(encoding='utf-8'))
        except json.JSONDecodeError as e:
            self.errors.append(f"Invalid JSON: {e}")
            return False
        except Exception as e:
            self.errors.append(f"Load error: {e}")
            return False
        
        # Validate required fields
        for fld in ["uats_version", "api", "request", "expected"]:
            if fld not in self.spec:
                self.errors.append(f"Missing required field: {fld}")
        
        # Validate request
        request = self.spec.get("request", {})
        if "method" not in request:
            self.errors.append("request.method is required")
        if "path" not in request:
            self.errors.append("request.path is required")
        
        # Validate expected
        expected = self.spec.get("expected", {})
        if "status" not in expected:
            self.errors.append("expected.status is required")
        
        return len(self.errors) == 0
    
    @property
    def api_name(self) -> str:
        return self.spec.get("api", {}).get("name", "unknown")
    
    @property
    def metadata(self) -> Dict:
        return self.spec.get("metadata", {})

    @property
    def config(self) -> Dict:
        return self.spec.get("config", {})
    
    @property
    def auth(self) -> Dict:
        return self.spec.get("auth", {})
    
    @property
    def variables(self) -> Dict:
        return self.spec.get("variables", {})
    
    @property
    def request(self) -> Dict:
        return self.spec.get("request", {})
    
    @property
    def expected(self) -> Dict:
        return self.spec.get("expected", {})
    
    @property
    def setup(self) -> List[Dict]:
        return self.spec.get("setup", [])
    
    @property
    def teardown(self) -> List[Dict]:
        return self.spec.get("teardown", [])
    
    @property
    def variants(self) -> List[Dict]:
        return self.spec.get("variants", [])
    
    @property
    def captures(self) -> Dict:
        return self.spec.get("captures", {})

    @property
    def spec_hash(self) -> Optional[str]:
        """Get the SHA256 hash from config, if present."""
        return self.spec.get("config", {}).get("sha256")

    def verify_hash(self) -> Tuple[bool, str]:
        """
        Verify the spec file's SHA256 hash.
        Returns (valid, message).
        """
        expected = self.spec_hash
        if not expected:
            return True, "No hash (skipped)"

        actual = compute_spec_hash(self.spec)
        if expected == actual:
            return True, f"Hash valid: {actual[:12]}..."
        else:
            return False, f"Hash mismatch: expected {expected[:12]}..., got {actual[:12]}..."


# ============================================================
# VARIABLE RESOLVER
# ============================================================

class VariableResolver:
    """Resolve variables in strings and objects."""
    
    def __init__(self, variables: Dict, env_prefix: str = ""):
        self.variables = variables.copy()
        self.env_prefix = env_prefix
        self._resolve_variable_sources()
    
    def _resolve_variable_sources(self):
        """Resolve variables from env, generators, etc."""
        resolved = {}
        for name, value in self.variables.items():
            if isinstance(value, dict):
                if "env" in value:
                    env_name = value["env"]
                    resolved[name] = os.environ.get(env_name, value.get("default", ""))
                elif "generator" in value:
                    gen = value["generator"]
                    if gen == "uuid":
                        import uuid
                        resolved[name] = str(uuid.uuid4())
                    elif gen == "timestamp":
                        resolved[name] = datetime.utcnow().isoformat() + "Z"
                    elif gen == "timestamp_ms":
                        resolved[name] = str(int(time.time() * 1000))
                    elif gen == "random_int":
                        import random
                        resolved[name] = str(random.randint(1, 1000000))
                    elif gen == "random_string":
                        import random
                        import string
                        resolved[name] = ''.join(random.choices(string.ascii_lowercase, k=12))
                    else:
                        resolved[name] = value.get("default", "")
                else:
                    resolved[name] = str(value)
            else:
                resolved[name] = str(value)
        self.variables = resolved
    
    def resolve(self, obj: Any) -> Any:
        """Recursively resolve variables in object."""
        if isinstance(obj, str):
            return self._resolve_string(obj)
        elif isinstance(obj, dict):
            return {k: self.resolve(v) for k, v in obj.items()}
        elif isinstance(obj, list):
            return [self.resolve(item) for item in obj]
        return obj
    
    def _resolve_string(self, s: str) -> str:
        """Resolve {{variable}} and ${ENV_VAR} in string."""
        # Handle {{variable}}
        pattern = r'\{\{(\w+)\}\}'
        def replace_var(match):
            var_name = match.group(1)
            return self.variables.get(var_name, match.group(0))
        s = re.sub(pattern, replace_var, s)
        
        # Handle ${ENV_VAR}
        pattern = r'\$\{(\w+)\}'
        def replace_env(match):
            env_name = match.group(1)
            return os.environ.get(env_name, match.group(0))
        s = re.sub(pattern, replace_env, s)
        
        return s
    
    def set(self, name: str, value: Any):
        """Set a variable value."""
        self.variables[name] = str(value) if value is not None else ""


# ============================================================
# HTTP CLIENT
# ============================================================

class HTTPClient:
    """HTTP client for making API requests."""
    
    def __init__(self, base_url: str, timeout: int = 30, verify_ssl: bool = True):
        self.base_url = base_url.rstrip('/')
        self.timeout = timeout
        self.verify_ssl = verify_ssl
        self.session = requests.Session()
    
    def request(
        self,
        method: str,
        path: str,
        headers: Optional[Dict] = None,
        query: Optional[Dict] = None,
        body: Any = None,
        content_type: str = "application/json"
    ) -> Tuple[requests.Response, float]:
        """
        Make HTTP request.
        
        Returns: (response, response_time_ms)
        """
        url = f"{self.base_url}{path}"
        
        req_headers = {"Content-Type": content_type}
        if headers:
            req_headers.update(headers)
        
        # Prepare body
        data = None
        json_body = None
        if body is not None:
            if content_type == "application/json":
                json_body = body
            else:
                data = body if isinstance(body, str) else json.dumps(body)
        
        start_time = time.time()
        response = self.session.request(
            method=method.upper(),
            url=url,
            headers=req_headers,
            params=query,
            json=json_body,
            data=data,
            timeout=self.timeout,
            verify=self.verify_ssl
        )
        response_time_ms = (time.time() - start_time) * 1000
        
        return response, response_time_ms
    
    def set_auth(self, auth_config: Dict, token_override: Optional[str] = None):
        """Configure authentication."""
        auth_type = auth_config.get("type", "none")
        
        if auth_type == "none":
            return
        
        if auth_type == "bearer":
            bearer = auth_config.get("bearer", {})
            token = token_override or bearer.get("token", "")
            prefix = bearer.get("prefix", "Bearer")
            if token:
                self.session.headers["Authorization"] = f"{prefix} {token}"
        
        elif auth_type == "basic":
            basic = auth_config.get("basic", {})
            self.session.auth = (basic.get("username", ""), basic.get("password", ""))
        
        elif auth_type == "api_key":
            api_key = auth_config.get("api_key", {})
            name = api_key.get("name", "X-API-Key")
            value = token_override or api_key.get("value", "")
            location = api_key.get("in", "header")
            if location == "header":
                self.session.headers[name] = value
        
        elif auth_type == "custom":
            custom = auth_config.get("custom", {})
            for name, value in custom.get("headers", {}).items():
                self.session.headers[name] = value


# ============================================================
# ASSERTION ENGINE
# ============================================================

class AssertionEngine:
    """Evaluate assertions against response data."""
    
    def __init__(self, resolver: VariableResolver):
        self.resolver = resolver
    
    def check_status(self, actual: int, expected: Any) -> AssertionResult:
        """Check HTTP status code."""
        result = AssertionResult(
            name="status_code",
            path="status",
            status=Status.PASS,
            expected=expected,
            actual=actual
        )
        
        if isinstance(expected, int):
            if actual != expected:
                result.status = Status.FAIL
                result.message = f"Expected {expected}, got {actual}"
        elif isinstance(expected, list):
            if actual not in expected:
                result.status = Status.FAIL
                result.message = f"Expected one of {expected}, got {actual}"
        elif isinstance(expected, dict):
            min_status = expected.get("min", 100)
            max_status = expected.get("max", 599)
            if not (min_status <= actual <= max_status):
                result.status = Status.FAIL
                result.message = f"Expected {min_status}-{max_status}, got {actual}"
        
        return result
    
    def check_headers(self, actual_headers: Dict, expected_headers: Dict) -> List[AssertionResult]:
        """Check response headers."""
        results = []
        
        for name, expected in expected_headers.items():
            actual = actual_headers.get(name, actual_headers.get(name.lower()))
            
            result = AssertionResult(
                name=f"header:{name}",
                path=f"headers.{name}",
                status=Status.PASS,
                expected=expected,
                actual=actual
            )
            
            if isinstance(expected, str):
                expected = self.resolver.resolve(expected)
                if actual != expected:
                    result.status = Status.FAIL
                    result.message = f"Expected '{expected}', got '{actual}'"
            elif isinstance(expected, dict):
                result = self._check_matcher(f"header:{name}", f"headers.{name}", actual, expected)
            
            results.append(result)
        
        return results
    
    def check_body_assertions(self, body: Any, assertions: List[Dict]) -> List[AssertionResult]:
        """Check body assertions using JSONPath."""
        results = []
        
        for assertion in assertions:
            path = assertion.get("path", "$")
            name = assertion.get("name", path)
            optional = assertion.get("optional", False)
            
            # Extract value at path
            actual = self._extract_path(body, path)
            
            if actual is None and not optional:
                results.append(AssertionResult(
                    name=name,
                    path=path,
                    status=Status.FAIL,
                    expected="exists",
                    actual=None,
                    message=f"Path not found: {path}"
                ))
                continue
            
            if actual is None and optional:
                results.append(AssertionResult(
                    name=name,
                    path=path,
                    status=Status.SKIP,
                    message="Optional path not found"
                ))
                continue
            
            # Run assertion checks
            result = self._check_assertion(name, path, actual, assertion)
            results.append(result)
            
            # Capture value if requested
            if "capture_as" in assertion:
                self.resolver.set(assertion["capture_as"], actual)
        
        return results
    
    def check_body_exact(self, actual: Any, expected: Any, ignore_fields: List[str]) -> AssertionResult:
        """Check exact body match (after ignoring fields)."""
        result = AssertionResult(
            name="body_exact",
            path="body",
            status=Status.PASS,
            expected=expected,
            actual=actual
        )
        
        # Apply ignore_fields
        actual_filtered = self._filter_fields(actual, ignore_fields)
        expected_filtered = self._filter_fields(expected, ignore_fields)
        
        # Resolve variables in expected
        expected_resolved = self.resolver.resolve(expected_filtered)
        
        if actual_filtered != expected_resolved:
            result.status = Status.FAIL
            result.message = "Body does not match expected"
        
        return result
    
    def check_response_time(self, actual_ms: float, expected: Dict) -> AssertionResult:
        """Check response time."""
        result = AssertionResult(
            name="response_time",
            path="response_time_ms",
            status=Status.PASS,
            expected=expected,
            actual=actual_ms
        )
        
        max_ms = expected.get("max_ms")
        if max_ms and actual_ms > max_ms:
            result.status = Status.FAIL
            result.message = f"Response time {actual_ms:.0f}ms exceeds max {max_ms}ms"
        
        return result
    
    def _extract_path(self, data: Any, path: str) -> Any:
        """Extract value at JSONPath."""
        if not HAS_JSONPATH:
            # Fallback: simple dot notation
            if path == "$" or path == "":
                return data
            parts = path.lstrip("$.").split(".")
            current = data
            for part in parts:
                if isinstance(current, dict):
                    current = current.get(part)
                elif isinstance(current, list) and part.isdigit():
                    idx = int(part)
                    current = current[idx] if idx < len(current) else None
                else:
                    return None
            return current
        
        try:
            expr = jsonpath_parse(path)
            matches = expr.find(data)
            if not matches:
                return None
            return matches[0].value if len(matches) == 1 else [m.value for m in matches]
        except Exception:
            return None
    
    def _check_assertion(self, name: str, path: str, actual: Any, assertion: Dict) -> AssertionResult:
        """Check a single assertion."""
        result = AssertionResult(name=name, path=path, status=Status.PASS, actual=actual)
        
        # equals
        if "equals" in assertion:
            expected = self.resolver.resolve(assertion["equals"])
            result.expected = expected
            if actual != expected:
                result.status = Status.FAIL
                result.message = f"Expected '{expected}', got '{actual}'"
                return result
        
        # not_equals
        if "not_equals" in assertion:
            not_expected = self.resolver.resolve(assertion["not_equals"])
            if actual == not_expected:
                result.status = Status.FAIL
                result.expected = f"not {not_expected}"
                result.message = f"Should not equal '{not_expected}'"
                return result
        
        # type
        if "type" in assertion:
            expected_type = assertion["type"]
            result.expected = f"type:{expected_type}"
            type_map = {
                "string": str, "number": (int, float), "integer": int,
                "boolean": bool, "array": list, "object": dict, "null": type(None)
            }
            if expected_type in type_map:
                if not isinstance(actual, type_map[expected_type]):
                    result.status = Status.FAIL
                    result.message = f"Expected type {expected_type}, got {type(actual).__name__}"
                    return result
        
        # contains
        if "contains" in assertion:
            substr = self.resolver.resolve(assertion["contains"])
            result.expected = f"contains '{substr}'"
            if not isinstance(actual, str) or substr not in actual:
                result.status = Status.FAIL
                result.message = f"Does not contain '{substr}'"
                return result
        
        # regex
        if "regex" in assertion:
            pattern = assertion["regex"]
            result.expected = f"matches /{pattern}/"
            if not isinstance(actual, str) or not re.match(pattern, actual):
                result.status = Status.FAIL
                result.message = f"Does not match pattern /{pattern}/"
                return result
        
        # in (enum)
        if "in" in assertion:
            allowed = assertion["in"]
            result.expected = f"one of {allowed}"
            if actual not in allowed:
                result.status = Status.FAIL
                result.message = f"Value '{actual}' not in {allowed}"
                return result
        
        # range
        if "range" in assertion:
            rng = assertion["range"]
            min_val = rng.get("min")
            max_val = rng.get("max")
            result.expected = f"range [{min_val}, {max_val}]"
            if min_val is not None and actual < min_val:
                result.status = Status.FAIL
                result.message = f"Value {actual} below minimum {min_val}"
                return result
            if max_val is not None and actual > max_val:
                result.status = Status.FAIL
                result.message = f"Value {actual} above maximum {max_val}"
                return result
        
        # length
        if "length" in assertion:
            length = assertion["length"]
            actual_len = len(actual) if hasattr(actual, '__len__') else 0
            if "equals" in length and actual_len != length["equals"]:
                result.status = Status.FAIL
                result.expected = f"length == {length['equals']}"
                result.message = f"Length {actual_len} != {length['equals']}"
                return result
            if "min" in length and actual_len < length["min"]:
                result.status = Status.FAIL
                result.expected = f"length >= {length['min']}"
                result.message = f"Length {actual_len} < {length['min']}"
                return result
            if "max" in length and actual_len > length["max"]:
                result.status = Status.FAIL
                result.expected = f"length <= {length['max']}"
                result.message = f"Length {actual_len} > {length['max']}"
                return result
        
        # exists
        if "exists" in assertion:
            should_exist = assertion["exists"]
            result.expected = "exists" if should_exist else "not exists"
            exists = actual is not None
            if should_exist and not exists:
                result.status = Status.FAIL
                result.message = "Field does not exist"
                return result
            if not should_exist and exists:
                result.status = Status.FAIL
                result.message = "Field should not exist"
                return result
        
        return result
    
    def _check_matcher(self, name: str, path: str, actual: Any, matcher: Dict) -> AssertionResult:
        """Check a matcher object."""
        return self._check_assertion(name, path, actual, matcher)
    
    def _filter_fields(self, obj: Any, ignore_paths: List[str]) -> Any:
        """Remove ignored fields from object."""
        if not ignore_paths or not isinstance(obj, dict):
            return obj
        
        result = {}
        for key, value in obj.items():
            full_path = f"$.{key}"
            if full_path not in ignore_paths and key not in ignore_paths:
                if isinstance(value, dict):
                    result[key] = self._filter_fields(value, ignore_paths)
                else:
                    result[key] = value
        return result


# ============================================================
# VALIDATOR
# ============================================================

class Validator:
    """Validate API responses against UATS spec."""
    
    def __init__(
        self,
        spec: UATSLoader,
        client: HTTPClient,
        token: Optional[str] = None,
        skip_hash: bool = False
    ):
        self.spec = spec
        self.client = client
        self.token = token
        self.skip_hash = skip_hash
        self.resolver = VariableResolver(spec.variables)
        self.engine = AssertionEngine(self.resolver)
        self.last_response_body = None
    
    def validate(self, variant: Optional[Dict] = None) -> SpecResult:
        """Run validation."""
        # Merge variant overrides
        request = self._merge_variant(self.spec.request, variant.get("request") if variant else None)
        expected = self._merge_variant(self.spec.expected, variant.get("expected") if variant else None)
        
        result = SpecResult(
            spec_path=str(self.spec.spec_path),
            api_name=self.spec.api_name,
            method=request.get("method", "GET"),
            endpoint=request.get("path", "/"),
            status=Status.PASS,
            expected_status=expected.get("status"),
            variant_name=variant.get("name") if variant else None
        )
        
        # Resolve variables in request
        request = self.resolver.resolve(request)
        
        # Build path with path_params
        path = request.get("path", "/")
        path_params = request.get("path_params", {})
        for name, value in path_params.items():
            path = path.replace(f"{{{name}}}", str(value))
        result.endpoint = path
        
        # Configure auth
        auth = self.resolver.resolve(self.spec.auth)
        self.client.set_auth(auth, self.token)
        
        # Make request
        try:
            response, response_time = self.client.request(
                method=request.get("method", "GET"),
                path=path,
                headers=request.get("headers"),
                query=request.get("query"),
                body=request.get("body"),
                content_type=request.get("content_type", "application/json")
            )
        except requests.exceptions.ConnectionError as e:
            result.status = Status.ERROR
            result.error_message = f"Connection failed: {e}"
            return result
        except requests.exceptions.Timeout as e:
            result.status = Status.ERROR
            result.error_message = f"Request timeout: {e}"
            return result
        except Exception as e:
            result.status = Status.ERROR
            result.error_message = f"Request error: {e}"
            return result
        
        result.status_code = response.status_code
        result.response_time_ms = response_time
        
        # Parse response body
        try:
            body = response.json() if response.text else None
        except json.JSONDecodeError:
            body = response.text

        # Store parsed body for sequential mode access
        self.last_response_body = body

        # Check status code
        status_result = self.engine.check_status(response.status_code, expected.get("status"))
        result.assertions.append(status_result)
        if status_result.status == Status.FAIL:
            result.failed += 1
        else:
            result.passed += 1
        
        # Check headers
        if "headers" in expected:
            header_results = self.engine.check_headers(dict(response.headers), expected["headers"])
            for r in header_results:
                result.assertions.append(r)
                if r.status == Status.FAIL:
                    result.failed += 1
                elif r.status == Status.WARN:
                    result.warnings += 1
                else:
                    result.passed += 1
        
        # Check body assertions
        if "body_assertions" in expected and body is not None:
            body_results = self.engine.check_body_assertions(body, expected["body_assertions"])
            for r in body_results:
                result.assertions.append(r)
                if r.status == Status.FAIL:
                    result.failed += 1
                elif r.status == Status.WARN:
                    result.warnings += 1
                elif r.status == Status.SKIP:
                    result.skipped += 1
                else:
                    result.passed += 1
        
        # Check exact body match
        if "body" in expected and body is not None:
            ignore_fields = self.spec.config.get("ignore_fields", [])
            body_result = self.engine.check_body_exact(body, expected["body"], ignore_fields)
            result.assertions.append(body_result)
            if body_result.status == Status.FAIL:
                result.failed += 1
            else:
                result.passed += 1
        
        # Check response time
        if "response_time" in expected:
            time_result = self.engine.check_response_time(response_time, expected["response_time"])
            result.assertions.append(time_result)
            if time_result.status == Status.FAIL:
                result.failed += 1
            else:
                result.passed += 1
        
        # Capture values
        for name, capture in self.spec.captures.items():
            source = capture.get("from", "body")
            path = capture.get("path", "$")
            
            if source == "body" and body:
                value = self.engine._extract_path(body, path)
                self.resolver.set(name, value)
            elif source == "header":
                value = response.headers.get(path)
                self.resolver.set(name, value)
            elif source == "status":
                self.resolver.set(name, response.status_code)
        
        # Determine final status
        if result.failed > 0:
            result.status = Status.FAIL
        elif result.warnings > 0:
            result.status = Status.WARN
        
        return result
    
    def _merge_variant(self, base: Dict, override: Optional[Dict]) -> Dict:
        """Deep merge variant overrides into base."""
        if not override:
            return base.copy()
        
        result = base.copy()
        for key, value in override.items():
            if isinstance(value, dict) and key in result and isinstance(result[key], dict):
                result[key] = self._merge_variant(result[key], value)
            else:
                result[key] = value
        return result


# ============================================================
# REPORTER
# ============================================================

class Reporter:
    """Generate test reports."""
    
    def __init__(self, color: bool = True):
        self.color = color
    
    def _c(self, text: str, color: str) -> str:
        if not self.color:
            return text
        codes = {"green": "\033[92m", "red": "\033[91m", "yellow": "\033[93m",
                 "blue": "\033[94m", "cyan": "\033[96m", "bold": "\033[1m", "reset": "\033[0m"}
        return f"{codes.get(color, '')}{text}{codes['reset']}"
    
    def print_result(self, r: SpecResult):
        """Print single result."""
        icons = {Status.PASS: self._c("✓ PASS", "green"),
                 Status.FAIL: self._c("✗ FAIL", "red"),
                 Status.WARN: self._c("⚠ WARN", "yellow"),
                 Status.ERROR: self._c("✗ ERROR", "red")}
        
        variant_str = f" [{r.variant_name}]" if r.variant_name else ""
        
        print(f"\n{'='*60}")
        print(f"{self._c(r.api_name, 'bold')}{variant_str}")
        print(f"{r.method} {r.endpoint}")
        print(f"Status: {icons.get(r.status, r.status.value)}")
        print(f"HTTP: {r.status_code} (expected: {r.expected_status})")
        print(f"Response Time: {r.response_time_ms:.0f}ms")
        print(f"Assertions: {r.passed}/{r.total_assertions} passed")
        
        if r.error_message:
            print(f"{self._c('Error:', 'red')} {r.error_message}")
        
        failures = [a for a in r.assertions if a.status == Status.FAIL]
        if failures:
            print(f"\n{self._c('Failures:', 'red')}")
            for f in failures[:10]:
                print(f"  - {f.name}: {f.message}")
                if f.expected:
                    print(f"    Expected: {f.expected}")
                if f.actual is not None:
                    actual_str = str(f.actual)[:100]
                    print(f"    Actual: {actual_str}")
            if len(failures) > 10:
                print(f"  ... and {len(failures) - 10} more")
    
    def print_summary(self, report: TestReport):
        """Print summary."""
        print(f"\n{'='*60}")
        print(f"{self._c('UATS Test Summary', 'bold')}")
        print(f"{'='*60}")
        print(f"Base URL: {report.base_url}")
        print(f"Total Specs: {report.total_specs}")
        print(f"Total Variants: {report.total_variants}")
        print(f"Passed: {self._c(str(report.passed), 'green')}")
        print(f"Failed: {self._c(str(report.failed), 'red')}")
        print(f"Errors: {report.errors}")
        print(f"Pass Rate: {report.pass_rate:.1f}%")
    
    def save_report(self, report: TestReport, path: Path):
        """Save JSON report."""
        path.write_text(json.dumps(report.to_dict(), indent=2))
        print(f"\nReport saved: {path}")


# ============================================================
# RUNNER
# ============================================================

class Runner:
    """Main test runner."""
    
    def __init__(self, base_url: str, token: Optional[str] = None,
                 skip_hash: bool = False, timeout: int = 30,
                 include_tag: Optional[str] = None,
                 exclude_tag: Optional[str] = None):
        self.base_url = base_url
        self.token = token
        self.skip_hash = skip_hash
        self.timeout = timeout
        # Support comma-separated tags: "embedding_required,unts" → ["embedding_required", "unts"]
        self.include_tags = [t.strip() for t in include_tag.split(",")] if include_tag else []
        self.exclude_tags = [t.strip() for t in exclude_tag.split(",")] if exclude_tag else []
        self.reporter = Reporter()
    
    def run_spec(self, spec_path: Path) -> List[SpecResult]:
        """Run single spec (including variants)."""
        loader = UATSLoader(spec_path)

        if not loader.load():
            return [SpecResult(
                spec_path=str(spec_path),
                api_name="unknown",
                method="",
                endpoint="",
                status=Status.ERROR,
                error_message=f"Spec errors: {'; '.join(loader.errors)}"
            )]

        # Verify spec file hash (unless skipped)
        if not self.skip_hash:
            hash_valid, hash_msg = loader.verify_hash()
            if not hash_valid and "No hash" not in hash_msg:
                return [SpecResult(
                    spec_path=str(spec_path),
                    api_name=loader.api_name,
                    method="",
                    endpoint="",
                    status=Status.ERROR,
                    error_message=f"Spec hash verification failed: {hash_msg}",
                    hash_verified=False
                )]

        # CLI base_url takes precedence over spec's base_url
        # This allows testing against different environments (ports are dynamic)
        spec_base_url = loader.spec.get("api", {}).get("base_url", "")
        if spec_base_url.startswith("$"):
            # Resolve from environment
            env_var = spec_base_url.strip("${}")
            spec_base_url = os.environ.get(env_var, "")

        # Use CLI base_url if provided, otherwise fall back to spec
        effective_base_url = self.base_url if self.base_url else spec_base_url

        client = HTTPClient(
            base_url=effective_base_url,
            timeout=loader.config.get("timeout_ms", self.timeout * 1000) // 1000,
            verify_ssl=loader.config.get("verify_ssl", True)
        )
        
        results = []

        # Track hash verification status
        hash_verified = False
        if not self.skip_hash and loader.spec_hash:
            hash_valid, _ = loader.verify_hash()
            hash_verified = hash_valid

        # Check if entire spec is skipped (metadata.skip)
        if loader.metadata.get("skip", False):
            skip_reason = loader.metadata.get("skip_reason", "Skipped by metadata.skip=true")
            result = SpecResult(
                spec_path=str(spec_path),
                api_name=loader.api_name,
                method=loader.request.get("method", "GET"),
                endpoint=loader.request.get("path", "/"),
                status=Status.SKIP,
                error_message=skip_reason,
                hash_verified=hash_verified
            )
            self.reporter.print_result(result)
            results.append(result)
            return results

        # Run main spec
        validator = Validator(loader, client, self.token, self.skip_hash)
        result = validator.validate()
        result.hash_verified = hash_verified
        self.reporter.print_result(result)
        results.append(result)

        # Run variants
        sequential = loader.config.get("sequential", False)
        prev_response_body = None

        for variant in loader.variants:
            if variant.get("skip", False):
                continue

            validator = Validator(loader, client, self.token, self.skip_hash)

            # For sequential mode, inject prev_response fields as variables
            if sequential and prev_response_body is not None:
                if isinstance(prev_response_body, dict):
                    for k, v in prev_response_body.items():
                        validator.resolver.set(f"prev_{k}", v)

            result = validator.validate(variant)
            result.hash_verified = hash_verified
            self.reporter.print_result(result)
            results.append(result)

            # Capture response body for next variant in sequential mode
            if sequential:
                prev_response_body = validator.last_response_body

        return results
    
    def run_all(self, spec_dir: Path, pattern: str = "*.uats.json") -> TestReport:
        """Run all specs in directory."""
        specs = sorted(spec_dir.glob(pattern))

        # Tag-based filtering (supports comma-separated values)
        if self.include_tags or self.exclude_tags:
            filtered = []
            for sp in specs:
                try:
                    with open(sp) as f:
                        cfg = json.load(f).get("config", {})
                    tags = cfg.get("tags", [])
                except Exception:
                    tags = []
                if self.include_tags and not any(t in tags for t in self.include_tags):
                    continue
                if self.exclude_tags and any(t in tags for t in self.exclude_tags):
                    continue
                filtered.append(sp)
            specs = filtered

        all_results = []
        passed = failed = errors = skipped = 0

        for spec in specs:
            results = self.run_spec(spec)
            all_results.extend(results)

            for result in results:
                if result.status == Status.PASS:
                    passed += 1
                elif result.status == Status.SKIP:
                    skipped += 1
                elif result.status == Status.ERROR:
                    errors += 1
                else:
                    failed += 1
        
        report = TestReport(
            timestamp=datetime.now().isoformat(),
            base_url=self.base_url,
            total_specs=len(specs),
            total_variants=len(all_results),
            passed=passed,
            failed=failed,
            errors=errors,
            results=all_results
        )
        
        self.reporter.print_summary(report)
        return report


# ============================================================
# CLI
# ============================================================

def main():
    parser = argparse.ArgumentParser(description=f"UATS Test Runner v{UATS_VERSION}")
    sub = parser.add_subparsers(dest="cmd", required=True)

    # validate
    val = sub.add_parser("validate", help="Validate single spec")
    val.add_argument("--spec", type=Path, required=True)
    val.add_argument("--base-url", type=str, required=True)
    val.add_argument("--token", type=str, help="Auth token (overrides spec)")
    val.add_argument("--timeout", type=int, default=30, help="Timeout in seconds")
    val.add_argument("--report", type=Path)
    val.add_argument("--skip-hash", action="store_true", help="Skip spec file hash verification")

    # validate-all
    val_all = sub.add_parser("validate-all", help="Validate all specs")
    val_all.add_argument("--spec-dir", type=Path, required=True)
    val_all.add_argument("--base-url", type=str, required=True)
    val_all.add_argument("--pattern", type=str, default="*.uats.json")
    val_all.add_argument("--token", type=str, help="Auth token (overrides spec)")
    val_all.add_argument("--timeout", type=int, default=30)
    val_all.add_argument("--report", type=Path)
    val_all.add_argument("--skip-hash", action="store_true", help="Skip spec file hash verification")
    val_all.add_argument("--include-tag", type=str, dest="include_tag",
                         help="Only run specs with this tag (comma-separated for multiple)")
    val_all.add_argument("--exclude-tag", type=str, dest="exclude_tag",
                         help="Skip specs with this tag (comma-separated for multiple)")

    # add-hashes
    add_hash = sub.add_parser("add-hashes", help="Add SHA256 hashes to spec files")
    add_hash.add_argument("--spec-dir", type=Path, required=True)
    add_hash.add_argument("--pattern", type=str, default="*.uats.json")

    # verify-hashes
    verify_hash = sub.add_parser("verify-hashes", help="Verify SHA256 hashes of spec files")
    verify_hash.add_argument("--spec-dir", type=Path, required=True)
    verify_hash.add_argument("--pattern", type=str, default="*.uats.json")

    args = parser.parse_args()
    
    if args.cmd == "validate":
        runner = Runner(args.base_url, args.token, args.skip_hash, args.timeout)
        results = runner.run_spec(args.spec)
        
        if args.report:
            passed = sum(1 for r in results if r.status == Status.PASS)
            failed = sum(1 for r in results if r.status == Status.FAIL)
            errors = sum(1 for r in results if r.status == Status.ERROR)
            report = TestReport(
                datetime.now().isoformat(), args.base_url,
                1, len(results), passed, failed, errors, results
            )
            runner.reporter.save_report(report, args.report)
        
        has_failures = any(r.status in (Status.FAIL, Status.ERROR) for r in results)
        sys.exit(1 if has_failures else 0)
    
    elif args.cmd == "validate-all":
        runner = Runner(args.base_url, args.token, args.skip_hash, args.timeout,
                        include_tag=getattr(args, 'include_tag', None),
                        exclude_tag=getattr(args, 'exclude_tag', None))
        report = runner.run_all(args.spec_dir, args.pattern)

        if args.report:
            runner.reporter.save_report(report, args.report)

        sys.exit(0 if report.failed == 0 and report.errors == 0 else 1)

    elif args.cmd == "add-hashes":
        specs = sorted(args.spec_dir.glob(args.pattern))
        if not specs:
            print(f"No specs found matching {args.pattern} in {args.spec_dir}")
            sys.exit(1)

        print(f"Adding SHA256 hashes to {len(specs)} spec files...\n")
        success_count = 0
        for spec_path in specs:
            success, message = add_hash_to_spec(spec_path)
            status = "✓" if success else "✗"
            print(f"  {status} {spec_path.name}: {message}")
            if success:
                success_count += 1

        print(f"\n{success_count}/{len(specs)} specs updated")
        sys.exit(0 if success_count == len(specs) else 1)

    elif args.cmd == "verify-hashes":
        specs = sorted(args.spec_dir.glob(args.pattern))
        if not specs:
            print(f"No specs found matching {args.pattern} in {args.spec_dir}")
            sys.exit(1)

        print(f"Verifying SHA256 hashes for {len(specs)} spec files...\n")
        valid_count = 0
        no_hash_count = 0
        for spec_path in specs:
            valid, message, expected, actual = verify_spec_hash(spec_path)
            if valid:
                if "No hash" in message:
                    print(f"  - {spec_path.name}: {message}")
                    no_hash_count += 1
                else:
                    print(f"  ✓ {spec_path.name}: {message}")
                    valid_count += 1
            else:
                print(f"  ✗ {spec_path.name}: {message}")
                if expected and actual:
                    print(f"      Expected: {expected}")
                    print(f"      Actual:   {actual}")

        print(f"\nResults: {valid_count} valid, {no_hash_count} no hash, {len(specs) - valid_count - no_hash_count} invalid")
        # Exit with error only if there are mismatches (not for missing hashes)
        has_invalid = len(specs) - valid_count - no_hash_count > 0
        sys.exit(1 if has_invalid else 0)


if __name__ == "__main__":
    main()
