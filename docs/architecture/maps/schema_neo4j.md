SCHEMA|neo4j|v1|Neo4j graph schema:nodes,edges,layer hierarchy
NODES:
  MemoryNode|primary memory,3072-dim vector index:memNodeEmbedding
  Observation|append-only events→MemoryNode
  HiddenPattern|L1+ DBSCAN aggregation,inherits parent embedding
  EmergentConcept|LLM-named L5 concepts(Phase 103)
  Concern|cross-cutting(auth,error-handling,validation,logging,caching)
  Comparison|architectural(ModuleA vs ModuleB)
  Constraint|enforced rules(must|must_not|should|should_not)
  TemporalPattern|time-based patterns
  ConfigPattern|configuration summaries
  ConversationObs|CMS observations(secondary label)
  ConversationTheme|CMS themes

EDGES:
  ASSOCIATED_WITH|structural:file→function,module→class
  CO_ACTIVATED_WITH|Hebbian:co-retrieval strength,tanh soft-cap,cautious decay
  GROUNDED_BY|skip-connect:L5→L0 prevents grounding loss
  IMPLEMENTS_CONCERN|file→concern node
  COMPARED_IN|module→comparison node
  BRIDGES|L5↔L5 cross-domain
  COMPOSES_WITH|L5↔L5 composition
  CONTRADICTS|known conflict between concepts
  THEME_OF|theme→memory node
  GUIDANCE_OUTCOME|jiminy feedback:followed|partial_compliance|ignored|contradicted|not_applicable (props: guidance_type, outcome_type, similarity, guidance_id, session_id, created_at)

LAYERS:
  L0:base observations|eps=0.10,minSamples=3
  L1:hidden aggregators|eps=0.14,minSamples=3
  L2:concrete concepts|eps=0.18,minSamples=2
  L3:domain concepts|eps=0.22,minSamples=2
  L4:abstract concepts|eps=0.26,minSamples=2
  L5:emergent concepts|LLM-named,very loose clustering
