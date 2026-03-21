# Emergent Communication Between AI Agents: Academic Research

**Research Date**: 2026-03-21
**Purpose**: J17 protocol design — can AI agents negotiate their own communication protocols?
**Researcher**: Claude (Opus 4.6) via web search

---

## 1. Foundational Papers (2016-2017) — "The Big Three"

### Foerster et al. — "Learning to Communicate with Deep Multi-Agent RL" (NIPS 2016)

First major deep RL paper on learned communication. Two approaches: RIAL (Reinforced Inter-Agent Learning, deep Q-learning) and DIAL (Differentiable Inter-Agent Learning, backpropagating error through noisy channels). Agents learn protocols end-to-end in partially observable environments.

- https://arxiv.org/abs/1605.06676
- Code: https://github.com/iassael/learning-to-communicate

### Lazaridou, Peysakhovich & Baroni — "Multi-Agent Cooperation and the Emergence of (Natural) Language" (ICLR 2017)

DeepMind/FAIR. Referential games: sender sees target image, transmits message from fixed vocabulary, receiver identifies target. Demonstrates cooperative pressure drives emergence of structured protocol.

- https://arxiv.org/abs/1612.07182

### Mordatch & Abbeel — "Emergence of Grounded Compositional Language" (AAAI 2018)

Language emerges as streams of abstract discrete symbols with defined vocabulary and syntax. When language unavailable, non-verbal communication (pointing, guiding) emerges instead. Language is grounded in the physical environment.

- https://arxiv.org/abs/1703.04908
- Code: https://github.com/bkgoksel/emergent-language

### Havrylov & Titov — "Emergence of Language with Multi-agent Games" (NIPS 2017)

Variable-length discrete symbol sequences. Gumbel-Softmax relaxation converges faster than REINFORCE. Emergent protocol exhibits **compositionality** and **variability** — same information phrased differently, a hallmark of natural languages.

- https://arxiv.org/abs/1705.11192

### Kottur et al. — "Natural Language Does Not Emerge 'Naturally'" (EMNLP 2017, Best Short Paper)

**Critical warning**: Without explicit inductive biases, agents develop degenerate or opaque codes. Emergent languages are NOT automatically compositional or human-interpretable.

- Code: https://github.com/kdexd/lang-emerge-parlai

---

## 2. LLM-Era Emergent Communication (2024-2026)

### Emergent Machine Language Between LLMs (OpenReview, October 2025)

**Most directly relevant to J17.** Two LLM agents develop a shared non-human-interpretable language for 541 objects through only **4 rounds of communication** (max 3 attempts each). The speaker retrieves semantically similar words before composing; the listener decodes based on structural proximity.

Emergent language exhibits **compositionality, generalizability, morphemes, and polysemy** — all defining features of human language. Proves LLMs do NOT need natural language to communicate efficiently.

- https://openreview.net/forum?id=zy06mHNoO2

### Emergent Social Conventions in LLM Populations (Science Advances, 2025)

Populations of 24-200 LLM agents spontaneously develop universally adopted conventions through pairwise interactions without central coordination. **Strong collective biases emerge even when individual agents exhibit no bias.** Adversarial minority groups can drive change by imposing alternatives.

- https://www.science.org/doi/10.1126/sciadv.adu9368
- https://arxiv.org/abs/2410.08948

### Generative Emergent Communication (January 2025)

LLMs learn a statistical approximation of a "collective world model" encoded in human language. Models language emergence as decentralized Bayesian inference (Collective Predictive Coding).

- https://arxiv.org/abs/2501.00226

### Emergent Coordination Without Explicit Communication (October 2025)

Information-theoretic framework detects higher-order coordination in multi-agent LLM systems even WITHOUT explicit communication channels. Distinguishes spurious temporal coupling from genuine cross-agent synergy.

- https://arxiv.org/abs/2510.05174

---

## 3. Beyond Natural Language: Latent Space Communication

### CIPHER — Probability-Weighted Embeddings (ICLR 2024)

Bypasses token sampling — generates weighted averages of all token embeddings in vocabulary. Agent A generates CIPHER embeddings, inserts them into Agent B's input as embeddings. **Outperforms NL debate by 0.5-5.0%** across five reasoning tasks with no model weight modification.

- https://arxiv.org/abs/2310.06272
- Code: https://github.com/chaudatascience/cipher_multiagent_debate

### DroidSpeak — KV Cache Sharing (Microsoft, November 2024)

For LLMs sharing a base model, shares intermediate data (KV caches) selectively per layer. **2.78x speedup** in prefill latency with negligible accuracy loss.

- https://arxiv.org/abs/2411.02820

### Interlat — Full Latent Space Communication (November 2025)

Agents transmit last hidden states through communication adapter. Entirely bypasses NL. Further compression: **up to 24x inference speedup**. Works across heterogeneous models.

- https://arxiv.org/abs/2511.09149

### LatentMAS — Training-Free Latent Collaboration (November 2025)

End-to-end training-free pure latent collaboration. Auto-regressive latent thought generation with shared working memory. **14.6% higher accuracy, 70.8%-83.7% token reduction, 4x-4.3x faster**.

- https://arxiv.org/abs/2511.20639
- Code: https://github.com/Gen-Verse/LatentMAS

### State Delta Trajectory Encoding (EMNLP 2025)

