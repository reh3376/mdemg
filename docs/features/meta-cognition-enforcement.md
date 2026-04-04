---
created: 2026-02-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "80"
---

# CMS ANN Meta-Cognition & Self-Improvement Enforcement

## Summary

**Feature**: Meta-Cognition Enforcement
**Summary**: Transforms MDEMG from passive memory retrieval to active anomaly detection and enforcement. When memory state is degraded, the system emits signals through API responses and hooks to force investigation.

## Vision & Goals

A cognitive substrate that cannot detect its own degradation is unreliable. Meta-cognition enforcement ensures that MDEMG actively monitors its own health — detecting empty resumes, missing themes, and failed recalls — and escalates through API responses, HTTP headers, and hook-based circuit breakers. This is the foundation of RSIC self-improvement: the system must first know something is wrong before it can fix it.

## Current State

### Architecture

**Server-Side Anomaly Detection** — Resume and recall handlers check for anomalous states:

1. **Empty Resume** (CRITICAL): `countSpaceNodes()` finds conversation_observation nodes. If nodes exist but resume returned 0 observations, anomaly emitted.
2. **No Themes** (MEDIUM): Observations returned but 0 themes, anomaly emitted.
3. **Empty Recall** (HIGH): Query >20 chars but 0 results, anomaly emitted.

Anomalies embedded in both response body (`anomalies` array, `memory_state` field) and HTTP headers (`X-MDEMG-Memory-State`, `X-MDEMG-Anomaly`).

False-positive guard: Genuinely empty spaces (0 conversation_observation nodes) are NOT anomalous.

**Hook Circuit Breakers:**

- **session-start.sh**: 0-observation resume -> CRITICAL warning -> auto-fires RSIC micro assessment -> RSIC health summary. Health < 0.5 appends degraded investigation checklist.
- **prompt-context.sh**: Empty recall for non-trivial queries -> warning + session health ribbon.
- **post-tool-observe.py**: Detects `X-MDEMG-Memory-State: degraded` in curl output -> records error observations.
- **pre-compact.sh**: Queries session health before compaction -> includes in context snapshot.

**Multi-Dimensional Watchdog** — monitors session health score, observation rate per hour, consolidation age. Critical session health (<0.2) combined with moderate decay triggers escalation.

### Workflow

**Behavioral Learning Loop** — `SignalLearner` tracks signal effectiveness using Hebbian learning:

- `RecordEmission(code)`: Signal emitted -> strength decays (agent didn't respond yet)
- `RecordResponse(code)`: Agent acted on signal -> strength boosts
- Strength range: 0.1 (floor) to 1.0 (ceiling)
- Default strength: 0.5, decay rate: 0.05, boost rate: 0.1

### Configuration

See Configuration Reference table below.

## Notes

### Known Limitations

- SignalLearner state is in-memory only — lost on server restart (see P1-1 in assessment report)
- Staleness thresholds are hardcoded in hooks

### Risks & Gaps

- No persistence for SignalLearner (identified in codebase assessment as P1-1)

### Future Improvements

- Persistent signal learner backed by Neo4j
- Configurable anomaly thresholds via env vars

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| GET | `/v1/conversation/session/anomalies` | Check session anomalies | N/A |
| GET | `/v1/self-improve/signals` | Signal effectiveness tracking | N/A |
| POST | `/v1/conversation/resume` | Resume with anomaly detection in response body + headers | `specs/resume.uats.json` |
| POST | `/v1/conversation/recall` | Recall with anomaly detection | `specs/recall.uats.json` |

## CLI Commands

None — anomaly detection is automatic via API responses and hooks.

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `METACOG_ENABLED` | `true` | Master toggle for meta-cognition |
| `METACOG_EMPTY_RESUME_CHECK` | `true` | Enable empty-resume anomaly detection |
| `METACOG_SIGNAL_DECAY_RATE` | `0.05` | Hebbian decay per ignored emission |
| `METACOG_SIGNAL_BOOST_RATE` | `0.1` | Hebbian boost per agent response |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| RSIC Engine (Phase 60b) | Feeds into — anomalies trigger RSIC micro assessments |
| CMS Hooks (Phase 43A) | Requires — hook circuit breakers enforce investigation |
| Watchdog | Enhances — multi-dimensional monitoring with session health |
| SignalLearner | Requires — tracks signal effectiveness via Hebbian learning |

## Related Files

- `internal/models/models.go` - AnomalySignal type, extended responses
- `internal/api/handlers_conversation.go` - Anomaly detection logic, session anomalies endpoint
- `internal/conversation/service.go` - Jiminy warning rationale on empty state
- `internal/ape/signal_learner.go` - Hebbian signal effectiveness tracker
- `internal/ape/watchdog.go` - Multi-dimensional monitoring
- `internal/ape/types_rsic.go` - WatchdogSignalProvider interface
- `internal/api/handlers_self_improve.go` - Signal tracking + signals endpoint
- `internal/config/config.go` - METACOG_* config vars
- `.claude/hooks/session-start.sh` - 0-obs detection, RSIC health display
- `.claude/hooks/prompt-context.sh` - Empty-recall warning, health ribbon
