# MDEMG API Overview (gMEM)

<!-- markdownlint-disable MD060 -->

Quick reference for all HTTP API endpoints as Mermaid flowcharts (method, path, brief summary).  
**Options** (query/body/path params) and full request/response details: see `docs/development/API_REFERENCE.md`.  
Contract specs: `docs/api/api-spec/uats/specs/*.uats.json`.

**Base URL:** `http://localhost:9999` (default; port may be in `.mdemg.port` when using dynamic allocation.)

---

## Table of contents

- [[#Health & readiness]]
- [[#Memory operations]]
- [[#Codebase ingestion (background jobs)]]
- [[#Freshness & sync]]
- [[#Learning & Hebbian edges]]
- [[#Conversation Memory System (CMS)]]
- [[#CMS templates (Phase 60)]]
- [[#CMS snapshots (Phase 60)]]
- [[#CMS org reviews (Phase 60)]]
- [[#Self-improvement cycle — RSIC (Phase 60b)]]
- [[#Cleanup & edge consistency]]
- [[#Webhooks]]
- [[#Linear integration (Phase 44)]]
- [[#System, plugins & monitoring]]

---

## Health & readiness

```mermaid
flowchart TB
  subgraph health["Health & readiness"]
    h1["GET /healthz<br/>Liveness check; returns OK"]
    h2["GET /readyz<br/>Readiness check Neo4j + embedding"]
    h3["GET /v1/embedding/health<br/>Embedding provider health probe"]
  end
```

---

## Memory operations

```mermaid
flowchart TB
  subgraph mem["Memory operations"]
    m1["POST /v1/memory/retrieve<br/>Semantic retrieval + graph expansion"]
    m2["POST /v1/memory/ingest<br/>Ingest single observation"]
    m3["POST /v1/memory/ingest/batch<br/>Batch ingest"]
    m4["POST /v1/memory/reflect<br/>Deep reflection on topic"]
    m5["GET /v1/memory/stats<br/>Graph statistics"]
    m6["POST /v1/memory/consolidate<br/>Hidden-layer consolidation"]
    m7["POST /v1/memory/archive/bulk<br/>Bulk archive nodes"]
    m8["* /v1/memory/nodes/id<br/>Node CRUD"]
    m9["GET /v1/memory/symbols<br/>Search code symbols"]
    m10["GET /v1/memory/distribution<br/>Score distribution"]
    m11["GET /v1/memory/cache/stats<br/>Cache stats"]
    m12["DELETE /v1/memory/cache<br/>Clear query cache"]
    m13["GET /v1/memory/query/metrics<br/>Query performance metrics"]
    m14["POST /v1/memory/consult<br/>SME-style Q&A"]
    m15["POST /v1/memory/suggest<br/>Suggest related concepts"]
  end
```

---

## Codebase ingestion (background jobs)

```mermaid
flowchart TB
  subgraph ingest["Codebase ingestion"]
    i1["POST /v1/memory/ingest/trigger<br/>Start background job"]
    i2["GET /v1/memory/ingest/status/id<br/>Job progress"]
    i3["POST /v1/memory/ingest/cancel/id<br/>Cancel job"]
    i4["GET /v1/memory/ingest/jobs<br/>List jobs"]
    i5["POST /v1/memory/ingest/files<br/>Ingest specific files"]
    i6["* /v1/memory/ingest-codebase<br/>Deprecated"]
  end
```

---

## Freshness & sync

```mermaid
flowchart TB
  subgraph fresh["Freshness & sync"]
    f1["GET /v1/memory/spaces/id/freshness<br/>Space freshness status"]
    f2["GET /v1/memory/freshness<br/>Batch freshness"]
  end
```

---

## Learning & Hebbian edges

```mermaid
flowchart TB
  subgraph learn["Learning & Hebbian"]
    l1["POST /v1/learning/prune<br/>Prune low-weight edges"]
    l2["GET /v1/learning/stats<br/>Learning edge stats"]
    l3["POST /v1/learning/freeze<br/>Freeze learning"]
    l4["POST /v1/learning/unfreeze<br/>Unfreeze learning"]
    l5["GET /v1/learning/freeze/status<br/>Freeze status"]
  end
```

---

## Conversation Memory System (CMS)

```mermaid
flowchart TB
  subgraph cms["Conversation Memory System"]
    c1["POST /v1/conversation/observe<br/>Store observation"]
    c2["POST /v1/conversation/correct<br/>Store correction"]
    c3["POST /v1/conversation/resume<br/>Resume session"]
    c4["POST /v1/conversation/recall<br/>Recall memory"]
    c5["POST /v1/conversation/consolidate<br/>Consolidate themes"]
    c6["GET /v1/conversation/volatile/stats<br/>Volatile stats"]
    c7["POST /v1/conversation/graduate<br/>Graduate to permanent"]
    c8["GET /v1/conversation/session/health<br/>Session health"]
  end
```

---

## CMS templates (Phase 60)

```mermaid
flowchart TB
  subgraph tmpl["CMS templates"]
    t1["GET /v1/conversation/templates<br/>List templates"]
    t2["POST /v1/conversation/templates<br/>Create template"]
    t3["GET /v1/conversation/templates/id<br/>Get template"]
    t4["PUT /v1/conversation/templates/id<br/>Update template"]
    t5["DELETE /v1/conversation/templates/id<br/>Delete template"]
  end
```

---

## CMS snapshots (Phase 60)

```mermaid
flowchart TB
  subgraph snap["CMS snapshots"]
    s1["GET /v1/conversation/snapshot<br/>List snapshots"]
    s2["POST /v1/conversation/snapshot<br/>Create snapshot"]
    s3["GET /v1/conversation/snapshot/latest<br/>Latest for session"]
    s4["POST /v1/conversation/snapshot/cleanup<br/>Cleanup old snapshots"]
    s5["GET /v1/conversation/snapshot/id<br/>Get by ID"]
    s6["DELETE /v1/conversation/snapshot/id<br/>Delete snapshot"]
  end
```

---

## CMS org reviews (Phase 60)

```mermaid
flowchart TB
  subgraph org["CMS org reviews"]
    o1["GET /v1/conversation/org-reviews<br/>List pending reviews"]
    o2["GET /v1/conversation/org-reviews/stats<br/>Review statistics"]
    o3["POST /v1/conversation/org-reviews/id/decision<br/>Approve or reject"]
    o4["POST /v1/conversation/observations/id/flag-org<br/>Flag for review"]
  end
```

---

## Self-improvement cycle — RSIC (Phase 60b)

```mermaid
flowchart TB
  subgraph rsic["Self-improvement RSIC"]
    r1["POST /v1/self-improve/assess<br/>On-demand assessment"]
    r2["GET /v1/self-improve/report<br/>Active task report"]
    r3["GET /v1/self-improve/report/cycle_id<br/>Cycle report"]
    r4["POST /v1/self-improve/cycle<br/>Full RSIC cycle"]
    r5["GET /v1/self-improve/history<br/>Cycle history"]
    r6["GET /v1/self-improve/calibration<br/>Calibration metrics"]
    r7["GET /v1/self-improve/health<br/>Watchdog health"]
  end
```

---

## Cleanup & edge consistency

```mermaid
flowchart TB
  subgraph cleanup["Cleanup & edge consistency"]
    u1["POST /v1/memory/cleanup/orphans<br/>Orphan archive/delete"]
    u2["POST /v1/memory/cleanup/schedule<br/>Schedule cleanup"]
    u3["GET /v1/memory/cleanup/schedules<br/>List schedules"]
    u4["GET /v1/memory/cleanup/stats<br/>Cleanup stats"]
    u5["GET /v1/memory/edges/stale/stats<br/>Stale edge stats"]
    u6["POST /v1/memory/edges/stale/refresh<br/>Refresh stale edges"]
  end
```

---

## Webhooks

```mermaid
flowchart TB
  subgraph web["Webhooks"]
    w1["POST /v1/webhooks/linear<br/>Linear webhook verified"]
    w2["POST /v1/webhooks/source<br/>Generic webhook"]
  end
```

---

## Linear integration (Phase 44)

```mermaid
flowchart TB
  subgraph linear["Linear integration"]
    L1["GET /v1/linear/issues<br/>List issues"]
    L2["POST /v1/linear/issues<br/>Create issue"]
    L3["GET /v1/linear/issues/id<br/>Read issue"]
    L4["PUT /v1/linear/issues/id<br/>Update issue"]
    L5["DELETE /v1/linear/issues/id<br/>Delete issue"]
    L6["GET /v1/linear/projects<br/>List projects"]
    L7["POST /v1/linear/projects<br/>Create project"]
    L8["GET /v1/linear/projects/id<br/>Read project"]
    L9["PUT /v1/linear/projects/id<br/>Update project"]
    L10["POST /v1/linear/comments<br/>Create comment"]
  end
```

---

## System, plugins & monitoring

```mermaid
flowchart TB
  subgraph sys["System, plugins & monitoring"]
    x1["GET /v1/metrics<br/>Prometheus metrics"]
    x2["GET /v1/metrics/snapshot<br/>JSON metrics snapshot"]
    x3["GET /v1/modules<br/>List modules"]
    x4["POST /v1/modules/id<br/>Module sync"]
    x5["GET /v1/plugins<br/>List plugins"]
    x6["POST /v1/plugins<br/>Register plugin"]
    x7["PUT /v1/plugins<br/>Update plugin"]
    x8["DELETE /v1/plugins<br/>Remove plugin"]
    x9["POST /v1/plugins/create<br/>Create from spec"]
    x10["GET /v1/ape/status<br/>APE scheduler"]
    x11["POST /v1/ape/trigger<br/>Trigger APE"]
    x12["POST /v1/feedback<br/>Submit feedback"]
    x13["GET /v1/system/capability-gaps<br/>List gaps"]
    x14["* /v1/system/capability-gaps/id<br/>Gap CRUD"]
    x15["GET /v1/system/gap-interviews<br/>Interviews"]
    x16["* /v1/system/gap-interviews/id<br/>Interview CRUD"]
    x17["GET /v1/system/pool-metrics<br/>Neo4j pool"]
    x18["GET /v1/jobs/id/stream<br/>SSE job progress"]
  end
```

---

## Related docs

- **Full API reference:** `docs/development/API_REFERENCE.md`
- **UATS specs (per-endpoint):** `docs/api/api-spec/uats/specs/*.uats.json`
- **Contributing & testing:** `CONTRIBUTING.md`
