# MDEMG Sidecar FAQ

Status: Draft  
Date: 2026-02-27  
Owner: MDEMG Core  
Audience: Developers adopting sidecar in personal or team repos

---

## 1. What problem does sidecar solve?

It standardizes setup of MDEMG + CMS in any repo, including runtime, config, and agent attachment, so you avoid manual per-repo wiring.

## 2. Do I need to run everything locally?

No. Use `studio-remote` profile to run heavy services on MacStudio while controlling from MacBook.

## 3. Does sidecar overwrite existing hooks or agent configs?

It should not overwrite silently. It creates backups and attempts safe merges. Use explicit force flags only when intended.

## 4. Is Codex support the same as Claude Code support?

Both are supported through adapter-specific integration logic. Config locations and merge behavior may differ.

## 5. What if Docker is unavailable?

Installation should fail fast with remediation steps. Future profile variants may support API-only partial workflows.

## 6. Is hash verification intended to block execution?

Hash signals are for change detection and review. Blocking behavior depends on configured gate policy, and integrity outcomes should be reported independently from verification outcomes.

## 7. What is the minimum validation after install?

Run:

```bash
mdemg sidecar doctor --format json
```

Then run one minimal ingest workflow.

## 8. How do I remove sidecar from a repo?

Use:

```bash
mdemg sidecar uninstall
```

Then verify no managed services/adapters remain attached.

## 9. Where do I find failures quickly?

Start with `doctor` JSON output and then `docs/sidecar/troubleshooting.md`.

## 10. When should I choose local vs studio-remote?

Use local for simplicity and portability; use studio-remote when resource-intensive containers affect MacBook performance.

## 11. Which document is the normative authority for sidecar behavior?

`docs/sidecar/roadmap.md` is the normative source for sidecar planning and implementation contracts in this directory.
