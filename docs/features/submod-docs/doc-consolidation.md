# Submodule Documentation Consolidation

**Date:** 2026-03-19
**Commit:** `4380c04` (main repo), submodule commits pushed to all 4 repos

---

## Problem

4 user-facing docs (`api-reference.md`, `cli-reference.md`, `cms-rsic-guide.md`, `ingestion-guide.md`) were duplicated across 4 submodules. They diverged independently — line counts ranged from 2,271 to 2,396 for the same doc. Critical content existed only in specific copies (e.g., `mdemg teardown` only in Windows, NR-4 neural training commands only in homebrew/apt-mdemg/linux, FSD-2026-001 endpoints only in homebrew/apt-mdemg). No canonical source existed in the main repo.

### Divergence Matrix (Before)

| File | homebrew | windows | linux | apt-mdemg |
|------|----------|---------|-------|-----------|
| api-reference.md | **3,591L** | 3,486L | 3,341L | **3,591L** |
| cli-reference.md | 2,374L | **2,396L** | 2,271L | 2,374L |
| cms-rsic-guide.md | 1,442L | **1,746L** | 1,481L | 1,442L |
| ingestion-guide.md | 1,041L | **1,206L** | 1,041L | 1,041L |

Bold = most content / most current for that doc.

---

## Solution: Single Canonical Location

### Canonical Files (`docs/user/`)

| File | Lines | Base Source | Merged Content |
|------|-------|-------------|----------------|
| `api-reference.md` | 3,707 | homebrew (richest) | Windows PowerShell/cmd.exe examples in Platform-Specific Notes section; fixed duplicate TOC entry |
| `cli-reference.md` | 2,658 | windows (has teardown) | NR-4 neural training CLI (`mdemg-neural-train`, `mdemg-neural-evaluate`), Linux/macOS install sections, all `NEURAL_*` env vars |
| `cms-rsic-guide.md` | 1,719 | windows (richest) | homebrew's Jiminy subsection structure, Linux systemd timer section, PowerShell in collapsible `<details>` blocks |
| `ingestion-guide.md` | 1,460 | windows (has PowerShell) | bash as primary shell, PowerShell in labeled subsections, cross-platform file watcher notes |

Platform-specific content uses `### macOS` / `### Linux` / `### Windows` subsections within each doc — no separate platform files.

### Submodule Stubs (16 files)

Each doc file in each submodule replaced with a one-line redirect:

```markdown
# Moved — see [cli-reference.md](https://github.com/reh3376/mdemg/blob/main/docs/user/cli-reference.md)
```

This preserves existing external links (no 404s) while funneling users to the canonical location.

### Submodule README Updates (4 READMEs)

Documentation table links changed from relative paths to absolute GitHub URLs:

```
Before: | [CLI Reference](docs/cli-reference.md) |
After:  | [CLI Reference](https://github.com/reh3376/mdemg/blob/main/docs/user/cli-reference.md) |
```

Inline CLI Reference links (e.g., "For complete reference documentation, see the CLI Reference") also updated.

### Main Repo README Update

Documentation section split into two subsections:

- **User Guides** — table linking to `docs/user/` (4 canonical docs + ELI5/Quickstart/FAQ)
- **Contributor & Architecture Docs** — existing internal docs (unchanged)

---

## How Updates Work Going Forward

1. Edit the single canonical file in `docs/user/` in the main repo
2. All submodule stubs automatically redirect to it — no sync needed
3. Submodule READMEs link to the canonical GitHub URL — always current

---

## Verification Checklist

| Check | Status |
|-------|--------|
| 4 canonical files in `docs/user/` | 3,707 + 2,658 + 1,719 + 1,460 = 9,544 lines |
| `mdemg teardown` in cli-reference | Line 1638 |
| `mdemg-neural-train` / `mdemg-neural-evaluate` (NR-4) in cli-reference | Line 1678 |
| All `NEURAL_*` env vars in cli-reference | Line 2491 |
| FSD-2026-001 endpoints (Jiminy, meta-learn, etc.) in api-reference | Lines 1699, 1812 |
| 16 stub redirects in submodules | All correct |
| 4 submodule READMEs point to canonical URLs | All 20 links updated |
| Main README split into User Guides + Contributor docs | Done |
| Submodules committed + pushed to `main` | 4/4 pushed |
| Main repo committed + pushed on `reh3376_dev01` | `4380c04` pushed |

---

## Files Touched

**New (main repo):**
- `docs/user/api-reference.md`
- `docs/user/cli-reference.md`
- `docs/user/cms-rsic-guide.md`
- `docs/user/ingestion-guide.md`

**Modified (main repo):**
- `README.md`

**Modified (per submodule, x4 submodules = 20 files):**
- `README.md` — doc links updated to canonical URLs
- `docs/api-reference.md` — replaced with stub redirect
- `docs/cli-reference.md` — replaced with stub redirect
- `docs/cms-rsic-guide.md` — replaced with stub redirect
- `docs/ingestion-guide.md` — replaced with stub redirect

**Total: 25 file touches**

---

## Documents Accessed

- `packaging/homebrew-mdemg/docs/*.md` (4 files — base for api-reference)
- `packaging/mdemg-windows/docs/*.md` (4 files — base for cli-reference, cms-rsic-guide, ingestion-guide)
- `packaging/mdemg_linux/docs/*.md` (4 files — Linux systemd content)
- `packaging/apt-mdemg/docs/*.md` (4 files — verified identical to homebrew)
- `packaging/homebrew-mdemg/README.md`
- `packaging/mdemg-windows/README.md`
- `packaging/mdemg_linux/README.md`
- `packaging/apt-mdemg/README.md`
- `README.md` (main repo)
- `docs/features/neural-training-pipeline.md` (NR-4/F21 feature doc for verification)
