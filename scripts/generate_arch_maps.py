#!/usr/bin/env python3
"""Generate T1-encoded architecture maps from source tree analysis.

Reads the source tree (no network, no build, no database) and outputs
10 compact architecture maps to docs/architecture/maps/.

Maps that have been UITS-optimized (converged) are skipped by default to
preserve hand-tuned T1 encoding. Use --force to overwrite all maps.

Usage:
    python3 scripts/generate_arch_maps.py              # Generate non-optimized maps only
    python3 scripts/generate_arch_maps.py --checksum   # Compare checksums (skips optimized maps)
    python3 scripts/generate_arch_maps.py --dry-run    # Print to stdout, don't write files
    python3 scripts/generate_arch_maps.py --force      # Overwrite ALL maps including optimized
"""

import argparse
import glob
import hashlib
import json
import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
MAPS_DIR = ROOT / "docs" / "architecture" / "maps"
UITS_SPECS_DIR = ROOT / "docs" / "tests" / "uits" / "specs"


# ---------------------------------------------------------------------------
# Data collection helpers
# ---------------------------------------------------------------------------

def count_files(pattern: str) -> int:
    return len(glob.glob(str(ROOT / pattern)))


def list_internal_packages() -> list[str]:
    internal = ROOT / "internal"
    if not internal.is_dir():
        return []
    return sorted(d.name for d in internal.iterdir() if d.is_dir() and not d.name.startswith("."))


def get_go_imports(pkg: str) -> list[str]:
    """Get internal/ imports for a given internal package."""
    pkg_dir = ROOT / "internal" / pkg
    imports = set()
    mod_path = "github.com/reh3376/mdemg/internal/"
    for gofile in pkg_dir.glob("*.go"):
        try:
            text = gofile.read_text(errors="replace")
        except OSError:
            continue
        for m in re.finditer(rf'"{re.escape(mod_path)}(\w+)"', text):
            dep = m.group(1)
            if dep != pkg:
                imports.add(dep)
    return sorted(imports)


def count_uats_variants() -> tuple[int, int]:
    """Return (spec_count, variant_count) for UATS."""
    specs_dir = ROOT / "docs" / "api" / "api-spec" / "uats" / "specs"
    spec_count = 0
    variant_count = 0
    for f in specs_dir.glob("*.uats.json"):
        try:
            data = json.loads(f.read_text())
        except (json.JSONDecodeError, OSError):
            continue
        spec_count += 1
        variant_count += len(data.get("variants", []))
    return spec_count, variant_count


def get_proto_services() -> list[str]:
    """Extract gRPC service names from proto files."""
    services = []
    for proto in (ROOT / "api" / "proto").glob("*.proto"):
        try:
            text = proto.read_text()
        except OSError:
            continue
        for m in re.finditer(r"service\s+(\w+)\s*\{", text):
            services.append(m.group(1))
    return sorted(services)


def get_neo4j_labels() -> list[tuple[str, str]]:
    """Extract node labels from migration files. Returns [(label, purpose)]."""
    labels = {}
    migrations_dir = ROOT / "migrations"
    for mig in sorted(migrations_dir.glob("V*.cypher")):
        try:
            text = mig.read_text()
        except OSError:
            continue
        for m in re.finditer(r"CREATE\s+(?:CONSTRAINT|INDEX)\s+\w+\s+(?:IF NOT EXISTS\s+)?(?:FOR|ON)\s+\(?\w+:([\w]+)", text, re.IGNORECASE):
            label = m.group(1)
            if label not in labels:
                labels[label] = ""
    return sorted(labels.items())


def get_ci_workflows() -> list[tuple[str, str]]:
    """Return [(filename, trigger_summary)] for CI workflows."""
    workflows = []
    wf_dir = ROOT / ".github" / "workflows"
    if not wf_dir.is_dir():
        return []
    for yml in sorted(wf_dir.glob("*.yml")):
        try:
            text = yml.read_text()
        except OSError:
            continue
        name_match = re.search(r"^name:\s*(.+)$", text, re.MULTILINE)
        name = name_match.group(1).strip().strip("'\"") if name_match else yml.stem
        workflows.append((yml.name, name))
    return workflows


# ---------------------------------------------------------------------------
# Map generators — each returns the full map content as a string
# ---------------------------------------------------------------------------

