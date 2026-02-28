# Sidecar Release Notes Template

Status: v0.1.0 Template
Owner: MDEMG Core  
Applies To: `mdemg` releases containing sidecar changes

---

## Release Metadata

1. Version:
2. Release Date:
3. Commit Range:
4. Maintainers:
5. CI Gate Mode Changes (`observe` / `soft` / `block`):

---

## Summary

Briefly describe what changed and why it matters.

---

## New Features

1.
2.
3.

---

## Improvements

1.
2.
3.

---

## Fixes

1.
2.
3.

---

## Breaking Changes

List breaking changes and required actions.

---

## Migration Steps

Step-by-step migration guidance:

1.
2.
3.
4. If report schema changed, include schema version transition and compatibility note.

---

## Upgrade and Rollback

Upgrade:

```bash
mdemg sidecar upgrade
```

Rollback:

1.
2.
3.

---

## Profile-Specific Notes

## Local Profile

1.
2.

## Studio-Remote Profile

1.
2.

---

## Agent Adapter Notes

## Claude Code

1.
2.

## Codex

1.
2.

---

## Known Issues

1.
2.
3.

---

## Verification Checklist

1. `mdemg sidecar status` works.
2. `mdemg sidecar doctor` passes required checks.
3. Install path validated on clean machine(s).
4. Attach-agent flows validated for supported adapters.
5. JSON report outputs validate against current schemas.
6. Implementation journal updated for material contract changes.

---

## References

1. Roadmap: `docs/sidecar/roadmap.md`
2. Installation guide: `docs/sidecar/installation.md`
3. Troubleshooting guide: `docs/sidecar/troubleshooting.md`
4. Report schema inventory: `docs/sidecar/schemas/README.md`
5. Implementation journal: `docs/sidecar/implementation-journal.md`
