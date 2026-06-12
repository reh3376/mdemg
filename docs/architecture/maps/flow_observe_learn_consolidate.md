FLOW|observe-learn-consolidate|v1|observation ingestion,Hebbian learning,consolidation pipeline
IN:POST /v1/conversation/observe
→api.handleObserve
→con.Observe|surprise-score+embed+quality-gate
  ├─emb.Embed|content→3072-dim
  ├─anomaly.Check|duplicate detection
  └─neo4j.MERGE|MemoryNode+Observation
OUT:obs_id+node_id+surprise_score

TRIGGER:co-retrieval activates learning
→lrn.ApplyCoactivation|+ApplySymbolCoactivation,CoactivateSession,ApplyNegativeFeedback (4 Hebbian paths, all federated to reinforcement_events)
  └─neo4j|CREATE/MERGE CO_ACTIVATED_WITH edges,Hebbian formula+tanh soft-cap

TRIGGER:post-ingest or POST /v1/memory/consolidate or RSIC trigger_consolidation
→hid.RunConsolidation|22-phase pipeline
  ├─hid.ComputeDistanceMatrix|O(n²) pairwise embedding similarity
  ├─hid.DBSCANWithMatrix|5× adaptive iterations (eps scales 0.10→0.26 per layer)
  ├─hid.ForwardPass/BackwardPass|forward L0→L5,backward L5→L0 (GraphSAGE-style)
  ├─hid.CreateConcernNodes|cross-cutting concern detection
  ├─hid.CreateComparisonNodes|architectural comparison
  ├─hid.CreateConstraintNodes|constraint lifecycle
  └─hid.DynamicEmergence|LLM-named concepts (Phase 103)