def gen_dict_pkg_codes() -> str:
    pkgs = list_internal_packages()
    # Brief descriptions — these encode domain knowledge
    descs = {
        "anomaly": "meta-cognitive anomaly detection",
        "ape": "Active Participant Engine, RSIC self-improvement",
        "api": "HTTP handlers + middleware + server",
        "auth": "authentication providers (none,apikey,jwt)",
        "backpressure": "memory pressure monitoring",
        "backup": "Neo4j backup/restore + scheduling",
        "circuitbreaker": "circuit breaker for external calls",
        "cli": "unified CLI commands (serve,init,db,ingest,...)",
        "config": "environment-based configuration (~3500 lines)",
        "consulting": "agent consulting service (consult,suggest,constraints)",
        "conversation": "CMS (observe,recall,resume,correct,consolidate)",
        "db": "Neo4j driver + schema validation + migrations",
        "devspace": "agent workspace isolation",
        "embeddings": "embedding providers (OpenAI,Ollama) + cache",
        "encoding": "J17 protocol encoding (T1/T2/T3 tiers)",
        "filewatcher": "file change detection for auto-ingest",
        "gaps": "capability gap detection + interviews",
        "guardrail": "MCP guardrail validation pipeline",
        "hidden": "hidden layer abstraction/consolidation pipeline",
        "jiminy": "inner voice guidance service + J17 protocol",
        "jobs": "background job manager (ingest,backup,scraper)",
        "languages": "language parser implementations",
        "learning": "Hebbian learning (CO_ACTIVATED_WITH edges)",
        "llmclient": "unified LLM client (OpenAI/Ollama)",
        "mathutil": "math utilities (cosine similarity, etc.)",
        "metalearn": "global meta-learning (cross-space promotion)",
        "metrics": "Prometheus metrics + determinism scoring",
        "models": "shared data models",
        "optimistic": "optimistic locking with retry",
        "plugins": "plugin manager + scaffold + registry",
        "ratelimit": "rate limiting middleware",
        "retrieval": "core retrieval pipeline (vector+activation+scoring+cache)",
        "sanitize": "prompt injection sanitization",
        "scraper": "web scraper ingestion",
        "secrets": "system keychain integration",
        "sidecar": "sidecar lifecycle management",
        "summarize": "LLM summary service",
        "symbols": "symbol extraction (tree-sitter)",
        "transfer": "space transfer (export/import)",
        "unts": "UNTS hash verification service",
        "validation": "input validation helpers",
    }
    lines = [f"DICT|pkg-codes|v1|internal package quick-reference glossary ({len(pkgs)} packages)"]
    for pkg in pkgs:
        desc = descs.get(pkg, "")
        lines.append(f"  {pkg}|{desc}")
    return "\n".join(lines) + "\n"


def gen_dep_pkg_graph() -> str:
    """Generate package dependency graph from Go import analysis."""
    pkgs = list_internal_packages()
    lines = [
        "DEP|pkg-graph|v1|internal package import graph",
        "LEGEND:→=imports,:annotation=reason-for-dependency,leaf=no internal deps",
        "",
        "GRAPH:",
    ]
    why_annotations = []
    for pkg in pkgs:
        deps = get_go_imports(pkg)
        if not deps:
            lines.append(f"  {pkg}|leaf:no internal deps")
        else:
            dep_str = ",".join(deps)
            lines.append(f"  {pkg}→{dep_str}")

    # Known why-annotations (domain knowledge)
    lines.extend([
        "",
        "WHY-ANNOTATIONS:",
        "  ret depends on plg because retrieval calls MatchIngestionModule() as fallback to dispatch unrecognized file types to the correct plugin module for parsing",
    ])
    return "\n".join(lines) + "\n"


def gen_flow_retrieval() -> str:
    return """FLOW|retrieval|v1|retrieval request pipeline from HTTP to response
POST /v1/memory/retrieve
→ api.handleRetrieve|parse+validate request
→ retrieval.Service.Retrieve|orchestrate pipeline
  → embeddings.Embed(query)|get 3072-dim vector
  → db.VectorSearch|top-K candidates from memNodeEmbedding index
  → symbols.Search|pattern-match symbol names
  → db.BoundedExpand|1-hop fetch (max depth=3)
  → retrieval.spreadActivation|in-memory with decay
  → retrieval.score|vector(0.55)+activation(0.30)+recency(0.10)+confidence(0.05)-hub(0.08)-redundancy(0.12)
  → cache.Store|TTL-LRU cache
→ api.writeJSON|200 + scored results
"""


