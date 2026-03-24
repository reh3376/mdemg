FLOW|retrieval|v1|retrieval request pipeline from HTTP to response
IN:POST /v1/memory/retrieve
→api.handleRetrieve|validate+auth-ctx
→ret.Retrieve|orchestrate
  ├─emb.Embed|query→3072-dim vector
  ├─neo4j.VectorSearch|memNodeEmbedding index→top-K candidates
  ├─sym.PatternMatch|if #symbol query→symbol candidates
  ├─hid.BoundedExpand|1-hop graph expansion,max-depth=3
  ├─lrn.SpreadActivation|CO_ACTIVATED_WITH edges,squared activation,local-first
  ├─ret.Score|vector:0.55+activation:0.30+recency:0.10+confidence:0.05-hub:0.08-redundancy:0.12
  ├─ret.Cache|TTL-LRU check/store
  └─ret.CollectTraining|write (query,candidate,score) JSONL→neural sidecar training
OUT:mdl.RetrieveResponse|results[]+evidence_metrics+has_more
