# Hybrid Retrieval & LLM Re-ranking Specification

**Target**: Improve retrieval accuracy from ~0.56 to ~0.69 average (23% improvement)

## Executive Summary

Current retrieval relies solely on vector similarity, which misses:

1. **Keyword matches** - Query "lib/graphql" doesn't strongly match nodes without similar embedding
2. **Semantic understanding** - Embeddings can't capture nuanced query intent

This spec introduces two complementary improvements:

1. **Hybrid Retrieval**: Combine vector search with keyword/full-text search (BM25)
2. **LLM Re-ranking**: Use lightweight LLM to re-order top candidates based on actual relevance

## Architecture Overview

```
Query Text
    │
    ├──────────────────────┬────────────────────┐
    │                      │                    │
    ▼                      ▼                    ▼
┌─────────────┐    ┌─────────────┐    ┌─────────────────┐
│ Vector Search│    │ BM25 Search │    │ Query Expansion │
│ (embedding)  │    │ (keywords)  │    │   (optional)    │
└─────────────┘    └─────────────┘    └─────────────────┘
    │                      │                    │
    ▼                      ▼                    ▼
┌─────────────────────────────────────────────────────────┐
│              Reciprocal Rank Fusion (RRF)               │
│         Combines results from multiple retrievers       │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│                  Spreading Activation                    │
│               (existing graph traversal)                │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│                   LLM Re-ranking                        │
│   Re-score top-N candidates using semantic judgment     │
└─────────────────────────────────────────────────────────┘
    │
    ▼
Final Results (top-K)
```

---

## Part 1: Hybrid Retrieval (Vector + BM25)

### 1.1 Problem Statement

Vector similarity alone fails when:

- Query contains specific identifiers ("DeltaSyncModule", "useAgentSocket")
- Query mentions paths ("lib/graphql", "frontend/")
- Query uses domain terminology not well-represented in embeddings

### 1.2 Solution: BM25 Full-Text Search

BM25 (Best Matching 25) is a ranking function that scores documents based on:

- Term frequency (TF) in the document
- Inverse document frequency (IDF) across corpus
- Document length normalization

**Neo4j Implementation**: Use Neo4j's full-text index with Lucene analyzer.

### 1.3 Implementation Plan

#### Step 1: Create Full-Text Index

```cypher
-- Migration V0006__fulltext_index.cypher
CREATE FULLTEXT INDEX memNodeFullText IF NOT EXISTS
FOR (n:MemoryNode)
ON EACH [n.name, n.path, n.summary, n.description]
OPTIONS {
  indexConfig: {
    `fulltext.analyzer`: 'standard-folding',
    `fulltext.eventually_consistent`: false
  }
};
```

#### Step 2: BM25 Query Function

```go
// internal/retrieval/bm25.go

type BM25Result struct {
    NodeID string
    Score  float64
    Path   string
    Name   string
}

func (s *Service) BM25Search(ctx context.Context, spaceID, query string, topK int) ([]BM25Result, error) {
    sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
    defer sess.Close(ctx)

    cypher := `
    CALL db.index.fulltext.queryNodes("memNodeFullText", $query)
    YIELD node, score
    WHERE node.space_id = $spaceId
      AND NOT coalesce(node.is_archived, false)
    RETURN node.node_id AS node_id,
           node.path AS path,
           node.name AS name,
           score
    ORDER BY score DESC
    LIMIT $topK`

    // Execute and return results...
}
```

#### Step 3: Reciprocal Rank Fusion (RRF)

RRF combines ranked lists without requiring score normalization:

```
RRF_score(d) = Σ 1 / (k + rank_i(d))
```

Where:

- `k` is a constant (typically 60)
- `rank_i(d)` is the rank of document d in list i

```go
// internal/retrieval/fusion.go

const RRFConstant = 60

type FusedCandidate struct {
    NodeID    string
    RRFScore  float64
    VectorRank int
    BM25Rank   int
    // Original data...
}

func ReciprocalRankFusion(vectorResults []Candidate, bm25Results []BM25Result) []FusedCandidate {
    scores := make(map[string]float64)

    // Add vector scores
    for rank, c := range vectorResults {
        scores[c.NodeID] += 1.0 / float64(RRFConstant + rank + 1)
    }

    // Add BM25 scores
    for rank, r := range bm25Results {
        scores[r.NodeID] += 1.0 / float64(RRFConstant + rank + 1)
    }

    // Convert to sorted slice...
}
```