def gen_flow_olc() -> str:
    return """FLOW|observe-learn-consolidate|v1|observation lifecycle from ingest to emergent concept
OBSERVE:
  POST /v1/conversation/observe
  → conversation.Observe|create Observation node + embed
  → learning.CoActivate|strengthen CO_ACTIVATED_WITH edges (Hebbian)
  → jiminy.Guide|proactive guidance if constraints detected

LEARN:
  automatic via CO_ACTIVATED_WITH edge weights
  → edges strengthened by co-recall frequency
  → edges decayed by temporal distance
  → graduation: volatile→permanent when recall_count >= threshold

CONSOLIDATE:
  POST /v1/memory/consolidate
  → hidden.Pipeline.Run|execute registered NodeCreator steps in phase order
    → phase 10-20: concern,config,comparison,temporal,constraint nodes
    → DBSCAN clustering: group similar MemoryNodes
    → phase 25-30: dynamic_edges,emergent_l5 (post-clustering)
  → result: L1-L5 hierarchy (observations→themes→concepts→abstract→emergent)
"""


def gen_flow_jiminy() -> str:
    return """FLOW|jiminy-guide|v1|Jiminy inner voice guidance pipeline
POST /v1/jiminy/guide
→ jiminy.Service.Guide|orchestrate guidance
  → conversation.Recall|fetch relevant constraints+observations
  → jiminy.matchConstraints|identify applicable constraint codes
  → jiminy.classifySeverity|priority:critical>high>medium>low
  → jiminy.formatGuidance|build guidance items with confidence
  → jiminy.encodeJ17|select encoding tier
    T1 coded: ~15 tokens, 80% of traffic, constraint codes
    T2 telegraphic: ~50-100 tokens, 15%, structured summaries
    T3 full NL: ~200+ tokens, 5%, novel/complex guidance
  → jiminy.trustScore|per-session trust (escalation tracking)
→ response: guidance[],prompt_augmentation,confidence,protocol info
"""


def gen_flow_rsic() -> str:
    return """FLOW|rsic-cycle|v1|RSIC self-improvement cycle
POST /v1/self-improve/cycle
→ ape.Orchestrator.RunCycle|full RSIC loop
  REFLECT:
    → ape.Assessor.Assess|compute health across 8 dimensions
      retrieval_quality,memory_health,edge_health,guidance_calibration,
      protocol_maturity,safety_compliance,synergy_health,encoding_quality
    → ape.SelfReflect|pattern matching (21 reflection patterns)
    → ape.LLMReflect|optional LLM-powered insight generation
  SELECT:
    → ape.SelectAction|pick highest-priority actionable insight
    → ape.ValidateAction|safety policy check (phase 88)
  IMPROVE:
    → ape.ExecuteAction|run action (14 action types)
  CALIBRATE:
    → ape.Assessor.Assess|re-assess (metrics_after)
    → ape.UpdateCalibration|update confidence based on outcome
    → ape.CheckRollback|auto-rollback if reversible + no improvement
"""


def gen_schema_neo4j() -> str:
    migration_count = count_files("migrations/V*.cypher")
    return f"""SCHEMA|neo4j|v1|Neo4j graph schema ({migration_count} migrations)
NODE-LABELS:
  TapRoot|singleton per space_id|anchor for all space data
  MemoryNode|main memory nodes|3072-dim embedding,content,layer,score
  Observation|append-only events|linked to MemoryNode,temporal
  SymbolNode|extracted code symbols|name,type,file,line
  ConceptNode|hidden layer abstractions|L1-L5 hierarchy
  Theme|consolidated topic clusters|from DBSCAN clustering

EDGE-TYPES:
  HAS_MEMORY|TapRoot→MemoryNode|space ownership
  OBSERVED_IN|Observation→MemoryNode|observation linkage
  CO_ACTIVATED_WITH|MemoryNode↔MemoryNode|Hebbian learning weight
  ABSTRACTS|ConceptNode→MemoryNode|hidden layer membership
  RELATED_TO|SymbolNode→SymbolNode|code relationships

INDEXES:
  memNodeEmbedding|vector|3072-dim cosine similarity
  obs_space_session|btree|Observation(space_id,session_id)
  mem_space_layer|btree|MemoryNode(space_id,layer)

CONSTRAINTS:
  unique_taproot|TapRoot(space_id)|one root per space
  unique_symbol|SymbolNode(space_id,name,file)|no duplicate symbols
"""


