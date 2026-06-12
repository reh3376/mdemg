DEP|pkg-graph|v2|internal package import graph (NOTE: post-v0.7 packages tsdb/alert/supervisor/eventgraph/jobhealth/etc. not yet mapped — regeneration tooling pending, see DOC-AUDIT-001b findings)
LEGEND:→=imports,:annotation=reason-for-dependency,leaf=no internal deps

GRAPH:
ret→cfg,mdl,met,emb:indirect,plg:MatchIngestionModule-fallback,llm:rerank
hid→cfg,llm:emergence-naming
lrn→cfg,mdl,met
jim→cfg,mdl,emb,llm,ath:indirect
ape→cfg,met,ret:assess,plg:module-health,hid,jim,llm,tsd,con,enc,cbr
con→cfg,emb,met
cns→cfg,mdl,emb,ret,llm,sym
grd→emb:constraint-retrieval,llm:evaluator-via-llmclient,cbr:breaker
scr→emb,plg,lng:markdown-parser
mta→emb,llm,mdl
emb→met,cbr,rlm (no cfg import — provider selection happens in caller wiring)
lng→leaf:no internal deps
sym→leaf:no internal deps
cfg→leaf:no internal deps
mdl→leaf:no internal deps
plg→leaf:no internal deps
ath→leaf:no internal deps
api→ret,hid,lrn,jim,ape,con,cns,grd,lng,sym,emb,cfg,mdl,plg,ath,scr,xfr,mta,llm,unt,bkp,met,gap

WHY-ANNOTATIONS:
  ret depends on plg via PluginReasoningProvider (rerank dispatch, internal/retrieval/reasoning.go); MatchIngestionModule lives in plugins and is called from scraper, not retrieval
  ret→emb:indirect embedding lookup via cache
  ret→llm:rerank uses LLM for cross-encoder re-ranking
  hid→llm:emergence-naming uses LLM to name emergent L5 concepts
  grd→emb:constraint-retrieval finds constraints via embedding similarity
  emb wraps providers in CachedEmbedder(+rate-limit) chains; provider selection is done by the caller (serve wiring), not inside emb
