# MDEMG Sidecar Security and Operations Guide

Status: Draft  
Date: 2026-02-27  
Owner: MDEMG Core  
Audience: Operators and maintainers

---

## 1. Security Goals

1. Protect secrets used by sidecar runtime and adapters.
2. Prevent unsafe mutation of repo and agent configuration.
3. Maintain auditable operational state across local and remote profiles.

---

## 2. Secret Handling

Rules:

1. Do not store API keys or credentials in plaintext docs or committed config.
2. Prefer keychain/env-based secret resolution.
3. Redact secrets in logs and diagnostic outputs.

Minimum checks:

1. Secret values never appear in `sidecar status`.
2. `doctor` outputs only redacted indicators.

---

## 3. File and Permission Hygiene

1. Sidecar-generated files should use least-privilege permissions.
2. Backup files containing config should not be world-readable.
3. Uninstall flow should remove sensitive generated artifacts unless retention is explicitly requested.

---

## 4. Remote Host Hardening (`studio-remote`)

1. Use SSH keys instead of password auth where possible.
2. Restrict remote account privileges to required runtime operations.
3. Keep remote Docker host and OS patched.
4. Limit exposed ports to required endpoints only.

---

## 5. Operational Logging Policy

1. Keep operational logs in `.mdemg/logs/`.
2. Separate runtime status logs from error details.
3. Retain recent logs for troubleshooting; rotate to avoid uncontrolled growth.
4. Keep machine-readable status/doctor/install reports in `.mdemg/generated/` for audit trails.

---

## 6. Incident Response Basics

If compromise or severe misconfiguration is suspected:

1. Stop sidecar runtime: `mdemg sidecar down`.
2. Preserve logs and current config artifacts.
3. Rotate exposed credentials.
4. Restore known-good config from backups.
5. Rebuild and re-run `doctor`.

---

## 7. Operational Controls Checklist

1. `doctor` run before production-like use.
2. Backup created before any adapter mutation.
3. Upgrade tested with rollback in non-critical repo first.
4. Remote profile connectivity monitored for degraded states.
5. CI gate mode (`observe` / `soft` / `block`) aligns with current stability and risk tolerance.