def gen_svc_external() -> str:
    return """SVC|external|v1|external services,ports,protocols
SERVICES:
  mdemg-server|HTTP|:9999|150+ REST endpoints
  neo4j|Bolt|:7687|graph DB
  neo4j-browser|HTTP|:7474|admin UI
  neural-sidecar|HTTP|:8100|capabilities:rerank,NLI,tier-predict
  mcp-server|stdio|subprocess|IDE integration via MCP protocol

SIDECAR-ENDPOINTS:
  POST /rerank|cross-encoder re-ranking
  POST /nli|NL inference comprehension scoring
  POST /protocol/predict-tier|ML J17 tier prediction
  GET /health|sidecar health

GRPC-SERVICES:dir:api/proto/
  ModuleLifecycle|ops:handshake,health,shutdown|role:lifecycle management for all plugin modules
  IngestionModule|ops:Matches,Parse,Sync|role:ingest content into observations
  ReasoningModule|ops:Rerank|role:reorder retrieval candidates by semantic relevance score
  APEModule|ops:Evaluate|role:background self-improvement maintenance
  CRUDModule|ops:generic entity CRUD|role:external system integration via Linear module
  SpaceTransfer|ops:export,import|role:space migration between instances
  DevSpace|ops:agent workspace isolation|role:isolated dev environments
  HashVerification|ops:verify|role:UNTS spec integrity registry
"""


def gen_dist_channels() -> str:
    return """DIST|channels|v1|distribution channels and packaging
PLATFORMS:
  macOS|homebrew tap:reh3376/mdemg|formula:mdemg|arm64+amd64
  Linux|tarball+deb+AUR|apt repo:apt-mdemg (GitHub Pages)|amd64+arm64
  Windows|scoop bucket:reh3376/mdemg-windows|manifest:mdemg.json|amd64

COMPANION-APPS:
  macOS menubar|SwiftUI|submodule:packaging/mdemg-menubar|tabs:Status,Memory,Learning,Neo4j,Config,RSIC,Synergy,Jiminy
  Linux sidebar|Tauri 1.x (Rust+JS)|submodule:packaging/mdemg-linux-sidebar|tabs:Status,Memory,Learning,Neo4j,Config,Logs,RSIC

CI-WORKFLOWS:
  ci.yml|push+PR:build,test,lint,security
  release.yml|tag:goreleaser→GitHub Release→brew+apt+scoop
  parser-tests.yml|push:UPTS conformance
  apt-publish.yml|post-release:sign+publish .deb to APT repo
  branch-naming.yml|push:enforce naming convention
  uxts-canonical-specs.yml|push:canonical dialect guard
"""


def gen_uxts_frameworks() -> str:
    uats_specs, uats_variants = count_uats_variants()
    uits_specs = count_files("docs/tests/uits/specs/*.uits.json")
    uets_specs = count_files("docs/tests/uets/specs/*.uets.json")
    upts_specs = count_files("docs/lang-parser/lang-parse-spec/upts/specs/*.upts.json")
    udts_canonical = count_files("docs/api/api-spec/udts/specs/*.udts.json")
    udts_drafts = count_files("docs/api/api-spec/udts/drafts/*.udts.json")
    ubts_specs = count_files("docs/tests/ubts/specs/*.ubts.json")
    ubts_profiles = count_files("docs/tests/ubts/profiles/*.profile.json")
    usts_canonical = count_files("docs/tests/usts/specs/*.usts.json")
    usts_drafts = count_files("docs/tests/usts/drafts/*.usts.json")
    uobs_canonical = count_files("docs/tests/uobs/specs/*.uobs.json")
    uobs_drafts = count_files("docs/tests/uobs/drafts/*.uobs.json")
    uots_specs = count_files("docs/api/api-spec/uots/specs/*.uots.json")
    uvts_canonical = count_files("docs/tests/uvts/specs/*.uvts.json")
    uvts_drafts = count_files("docs/tests/uvts/drafts/*.uvts.json")
    uams_specs = count_files("docs/tests/uams/specs/*.uams.json")

    return f"""UXTS|frameworks|v1|unified test specification frameworks
#name|scope|specs|runner|CI
UATS|HTTP contract|{uats_specs} specs,{uats_variants} variants|runner:active|CI:merge-blocking
UPTS|parser conformance|{upts_specs}s|runner:active|CI:merge-blocking
UDTS|gRPC contract|{udts_canonical} canonical,{udts_drafts} drafts|runner:active|CI:canonical-guard
UBTS|benchmark regression|{ubts_specs}s,{ubts_profiles} profiles|runner:active|CI:soft-fail
USTS|security|{usts_canonical} canonical,{usts_drafts} drafts|runner:active|CI:merge-blocking
UOBS|observability runtime|{uobs_canonical} canonical,{uobs_drafts} draft|runner:active|CI:soft-fail
UOTS|observability artifacts|{uots_specs}s|runner:active|CI:soft-fail
UNTS|hash verification|registry|runner:active|CI:merge-blocking
UETS|emergence eval:LLM concept-naming quality|{uets_specs}s|runner:active|CI:soft-fail
UITS|iterative-improvement:T1 encoding comprehension|{uits_specs}s|runner:active|CI:soft-fail
UVTS|semantic validation|{uvts_canonical} canonical,{uvts_drafts} draft|runner:demoted|reason:GAP-04:inline-grading-missing|CI:none
UAMS|auth methods|{uams_specs}s|runner:UNBUILT|CI:none→GAP-21:pending

STATUS-EXCEPTIONS:
  UAMS|runner:UNBUILT|only framework with no runner implementation→GAP-21
  UVTS|runner:demoted|needs inline grading before CI activation→GAP-04
"""


