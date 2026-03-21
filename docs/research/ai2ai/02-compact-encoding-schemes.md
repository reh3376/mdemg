# Compact Encoding Schemes for AI-to-AI Communication

**Research Date**: 2026-03-21
**Purpose**: J17 protocol design — token-efficient representations and semantic compression
**Researcher**: Claude (Opus 4.6) via web search

---

## 1. Token-Efficient Representations & Prompt Compression

### 1a. Hard Compression (Token Removal)

**LLMLingua / LLMLingua-2** (Microsoft Research, EMNLP 2023 / ACL 2024)

Uses a small language model (GPT-2 or LLaMA-7B) to identify and remove unimportant tokens. Achieves **up to 20x compression** with minimal performance loss. Coarse-to-fine: budget controller maintains semantic integrity, then token-level iterative compression.

LLMLingua-2 uses a BERT-level encoder trained via data distillation from GPT-4. **3x-6x faster**, 2x-5x compression ratios.

- https://www.llmlingua.com/
- Paper: https://arxiv.org/abs/2310.05736
- Code: https://github.com/microsoft/LLMLingua

**J17 relevance**: Drop-in solution — compress guidance text before injection. 2-5x token reduction with no protocol changes needed. Could be a quick win before building a full protocol.

### 1b. Soft Prompt / Gist Token Compression

**Gist Tokens** (Mu et al., NeurIPS 2023): Train LLMs to compress prompts into virtual token activations. **Up to 26x compression**, 40% FLOPs reduction. Training adds zero cost over standard instruction finetuning.

- Paper: https://arxiv.org/abs/2304.08467
- Code: https://github.com/jayelm/gisting

**In-Context Autoencoder (ICAE)** (Ge et al., ICLR 2024): Compresses long context into 128 compact "memory slots" using LoRA-adapted LLM encoder. **4x context compression**, ~20GB GPU memory savings.

- Paper: https://arxiv.org/abs/2307.06945
- Code: https://github.com/getao/icae

**J17 relevance**: These operate "below the token layer" at representation level. Require both parties to share model architecture — not applicable when Jiminy (Go service) sends to Claude (external API). But they establish that **75-85% of NL tokens are overhead**.

### 1c. Agent-Specific Context Compression

**ACON** (Kang et al., 2025): Dynamic compression of environment observations and interaction histories. **26-54% peak token reduction**, 95%+ accuracy preserved. Uses guideline optimization pipeline refined via failure analysis.

- Paper: https://arxiv.org/abs/2510.00615

**AgentPrune** (2024): One-shot pruning of spatial-temporal message-passing graph in multi-agent systems. **28.1%-72.8% token reduction**, costs reduced from $43.7 to $5.6.

- Paper: https://arxiv.org/abs/2410.02506
- Code: https://github.com/yanweiyue/AgentPrune

**J17 relevance**: ACON's approach of compressing observation history with optimizable NL guidelines directly applicable to Jiminy guidance compression.

---

## 2. Structured Binary/Schema-Based Protocols

### Binary Serialization Formats

No published research adapting Protobuf, MessagePack, or CBOR for LLM-to-LLM dialogue. Key formats:

| Format | Characteristics | vs JSON |
|--------|----------------|---------|
| MessagePack | Schema-free, self-describing | 30-50% smaller |
| CBOR | Designed for constrained devices | Similar to MessagePack |
| Protobuf | Requires predefined schemas | Smallest payloads, rigid |

**J17 relevance**: Since the receiver is an LLM that processes text tokens (not binary), binary serialization provides **no direct benefit**. The LLM would need to decode binary back to text. Compression must happen at the **semantic** level, not the wire level.

### LACP Structured Messages

LACP's three universal message types (`PLAN`, `ACT`, `OBSERVE`) with JWS envelopes provide a structured-but-text-readable format. ~3% latency overhead, ~30% size overhead for large messages.

- Paper: https://arxiv.org/abs/2510.13821
- Code: https://github.com/LiXin97/LACP

---

## 3. Semantic Compression Research

### 3a. "Why Do AI Agents Communicate in Human Language?" (June 2025)

**The strongest theoretical case for J17.** Key arguments from arXiv:2506.02739:

1. **Semantic misalignment**: LLMs operate in high-dimensional vector spaces, but natural language is discrete and low-dimensional. Projecting from internal states to tokens is lossy and non-invertible.
2. **Protocol-induced misbehavior**: Natural language allows agents to *describe* completion without *executing* it ("pseudo-execution").
3. **Communication overhead**: Redundant confirmations, explanations, speculative dialogue cause reasoning loops (observed in AutoGPT).

Authors propose **structured tensor communication** — replacing discrete tokens with direct semantic state exchange.

- Paper: https://arxiv.org/abs/2506.02739

### 3b. Latent Space Communication (Activation-Level)

**Communicating Activations** (Balesni et al., January 2025): Two LLMs communicate by injecting one model's intermediate activations into another's forward pass, bypassing tokens. **Up to 27% performance improvement** over NL debate at **1/4 compute cost**.

- Paper: https://arxiv.org/abs/2501.14082

**Interlat**: Uses last hidden states as direct representation of agent's "mind." **Up to 24x inference speedup**.

- Paper: https://arxiv.org/abs/2511.09149

**LatentMAS**: Training-free pure latent collaboration. Auto-regressive latent thought generation with shared working memory. **14.6% higher accuracy, 70.8%-83.7% token reduction, 4x-4.3x faster**.

