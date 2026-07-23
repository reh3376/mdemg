# FTLOOP-DRILL-001 — Overnight Runbook (self-contained)

> Executed by the scheduled overnight session. Repo: `/Users/reh3376/mdemg`,
> branch `reh3376_dev01`. Operator authorization: "run the ftloop drill
> overnight" (2026-07-22). HARD RULES: never run `mdemg ft-loop promote`;
> never restart the production llama-server (:8102) with the candidate;
> revert env in teardown the same night.

## 0. Pre-checks (abort the drill if any fails; report instead)

```bash
cd /Users/reh3376/mdemg && git pull --ff-only
curl -s -m 5 http://localhost:9999/healthz | python3 -c "import json,sys; print(json.load(sys.stdin).get('status'))"   # ok
curl -s -m 5 http://127.0.0.1:8102/v1/models >/dev/null && echo llama-up
prod_pid=$(pgrep -f "llama-server.*8102" | head -1); echo "prod llama pid: $prod_pid"   # record for E2
df -g / | tail -1 | awk '{print "free_gb:", $4}'                                        # ≥ 100
lsof -nP -iTCP:18102 -sTCP:LISTEN | tail -1 || echo "18102 free"
docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics -tAc \
  "SELECT count(*) FROM ft_training_cycles WHERE status NOT IN ('promoted','failed','rolled_back')"  # 0 open
ls ~/.mdemg/ft-loop.lease 2>/dev/null && echo "STALE LEASE — investigate" || echo "no lease"
```

## 1. Stage env + restart

Append to `.env` (do NOT commit `.env`):

```
FT_LOOP_ENABLED=true
FT_LOOP_EXPORT_SINCE_DAYS=1
FT_LOOP_GATE_TASK_FILTER=consulting.classify
FT_LOOP_POLL_INTERVAL_SEC=30
```

```bash
launchctl kickstart -k gui/$(id -u)/com.mdemg.server
sleep 12; curl -s -m 5 http://localhost:9999/healthz >/dev/null && echo up
grep -E "ft-loop" ~/.mdemg/logs/server.log | tail -3   # controller loop start line expected
```

## 2. Open the cycle (the Gate.OpenCycle row shape, CUIDv2 id)

```bash
cd /Users/reh3376/mdemg
cid=$(cat > /tmp/gen_cuid.go <<'EOF'
package main
import ("fmt"; "github.com/nrednav/cuid2")
func main() { fmt.Print(cuid2.Generate()) }
EOF
go run /tmp/gen_cuid.go)
echo "cycle_id=$cid"
docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics -c \
  "INSERT INTO ft_training_cycles (time, cycle_id, model_version, status, stage) \
   VALUES (now(), '$cid', 'mdemg-llm-v1', 'triggered', 'triggered')"
```

(`go run` with a file under /tmp still resolves the repo module if run from
the repo root via `go run` on a path outside the module — if it errors,
create the file under the repo as `tools_drill_cuid.go` with a
`//go:build ignore` tag and `go run` that instead, then delete it.)

## 3. Supervise (poll ≤ every 5 min; total expected 45–120 min)

```bash
# ledger state machine: triggered → curating → training → gating → promote_pending|failed
docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics -tAc \
  "SELECT time, status, stage, COALESCE(failure_reason,'') FROM ft_training_cycles WHERE cycle_id='$cid' ORDER BY time"
# per-stage jobhealth
docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics -tAc \
  "SELECT time, job_name, success, latency_ms, COALESCE(error_message,'') FROM scheduled_job_events WHERE job_name LIKE 'ft-loop:%' AND time > now() - interval '3 hours' ORDER BY time"
# logs
grep -E "ft-loop" ~/.mdemg/logs/server.log | tail -20
# lease + quiesce evidence while training:
ls -la ~/.mdemg/ft-loop.lease 2>/dev/null
```

Notes from Epic 6 (expected behaviors, not errors): curate runs
`paradigm_router` with cwd `neural/` in the venv; controller copies
`val.jsonl`→`valid.jsonl`; convert = `mlx_lm.fuse --dequantize` → bf16 →
`convert_hf_to_gguf` → `llama-quantize Q5_K_M` (~5 min, ~60 GB transient);
gate serves the candidate on :18102 and runs the filtered benchmark
(no-zero-call guard). Training saturates the box — this is the reason for
the overnight window; do not run other heavy work.

If a stage FAILS: that is a VALID drill outcome — capture the ledger
failure_reason + the stage's jobhealth error + relevant log lines, confirm
the cycle reached terminal `failed`, and continue to teardown. Fix specs are
follow-up work, not same-night work (unless trivial).

## 4. Terminal state

- `promote_pending`: record the gate score from the logs/benchmark report.
  **DO NOT PROMOTE.** The cycle legitimately stays open in promote_pending —
  record that the operator will decide; note it in drill_record.md and tell
  the operator in the morning report. (Single-flight: a pending cycle
  suppresses new triggers — acceptable; the operator resolves it.)
- `failed`: record reason; ledger is terminal, nothing to resolve.

## 5. Teardown (same night, always)

```bash
cd /Users/reh3376/mdemg
# remove the 4 drill lines from .env (keep everything else byte-identical)
python3 - <<'EOF'
lines = open('.env').read().splitlines(keepends=True)
drop = {"FT_LOOP_ENABLED=true","FT_LOOP_EXPORT_SINCE_DAYS=1",
        "FT_LOOP_GATE_TASK_FILTER=consulting.classify","FT_LOOP_POLL_INTERVAL_SEC=30"}
open('.env','w').writelines(l for l in lines if l.strip() not in drop)
print("env reverted")
EOF
launchctl kickstart -k gui/$(id -u)/com.mdemg.server
sleep 12; curl -s -m 5 http://localhost:9999/healthz >/dev/null && echo up
# verify: lease gone, production llama-server SAME pid/model as step 0
ls ~/.mdemg/ft-loop.lease 2>/dev/null || echo "lease released"
pgrep -f "llama-server.*8102" | head -1        # compare to recorded prod_pid
curl -s http://127.0.0.1:8102/v1/models | head -c 120
rm -f /tmp/gen_cuid.go
```

## 6. Evidence + docs (before ending the overnight session)

Write `docs/development/ftloop-drill-001/drill_record.md`: timeline table
(stage, start, duration, outcome), cycle_id, gate score or failure reason,
lease/quiesce evidence, prod-pid before/after, any surprises. Update
CHANGELOG + CLAUDE.md FT-RECURSIVE-002 note ("remaining" → drill complete,
one-line result) + post.md. Commit (`docs(ftloop-drill-001): …` +
fix-commits for any surprise defects) and push. Leave a clear morning
summary as the final message.