# ---------------------------------------------------------------------------
# Map registry
# ---------------------------------------------------------------------------

GENERATORS = {
    "dict_pkg_codes.md": gen_dict_pkg_codes,
    "dep_pkg_graph.md": gen_dep_pkg_graph,
    "flow_retrieval.md": gen_flow_retrieval,
    "flow_observe_learn_consolidate.md": gen_flow_olc,
    "flow_jiminy_guide.md": gen_flow_jiminy,
    "flow_rsic_cycle.md": gen_flow_rsic,
    "schema_neo4j.md": gen_schema_neo4j,
    "svc_external.md": gen_svc_external,
    "dist_channels.md": gen_dist_channels,
    "uxts_frameworks.md": gen_uxts_frameworks,
}


# ---------------------------------------------------------------------------
# UITS optimization check
# ---------------------------------------------------------------------------

def is_uits_optimized(map_filename: str) -> bool:
    """Check if a map has a corresponding UITS spec with metadata.optimized=true."""
    stem = map_filename.replace(".md", "")
    uits_spec = UITS_SPECS_DIR / f"{stem}.uits.json"
    if not uits_spec.exists():
        return False
    try:
        data = json.loads(uits_spec.read_text())
        return data.get("metadata", {}).get("optimized", False) is True
    except (json.JSONDecodeError, OSError):
        return False


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def compute_checksum(content: str) -> str:
    return hashlib.sha256(content.encode()).hexdigest()[:16]


def main():
    parser = argparse.ArgumentParser(description="Generate T1 architecture maps")
    parser.add_argument("--checksum", action="store_true",
                        help="Compare checksums only; exit 1 if stale (skips optimized maps)")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print to stdout instead of writing files")
    parser.add_argument("--force", action="store_true",
                        help="Overwrite ALL maps including UITS-optimized ones")
    args = parser.parse_args()

    os.chdir(ROOT)
    stale = []
    skipped = []
    generated = 0

    for filename, generator in GENERATORS.items():
        # Check UITS optimization flag — skip optimized maps unless --force
        if not args.force and is_uits_optimized(filename):
            skipped.append(filename)
            continue

        content = generator()
        new_checksum = compute_checksum(content)
        filepath = MAPS_DIR / filename

        if args.checksum:
            if filepath.exists():
                old_checksum = compute_checksum(filepath.read_text())
                if old_checksum != new_checksum:
                    stale.append(filename)
            else:
                stale.append(filename)
        elif args.dry_run:
            print(f"=== {filename} ===")
            print(content)
        else:
            MAPS_DIR.mkdir(parents=True, exist_ok=True)
            filepath.write_text(content)
            print(f"  ✓ {filename} ({new_checksum})")
            generated += 1

    if skipped and not args.checksum:
        print(f"\n  ⊘ Skipped {len(skipped)} UITS-optimized map(s): {', '.join(skipped)}")
        if not args.force:
            print("    Use --force to overwrite optimized maps")

    if args.checksum:
        if skipped:
            print(f"Skipped {len(skipped)} UITS-optimized: {', '.join(skipped)}")
        if stale:
            print(f"Stale maps ({len(stale)}): {', '.join(stale)}")
            print("Run: python3 scripts/generate_arch_maps.py")
            sys.exit(1)
        else:
            print("All non-optimized maps up to date.")
            sys.exit(0)
    elif not args.dry_run:
        total = len(GENERATORS)
        print(f"\nGenerated {generated}/{total} maps in {MAPS_DIR.relative_to(ROOT)}/ ({len(skipped)} optimized, skipped)")


if __name__ == "__main__":
    main()
