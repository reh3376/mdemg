# Subagent Brief: Phase 2 Verification

**Assignment:** Verify Phase 2 DevSpace hub implementation.

---

## Main agent: monitoring

- **Expect periodic updates:** Subagent must report at Checkpoint 1, 2, 3, then Final summary. If you only see one message with no checkpoints, ask for checkpoint-by-checkpoint updates.
- **Unstuck criteria:** If subagent reports BLOCKED, help with the specific error (Neo4j, env, test failure); then ask them to retry from that checkpoint.
- **Done criteria:** Subagent provides the **Final summary** (what they ran, results, issues, recommendation). Only then consider Phase 2 verification complete.
- **Do not** mark Phase 2 "complete" in specs until the subagent’s final summary recommends it and user concurs.

---  
**You must:** run the verification steps below, report at each checkpoint, and end with a full summary.  
**Do not mark Phase 2 "complete"** until all steps are done and you report the final summary.

---

## Your objective

1. Run the space-transfer server with DevSpace enabled.
2. Run UDTS contract tests against it.
3. Optionally run a minimal end-to-end publish/pull flow.
4. Report progress at each checkpoint below; if blocked, report what you tried and the error.
5. When all work is done, provide a **full summary of your activity** (what you ran, results, any issues, recommendation).

---

## Prerequisites (you may need to set up)

- **Neo4j:** Phase 2 hub runs in the same process as SpaceTransfer, which requires Neo4j for `serve`.
  - Start Neo4j: `docker compose up -d` (from repo root).
  - Set env: `NEO4J_URI`, `NEO4J_USER`, `NEO4J_PASS` (e.g. `bolt://localhost:7687`, `neo4j`, `testpassword`).
- **Go:** `go build ./...` and `go test ./tests/udts/...` (without `UDTS_TARGET`) should already pass/skip.

---

## Checkpoint 1 — Server start

**Task:** Start the gRPC server with DevSpace enabled.

**Commands:**

```bash
cd <repo_root>
export NEO4J_URI=bolt://localhost:7687
export NEO4J_USER=neo4j
export NEO4J_PASS=testpassword
go run ./cmd/space-transfer serve -port 50051 -enable-devspace -devspace-data-dir .devspace/data
```

**Success:** Server logs show "DevSpace hub enabled" and "SpaceTransfer gRPC listening on :50051". No immediate exit or panic.

**You must report:**  
"Checkpoint 1: [PASS/FAIL/BLOCKED]. [One line: what happened.]"  
If BLOCKED, say what you tried and the exact error so the main agent can help.

---

## Checkpoint 2 — UDTS contract tests

**Task:** Run UDTS tests against the running server (in a **second** terminal).

**Commands:**

```bash
cd <repo_root>
UDTS_TARGET=localhost:50051 go test ./tests/udts/... -v -count=1
```

**Success:** All tests run (no skip due to missing UDTS_TARGET). At least: TestSpaceTransferListSpaces, TestSpaceTransferSpaceInfo, TestDevSpaceRegisterAgent, TestDevSpaceListExports, TestDevSpacePullExport. All PASS or expected status (e.g. PullExport NOT_FOUND for nonexistent id).

**You must report:**  
"Checkpoint 2: [PASS/FAIL/BLOCKED]. [One line: what happened.]"  
If any test failed, paste the failing test name and last few lines of output.

---

## Checkpoint 3 — Optional E2E (publish then pull)

**Task:** If you have a small `.mdemg` file (or can export one with `space-transfer export -space-id demo -output /tmp/demo.mdemg`), use a gRPC client to:

1. RegisterAgent(dev_space_id, agent_id).
2. PublishExport (stream: header chunk then file bytes).
3. ListExports and confirm the new export appears.
4. PullExport(dev_space_id, export_id) and confirm you receive the same byte length.

If you cannot run a gRPC client easily, say "Checkpoint 3: SKIPPED (no gRPC client)." and that is acceptable.

**You must report:**  
"Checkpoint 3: [PASS/SKIPPED/BLOCKED]. [One line.]"

---

## Checkpoint 4 — Final summary (required)

**Task:** After all checkpoints, write a single **Final summary** section that includes:

1. **What you ran:** Commands and environment (no secrets).
2. **Results:** Checkpoint 1/2/3 outcomes (PASS/FAIL/SKIP/BLOCKED).
3. **Issues:** Any errors, flakes, or missing dependencies.
4. **Recommendation:** Whether Phase 2 should be marked **complete** or remain **awaiting testing** (and why).

Keep this document updated as you go (e.g. fill in checkpoint results), then paste the Final summary into the chat for the main agent / user.

---

## If you get stuck

- **Server won’t start:** Check Neo4j is up and env vars are set; paste the exact log/panic.
- **Tests skip:** Ensure `UDTS_TARGET=localhost:50051` is set and server is running.
- **Tests fail:** Paste the failing test name and the relevant output (last 10–20 lines).
- Report "BLOCKED" at the relevant checkpoint and describe what you tried so the main agent can unblock.

---

## Reference

- Master plan: `docs/specs/development-space-collaboration.md`
- Phase 2 spec: `docs/specs/phase-devspace-hub.md`
- Server: `cmd/space-transfer main.go` (serve with `-enable-devspace`)
- UDTS tests: `tests/udts/contract_test.go`