### 1.4 Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `HYBRID_RETRIEVAL_ENABLED` | `true` | Enable hybrid vector+BM25 |
| `BM25_WEIGHT` | `0.3` | Weight of BM25 in fusion (0-1) |
| `VECTOR_WEIGHT` | `0.7` | Weight of vector in fusion (0-1) |
| `BM25_TOP_K` | `100` | Candidates from BM25 search |

### 1.5 Expected Impact

| Scenario | Before | After | Improvement |
|----------|--------|-------|-------------|
| Path queries ("lib/graphql") | ~0.44 | ~0.65 | +48% |
| Identifier queries ("DeltaSyncModule") | ~0.47 | ~0.70 | +49% |
| Conceptual queries | ~0.58 | ~0.58 | 0% (no regression) |
| **Overall average** | ~0.56 | ~0.62 | **+11%** |

---

## Part 2: LLM Re-ranking

### 2.1 Problem Statement

Even with hybrid retrieval, the top-10 results may be:

- Topically related but not answering the question
- Missing the most relevant result buried at position 15-20
- Scoring highly due to keyword match but semantically irrelevant

### 2.2 Solution: LLM Re-ranking

Use a lightweight LLM to re-score the top-N candidates based on:

- How well the node's content answers the query
- Semantic relevance beyond keyword/embedding match

### 2.3 Implementation Plan

#### Step 1: Re-ranking Prompt Template

```go
// internal/retrieval/rerank.go

const RerankPromptTemplate = `You are a relevance judge for a code knowledge base.

Query: {{.Query}}

Rate how relevant each candidate is to answering this query.
Score from 0.0 (irrelevant) to 1.0 (perfectly relevant).

Candidates:
{{range $i, $c := .Candidates}}
[{{$i}}] {{$c.Name}}
Path: {{$c.Path}}
Summary: {{$c.Summary}}
{{end}}

Return JSON array of scores in order: [0.85, 0.32, ...]
Only return the JSON array, nothing else.`
```

#### Step 2: Re-ranking Service

```go
type RerankRequest struct {
    Query      string
    Candidates []Candidate
    TopN       int  // How many to re-rank (default: 30)
    ReturnK    int  // How many to return (default: 10)
}

type RerankResult struct {
    NodeID       string
    OriginalRank int
    RerankScore  float64
    FinalRank    int
}

func (s *Service) Rerank(ctx context.Context, req RerankRequest) ([]RerankResult, error) {
    // 1. Build prompt from template
    // 2. Call LLM (OpenAI GPT-4o-mini or local)
    // 3. Parse scores
    // 4. Combine with original scores: final = α*original + β*rerank
    // 5. Sort and return top-K
}
```

#### Step 3: Integration with Retrieval Pipeline

```go
func (s *Service) Retrieve(ctx context.Context, req models.RetrieveRequest) (models.RetrieveResponse, error) {
    // ... existing vector recall, hybrid fusion, activation ...

    // Scoring (existing)
    results := ScoreAndRank(cands, act, edges, topK*3, s.cfg, req.QueryText)

    // NEW: LLM Re-ranking (if enabled and query_text provided)
    if s.cfg.RerankEnabled && req.QueryText != "" {
        results = s.Rerank(ctx, RerankRequest{
            Query:      req.QueryText,
            Candidates: results,
            TopN:       30,
            ReturnK:    topK,
        })
    }

    return models.RetrieveResponse{Results: results}, nil
}
```

### 2.4 LLM Provider Options

| Provider | Model | Latency | Cost | Quality |
|----------|-------|---------|------|---------|
| OpenAI | gpt-4o-mini | ~500ms | $0.15/1M tokens | High |
| OpenAI | gpt-3.5-turbo | ~300ms | $0.50/1M tokens | Medium |
| Local | Ollama (llama3) | ~800ms | Free | Medium |
| Local | Ollama (mistral) | ~600ms | Free | Medium |

**Recommendation**: Start with gpt-4o-mini for quality, with fallback to local.

