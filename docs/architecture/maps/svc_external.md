SVC|external|v2|external services,ports,protocols
SERVICES:
  mdemg-server|HTTP|:9999|~185 REST endpoints (see docs/api/route_consumer_inventory.json)
  neo4j|Bolt|:7687|graph DB
  neo4j-browser|HTTP|:7474|admin UI
  timescaledb|postgres|:5433 host (5432 internal)|telemetry plane: mdemg_metrics DB,hypertables V0001+,user mdemg
  llama-server|HTTP|127.0.0.1:8102|MANDATORY LLM runtime (llama.cpp,OpenAI-compat) serving mdemg-llm-v1.Q5_K_M.gguf; mdemg refuses to start without it (16 LLM call sites)
  nli-sidecar|HTTP|127.0.0.1:8101|J17 NLI sidecar (Python):rerank,NLI,tier-predict
  grafana|HTTP|:3001|dashboards (compose); alerting is server-native — Grafana optional
  mcp-server|stdio|subprocess|IDE integration via MCP protocol (23 tools,MDEMG_SPACE_ID-scoped)

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
