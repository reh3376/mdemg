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

from mdemg_usage_export_docs import (  # noqa: E402
    classify_surface,
    sha256_str,
    build_query,
)
from mdemg_usage_curate import (  # noqa: E402
    build_qa_row,
    derive_feature_name,
    is_junk_path,
    is_nav_header,
    is_valid_mdemg_docs_path,
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
    """Task #148: classify_surface anchors to mdemg-docs/ prefix."""
    print("test_classify_surface:")
    # Positive: mdemg-docs/-anchored paths for each sub-surface
    check("mdemg-docs features", classify_surface("mdemg-docs/features/beta-share/000__whole-file") == "features")
    check("mdemg-docs user", classify_surface("mdemg-docs/user/multi-instance/000__whole-file") == "user_api")
    check("mdemg-docs api", classify_surface("mdemg-docs/api/inventory/000__whole-file") == "user_api")
    check("mdemg-docs cli-help", classify_surface("mdemg-docs/cli-help/adapter/000__cli-adapter") == "cli-help")
    check("mdemg-docs claude", classify_surface("mdemg-docs/claude/architecture-notes/000__whole-file") == "CLAUDE.md")

    # Task #148 negative: no-prefix, wrong-prefix, and leak-class paths must all reject
    check("legacy /docs/features rejected", classify_surface("/docs/features/foo") is None)
    check("legacy /docs/user rejected", classify_surface("/docs/user/foo") is None)
    check("legacy /docs/api rejected", classify_surface("/docs/api/foo") is None)
    check("legacy /CLAUDE.md rejected", classify_surface("/CLAUDE.md") is None)
    check("legacy claude-docs/ rejected", classify_surface("claude-docs/cli-reference/foo") is None)
    check("venv symbol leak rejected", classify_surface("/docs/api/api-spec/uats/.venv/lib/python3.12/site-packages/urllib3/exceptions.py#EmptyPoolError") is None)
    check("empty path", classify_surface("") is None)
    check("random path", classify_surface("/random/path") is None)


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
    # Task #148 — path prefix rejection
    check(
        "claude-docs/ path rejected",
        build_qa_row({**node, "path": "claude-docs/cli-reference/settings/000__whole-file"}, 0) is None,
    )
    check(
        "/docs/api absolute-slash path rejected",
        build_qa_row({**node, "path": "/docs/api/inventory/foo"}, 0) is None,
    )
    check(
        "venv leak path rejected",
        build_qa_row({**node, "path": "/docs/api/api-spec/uats/.venv/lib/python3.12/site-packages/urllib3/exceptions.py#EmptyPoolError"}, 0) is None,
    )
    check(
        "empty path rejected",
        build_qa_row({**node, "path": ""}, 0) is None,
    )


def test_is_valid_mdemg_docs_path() -> None:
    """Task #148: prefix predicate itself."""
    print("test_is_valid_mdemg_docs_path:")
    check("prefix positive features", is_valid_mdemg_docs_path("mdemg-docs/features/foo/000__bar"))
    check("prefix positive user", is_valid_mdemg_docs_path("mdemg-docs/user/foo/000__bar"))
    check("prefix positive api", is_valid_mdemg_docs_path("mdemg-docs/api/foo/000__bar"))
    check("prefix positive claude", is_valid_mdemg_docs_path("mdemg-docs/claude/foo/000__bar"))
    check("prefix positive cli-help", is_valid_mdemg_docs_path("mdemg-docs/cli-help/foo/000__bar"))
    check("wrong prefix claude-docs", not is_valid_mdemg_docs_path("claude-docs/cli-reference/foo"))
    check("wrong prefix /docs", not is_valid_mdemg_docs_path("/docs/features/foo"))
    check("wrong prefix venv-symbol", not is_valid_mdemg_docs_path("/docs/api/api-spec/uats/.venv/lib/foo.py#Bar"))
    check("empty path", not is_valid_mdemg_docs_path(""))
    check("mdemg-docs without slash", not is_valid_mdemg_docs_path("mdemg-docs"))
    # Edge: mdemg-docsX substring should NOT match (exact prefix rule)
    check("mdemg-docs-fake NOT matched (exact prefix)", not is_valid_mdemg_docs_path("mdemg-docs-fake/features/foo"))


def test_build_query_starts_with_prefix() -> None:
    """Task #148: exporter query anchors WHERE clause to mdemg-docs/ prefix."""
    print("test_build_query_starts_with_prefix:")
    q = build_query()
    check("query contains STARTS WITH", "STARTS WITH" in q, detail=q)
    check("query contains mdemg-docs/", "mdemg-docs/" in q, detail=q)
    # Regression guard: the pre-#148 loose predicate must be gone
    check("query does NOT use loose CONTAINS 'docs/features'", "CONTAINS 'docs/features'" not in q)
    check("query does NOT use CONTAINS 'docs/api'", "CONTAINS 'docs/api'" not in q)
    check("query still excludes archived", "is_archived, false" in q)
    check("query still requires non-empty content", "n.content <> ''" in q)


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
    test_is_valid_mdemg_docs_path()
    test_build_query_starts_with_prefix()
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