### 2.5 Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `RERANK_ENABLED` | `true` | Enable LLM re-ranking |
| `RERANK_PROVIDER` | `openai` | LLM provider (openai/ollama) |
| `RERANK_MODEL` | `gpt-4o-mini` | Model for re-ranking |
| `RERANK_TOP_N` | `30` | Candidates to re-rank |
| `RERANK_WEIGHT` | `0.4` | Weight of rerank score in final |
| `RERANK_TIMEOUT_MS` | `3000` | Timeout for rerank call |

### 2.6 Expected Impact

| Scenario | Before | After | Improvement |
|----------|--------|-------|-------------|
| Complex questions | ~0.52 | ~0.68 | +31% |
| "Why" questions | ~0.48 | ~0.62 | +29% |
| Relationship questions | ~0.58 | ~0.70 | +21% |
| **Overall average** | ~0.62 (with hybrid) | ~0.69 | **+11%** |

---

## Part 3: Combined Pipeline

### 3.1 Full Retrieval Flow

```
1. Query Text arrives
     │
2. [Optional] Query Expansion
     │  - Extract entities, synonyms
     │  - Expand "graphql" → "graphql OR apollo OR gql"
     │
3. Parallel Search
     ├─ Vector Search (embedding similarity)
     └─ BM25 Search (keyword matching)
     │
4. Reciprocal Rank Fusion
     │  - Combine vector + BM25 rankings
     │  - Produce unified candidate list
     │
5. Graph Expansion
     │  - Fetch neighboring nodes
     │  - Build activation subgraph
     │
6. Spreading Activation
     │  - Compute activation scores
     │
7. Initial Scoring
     │  - α*vector + β*activation + γ*recency + boosts - penalties
     │  - Produce top-30 candidates
     │
8. LLM Re-ranking
     │  - Re-score top-30 with LLM judgment
     │  - Final = δ*initial + ε*rerank
     │
9. Return top-K results
```

### 3.2 Latency Budget

| Stage | Target Latency | Notes |
|-------|----------------|-------|
| Vector Search | 50ms | Neo4j vector index |
| BM25 Search | 30ms | Neo4j fulltext index |
| RRF Fusion | 5ms | In-memory computation |
| Graph Expansion | 100ms | 2-hop fetch |
| Activation | 20ms | In-memory |
| Initial Scoring | 10ms | In-memory |
| LLM Re-ranking | 500-800ms | External API call |
| **Total** | **~750ms** | Acceptable for retrieval |

### 3.3 Graceful Degradation

```go
// If rerank fails or times out, return initial results
if s.cfg.RerankEnabled {
    ctx, cancel := context.WithTimeout(ctx, s.cfg.RerankTimeout)
    defer cancel()

    reranked, err := s.Rerank(ctx, req)
    if err != nil {
        log.Printf("WARN: rerank failed, using initial results: %v", err)
        return initialResults, nil
    }
    return reranked, nil
}
```

---

## Implementation Roadmap

### Phase 1: Hybrid Retrieval (Week 1)

- [ ] Create V0006 migration for fulltext index
- [ ] Implement BM25Search function
- [ ] Implement ReciprocalRankFusion
- [ ] Add configuration variables
- [ ] Update Retrieve to use hybrid search
- [ ] Benchmark: target +5-8% improvement

### Phase 2: LLM Re-ranking (Week 2)

- [ ] Implement rerank prompt template
- [ ] Implement Rerank service with OpenAI
- [ ] Add Ollama fallback
- [ ] Integrate into retrieval pipeline
- [ ] Add configuration variables
- [ ] Benchmark: target +5-8% additional improvement

### Phase 3: Optimization (Week 3)

- [ ] Query expansion (optional)
- [ ] Caching for repeated queries
- [ ] Batch re-ranking optimization
- [ ] Performance tuning
- [ ] Final benchmark: target 0.69+ average

---

## Success Metrics

| Metric | Current (v8) | Target | Method |
|--------|--------------|--------|--------|
| Average Score | 0.562 | 0.69 | 100-question benchmark |
| Min Score | 0.394 | 0.50 | Worst-case improvement |
| architecture_structure | 0.553 | 0.65 | Category-specific |
| p95 Latency | ~200ms | <1000ms | With re-ranking |

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| LLM latency spikes | Slow retrieval | Timeout + fallback to initial results |
| LLM API costs | Budget overrun | Configurable, can disable in production |
| BM25 index size | Storage growth | ~10% increase, acceptable |
| Score distribution shift | Benchmark regression | A/B testing, gradual rollout |
