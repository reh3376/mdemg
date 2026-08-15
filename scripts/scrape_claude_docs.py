#!/usr/bin/env python3
"""CLAUDE-DOCS-TRAINING-001 Epic 2 — scrape Claude Code docs to raw markdown.

Fetches each URL from configs/scrape/claude_docs.yaml. Each URL is a docs page
that also serves raw markdown at URL + ".md" — no HTML parsing needed.

Contract:
- Respect the configured rate_limit_seconds between fetches.
- Respect robots.txt (validated at Epic 1 discovery time; documented in config).
- Cache to output_dir/<slug>.md keyed on the URL slug (e.g., overview.md,
  agent-sdk--overview.md).
- Write scrape_manifest.json with per-URL: fetch_ts, http_status, size_bytes,
  content_sha256, title (parsed from first `# ...` header). Manifest is the
  source of truth for downstream curation.
- Idempotent: re-runs skip URLs whose local .md file exists AND has matching
  SHA in the manifest; use --force to re-fetch.
- Fail-open: 4xx/5xx logged + counted; scrape continues. Final report shows
  success rate.

Usage:
    python3 scripts/scrape_claude_docs.py                          # incremental
    python3 scripts/scrape_claude_docs.py --config <path>          # override
    python3 scripts/scrape_claude_docs.py --force                  # re-fetch all
    python3 scripts/scrape_claude_docs.py --limit 10               # first 10 (smoke)
    python3 scripts/scrape_claude_docs.py --dry-run                # print plan only

Sprint: docs/development/claude-docs-training-001/
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
import time
from dataclasses import dataclass, field, asdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib import request as urlreq
from urllib.error import HTTPError, URLError

try:
    import yaml
except ImportError:
    print("error: PyYAML not installed. Install with: pip install pyyaml", file=sys.stderr)
    sys.exit(2)


REPO_ROOT = Path(__file__).resolve().parent.parent


@dataclass
class ScrapeConfig:
    domain: str
    scheme: str
    rate_limit_seconds: float
    timeout_seconds: int
    user_agent: str
    output_dir: str
    manifest_path: str
    urls: list[str]


@dataclass
class UrlRecord:
    url: str
    slug: str
    md_path: str
    fetch_ts_utc: str
    http_status: int
    size_bytes: int
    content_sha256: str
    title: str
    error: str | None = None


@dataclass
class ScrapeManifest:
    config_source: str
    started_at_utc: str
    completed_at_utc: str
    duration_seconds: float
    total_urls: int
    fetched_ok: int
    fetched_error: int
    cached_skipped: int
    records: list[dict[str, Any]] = field(default_factory=list)


def load_config(path: Path) -> ScrapeConfig:
    raw = yaml.safe_load(path.read_text(encoding="utf-8"))
    return ScrapeConfig(
        domain=raw["domain"],
        scheme=raw.get("scheme", "https"),
        rate_limit_seconds=float(raw.get("rate_limit_seconds", 1.0)),
        timeout_seconds=int(raw.get("timeout_seconds", 30)),
        user_agent=raw["user_agent"],
        output_dir=raw["output_dir"],
        manifest_path=raw["manifest_path"],
        urls=list(raw["urls"]),
    )


def url_slug(path: str) -> str:
    # Convert /docs/en/agent-sdk/overview → agent-sdk--overview.md
    # (docs/en prefix stripped; further slashes become double-dash).
    p = path.lstrip("/")
    for prefix in ("docs/en/", "docs/"):
        if p.startswith(prefix):
            p = p[len(prefix):]
            break
    p = p.replace("/", "--")
    # No trailing slash. Strip any query.
    p = p.split("?", 1)[0].rstrip("-")
    return p


def parse_title(md_body: str) -> str:
    # Prefer YAML frontmatter `title:`; fall back to first `# ...` header.
    if md_body.startswith("---"):
        end = md_body.find("\n---", 3)
        if end > 0:
            fm = md_body[3:end]
            for line in fm.splitlines():
                if line.strip().startswith("title:"):
                    return line.split(":", 1)[1].strip().strip("\"'")
    for line in md_body.splitlines()[:30]:
        m = re.match(r"^#\s+(.+)$", line.strip())
        if m:
            return m.group(1).strip()
    return ""


def sha256_hex(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def fetch(url: str, ua: str, timeout: int) -> tuple[int, bytes]:
    req = urlreq.Request(url, headers={"User-Agent": ua})
    try:
        with urlreq.urlopen(req, timeout=timeout) as resp:  # noqa: S310 — public docs URL
            return resp.status, resp.read()
    except HTTPError as e:
        return e.code, b""
    except (URLError, TimeoutError) as e:
        return -1, str(e).encode("utf-8")


def load_existing_manifest(manifest_path: Path) -> dict[str, dict[str, Any]]:
    if not manifest_path.exists():
        return {}
    try:
        data = json.loads(manifest_path.read_text(encoding="utf-8"))
        # Map url → record for quick lookup.
        return {r["url"]: r for r in data.get("records", [])}
    except Exception as e:
        print(f"warn: could not parse existing manifest ({e}); starting fresh", file=sys.stderr)
        return {}


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--config", default=str(REPO_ROOT / "configs/scrape/claude_docs.yaml"))
    ap.add_argument("--force", action="store_true", help="re-fetch even if cached")
    ap.add_argument("--limit", type=int, default=0, help="fetch at most N URLs (smoke)")
    ap.add_argument("--dry-run", action="store_true", help="print plan only")
    args = ap.parse_args()

    config_path = Path(args.config)
    if not config_path.exists():
        print(f"error: config not found: {config_path}", file=sys.stderr)
        return 2

    cfg = load_config(config_path)
    urls = cfg.urls[: args.limit] if args.limit else cfg.urls

    print(f"scrape config: {config_path}")
    print(f"  domain:  {cfg.scheme}://{cfg.domain}")
    print(f"  ua:      {cfg.user_agent}")
    print(f"  rate:    {cfg.rate_limit_seconds}s between fetches")
    print(f"  timeout: {cfg.timeout_seconds}s per fetch")
    print(f"  urls:    {len(urls)} (of {len(cfg.urls)} total in config)")
    print(f"  output:  {cfg.output_dir}")
    print(f"  manifest:{cfg.manifest_path}")
    if args.force:
        print("  force:   re-fetching all")
    print()

    if args.dry_run:
        for u in urls:
            print(f"  would fetch: {cfg.scheme}://{cfg.domain}{u}.md → {url_slug(u)}.md")
        return 0

    output_dir = REPO_ROOT / cfg.output_dir
    output_dir.mkdir(parents=True, exist_ok=True)
    manifest_path = REPO_ROOT / cfg.manifest_path
    manifest_path.parent.mkdir(parents=True, exist_ok=True)

    existing = load_existing_manifest(manifest_path) if not args.force else {}

    started = time.time()
    fetched_ok = 0
    fetched_error = 0
    cached_skipped = 0
    records: list[UrlRecord] = []

    for i, path in enumerate(urls):
        url = f"{cfg.scheme}://{cfg.domain}{path}"
        md_url = url + ".md"
        slug = url_slug(path)
        md_local = output_dir / f"{slug}.md"

        # Cache hit check: local file exists AND has matching SHA in manifest.
        if not args.force and md_local.exists() and url in existing:
            prior = existing[url]
            local_sha = sha256_hex(md_local.read_bytes())
            if local_sha == prior.get("content_sha256"):
                cached_skipped += 1
                records.append(UrlRecord(
                    url=url,
                    slug=slug,
                    md_path=str(md_local.relative_to(REPO_ROOT)),
                    fetch_ts_utc=prior["fetch_ts_utc"],
                    http_status=prior["http_status"],
                    size_bytes=prior["size_bytes"],
                    content_sha256=prior["content_sha256"],
                    title=prior.get("title", ""),
                ))
                print(f"  [{i+1}/{len(urls)}] cached  {slug} ({prior['size_bytes']} b)")
                continue

        # Fetch
        status, body = fetch(md_url, cfg.user_agent, cfg.timeout_seconds)
        ts = now_iso()

        if status == 200 and body:
            md_local.write_bytes(body)
            text = body.decode("utf-8", errors="replace")
            rec = UrlRecord(
                url=url,
                slug=slug,
                md_path=str(md_local.relative_to(REPO_ROOT)),
                fetch_ts_utc=ts,
                http_status=200,
                size_bytes=len(body),
                content_sha256=sha256_hex(body),
                title=parse_title(text),
            )
            fetched_ok += 1
            print(f"  [{i+1}/{len(urls)}] ok      {slug} ({len(body)} b) — {rec.title[:60]}")
        else:
            fetched_error += 1
            err = f"http_{status}" if status > 0 else body.decode("utf-8", errors="replace")[:200]
            rec = UrlRecord(
                url=url,
                slug=slug,
                md_path=str(md_local.relative_to(REPO_ROOT)),
                fetch_ts_utc=ts,
                http_status=status,
                size_bytes=0,
                content_sha256="",
                title="",
                error=err,
            )
            print(f"  [{i+1}/{len(urls)}] ERR     {slug} — {err}")
        records.append(rec)

        # Rate limit
        if i + 1 < len(urls):
            time.sleep(cfg.rate_limit_seconds)

    completed = time.time()

    manifest = ScrapeManifest(
        config_source=str(config_path.relative_to(REPO_ROOT)),
        started_at_utc=datetime.fromtimestamp(started, tz=timezone.utc).isoformat(timespec="seconds"),
        completed_at_utc=datetime.fromtimestamp(completed, tz=timezone.utc).isoformat(timespec="seconds"),
        duration_seconds=round(completed - started, 2),
        total_urls=len(urls),
        fetched_ok=fetched_ok,
        fetched_error=fetched_error,
        cached_skipped=cached_skipped,
        records=[asdict(r) for r in records],
    )
    manifest_path.write_text(json.dumps(asdict(manifest), indent=2), encoding="utf-8")

    print()
    print(f"scrape complete in {manifest.duration_seconds}s:")
    print(f"  fetched_ok:     {fetched_ok}")
    print(f"  cached_skipped: {cached_skipped}")
    print(f"  fetched_error:  {fetched_error}")
    print(f"  success_rate:   {(fetched_ok + cached_skipped) / len(urls) * 100:.1f}%")
    print(f"  manifest:       {manifest_path.relative_to(REPO_ROOT)}")

    # Epic 1 gate: >=95% success rate
    success_rate = (fetched_ok + cached_skipped) / len(urls)
    if success_rate < 0.95:
        print(f"error: success rate {success_rate*100:.1f}% below Epic 1 gate (95%)", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
