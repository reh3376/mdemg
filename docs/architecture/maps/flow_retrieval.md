FLOW|retrieval|v2|retrieval request pipeline from HTTP to response (RRF default since Phase 13.1)
IN:POST /v1/memory/retrieve
→api.handleRetrieve|validate+auth-ctx; embeds query_text server-side when no embedding supplied
→ret.Retrieve|orchestrate
  ├─emb.Embed|query→3072-dim vector (CachedEmbedder chain)
  ├─neo4j.VectorSearch|memNodeEmbedding index→top-K candidates
  ├─ret.BM25|lexical candidates fused with vector set
  ├─sym.PatternMatch|if #symbol query→symbol candidates
  ├─ret.SpreadingActivation|internal/retrieval/activation.go,CO_ACTIVATED_WITH+structural edges,squared activation
  ├─ret.ScoreAndRankRRF|DEFAULT (RETRIEVAL_COLUMN_VOTING_ENABLED=true):RRF fusion over columns Embedding:0.50+BM25:0.20+Graph:0.15+Structural:0.15(+Context column per-category)→fused score+consensus_strength; fail-open→legacy
  │  └─ret.ScoreAndRank|LEGACY/fallback:vector:0.60+activation:0.20+recency:0.15+confidence:0.05-hub:0.08-redundancy:0.12
  ├─ret.SparseGate|internal/retrieval/gate.go,default-on (Phase 14.1.1):percentile-activation trim pre-rerank,SPARSE_MIN_ACTIVE floor
  ├─ret.Rerank|rerank.go/rerank_neural.go:cross-encoder/LLM rerank stage when enabled
  ├─ret.Cache|TTL-LRU check/store,namespace keyed by scorerVersion (weight/hop changes flip namespace)
  └─ret.CollectTraining|write (query,candidate,score) JSONL→neural sidecar training (rerank_collector.go)
OUT:mdl.RetrieveResponse|results[]+consensus_strength+debug+has_more (unwrapped; RRF score scale ~0.49–0.58 top — not the legacy 0–1+ scale)
