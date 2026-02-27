# MDEMG Sidecar Documentation Index

Status: Draft  
Date: 2026-02-27

Use this index as the canonical reading order for sidecar adoption.
Normative authority for sidecar behavior is `docs/sidecar/roadmap.md`.
External UxTS documents are informative references only for this doc set.

---

## 1. Start Here

1. Roadmap: `docs/sidecar/roadmap.md`
2. Installation guide: `docs/sidecar/installation.md`

---

## 2. Configure and Operate

1. Configuration reference: `docs/sidecar/configuration.md`
2. Maintenance procedures: `docs/sidecar/maintenance.md`
3. Security and operations controls: `docs/sidecar/security-and-ops.md`

---

## 3. Resolve Problems

1. Troubleshooting matrix: `docs/sidecar/troubleshooting.md`
2. FAQ: `docs/sidecar/faq.md`

---

## 4. Release and Change Management

1. Release note template: `docs/sidecar/release-notes-template.md`

---

## 5. Governance and Contracts

1. Report schema inventory: `docs/sidecar/schemas/README.md`
2. Implementation decision journal: `docs/sidecar/implementation-journal.md`

---

## 6. Documentation Validation Checklist

Before promoting sidecar beyond draft:

1. Follow installation guide on a clean repo and verify all command examples.
2. Validate configuration examples against implemented schema.
3. Execute maintenance and rollback steps end-to-end.
4. Confirm every doctor failure class is mapped in troubleshooting.
5. Confirm schema inventory and implementation journal are updated for any contract changes.
