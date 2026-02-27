# UxXTS01 Decision Register

Status: Active

| ID | Topic | Options | Selected | Rationale | Timestamp |
|---|---|---|---|---|---|
| DR-001 | Naming grammar (`U<X>TS` vs `U<XX>TS`) | Keep single-char wildcard / allow 1-2 alnum | `U<XX>TS` where `XX` = 1..2 alphanumeric chars (`U[A-Z0-9]{1,2}TS`) | Preserves original spirit while enabling finer domain specificity; backward compatible with legacy names | 2026-02-27T12:22:00Z |
| DR-002 | Baseline source selection | Lean v3.1 / Full v3.2 | Full v3.2 normative baseline + lean v3.1 usability layer | Maximizes rigor without sacrificing adoption practicality | 2026-02-27T12:22:00Z |
| DR-003 | Governance burden at start | Full-only / Progressive tiers | Progressive tiers (Core -> Managed -> Full) | Reduces adoption friction while preserving hardening path | 2026-02-27T12:22:00Z |
