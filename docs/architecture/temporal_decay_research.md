# Temporal Decay Research for MDEMG

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Research Date**: 2026-01-30
**Author**: Claude (Sonnet 4.5)
**Purpose**: Comprehensive research on temporal decay mechanisms for integration into MDEMG retrieval system

---

## Executive Summary

This document synthesizes research on temporal decay mechanisms from cognitive psychology, information retrieval, and AI memory systems. It provides theoretical foundations, mathematical formulations, implementation patterns, and specific recommendations for integrating temporal decay into MDEMG's retrieval scoring system.

**Key Finding**: MDEMG currently implements exponential decay for edge pruning but does not apply temporal decay to retrieval scoring beyond basic recency. Power law decay (ACT-R style) is recommended for human-like memory patterns, while exponential decay is better for simple recency bias in search-like scenarios.

---

## 1. Theoretical Foundations

### 1.1 Ebbinghaus Forgetting Curve

The [Ebbinghaus forgetting curve](https://en.wikipedia.org/wiki/Forgetting_curve) is one of the earliest mathematical models of memory decay, showing that about 50% of newly learned information is lost within an hour, and up to 70% within 24 hours without reinforcement.

**Mathematical Formulations**:

1. **Exponential Decay** (most common):

   ```
   R = exp(-t/S)
   ```

   Where:
   - R = retrievability (memory retention, 0.0-1.0)
   - t = time elapsed since learning
   - S = relative memory strength

2. **Ebbinghaus' Original Formula**:

   ```
   b = 100k / (c * log(t) + k)
   ```

   Where:
   - b = savings (retained knowledge)
   - t = time in minutes
   - c = 1.25, k = 1.84 (empirical constants)

3. **Power Law Formula** (Wixted & Carpenter 2007):

   ```
   P(recall) = m(1 + ht)^(-f)
   ```

   Where:
   - m = degree of initial learning (probability at t=0)
   - h = scaling factor on time
   - f = exponential memory decay factor

**Key Insights**:

- Forgetting follows exponential decay initially, but when memories of different stability are mixed, the aggregate curve approximates a power law
- Replication studies ([Murre & Dros, 2015](https://pmc.ncbi.nlm.nih.gov/articles/PMC4492928/)) confirm the original findings remain valid
- The decay rate is not constant—it depends on initial encoding strength and reinforcement

**Sources**:

- [Replication and Analysis of Ebbinghaus' Forgetting Curve - PMC](https://pmc.ncbi.nlm.nih.gov/articles/PMC4492928/)
- [Forgetting curve - Wikipedia](https://en.wikipedia.org/wiki/Forgetting_curve)
- [Ebbinghaus's Forgetting Curve: How to Overcome It](https://whatfix.com/blog/ebbinghaus-forgetting-curve/)

### 1.2 ACT-R Cognitive Architecture

[ACT-R (Adaptive Control of Thought—Rational)](https://en.wikipedia.org/wiki/ACT-R) is a cognitive architecture that models human memory using power law decay.

**Base-Level Activation Formula**:

```
Bi = ln(Σⁿⱼ₌₁ tⱼ^(-d))
```

Where:

- Bi = base-level activation of memory chunk i
- n = number of prior occurrences (frequency effect)
- tⱼ = time since jth occurrence
- d = decay parameter (typically 0.5 in ACT-R)

**Why Power Law vs Exponential?**

- Power law better matches empirical human memory data
- Captures both frequency (how often) and recency (how recently) effects
- Every retrieval creates a new memory trace that decays independently
- The sum of multiple exponential decays approximates a power law

**Practical Implications for MDEMG**:

- Memories accessed frequently should decay slower
- Recent activations should have stronger influence
- Multiple retrievals should reinforce memory strength

**Sources**:

- [ACT-R - Wikipedia](https://en.wikipedia.org/wiki/ACT-R)
- [An Integrated Computational Framework for the Neurobiology of Memory Based on the ACT-R Declarative Memory System](https://link.springer.com/article/10.1007/s42113-023-00189-y)
- [Hybrid Personalization Using Declarative and Procedural Memory Modules of the Cognitive Architecture ACT-R](https://arxiv.org/html/2505.05083v1)

### 1.3 Biological Memory Systems

**Hippocampus vs Neocortex**:

- **Hippocampus**: Rapid encoding, fast decay (hours to days)
- **Neocortex**: Slow consolidation, slower decay (months to years)
- **Systems Consolidation**: Memories migrate from hippocampus to neocortex through repeated reactivation

**Parallels to MDEMG**:

- Layer 0 (observations) = hippocampus-like fast encoding
- Layer 1+ (concepts) = neocortex-like consolidated knowledge
- Consolidation process strengthens important patterns while pruning transient details

---

## 2. Implementation Patterns in Production Systems

### 2.1 Recommendation Systems

Modern recommendation systems extensively use temporal decay to prioritize recent user behavior.

**Common Formulas**:

1. **Exponential Decay**:

   ```
   W = e^(-λt)
   ```

   Where:
   - W = weight
   - λ = decay rate
   - t = time since interaction

2. **Half-Life Decay**:

   ```
   W = 0.5^(t / h)
   ```

   Where:
   - h = half-life period
   - t = time elapsed

**Implementation Strategies**:

- Apply decay during training (SGD step) rather than pre-processing
- Session-based: treat 30-minute windows as atomic units
- Recent co-occurrences get higher weight than older ones
- Exponential decay with λ values typically 0.001 to 0.1 per day

**Key Findings**:

- [Recency weighting](https://customers.ai/recency-weighted-scoring) significantly improves click-through rates
- [Session-based recommenders](https://www.mdpi.com/2079-9292/15/1/84) use GRUs or attention mechanisms for short-term preferences
- Time decay is "quintessential" for POI (point-of-interest) recommendations to avoid stale results

**Sources**:

- [Application of collaborative filtering algorithm based on time decay function in music teaching recommendation model - PMC](https://pmc.ncbi.nlm.nih.gov/articles/PMC11639207/)
- [Recency-based spatio-temporal similarity exploration for POI recommendation in location-based social networks](https://www.frontiersin.org/journals/sustainable-cities/articles/10.3389/frsc.2024.1331642/full)
- [A Half-Life Decaying Model for Recommender Systems with Matrix Factorization](https://ceur-ws.org/Vol-2038/paper1.pdf)
- [Exponential Decay Function-Based Time-Aware Recommender System](https://thesai.org/Downloads/Volume13No10/Paper_71-Exponential_Decay_Function_Based_Time_Aware_Recommender_System.pdf)

### 2.2 Search Engines & Freshness Signals

Search engines (Google, Bing, AI search) use sophisticated temporal signals to balance freshness and relevance.

**Freshness Signal Strategies**:

- **Query-Dependent**: Time-sensitive queries (news, events) heavily weight recency; evergreen topics prioritize quality
- **Content Decay Patterns**: Citation rates drop 40-60% after 90 days without updates
- **Temporal Metadata**: "Last-modified" dates, publication dates, update frequency
- **Half-Life Decay**: Blend semantic similarity with temporal decay in re-ranking

**Implementation Details**:

- AI algorithms dynamically adjust recency weighting based on query intent
- Fused semantic-temporal scores: `score = α * cosine_sim + β * time_decay`
- Content refresh cycles: weeks 1-4 (peak), weeks 5-12 (decline), weeks 13-26 (decay)

**Sources**:

- [How Content Freshness Helps Ranking in ChatGPT - Recency Bias for LLMs](https://www.mattakumar.com/blog/how-to-rank-in-chatgpt-using-recency-bias/)
- [5 AI Search Ranking Signals That Make or Break Your Content Visibility in 2026](https://espy-go.com/resources/ai-search-algorithm-factors/)
- [Solving Freshness in RAG: A Simple Recency Prior and the Limits of Heuristic Trend Detection](https://arxiv.org/html/2509.19376)

### 2.3 Memory-Augmented Neural Networks

#### MemGPT

[MemGPT](https://research.memgpt.ai/) implements hierarchical memory with "cognitive triage" for strategic forgetting.

**Key Mechanisms**:

- **Memory Tiers**: Main memory (fixed context), archival storage (unlimited)
- **Strategic Forgetting**: Summarization and targeted deletion
- **Cognitive Triage**: LLM evaluates future value of information fragments
  - High priority: User preferences, core facts, critical details
  - Low priority: Transient conversation, repetitive information
- **Temporal Reasoning**: Models ordering and dating of events over long horizons

**Lessons for MDEMG**:

- Importance scoring should guide retention vs decay
- Metadata about access patterns informs triage decisions
- Both under-retrieval (missing relevant) and over-retrieval (noise) harm performance

**Sources**:

- [MemGPT: Towards LLMs as Operating Systems](https://arxiv.org/pdf/2310.08560)
- [MemGPT: Engineering Semantic Memory through Adaptive Retention and Context Summarization](https://informationmatters.org/2025/10/memgpt-engineering-semantic-memory-through-adaptive-retention-and-context-summarization/)

#### Other Memory Systems (2025-2026)

- **EverMemOS**: Self-organizing memory OS for long-horizon reasoning
- **Zep**: Temporal knowledge graph architecture for agent memory
- **Mem0**: Production-ready agents with scalable long-term memory

---

## 3. Mathematical Formulations: Deep Dive

### 3.1 Power Law vs Exponential Decay: When to Use Each

**Exponential Decay**:

```
score *= exp(-λ * t)
```

- **Characteristics**: Constant decay rate, memoryless process
- **Use When**:
  - Systems in thermodynamic equilibrium (boring, well-behaved)
  - Simple recency bias needed (e.g., search freshness)
  - Constant importance decay is desired
  - Computational simplicity is critical
- **Example**: News articles (yesterday's news is old news)

**Power Law Decay**:

```
score *= 1 / (1 + t)^α
```

- **Characteristics**: No characteristic scale, "heavy tail" retention
- **Use When**:
  - Modeling human-like memory
  - Frequency and recency both matter
  - Some old memories should persist strongly
  - Complex, interesting system dynamics
- **Example**: Academic papers (old seminal papers remain relevant)

**Key Distinction**:
[Power law vs exponential decay research](https://memory.psych.upenn.edu/files/pubs/KahaAdle02.pdf) shows exponential decay can be decisively identified from power law only if flux decays several orders of magnitude. In practice, short time scales often appear as power law, long time scales as exponential.

**Sources**:

- [Note on the power law of forgetting - Michael J. Kahana](https://memory.psych.upenn.edu/files/pubs/KahaAdle02.pdf)
- [Power Law versus Exponential State Transition Dynamics - PMC](https://pmc.ncbi.nlm.nih.gov/articles/PMC2996311/)
- [Power law - Wikipedia](https://en.wikipedia.org/wiki/Power_law)

### 3.2 Hybrid Approaches

**Weighted Combination**:

```
score = α * vector_sim + β * activation + γ * recency + δ * importance
recency = exp(-ρ * days)  or  1 / (1 + days)^σ
```

**Importance Weighting**:

```
decay_rate = base_rate * (1 - importance_score)
score *= exp(-decay_rate * days)
```

- High importance → slower decay
- Low importance → faster decay

**Access-Aware Decay**:

```
effective_age = days_since_creation - β * days_since_last_access
score *= exp(-λ * effective_age)
```

- Recent access "resets" the age partially

### 3.3 Adaptive Decay Rates

**Context-Dependent**:

```
if is_evergreen_content(node):
    decay_rate = 0.01  # Slow decay
elif is_time_sensitive(node):
    decay_rate = 0.5   # Fast decay
else:
    decay_rate = 0.1   # Default
```

**Access Pattern Learning**:

```
decay_rate = base_rate / (1 + log(1 + access_count))
```

- Frequently accessed nodes decay slower

---

## 4. Temporal Position Encoding (Transformer-Inspired)

### 4.1 Sinusoidal Encoding

From the ["Attention Is All You Need"](https://kazemnejad.com/blog/transformer_architecture_positional_encoding/) paper:

```
PE(t, 2i)   = sin(t / 10000^(2i/d))
PE(t, 2i+1) = cos(t / 10000^(2i/d))
```

Where:

- t = time position (e.g., days since creation)
- i = dimension index
- d = embedding dimensions

**Properties**:

- No learned parameters (reduces overfitting)
- Generalizes to unseen time ranges
- Encodes relative temporal distances
- Symmetric decay between neighboring time-steps

**For MDEMG**:
Could augment node embeddings with temporal encoding to make vector similarity time-aware.

**Sources**:

- [Transformer Architecture: The Positional Encoding](https://kazemnejad.com/blog/transformer_architecture_positional_encoding/)
- [Understanding Sinusoidal Positional Encoding in Transformers](https://medium.com/@pranay.janupalli/understanding-sinusoidal-positional-encoding-in-transformers-26c4c161b7cc)
- [Positional Encoding in Transformer-Based Time Series Models: A Survey](https://arxiv.org/html/2502.12370v1)

### 4.2 Learned Temporal Embeddings

**Approach**: Train a small neural network to map `(days_since_creation, days_since_access, access_count)` → temporal_factor

**Advantages**:

- Adaptive to data patterns
- Can learn complex decay curves
- Flexibility for different content types

**Disadvantages**:

- Requires training data
- Risk of overfitting
- Less interpretable

Most pretrained models ([BERT](https://www.ibm.com/think/topics/positional-encoding)) use learnable embeddings for flexibility, though sinusoidal encoding has unique extrapolability.

---

## 5. Practical Considerations for Implementation

### 5.1 When to Apply Decay

**Option 1: Read-Time (Query Time)**

- Calculate decay factor during retrieval
- Advantages: Always current, no batch jobs
- Disadvantages: Adds latency per query

**Option 2: Write-Time (Update Time)**

- Periodically update scores in database
- Advantages: Fast queries, pre-computed
- Disadvantages: Stale between updates, batch job overhead

**Option 3: Hybrid**

- Store `created_at`, `last_accessed_at` timestamps
- Compute decay on-the-fly during scoring
- Update access timestamps on retrieval

**Recommendation for MDEMG**: Hybrid approach (Option 3)

- Already have timestamp infrastructure (`updated_at`, `created_at`)
- Scoring happens in-memory after vector recall (minimal overhead)
- Decay job can prune edges, retrieval can apply decay to scores

### 5.2 Handling "Evergreen" Memories

Not all memories should decay equally. Some knowledge is timeless.

**Strategies**:

1. **Tag-Based Exemption**:

   ```go
   if hasTag(node.Tags, "evergreen") || hasTag(node.Tags, "definition") {
       decay_factor = 1.0  // No decay
   }
   ```

2. **Layer-Based Decay Rates**:

   ```go
   switch node.Layer {
   case 0:  // Observations
       decay_rate = 0.15  // Fast decay
   case 1:  // Hidden/concepts
       decay_rate = 0.05  // Medium decay
   case 2+: // High-level concepts
       decay_rate = 0.01  // Slow decay
   }
   ```

3. **Importance-Adjusted Decay**:

   ```go
   if node.Confidence > 0.9 {
       decay_rate *= 0.5  // Halve decay for high-confidence nodes
   }
   ```

### 5.3 Decay Granularity

**Time Units**:

- Days: Good for most content (MDEMG's current unit)
- Hours: For real-time/conversation memory
- Interactions: For session-based decay (N queries since last access)

**Update Frequency**:

- Continuous: On every retrieval (access updates timestamp)
- Daily: Batch job runs once per day
- On-demand: Triggered by API endpoint

### 5.4 Computational Cost

**Complexity Analysis**:

- Exponential: `exp(-λt)` → ~10-20 CPU cycles
- Power law: `1/(1+t)^α` → ~5-10 CPU cycles (if α is integer)
- Logarithmic: `log(1+t)` → ~20-30 CPU cycles

For 1000 candidates scored per query:

- Exponential decay adds ~0.01-0.02ms per query
- Negligible compared to vector search (10-100ms) and LLM reranking (500-2000ms)

**Recommendation**: Computational cost is not a blocker for any approach.

---

## 6. MDEMG-Specific Analysis

### 6.1 Current Implementation Review

**Existing Decay Mechanisms**:

1. **Edge Decay** (`cmd/decay/main.go`):

   ```go
   // Line 44-60: calculateDecay function
   decayFactor := math.Exp(-decayRate * daysSince)
   newWeight = e.Weight * decayFactor
   ```

   - Applies to `CO_ACTIVATED_WITH` edge weights
   - Formula: `w_new = w_old * exp(-0.1 * days)`
   - Used for pruning weak edges, not retrieval scoring

2. **Recency in Scoring** (`internal/retrieval/scoring.go`):

   ```go
   // Line 348-355: recency factor
   ageDays := now.Sub(c.UpdatedAt).Hours() / 24.0
   r := math.Exp(-rho * ageDays)
   // ...
   s := vecComponent + actComponent + recComponent + ...
   ```

   - Uses `cfg.ScoringRho` (default unknown, likely ~0.01-0.05)
   - Applies to final score via `gamma * r` term
   - Only considers `updated_at`, not `last_accessed_at`

**Timestamp Fields Available**:

- `created_at`: When node was first ingested
- `updated_at`: When node was last modified
- `last_accessed_at`: (Used in edge decay, may not be on nodes)

**Gap Analysis**:

- ✅ Has basic exponential recency decay
- ❌ No access-aware decay (doesn't track retrieval events)
- ❌ No layer-specific decay rates
- ❌ No importance-weighted decay
- ❌ No adaptive decay based on access patterns
- ❌ Decay applied uniformly to all node types

### 6.2 Proposed Integration Points

**Priority 1: Extend Scoring Formula**

**Current** (`scoring.go:384`):

```go
s := vecComponent + actComponent + recComponent + confComponent +
     pb + cb - hubPenComponent - redPenComponent
```

**Proposed Enhancement**:

```go
// Add layer-specific and importance-weighted decay
recencyDecay := calculateRecencyDecay(c, now, cfg)
importanceBoost := calculateImportanceBoost(c)
temporalScore := recComponent * recencyDecay * importanceBoost

s := vecComponent + actComponent + temporalScore + confComponent +
     pb + cb - hubPenComponent - redPenComponent
```

**Priority 2: Implement `calculateRecencyDecay`**

```go
func calculateRecencyDecay(c Candidate, now time.Time, cfg config.Config) float64 {
    // Age since creation
    ageDays := now.Sub(c.UpdatedAt).Hours() / 24.0

    // Layer-specific decay rates
    var decayRate float64
    switch c.Layer {
    case 0:
        decayRate = cfg.Layer0DecayRate  // e.g., 0.15 (fast)
    case 1:
        decayRate = cfg.Layer1DecayRate  // e.g., 0.05 (medium)
    default:
        decayRate = cfg.Layer2DecayRate  // e.g., 0.01 (slow)
    }

    // Importance adjustment (high confidence = slower decay)
    if c.Confidence > 0.8 {
        decayRate *= 0.5
    }

    // Exponential decay (simple, fast)
    return math.Exp(-decayRate * ageDays)

    // OR Power law decay (human-like memory)
    // alpha := cfg.MemoryDecayAlpha  // e.g., 0.5
    // return 1.0 / math.Pow(1.0 + ageDays, alpha)
}
```

**Priority 3: Add Access Tracking**

Update `updated_at` timestamp on retrieval:

```go
// In Retrieve service after scoring
for _, result := range topKResults {
    go s.updateAccessTimestamp(ctx, result.NodeID)
}
```

Modify decay to consider last access:

```go
// Use last_accessed_at if available, else updated_at
effectiveAge := daysSince(c.LastAccessedAt, now)
if effectiveAge > daysSince(c.UpdatedAt, now) {
    effectiveAge = daysSince(c.UpdatedAt, now)  // Use more recent timestamp
}
```

### 6.3 Configuration Parameters

Add to `internal/config/config.go`:

```go
type Config struct {
    // ... existing fields ...

    // Temporal Decay Parameters
    Layer0DecayRate     float64 `env:"LAYER0_DECAY_RATE" envDefault:"0.15"`      // Observations
    Layer1DecayRate     float64 `env:"LAYER1_DECAY_RATE" envDefault:"0.05"`      // Hidden/concepts
    Layer2DecayRate     float64 `env:"LAYER2_DECAY_RATE" envDefault:"0.01"`      // High-level
    MemoryDecayMode     string  `env:"MEMORY_DECAY_MODE" envDefault:"exponential"` // exponential, power_law, hybrid
    MemoryDecayAlpha    float64 `env:"MEMORY_DECAY_ALPHA" envDefault:"0.5"`      // Power law exponent
    ImportanceDecayMult float64 `env:"IMPORTANCE_DECAY_MULT" envDefault:"0.5"`   // High importance decay multiplier
    TrackAccessTime     bool    `env:"TRACK_ACCESS_TIME" envDefault:"true"`      // Update timestamps on retrieval
}
```

### 6.4 Impact on Benchmark Scores

**Hypothesis**: Temporal decay should improve benchmark performance on:

1. **Time-sensitive queries**: "Recent changes to...", "Latest implementation of..."
2. **Evolutionary codebases**: Prefer newer patterns over deprecated ones
3. **Bug fix tracking**: Recent fixes more relevant than old issues

**Potential Risks**:

1. **Over-decay**: Important foundational knowledge gets buried
2. **Benchmark bias**: If test questions don't specify recency, may hurt scores
3. **Cold start**: New ingestions have unfair advantage over older (but still relevant) nodes

**Mitigation Strategies**:

- Layer-specific decay (slower for concepts)
- Importance weighting (confidence score protects valuable nodes)
- Configurable decay rates (tune per use case)
- Hybrid mode (blend temporal and semantic scores)

---

## 7. Recommendations & Next Steps

### 7.1 Recommended Approach for MDEMG

**Phase 1: Extend Existing Recency Decay** (1-2 hours implementation)

1. Add layer-specific decay rates (fast for L0, slow for L2+)
2. Add importance-weighted decay (confidence > 0.8 gets 50% decay reduction)
3. Make decay mode configurable (exponential vs power law)
4. Add config parameters for tuning

**Phase 2: Access-Aware Decay** (2-3 hours)

1. Add `last_accessed_at` timestamp to nodes (schema update)
2. Update timestamp on retrieval (background goroutine)
3. Use last access time in decay calculation
4. Add access count field for frequency-based adjustments

**Phase 3: Adaptive Decay** (4-6 hours)

1. Implement tag-based decay exemptions (evergreen content)
2. Learn optimal decay rates per content type
3. Dynamic decay based on query patterns
4. A/B test different decay strategies

### 7.2 Tuning Guidelines

**Decay Rate Selection**:

- **0.01**: Very slow decay (99% retention after 1 week)
- **0.05**: Slow decay (95% retention after 1 week)
- **0.10**: Medium decay (90% retention after 1 week) — **MDEMG current default**
- **0.20**: Fast decay (82% retention after 1 week)
- **0.50**: Very fast decay (60% retention after 1 week)

**Power Law Alpha Selection**:

- **0.3**: Gentle decay, long tail
- **0.5**: ACT-R default, balanced
- **0.7**: Steeper decay, shorter tail
- **1.0**: Linear reciprocal (1/days)

**Testing Protocol**:

1. Baseline: Run benchmarks with current scoring
2. Experiment: Enable temporal decay with default rates
3. Tune: Adjust layer-specific rates based on benchmark results
4. Validate: Re-run benchmarks, measure delta
5. Monitor: Track query latency and memory usage

### 7.3 Success Metrics

**Retrieval Quality**:

- Benchmark score delta (target: +3-5% for time-sensitive queries)
- Evidence compliance rate (should maintain or improve)
- User feedback on relevance (qualitative)

**System Health**:

- Query latency delta (target: <5ms increase)
- Memory usage (timestamp storage minimal)
- Cache hit rate (may decrease initially due to timestamp updates)

**Decay Effectiveness**:

- Pruning rate of old edges (from decay job)
- Age distribution of top-K results (shift toward newer)
- Reactivation rate of "forgotten" nodes (should be low)

---

## 8. Conclusion

Temporal decay is a well-established technique in cognitive science, information retrieval, and AI memory systems. MDEMG already has a foundation for decay (edge pruning, recency scoring), but can significantly enhance retrieval quality by:

1. **Layer-specific decay rates** to match content importance
2. **Access-aware decay** to reinforce frequently retrieved knowledge
3. **Importance weighting** to protect high-confidence nodes
4. **Configurable decay modes** (exponential vs power law) for different use cases

The implementation complexity is low (1-6 hours across phases), computational overhead is negligible (<0.02ms per query), and the potential impact on retrieval quality is significant, particularly for time-sensitive queries and evolving codebases.

**Recommended First Step**: Implement Phase 1 (layer-specific + importance-weighted decay) and run whk-wms and pytorch benchmarks to measure impact.

---

## 9. References

### Cognitive Science & Memory

- [Replication and Analysis of Ebbinghaus' Forgetting Curve - PMC](https://pmc.ncbi.nlm.nih.gov/articles/PMC4492928/)
- [Forgetting curve - Wikipedia](https://en.wikipedia.org/wiki/Forgetting_curve)
- [ACT-R - Wikipedia](https://en.wikipedia.org/wiki/ACT-R)
- [An Integrated Computational Framework for the Neurobiology of Memory Based on the ACT-R Declarative Memory System](https://link.springer.com/article/10.1007/s42113-023-00189-y)
- [Note on the power law of forgetting - Michael J. Kahana](https://memory.psych.upenn.edu/files/pubs/KahaAdle02.pdf)

### Recommendation Systems

- [Application of collaborative filtering algorithm based on time decay function - PMC](https://pmc.ncbi.nlm.nih.gov/articles/PMC11639207/)
- [Recency-based spatio-temporal similarity exploration for POI recommendation](https://www.frontiersin.org/journals/sustainable-cities/articles/10.3389/frsc.2024.1331642/full)
- [A Half-Life Decaying Model for Recommender Systems with Matrix Factorization](https://ceur-ws.org/Vol-2038/paper1.pdf)
- [Exponential Decay Function-Based Time-Aware Recommender System](https://thesai.org/Downloads/Volume13No10/Paper_71-Exponential_Decay_Function_Based_Time_Aware_Recommender_System.pdf)

### Search Engines & AI Search

- [How Content Freshness Helps Ranking in ChatGPT](https://www.mattakumar.com/blog/how-to-rank-in-chatgpt-using-recency-bias/)
- [5 AI Search Ranking Signals That Make or Break Your Content Visibility in 2026](https://espy-go.com/resources/ai-search-algorithm-factors/)
- [Solving Freshness in RAG: A Simple Recency Prior](https://arxiv.org/html/2509.19376)

### Memory-Augmented Systems

- [MemGPT: Towards LLMs as Operating Systems](https://arxiv.org/pdf/2310.08560)
- [MemGPT: Engineering Semantic Memory through Adaptive Retention](https://informationmatters.org/2025/10/memgpt-engineering-semantic-memory-through-adaptive-retention-and-context-summarization/)

### Positional Encoding

- [Transformer Architecture: The Positional Encoding](https://kazemnejad.com/blog/transformer_architecture_positional_encoding/)
- [Understanding Sinusoidal Positional Encoding in Transformers](https://medium.com/@pranay.janupalli/understanding-sinusoidal-positional-encoding-in-transformers-26c4c161b7cc)
- [Positional Encoding in Transformer-Based Time Series Models: A Survey](https://arxiv.org/html/2502.12370v1)
- [What is Positional Encoding? | IBM](https://www.ibm.com/think/topics/positional-encoding)

---

**End of Document**
