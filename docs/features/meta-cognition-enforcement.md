# CMS ANN Meta-Cognition & Self-Improvement Enforcement

Phase 80 transforms MDEMG from passive memory retrieval to active anomaly detection and enforcement. When memory state is degraded, the system emits signals through API responses and hooks to force investigation.

## How It Works

### Server-Side Anomaly Detection

Resume and recall handlers check for anomalous states after computing results:

1. **Empty Resume** (CRITICAL): `countSpaceNodes()` finds conversation_observation nodes in the space. If nodes exist but resume returned 0 observations → anomaly emitted.
2. **No Themes** (MEDIUM): Observations returned but 0 themes → anomaly emitted.
3. **Empty Recall** (HIGH): Query >20 chars but 0 results → anomaly emitted.

Anomalies are embedded in both response body (`anomalies` array, `memory_state` field) and HTTP headers (`X-MDEMG-Memory-State`, `X-MDEMG-Anomaly`).

False-positive guard: A genuinely empty space (0 conversation_observation nodes) is NOT anomalous. The check specifically filters by `role_type='conversation_observation'` to avoid counting codebase nodes.

### Hook Circuit Breakers

Hooks mechanically enforce investigation:

- **session-start.sh**: Detects 0-observation resume → emits CRITICAL warning box → auto-fires RSIC micro assessment → displays RSIC health summary. If health < 0.5, appends degraded health investigation checklist.
- **prompt-context.sh**: Detects empty recall for non-trivial queries → emits warning. Appends session health ribbon.
- **post-tool-observe.py**: Detects `X-MDEMG-Memory-State: degraded` and empty resume patterns in curl output → records error observations.
- **pre-compact.sh**: Queries session health before compaction → includes in context snapshot. Warns if health < 0.3.

### Multi-Dimensional Watchdog

The Watchdog (previously temporal-decay only) now monitors:

- Session health score (via `WatchdogSignalProvider`)
- Observation rate per hour
- Consolidation age (seconds since last consolidation)

Critical session health (<0.2) combined with moderate decay triggers escalation.

### Behavioral Learning Loop

`SignalLearner` tracks signal effectiveness using Hebbian learning:

- `RecordEmission(code)`: Signal was emitted → strength decays (agent didn't respond yet)
- `RecordResponse(code)`: Agent acted on signal → strength boosts
- Strength range: 0.1 (floor) to 1.0 (ceiling)
- Default strength: 0.5, decay rate: 0.05, boost rate: 0.1

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `METACOG_ENABLED` | `true` | Master toggle for meta-cognition |
| `METACOG_EMPTY_RESUME_CHECK` | `true` | Enable empty-resume anomaly detection |
| `METACOG_SIGNAL_DECAY_RATE` | `0.05` | Hebbian decay per ignored emission |
| `METACOG_SIGNAL_BOOST_RATE` | `0.1` | Hebbian boost per agent response |

## Usage Examples

```bash
# Check session anomalies
curl -s "http://localhost:9999/v1/conversation/session/anomalies?session_id=claude-core&space_id=mdemg-dev" | jq

# Check signal effectiveness
curl -s "http://localhost:9999/v1/self-improve/signals" | jq

# Resume with anomaly detection (anomalies in response body)
curl -s -D- -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","max_observations":10}' | head -20
```

## Related Files

| File | Purpose |
|------|---------|
| `internal/models/models.go` | AnomalySignal type, extended responses |
| `internal/api/handlers_conversation.go` | Anomaly detection logic, session anomalies endpoint |
| `internal/conversation/service.go` | Jiminy warning rationale on empty state |
| `internal/ape/signal_learner.go` | Hebbian signal effectiveness tracker |
| `internal/ape/watchdog.go` | Multi-dimensional monitoring |
| `internal/ape/types_rsic.go` | WatchdogSignalProvider interface |
| `internal/api/handlers_self_improve.go` | Signal tracking + signals endpoint |
| `internal/api/rsic_adapters.go` | rsicWatchdogSignalAdapter |
| `internal/config/config.go` | METACOG_* config vars |
| `.claude/hooks/session-start.sh` | 0-obs detection, RSIC health display |
| `.claude/hooks/prompt-context.sh` | Empty-recall warning, health ribbon |

## Dependencies

- Phase 60b (RSIC) — assess/reflect/plan/dispatch/monitor/calibrate endpoints
- Phase 43A (CMS Enforcement) — hooks infrastructure
