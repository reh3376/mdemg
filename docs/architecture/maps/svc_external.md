SVC|external|v1|external services,ports,protocols
SERVICES:
  mdemg-server|HTTP|:9999|150+ REST endpoints
  neo4j|Bolt|:7687|graph DB
  neo4j-browser|HTTP|:7474|admin UI
  neural-sidecar|HTTP|:8100|capabilities:rerank,NLI,tier-predict
  mcp-server|stdio|subprocess|IDE integration via MCP protocol

SIDECAR-ENDPOINTS:
  POST /rerank|cross-encoder re-ranking
  POST /nli|NL inference comprehension scoring
  POST /protocol/predict-tier|ML J17 tier prediction
  GET /health|sidecar health

GRPC-SERVICES:dir:api/proto/
  ModuleLifecycle|ops:handshake,health,shutdown|role:lifecycle management for all plugin modules
  IngestionModule|ops:Matches,Parse,Sync|role:ingest content into observations
  ReasoningModule|ops:Rerank|role:reorder retrieval candidates by semantic relevance score
  APEModule|ops:Evaluate|role:background self-improvement maintenance
  CRUDModule|ops:generic entity CRUD|role:external system integration via Linear module
  SpaceTransfer|ops:export,import|role:space migration between instances
  DevSpace|ops:agent workspace isolation|role:isolated dev environments
  HashVerification|ops:verify|role:UNTS spec integrity registry
