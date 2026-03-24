DEP|pkg-graph|v1|internal package import graph
LEGEND:→=imports,:annotation=reason-for-dependency,leaf=no internal deps

GRAPH:
ret→cfg,mdl,met,emb:indirect,plg:MatchIngestionModule-fallback,llm:rerank
hid→cfg,llm:emergence-naming
lrn→cfg,mdl,met
jim→cfg,mdl,emb,llm,ath:indirect
ape→cfg,met,ret:assess,plg:module-health
con→cfg,emb,met
cns→cfg,mdl,emb,ret,llm,sym
grd→emb:constraint-retrieval
scr→emb,plg,lng:markdown-parser
mta→emb,llm,mdl
emb→met,cfg:provider-selection
lng→leaf:no internal deps
sym→leaf:no internal deps
cfg→leaf:no internal deps
mdl→leaf:no internal deps
plg→leaf:no internal deps
ath→leaf:no internal deps
api→ret,hid,lrn,jim,ape,con,cns,grd,lng,sym,emb,cfg,mdl,plg,ath,scr,xfr,mta,llm,unt,bkp,met,gap

WHY-ANNOTATIONS:
  ret depends on plg because retrieval calls MatchIngestionModule() as fallback to dispatch unrecognized file types to the correct plugin module for parsing
  ret→emb:indirect embedding lookup via cache
  ret→llm:rerank uses LLM for cross-encoder re-ranking
  hid→llm:emergence-naming uses LLM to name emergent L5 concepts
  grd→emb:constraint-retrieval finds constraints via embedding similarity
  emb→cfg:provider-selection chooses OpenAI vs Ollama based on config