Augments NL messages with token-wise differences between hidden states of adjacent tokens — captures reasoning process hidden behind inference. **0.3-17.3% improvement** over NL and CIPHER, especially in complex reasoning.

- https://arxiv.org/abs/2506.19209

### Thought Communication (NeurIPS 2025 Spotlight)

Formalizes "mind-to-mind" communication using latent variable model. Proves both shared and private latent thoughts between agent pairs can be **identified**. Implements sparsity-regularized autoencoder to extract latent thoughts from hidden states.

- https://arxiv.org/abs/2510.20733

---

## 4. Compression Through Shared Context (Pragmatics)

### Rational Speech Acts (RSA) Framework

Goodman & Frank (Science 2012, TiCS 2016): Models communication as recursive Bayesian reasoning. Pragmatic listener reasons about speaker who reasons about literal listener. **Shared common knowledge lowers communication cost while keeping meaning unambiguous** — the "jargon" phenomenon.

- https://langcog.stanford.edu/papers_new/goodman-2016-tics.pdf
- Tutorial: http://www.problang.org/chapters/01-introduction.html

### Collaborative RSA (2025)

Extends RSA to multi-turn task-oriented dialogues. Integrates multi-turn gain function grounded in interactive rate-distortion theory — direct formalization of cost vs. fidelity tradeoff.

- https://arxiv.org/abs/2507.14063

### Anti-Efficiency Warning (NeurIPS 2019)

**Chaabouni et al.**: Neural agents develop **anti-efficient** encoding — most frequent inputs get LONGEST messages, violating Zipf's Law. Emergent protocols do NOT automatically exploit frequency-based compression.

- https://arxiv.org/abs/1905.12561

### LazImpa: Recovering Efficiency (CoNLL 2020)

Zipf-like efficiency CAN be recovered by imposing explicit **length penalties on speakers** and pushing **listeners to guess early**. Shared-context compression requires the right inductive biases — it's not automatic.

- https://aclanthology.org/2020.conll-1.26/

---

## 5. Stability, Drift, and Self-Play

### Language Drift — The Core Problem

**Lee, Cho et al. (2019)**: Agents pretrained on NL experience **detrimental language drift** when given non-linguistic rewards. Protocol can "easily and radically diverge from natural language." Solutions: syntactic constraints (language model likelihood) + semantic constraints (visual grounding).

- https://arxiv.org/abs/1909.04499

### Scaling Up (ICLR 2022, DeepMind)

**Evtimova et al.**: First large-scale study. RL training becomes unstable at scale but responds to stabilization techniques. Topographic similarity (common structure metric) does NOT correlate with generalization performance.

- https://openreview.net/forum?id=AUGBfDIV9rL
- Code: https://github.com/google-deepmind/emergent_communication_at_scale

### Stability Mechanisms from MARL Literature

- **KL regularization**: Enforces proximity to reference policy
- **Decentralized consensus**: Local averages converge to global average
- **Communication topology learning**: Structure itself is learned by maximizing rewards

---

## 6. Research Toolkits

| Toolkit | Source | Focus |
|---------|--------|-------|
| **EGG** (Meta/FAIR) | https://github.com/facebookresearch/EGG | Canonical research toolkit. Referential games, reconstruction games. Gumbel-Softmax + REINFORCE. |
| DeepMind EmCom | https://github.com/google-deepmind/emergent_communication_at_scale | JAX/Haiku. Large-scale population experiments. |
| Awesome Emergent Languages | https://github.com/vermashresth/awesome-emergent-languages | Curated paper list covering full field. |

---

## 7. Survey Papers

**Peters et al. (2025)**: "Emergent Language: A Survey and Taxonomy" — 181 papers reviewed. Taxonomy covering grounding, compositionality, efficiency. Most thorough field overview.
- https://arxiv.org/abs/2409.02645

**Yan et al. (2025)**: "Beyond Self-Talk: Communication-Centric Survey of LLM-Based MAS" — Framework integrating system-level and internal communication.
- https://arxiv.org/abs/2502.14321

**Zhou et al. (2025)**: "Why Do AI Agents Communicate in Human Language?" — NL is structurally misaligned with LLMs' high-dimensional vector spaces, causing information loss and behavioral drift.
- https://arxiv.org/abs/2506.02739

---

## Key Takeaways for J17

1. **It is demonstrably possible.** LLM agents develop shared languages in 4 rounds for 541 concepts.

2. **Language drift is real and dangerous.** Without grounding, protocols diverge from interpretability. For a guardrail system, auditability is critical. Solution: hybrid approach with NL "shadow" alongside efficient protocol.

3. **Shared-context compression is NOT automatic.** Requires explicit inductive biases (length penalties, early-guess incentives).

4. **RSA provides theoretical backbone.** Formalizes how shared knowledge enables compressed communication.

5. **Latent space is the theoretical ceiling** (~75-85% token reduction) but requires model access. Not applicable to API-based communication.

6. **A hybrid is likely optimal**: Structured codes for common patterns, NL fallback for novel situations, latent space if model access becomes available.

7. **Stability requires active management**: KL regularization, consensus mechanisms, grounding constraints.

---

*Documents Accessed: Web search results across 40+ academic papers, GitHub repos, and survey articles on emergent communication, latent space communication, pragmatics, and multi-agent RL.*
