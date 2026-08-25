#!/usr/bin/env python3
"""MDEMG-USAGE-CORPUS-CURATE-001 Tier 1 unit tests.

Runs standalone (no pytest); exits 0 on all-pass. Wired to CI later via
`python3 scripts/test_mdemg_usage_curate.py`.

Sprint: docs/development/mdemg-usage-corpus-curate-001/
"""
from __future__ import annotations

import sys
from pathlib import Path

# Add scripts/ to sys.path so we can import the pipeline modules
sys.path.insert(0, str(Path(__file__).parent))

from mdemg_usage_export_docs import classify_surface, sha256_str  # noqa: E402
from mdemg_usage_curate import (  # noqa: E402
    build_qa_row,
    derive_feature_name,
    is_junk_path,
    is_nav_header,
    word_count,
)
from mdemg_usage_leak_audit import overlap, tri_grams  # noqa: E402


PASSES: list[str] = []
FAILS: list[str] = []


def check(name: str, ok: bool, detail: str = "") -> None:
    if ok:
        PASSES.append(name)
        print(f"  ✅ {name}")
    else:
        FAILS.append(f"{name}: {detail}")
        print(f"  ❌ {name}: {detail}")


def test_classify_surface() -> None:
    print("test_classify_surface:")
    check("features prefix", classify_surface("/docs/features/foo") == "features")
    check("user prefix", classify_surface("/docs/user/foo") == "user_api")
    check("api prefix", classify_surface("/docs/api/foo") == "user_api")
    check("cli-reference", classify_surface("claude-docs/cli-reference/foo") == "cli-help")
    check("CLAUDE.md", classify_surface("/CLAUDE.md") == "CLAUDE.md")
    check("unmatched", classify_surface("/random/path") is None)


def test_derive_feature_name() -> None:
    print("test_derive_feature_name:")
    check("feature with numeric prefix", derive_feature_name("mdemg-docs/features/beta-share/001__summary") == "beta share")
    check("feature no numeric", derive_feature_name("mdemg-docs/features/jiminy/foo") == "jiminy")
    check("user surface", derive_feature_name("/docs/user/foo") == "MDEMG user documentation")
    check("api surface", derive_feature_name("/docs/api/foo") == "MDEMG API documentation")
    check("unmatched path", derive_feature_name("/other/path") is None)


def test_is_nav_header() -> None:
    print("test_is_nav_header:")
    check("See also", is_nav_header("See also"))
    check("Related", is_nav_header("Related"))
    check("TOC lower", is_nav_header("toc"))
    check("real header not nav", not is_nav_header("How it works"))
    check("empty not nav", not is_nav_header(""))


def test_is_junk_path() -> None:
    print("test_is_junk_path:")
    check("template path junk", is_junk_path("mdemg-docs/features/template/001__summary"))
    check("real feature not junk", not is_junk_path("mdemg-docs/features/beta-share/001__summary"))


def test_word_count_and_sha() -> None:
    print("test_word_count_and_sha:")
    check("wc basic", word_count("one two three") == 3)
    check("wc empty", word_count("") == 0)
    check("sha256 shape", len(sha256_str("hi")) == 64)


def test_build_qa_row() -> None:
    print("test_build_qa_row:")
    node = {
        "node_id": "n_test_001",
        "path": "mdemg-docs/features/beta-share/002__how-it-works",
        "surface": "features",
        "section_header": "How it works",
        "content": "## How it works\n\nSome real content here about how the feature works, over the min_words threshold so this survives quality filter checks and is emitted.",
        "content_sha256": "abc",
    }
    row = build_qa_row(node, template_idx=0)
    check("row not None", row is not None)
    assert row is not None
    check("3 messages", len(row["messages"]) == 3)
    check("roles ordered", [m["role"] for m in row["messages"]] == ["system", "user", "assistant"])
    check("assistant is content", row["messages"][2]["content"] == node["content"])
    check("meta task_name", row["meta"]["task_name"] == "mdemg.usage")
    check("meta source_node_id", row["meta"]["source_node_id"] == "n_test_001")
    check("meta source_path", row["meta"]["source_path"] == node["path"])
    check("meta section_header", row["meta"]["section_header"] == "How it works")
    check("row_id stable prefix", row["meta"]["row_id"].startswith("mdemg_usage__"))
    check("user Q mentions header", "How it works" in row["messages"][1]["content"])

    # negative cases
    check("empty header → None", build_qa_row({**node, "section_header": ""}, 0) is None)
    check("empty content → None", build_qa_row({**node, "content": ""}, 0) is None)
    check("junk path → None", build_qa_row({**node, "path": "mdemg-docs/features/template/skeleton"}, 0) is None)
    check("nav header → None", build_qa_row({**node, "section_header": "See also"}, 0) is None)


def test_leak_audit_overlap() -> None:
    print("test_leak_audit_overlap:")
    a = "the quick brown fox jumps over the lazy dog"
    b = "the quick brown fox jumps over the lazy dog"
    check("identical == 1.0", overlap(a, b) == 1.0)

    c = "completely different words unrelated content sequences distinct rows"
    check("disjoint == 0.0", overlap(a, c) == 0.0)

    d = "the quick brown fox extra padding filler filler filler filler"
    ov = overlap(a, d)
    check("partial in (0,1)", 0.0 < ov < 1.0, f"got {ov}")

    check("tri_grams empty on <3 words", tri_grams("hi there") == set())


def main() -> int:
    print("MDEMG-USAGE-CORPUS-CURATE-001 — Tier 1 unit tests\n")
    test_classify_surface()
    test_derive_feature_name()
    test_is_nav_header()
    test_is_junk_path()
    test_word_count_and_sha()
    test_build_qa_row()
    test_leak_audit_overlap()

    print(f"\n{len(PASSES)} passed, {len(FAILS)} failed")
    if FAILS:
        print("Failures:")
        for f in FAILS:
            print(f"  - {f}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
