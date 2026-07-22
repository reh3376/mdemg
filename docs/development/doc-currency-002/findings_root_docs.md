# Findings — root docs (fixer: root-docs agent). Apply each; verify vs code first; minimal diffs.
NOTE: CMS.md config block (L499-544), VISION.md L281 training section, SECURITY.md table = RESERVED for the orchestrator (skip). Everything else below is yours.

## README.md
| doc:line | stale claim | current reality | fix |
|---|---|---|---|
| 161–172 + 685 | docs/benchmarks/... paths | files at docs/architecture/benchmarks/ | s|docs/benchmarks|docs/architecture/benchmarks| throughout |
| 186 | Grader grade_answers.py | canonical grader_v4.py | point at docs/architecture/benchmarks/grader_v4.py |
| 91 | "10 tabs"/"7 Grafana dashboards" | 11 tabs (backup, review, training); 8 dashboards | update counts + list |
| 289 | recursive loop "specced but not yet built" | built, ships default-off (FT_LOOP_ENABLED; internal/ftloop/) | reword |
| 461 | "27 UPTS" + list omits PHP | 28 incl. php.upts.json | 28 + add PHP |
| 502 | 6 observation types | 15 (internal/conversation/types.go:5-20) | say "15 types including …" |
| 526 | mcp command "/path/to/mdemg/bin/mdemg-mcp" | server is `mdemg mcp` | "command":"mdemg","args":["mcp"] |
| 581 | Blackbox Exporter 9115 row | not in observability compose | drop row; Grafana="dashboards only" |
| 585 | "10 panels" | overview has 12 | 12 |
| 113 | writes .claude/mcp.json | root .mcp.json is what's honored | say root .mcp.json |

## CONTRIBUTING.md
| 660 | GET /v1/metrics "Prometheus-style" | JSON graph metrics (handlers.go:971) | relabel |
| 668 | POST /v1/feedback row | route doesn't exist | delete row |
| 459 | "complete endpoint list" | 167 routes; list partial | drop "complete"; link docs/user/api-reference.md |
| 114 | "27 language parsers" | 28 | 28 |
| 292 | "Five hooks" | six (add pre-write-check.py row) | six + row |
| 308 | re-ingest "via bin/ingest-codebase" | ./bin/mdemg ingest --incremental --consolidate | correct command |
| 270–284,112 | 11 frameworks/"all 15" | 16 per UXTS matrix | sync table+count |

## VISION.md (line fixes only — L281 section reserved)
| 219 | "5 knowledge sources… 6-second timeout" | 4-source fan-out; config-driven 90s warm budget | update |
| 124,608,128 | 3 dead links | docs/architecture/benchmarks/…; docs/development/repo-to-public-roadmap.md | fix links |
| 269,291 | "7 dashboards"/"10-tab" | 8 / 11 | update |

## ELI5.md
| 169,198 | "< 6 seconds" | config-driven, background | soften to "a few seconds, in the background" |