- Paper: https://arxiv.org/abs/2511.20639
- Code: https://github.com/Gen-Verse/LatentMAS

**J17 relevance**: Not directly applicable (requires model internals access via API), but establishes the theoretical ceiling: **~75-85% of NL communication is redundant overhead**.

### 3c. Emergent Language in Multi-Agent RL (Lewis Games)

Neural agents develop discrete symbolic codes through Lewis signaling games. Key findings:

- Agents develop **compact task-optimized codes** from small vocabularies
- Emergent languages are efficient but lack compositionality and generalization
- Codes are opaque and task-specific
- Gumbel-softmax converges faster than pure RL

Key papers:
- Havrylov & Titov 2017: https://arxiv.org/abs/1705.11192
- Chaabouni et al. 2022: https://arxiv.org/abs/2209.15342
- Oxford Academic survey: https://academic.oup.com/jole/article/7/2/213/7128304

**J17 relevance**: Purpose-built codes CAN be far more compact than NL, but sacrifice interpretability. Hybrid approach (structured codes for common patterns, NL fallback for novel situations) combines best of both — exactly what Agora proposes.

### 3d. "Language Modeling is Compression" (ICLR 2024)

Establishes that LLMs are fundamentally compressors. Chinchilla 70B compresses ImageNet to 43.4% and LibriSpeech to 16.4% of raw size, beating domain-specific compressors.

**Key insight**: Since the receiver (Claude) is an excellent decompressor, send the **minimum viable signal** and let Claude reconstruct the full meaning. Redundancy in NL is the enemy.

- Paper: https://proceedings.iclr.cc/paper_files/paper/2024/file/3cbf627fa24fb6cb576e04e689b9428b-Paper-Conference.pdf

---

## 4. Domain-Specific Languages for Agent Commands

### Agora Protocol — Three-Tier Encoding (Oxford, 2024)

| Tier | When Used | Format | Cost |
|------|-----------|--------|------|
| Standard protocols | Frequent communication | JSON-Schema, OpenAPI | Lowest |
| LLM-written routines | Intermediate frequency | Dynamically negotiated JSON | Medium |
| Natural language | Rare/novel situations | Free text | Highest |

**5x cost reduction** over NL-only in 100-agent demo ($7.67 vs $36.23 for 1000 queries). Protocols self-organize: agents detect frequency thresholds, negotiate structured formats, share protocol documents (hash-identified specs).

- Paper: https://arxiv.org/abs/2410.11905
- Website: https://agoraprotocol.org/

### Policy-as-Prompt (Guardrail-Specific)

"AI Agent Code of Conduct" (arXiv:2509.23994): Translates NL policy documents into verifiable policy trees, compiled into "lightweight, prompt-based classifiers." Essentially a DSL for guardrail rules.

- Paper: https://arxiv.org/html/2509.23994v1

### ProtocolBench (2025)

Benchmarks agent communication protocols across four axes: task success, latency, message overhead, failure resilience. **Protocol choice impacts task time by up to 36%** and overhead by 3.5 seconds. Introduces ProtocolRouter — learned router selecting per-scenario protocols.

- Paper: https://openreview.net/forum?id=lqNqKUG2dn

---

## 5. Context-Window-Aware Communication

### Checkpointing Approaches

| Framework | State Mechanism |
|-----------|----------------|
| OpenAI Agents SDK | Session memory with automatic context management |
| LangGraph | Built-in checkpointing with thread IDs |
| Amazon Bedrock AgentCore | Persistent memory layer |
| Microsoft Agent Framework | AIContextProvider |

- https://cookbook.openai.com/examples/agents_sdk/session_memory
- https://docs.langchain.com/oss/python/langgraph/memory

### Context Poisoning Risk

Consolidation stage (compressing history into summaries) is most error-prone. Failure modes: losing info through over-pruning, promoting noise, introducing contradictions. Once a bad fact enters a summary, it persists.

- https://www.getmaxim.ai/articles/context-window-management-strategies-for-long-context-ai-agents-and-chatbots/

---

## Practical Token Savings Estimates for J17

| Technique | Reduction | Applicability |
|-----------|-----------|---------------|
| LLMLingua-style compression | 2-5x | Drop-in, no protocol changes |
| Telegraphic natural language | 2-3x | Authoring-time optimization |
| Structured encoding of common patterns | 5-10x | Requires protocol definition |
| Full latent communication | 10-25x | Requires model access (not API) |

**Realistic J17 target: 5-8x token reduction** by combining structured encoding for common constraint types with telegraphic NL for novel guidance.

---

## Concrete Encoding Example for Jiminy

Instead of natural language:
> "Remember that you should never delete nodes from the mdemg-dev space because it is protected and contains all conversation memory"

Structured compact format:
```
C:BLOCK|target:mdemg-dev|op:delete|reason:protected_space
```

Minimal JSON:
```json
{"t":"C","tgt":"mdemg-dev","op":"del","r":"protected"}
```

The "minimum viable signal" principle: Claude already knows what space protection means — no need to explain it. Send the code, let Claude decompress.

---

*Documents Accessed: Web search results for LLMLingua, Gist Tokens, ICAE, ACON, AgentPrune, LACP, A2A, Agora, latent space communication papers, Lewis games, Language Modeling is Compression, ProtocolBench.*
