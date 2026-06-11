# LoRA Fine-Tuning Glossary

**Author:** reh3376
**Date:** 2026-04-27

A LoRA (Low-Rank Adaptation) is a small set of trainable parameters that, when added on top of a frozen pretrained base model, adapts that model's behavior for a specific task or domain. The base model — which may be tens of gigabytes — stays frozen and unchanged; the LoRA adapter is typically tens to hundreds of megabytes and captures only the difference between the base model's behavior and the desired specialized behavior.

The architectural property that makes LoRA particularly powerful in production is that a single base model can host many adapters simultaneously, with the active adapter selected programmatically per request. A serving system holds the base model resident in GPU memory once, then loads multiple LoRA adapters into a small additional memory pool. Each incoming request specifies which adapter to apply; the runtime composes the selected adapter with the base model's weights at inference time and produces a response shaped by that adapter's training. The next request can specify a different adapter and get behavior shaped accordingly — without unloading and reloading the base model.

This pattern enables several deployment shapes that would otherwise be expensive: per-tenant customization in multi-tenant SaaS (each customer's adapter trained on their own data and selected when they call), per-task specialization (a single base model handling code generation, document summarization, and structured-output tasks via different adapters per call), per-domain expertise (legal, medical, financial adapters dispatched based on the request's domain), and rapid A/B testing of fine-tuning variants (multiple candidate adapters loaded simultaneously, requests routed across them by weight). vLLM, SGLang, TGI, and similar production inference engines all implement multi-LoRA serving as a first-class feature.

The glossary that follows defines many of th key terms you will encounter when building LoRA training workflows, organized by category.

---

## On unverified acronyms

Two acronyms in your list — **RTL** and **DFT** — don't match any standard fine-tuning vocabulary. My best guesses:


## Training paradigms

### SFT: Supervised Fine-Tuning

**Description.** The basic fine-tuning paradigm. The model is trained on (prompt, correct-completion) pairs to imitate desired outputs, using cross-entropy loss against the labeled completion. Either updates all model parameters (full SFT) or a subset via PEFT methods like LoRA. SFT is the foundation on which other fine-tuning paradigms (RLHF, DPO, RFT) build — almost every modern alignment pipeline begins with SFT.

**Use cases.** Domain adaptation (legal, medical, code-specific), instruction following, style and persona training, format compliance (JSON, structured outputs), and any setting where labeled (input, output) examples are available.

**Best used for.** Tasks where you have high-quality labeled data and need the model to imitate specific outputs reliably. **Almost always the first fine-tuning step in any multi-stage pipeline**.

**Where to find more info.** https://huggingface.co/docs/trl/en/sft_trainer

---

### RLHF: Reinforcement Learning from Human Feedback

**Description.** Three-stage post-training pipeline: (1) SFT on instruction data, (2) train a separate reward model on pairs of human-ranked responses, (3) optimize the language-model policy against the reward model using a reinforcement-learning algorithm (typically PPO). The dominant alignment technique through 2023; less common in 2025–2026 setups that have largely shifted to DPO or RFT.

**Use cases.** Aligning model outputs with human preferences for helpfulness, harmlessness, and honesty. ChatGPT, early Claude versions, Llama-2-Chat, and similar production models were trained with RLHF.

**Best used for.** Aligning models with subjective quality criteria where there is no ground-truth correctness — politeness, tone, helpfulness, safety. Less suitable for objective-correctness tasks where RFT or GRPO are more efficient.

**Where to find more info.** https://huggingface.co/blog/rlhf

---

### RLFT: Reinforcement Learning Fine-Tuning

**Description.** Umbrella term covering any post-training method that uses reinforcement-learning signal rather than supervised labels. Includes RLHF, RFT, DPO, GRPO, PRIME, and similar. Sometimes used interchangeably with "post-training RL." The term has become more visible since DeepSeek-R1 (early 2025) demonstrated that RL on verifiable rewards alone (without RLHF's human-preference stage) can produce strong reasoning capabilities.

**Use cases.** Any setting where a reward signal (human preference, rule-based grader, programmatic verifier) is more available or appropriate than labeled completions.

**Best used for.** As a category descriptor when distinguishing RL-based fine-tuning from supervised approaches. For specific deployment, choose the concrete method (DPO, RFT, GRPO).

**Where to find more info.** https://medium.com/data-science-at-microsoft/fine-tuning-llms-with-reinforcement-learning-ef84fe42d6a6

---

### DPO: Direct Preference Optimization

**Description.** Alignment technique that bypasses RLHF's separate reward-model training. The model learns directly from pairwise preference comparisons (preferred response vs rejected response) using a closed-form loss derived from the implicit reward. Mathematically equivalent to RLHF under specific assumptions; lighter-weight, more stable, no separate reward-model training needed. Standard in 2025–2026 alignment workflows.

**Use cases.** Preference alignment from pairwise human or AI-generated comparisons, style and tone adjustment, safety alignment with preference data, helpfulness training when preference pairs are easier to collect than absolute correctness labels.

**Best used for.** Refining a model that has already been SFT-trained, when preference pairs are available and human-preference RLHF is too complex to set up. Typically the second stage after SFT in modern alignment pipelines.

**Where to find more info.** https://arxiv.org/abs/2305.18290 (Rafailov et al., 2023)

---

### RFT: Reinforcement Fine-Tuning (OpenAI usage) / Rejection-sampling Fine-Tuning (academic usage)

**Description.** Two distinct methods share this acronym. *Reinforcement Fine-Tuning* (OpenAI, 2024+) is a custom-grader-based RL fine-tuning workflow: the user provides a programmatic grader, the model samples candidate responses, the grader scores each, and policy-gradient updates push the model toward higher-scoring outputs. *Rejection-sampling Fine-Tuning* (Yuan et al., 2023+) is an iterative SFT-on-self-generated-correct-outputs workflow: the model generates candidates, only correct ones (per task-specific criteria) are kept, the model is fine-tuned on the correct subset, and the cycle repeats. Both improve model performance on tasks with verifiable correctness, through different mechanisms.

**Use cases.** Math problem solving, code generation with test-based verification, structured-output tasks with format checkers, scientific reasoning with answer validators, any domain where outputs can be programmatically scored.

**Best used for.** Tasks with verifiable correctness criteria (mathematical answers, executable code, formal logic) where a grader function exists or can be written. Especially effective for smaller models on reasoning tasks.

**Where to find more info.** https://platform.openai.com/docs/guides/reinforcement-fine-tuning and https://arxiv.org/abs/2308.01825

---

### ORPO: Odds Ratio Preference Optimization

**Description.** A 2024 method (Hong et al.) combining SFT and preference optimization in a single training stage. Adds an odds-ratio penalty term to the standard SFT loss; the model simultaneously learns to imitate good completions and disprefer rejected ones. Eliminates the need for a separate preference-optimization pass after SFT, simplifying the pipeline from two stages to one.

**Use cases.** End-to-end fine-tuning when both SFT data and preference pairs are available. Reduces training-pipeline complexity for resource-constrained alignment workflows.

**Best used for.** Resource-constrained alignment where a two-stage SFT+DPO pipeline is too expensive, or when preference data and SFT data come from the same dataset and can be jointly optimized.

**Where to find more info.** https://arxiv.org/abs/2403.07691 (Hong et al., 2024)

---

### KTO: Kahneman-Tversky Optimization

**Description.** Preference-tuning method (Ethayarajh et al., 2024) using a loss function inspired by prospect theory from behavioral economics. Unlike DPO, KTO does not require pairwise preferences — it works with binary "thumbs up / thumbs down" feedback on individual responses, which is much cheaper to collect than pairwise comparisons.

**Use cases.** Settings where binary feedback is available but pairwise preferences are not (production user feedback, simple labeling tasks), large-scale preference learning where pairwise annotation is impractical.

**Best used for.** Production deployments with thumbs-up/thumbs-down user feedback, scaling preference learning when pairwise annotation is prohibitively expensive.

**Where to find more info.** https://arxiv.org/abs/2402.01306 (Ethayarajh et al., 2024)

---

### SimPO: Simple Preference Optimization

**Description.** Reference-free preference optimization (Meng et al., 2024). Drops the SFT-reference-model term that DPO retains; uses average log-probability as the implicit reward. Reduces memory cost (no reference model resident at training time), often improves stability, and matches or exceeds DPO on standard benchmarks.

**Use cases.** Memory-constrained preference training, large-scale alignment where loading a reference model alongside the policy is expensive, simplified DPO-style workflows.

**Best used for.** Drop-in replacement for DPO when memory is tight or when you want a simpler training loop. Particularly attractive for very large models where the reference model would be expensive to keep resident.

**Where to find more info.** https://arxiv.org/abs/2405.14734 (Meng et al., 2024)

---

### PPO: Proximal Policy Optimization

**Description.** A reinforcement-learning algorithm (Schulman et al., 2017) that clips policy updates to prevent destabilizing large changes. Inside RLHF, PPO is the standard choice for the policy-optimization stage. The clipping mechanism limits how far the new policy can move from the old policy in a single update, preventing the catastrophic policy collapse common in vanilla policy-gradient methods.

**Use cases.** RL fine-tuning of LLMs (within RLHF), continuous-control reinforcement learning (the original use case), any RL setting where update stability matters more than maximum step size.

**Best used for.** The traditional reward-model-based RLHF pipeline. Largely being replaced by GRPO in newer workflows because GRPO does not need a learned value function.

**Where to find more info.** https://arxiv.org/abs/1707.06347 (Schulman et al., 2017)

---

### GRPO: Group Relative Policy Optimization

**Description.** A variant of PPO introduced by DeepSeek (2024) that estimates advantages by comparing rollouts within a group of generations from the same prompt, rather than against a learned value function. Eliminates the need for a separate value/critic model, cutting memory cost roughly in half. Used heavily in DeepSeek-R1's training and now standard for reasoning-model RL fine-tuning. The method that made RL-on-verifiable-rewards practical at scale.

**Use cases.** Reasoning-model fine-tuning with verifiable rewards (math, code), any RL setting where memory pressure rules out a separate value model, RL fine-tuning of large LLMs in resource-constrained environments.

**Best used for.** RL fine-tuning of LLMs against verifiable rewards (correct math answer, passing tests, schema-valid output). The current default RL algorithm for reasoning-model post-training in open-source frameworks.

**Where to find more info.** https://arxiv.org/abs/2402.03300 (DeepSeekMath paper, where GRPO was introduced)

---

### PRIME: Process Reinforcement through Implicit Rewards

**Description.** A 2024–2025 RL fine-tuning method that uses implicit process rewards rather than only outcome rewards. Trains the model with feedback on intermediate reasoning steps as well as the final answer, using a learned implicit-reward model. Particularly relevant for long-form reasoning tasks where outcome-only signal is sparse and hard to learn from.

**Use cases.** Long chain-of-thought reasoning training, mathematical proof generation, multi-step planning tasks where intermediate steps matter, code synthesis with multi-stage verification.

**Best used for.** Reasoning tasks where outcome-only rewards are too sparse to learn efficiently and where process-level supervision is available or can be synthesized.

**Where to find more info.** https://arxiv.org/abs/2502.01456 (Cui et al., 2025)

---

### CLCFT: Curriculum Learning / Curriculum Fine-Tuning

**Description.** Training paradigm where examples are presented in order of increasing difficulty rather than random order. The model learns easy patterns first, then progressively harder ones. The pedagogical intuition is direct: the same way a human is taught arithmetic before calculus, a model learns more efficiently when its training distribution starts simple and grows complex. Difficulty can be measured by various proxies — input length, perplexity under a reference model, hand-curated difficulty labels, or task-specific complexity scores.

**Use cases.** Mathematical reasoning training where problem difficulty can be ranked, code generation where simple-to-complex progression helps, multi-step reasoning where building from short chains to long chains aids learning, any domain with an ordering on example difficulty.

**Best used for.** Fine-tuning where the training data has natural difficulty stratification and where empirical evidence shows order-of-presentation matters. Less essential for IID i.i.d. data; most useful when the task requires compositional or multi-step reasoning.

**Where to find more info.** https://arxiv.org/abs/2101.10382 (Soviany et al., curriculum learning survey) and https://arxiv.org/abs/2410.07064 (recent curriculum fine-tuning for LLM reasoning)

---

## Parameter-efficient fine-tuning (PEFT)

### PEFT: Parameter-Efficient Fine-Tuning

**Description.** Umbrella term for methods that update a small fraction of model parameters during fine-tuning, leaving the bulk of pretrained weights frozen. Distinct from full fine-tuning, which updates all parameters. PEFT methods enable fine-tuning of very large models (10B+ parameters) on consumer-grade hardware that could not host the optimizer state for full fine-tuning. HuggingFace's `peft` library is the standard implementation, supporting LoRA, DoRA, IA³, and others.

**Use cases.** Fine-tuning large models on limited hardware, training many task-specific adapters that share a base model, multi-tenant deployment where each user gets a custom adapter, rapid experimentation where full retraining is too expensive.

**Best used for.** Almost all modern fine-tuning workflows. Full fine-tuning is now reserved for cases where the base model is small or where the highest possible quality is required regardless of cost.

**Where to find more info.** https://huggingface.co/docs/peft

---

### LoRA: Low-Rank Adaptation

**Description.** The dominant PEFT method (Hu et al., 2021). Freezes the base model entirely and adds trainable low-rank decomposition matrices `B · A` (where `A` is `r × d` and `B` is `d × r` with `r << d`) to selected linear layers. The effective weight update is `ΔW = (α/r) · B · A`, where α is a scaling factor. Trains only ~0.1–1% of the model's total parameters. The resulting adapter is small (typically tens to hundreds of MB) and can be loaded on top of the base model at inference with minimal overhead.

**Use cases.** Domain adaptation, task-specific adapters, instruction tuning, multi-tenant deployments where each tenant has a custom adapter, rapid experimentation, fine-tuning models too large for full FT on available hardware.

**Best used for.** The default starting point for nearly all fine-tuning today. Most other PEFT methods are LoRA variants.

**Where to find more info.** https://arxiv.org/abs/2106.09685 (Hu et al., 2021)

---

### QLoRA: Quantized LoRA

**Description.** LoRA training where the base model is held in 4-bit quantization throughout training (Dettmers et al., 2023). Gradients still flow through the quantized weights, but only the activations and weights are quantized; the LoRA adapter itself trains in higher precision (typically bfloat16). Enables training of much larger models on smaller hardware — for example, fine-tuning a 65B-parameter model on a single 48GB GPU. Uses NF4 (NormalFloat 4-bit) quantization, double quantization, and paged optimizers for memory efficiency.

**Use cases.** Fine-tuning very large models (30B–70B+) on consumer or single-GPU hardware, memory-constrained training environments, situations where the base model would not fit in GPU memory at higher precision.

**Best used for.** When the base model would not fit in available GPU memory at the precision the LoRA adapter trains in. The standard choice for fine-tuning large models on limited hardware.

**Where to find more info.** https://arxiv.org/abs/2305.14314 (Dettmers et al., 2023)

---

### DoRA: Weight-Decomposed Low-Rank Adaptation

**Description.** Variant of LoRA (Liu et al., 2024, ICML Oral) that decomposes pretrained weights into magnitude and direction components, using LoRA only for directional updates while a separate trainable parameter handles magnitude. Closes much of the accuracy gap between LoRA and full fine-tuning. Often outperforms LoRA at the same rank, especially on commonsense reasoning and visual instruction tuning. No additional inference overhead — the magnitude/direction decomposition can be merged back into the base weights after training.

**Use cases.** Drop-in replacement for LoRA when higher quality is needed at similar parameter cost. Particularly effective on commonsense reasoning, visual instruction tuning, and image/video-text understanding tasks.

**Best used for.** When you would otherwise use LoRA but want closer-to-full-FT quality without taking on the cost of full fine-tuning. Supported as a flag (`use_dora=True`) in HuggingFace PEFT.

**Where to find more info.** https://arxiv.org/abs/2402.09353 (Liu et al., 2024)

---

### rsLoRA: Rank-Stabilized LoRA

**Description.** LoRA variant (Kalajdzievski, 2023) with corrected scaling factor. Standard LoRA scales updates by `α/r`, which causes gradients to vanish at high ranks; rsLoRA scales by `α/√r`, which preserves gradient magnitude as rank increases. The practical effect: high-rank LoRA training (r=64, 128, 256) actually helps with rsLoRA where it plateaus or degrades with standard LoRA.

**Use cases.** High-rank LoRA training where standard LoRA loses gradient signal, settings where higher-capacity adapters are needed (complex domain adaptation, multi-task adapters), any case where you want to scale rank to improve quality without hitting the standard-LoRA plateau.

**Best used for.** High-rank LoRA (r ≥ 64). At small ranks (r ≤ 16) the difference from standard LoRA is small.

**Where to find more info.** https://arxiv.org/abs/2312.03732 (Kalajdzievski, 2023)

---

### IA³: Infused Adapter by Inhibiting and Amplifying Inner Activations

**Description.** PEFT method (Liu et al., 2022) that learns scaling vectors applied to activations of selected linear layers, rather than weight updates. Even smaller parameter footprint than LoRA — typically 0.01% of base parameters or less. The adapter consists of three vectors per transformer layer (one for keys, values, and the FFN intermediate activation). Fast to train, very small to store, but generally lower-capacity than LoRA.

**Use cases.** Extreme parameter efficiency requirements, deployment on tiny storage budgets (mobile, edge), settings where many lightweight adapters need to be trained quickly.

**Best used for.** Cases where LoRA is still too expensive in storage or training cost. Less suitable when the task requires substantial adaptation capacity.

**Where to find more info.** https://arxiv.org/abs/2205.05638 (Liu et al., 2022, "Few-Shot Parameter-Efficient Fine-Tuning is Better and Cheaper than In-Context Learning")

---

## LoRA-specific terminology

### r (rank)

**Description.** The rank of the LoRA low-rank decomposition. Determines the dimensionality of the bottleneck in the `B · A` decomposition. Typical values: 8, 16, 32, 64. Higher rank gives more capacity at proportional parameter and memory cost. The most-tuned LoRA hyperparameter.

**Use cases.** Lower ranks (4–16) for simple instruction tuning and lightweight adaptation. Higher ranks (32–128) for complex domain adaptation or larger-quality requirements (especially with rsLoRA, where high ranks are stable).

**Best used for.** Always tuned per task. A common default is r=16 or r=32; expand if quality plateaus, contract if memory or speed is the bottleneck.

**Where to find more info.** https://arxiv.org/abs/2106.09685 §4

---

### lora_alpha (α)

**Description.** Scaling factor applied to the LoRA delta: effective contribution is `(α/r) · B · A`. Conventionally set to `2 · r` though this is often varied. Controls the magnitude of the LoRA update relative to the base weights — higher α means the LoRA delta has more influence on the output. Often treated as the LoRA "learning-rate scale" that's separate from the optimizer learning rate.

**Use cases.** Universal LoRA hyperparameter, set in every LoRA config.

**Best used for.** Default `α = 2r`. If gradients explode, lower α; if the adapter has no measurable effect, raise α (or check for other bugs first).

**Where to find more info.** https://huggingface.co/docs/peft/conceptual_guides/lora

---

### target_modules

**Description.** Which linear layers in the base model receive LoRA adapters. Standard targets for transformer LLMs: `q_proj`, `k_proj`, `v_proj`, `o_proj` (attention) and `gate_proj`, `up_proj`, `down_proj` (MLP). Sometimes restricted to attention only (cheaper, lower quality); sometimes extended to embedding layers (rare). The choice dictates both parameter count and adaptation expressiveness.

**Use cases.** Every LoRA config specifies target modules. Different model architectures have different module names; QLoRA defaults to all linear layers.

**Best used for.** "All linear layers" (attention + MLP) is the modern default for quality. Attention-only is acceptable for very lightweight adaptation.

**Where to find more info.** https://huggingface.co/docs/peft/main/en/conceptual_guides/lora#target-modules

---

### Adapter

**Description.** The trained PEFT delta — typically a LoRA adapter file. Distinct from the base model. Stored in PEFT format with `adapter_config.json` (specifying rank, α, target modules, base model, revision) and a weights file. Adapters are small (tens to hundreds of MB) and can be distributed independently of the base model.

**Use cases.** Distribution unit for fine-tuned models. Multi-tenant serving where each tenant gets a custom adapter on a shared base. Versioning and A/B testing of fine-tuning runs.

**Best used for.** Sharing fine-tuned models without re-uploading the base. The HuggingFace Hub stores millions of LoRA adapters this way.

**Where to find more info.** https://huggingface.co/docs/peft/package_reference/peft_model

---

### Merging / fusing

**Description.** Process of permanently combining a LoRA adapter into the base model weights, producing a single merged checkpoint. Removes inference-time adapter-application overhead but loses the ability to swap adapters. MLX supports this via `mlx_lm.fuse`; HuggingFace via `peft.merge_and_unload()`. The merged model behaves identically to the adapter+base combination but is treated as a standalone model.

**Use cases.** Production deployment where adapter-application overhead matters, distributing the fine-tuned model as a single artifact, eliminating the runtime dependency on PEFT.

**Best used for.** Once fine-tuning is complete and you've decided on the adapter you want to ship. After merging, the model is no longer extensible without retraining.

**Where to find more info.** https://huggingface.co/docs/peft/main/en/developer_guides/lora#merge-lora-weights-into-the-base-model

---

### Multi-LoRA serving

**Description.** Inference-time pattern where multiple LoRA adapters are loaded over a single base model and selected per request. Supported by vLLM (`--enable-lora`), SGLang, and TGI. Enables task-specific or per-tenant adapters without separate model loading. The base model occupies most GPU memory; each adapter adds only tens to hundreds of MB; switching adapters between requests is fast.

**Use cases.** Multi-tenant SaaS deployments where each customer has a custom adapter, A/B testing of fine-tuning variants, serving a portfolio of task-specific models without dedicating GPUs to each.

**Best used for.** Production deployments with many concurrent tenants or tasks. Wasteful for single-tenant single-task deployments.

**Where to find more info.** https://docs.vllm.ai/en/latest/features/lora.html

---

## Foundational ML and architectural terms

### MLP: Multi-Layer Perceptron

**Description.** A fully-connected feed-forward neural network. In transformer architecture, the `mlp` block (also called "FFN" or "feed-forward") sits after attention in each transformer layer; it typically expands the hidden dimension by 4× through one linear layer, applies a non-linearity (ReLU, GeLU, SwiGLU, or similar), then projects back. The MLP block is one of the standard groups of LoRA target modules — for Llama-style models, the target names are `gate_proj`, `up_proj`, `down_proj`.

**Use cases.** Universal building block in modern neural networks. The transformer's MLP block is one of two parameter-heavy regions (the other is attention) and is a primary LoRA target.

**Best used for.** Foundational concept, not a deployment choice.

**Where to find more info.** https://en.wikipedia.org/wiki/Multilayer_perceptron

---

### CoT: Chain-of-Thought

**Description.** Prompting and training pattern where the model produces explicit step-by-step reasoning before the final answer (Wei et al., 2022). At training time, CoT-distilled data includes the teacher model's reasoning traces in the completion; at inference time, CoT prompting elicits intermediate reasoning. Critical for reasoning models — modern reasoning architectures (DeepSeek-R1, o-series) train extensively on long CoT traces.

**Use cases.** Reasoning-model training data curation, math and code SFT data generation, intermediate-step supervision in RL fine-tuning, evaluation of reasoning-quality.

**Best used for.** Training data for tasks requiring multi-step reasoning. Always include CoT in training data when the production task benefits from explicit reasoning.

**Where to find more info.** https://arxiv.org/abs/2201.11903 (Wei et al., 2022)

---

### DCT: Discrete Cosine Transform

**Description.** A signal-processing transformation that decomposes a signal into a sum of cosine functions at different frequencies. Used in image/video compression (JPEG, MPEG). In recent LLM research, DCT has appeared in some weight-compression and quantization work — applying DCT to weight matrices and discarding high-frequency components before quantization. Not core fine-tuning vocabulary but adjacent.

**Use cases.** Weight compression, frequency-domain quantization research, lossy storage of LoRA adapters in research settings.

**Best used for.** Specialized model-compression research. Not part of standard fine-tuning workflows.

**Where to find more info.** https://en.wikipedia.org/wiki/Discrete_cosine_transform

---

## Inference and quantization

### MLX: Apple Machine Learning Framework

**Description.** Apple's ML framework optimized for Apple Silicon. Built on Metal compute shaders. Distinct from PyTorch's MPS backend — MLX is a complete framework with its own array API, automatic differentiation, and operator library. The `mlx-lm` package provides LLM training and inference; `mlx_lm.lora` is the LoRA-training entrypoint. MLX takes advantage of Apple's unified memory architecture, allowing the CPU and GPU to share a single memory pool — particularly relevant on M-series Macs with 64GB+ unified memory.

**Use cases.** Local LLM training on Apple Silicon (M1/M2/M3/M4/M5 series), inference workloads on Mac development machines, situations where unified memory architecture eliminates CPU↔GPU transfer overhead.

**Best used for.** LLM fine-tuning and inference on Apple Silicon when CUDA is not available. The standard choice for local LoRA training on M-series Macs.

**Where to find more info.** https://github.com/ml-explore/mlx and https://github.com/ml-explore/mlx-lm

---

### MPS: Metal Performance Shaders

**Description.** Apple's GPU compute layer. Two distinct usages in ML contexts: PyTorch's MPS backend (allowing `device='mps'` for Apple Silicon training within PyTorch), and the underlying Metal shader system that MLX is built on. PyTorch MPS and MLX are different — PyTorch MPS is a PyTorch backend; MLX is its own framework. PyTorch MPS has historically been less mature than CUDA, with various operator gaps; MLX, being purpose-built for Apple Silicon, often has better performance for Mac-native workflows.

**Use cases.** PyTorch training on Apple Silicon (when CUDA is unavailable), as the underlying compute substrate of MLX.

**Best used for.** Choose MLX over PyTorch+MPS for serious Apple Silicon ML work; PyTorch+MPS is a fallback for code that already exists in PyTorch.

**Where to find more info.** https://pytorch.org/docs/stable/notes/mps.html

---

### MoE: Mixture of Experts

**Description.** Architecture where multiple "expert" sub-networks coexist and a routing layer selects which experts process each token. Sparse MoE (only top-k experts active per token, k typically 2) is the dominant production form — Mixtral 8×7B, DeepSeek-MoE, Qwen-MoE all use this pattern. LoRA-on-MoE is harder than LoRA-on-dense because target-module selection interacts with routing, and Apple Silicon's Metal MTLResource cap (499K) constrains MoE-on-MLX training in ways that have forced MoE→dense pivots in some workflows.

**Use cases.** Scaling model capacity without proportionally scaling inference cost (only k experts run per token), specialized expert routing for multi-domain tasks, very large models where dense inference would be too expensive.

**Best used for.** Production inference of very large models. Less appropriate for resource-constrained training (especially on Apple Silicon, where Metal resource limits bite).

**Where to find more info.** https://arxiv.org/abs/2101.03961 (Switch Transformer, Fedus et al., 2021) and https://huggingface.co/blog/moe

---

### FP4 / FP8 / FP16 / BF16

**Description.** Floating-point precisions commonly used in modern LLM training and inference. FP16 (half precision, 16-bit) was the original mixed-precision-training standard. BF16 (bfloat16, 16-bit with FP32 dynamic range) is now standard for training because it avoids FP16's gradient-underflow issues. FP8 (8-bit floating point) is supported on H100-class hardware for training; FP4 (4-bit floating point, with variants like NF4 and mxfp4) is used for inference quantization and QLoRA-style training.

**Use cases.** Training precision selection (BF16 standard, FP8 on H100), inference precision selection (FP16/BF16 for high quality, FP8/FP4 for memory-constrained serving), mixed-precision pipelines.

**Best used for.** BF16 for training when supported (modern A100/H100/Apple Silicon). FP4 (NF4 specifically) for memory-constrained inference and QLoRA training.

**Where to find more info.** https://huggingface.co/docs/transformers/perf_train_gpu_one and https://arxiv.org/abs/2305.14314

---

### Q4 / Q8 / int4 / int8 / mxfp4

**Description.** Integer-and-microscaling quantization formats. Q4 is the most common for production LLM serving — model weights stored in 4-bit integers with associated scaling factors. Different Q4 variants exist: Q4_0 (simple grouped scaling), Q4_K_M (k-quants, used in llama.cpp), AWQ (activation-aware), GPTQ (generative-pretraining-aware), NF4 (NormalFloat, used in QLoRA), and mxfp4 (MicroScaling FP4, a 4-bit floating-point scheme using small block-wise scaling factors). Each trades quantization accuracy against compute and storage cost.

**Use cases.** Inference serving of large models on small hardware (4-bit quantization typically reduces memory ~4× vs FP16 with modest quality loss), QLoRA training (NF4), llama.cpp deployment (Q4_K_M).

**Best used for.** Q4_K_M (llama.cpp/GGUF) for CPU inference; NF4 for QLoRA training; AWQ or GPTQ for fastest GPU inference; mxfp4 for accuracy-sensitive 4-bit deployment on supported hardware.

**Where to find more info.** https://github.com/ggml-org/llama.cpp/blob/master/docs/quantize.md

---

### llama.cpp

**Description.** C++ inference engine for LLMs. Uses GGUF (GPT-Generated Unified Format) as its model file format. Supports a wide range of quantization formats and runs on CPU, CUDA, Metal, ROCm, and other backends. Supports GGUF LoRA adapters but with some compatibility constraints relative to the HuggingFace PEFT format.

**Use cases.** CPU-only inference, low-end hardware deployment, integration into desktop applications (LM Studio, Jan, Ollama all wrap llama.cpp), embedded inference scenarios.

**Best used for.** Inference on hardware without sufficient GPU memory for the full model, or on systems where Python/PyTorch deployment is impractical. Not for training.

**Where to find more info.** https://github.com/ggml-org/llama.cpp

---

### GGUF: GPT-Generated Unified Format

**Description.** llama.cpp's model file format. Single-file, contains weights and metadata. Supports a range of quantization formats. Distinct from HuggingFace's safetensors/PEFT format. Conversion is one-way easy (HF → GGUF) but lossy in some cases (LoRA adapter conversion can lose precision).

**Use cases.** Distribution format for llama.cpp-compatible quantized models. Standard format for the local-LLM community (Ollama, LM Studio, Jan).

**Best used for.** Distribution of quantized models for CPU/Metal inference. Use HuggingFace safetensors format for training and high-precision inference.

**Where to find more info.** https://github.com/ggml-org/ggml/blob/master/docs/gguf.md

---

### vLLM

**Description.** High-throughput LLM inference engine with multi-LoRA support and PagedAttention for efficient KV-cache management. Production-standard for serving many concurrent users at high throughput. Supports OpenAI-compatible API, structured outputs, speculative decoding, prefix caching, and tensor/pipeline parallelism.

**Use cases.** Production LLM serving at scale, multi-tenant deployment with per-tenant LoRA adapters, high-throughput inference workloads.

**Best used for.** Server-side production deployment of LLMs at scale. Not appropriate for desktop or embedded inference (use llama.cpp instead).

**Where to find more info.** https://docs.vllm.ai

---

### Ollama

**Description.** Convenience wrapper around llama.cpp with model management, a simple CLI, and an OpenAI-compatible API. Targets local desktop usage. Supports LoRA adapters via the `ADAPTER` directive in Modelfiles.

**Use cases.** Local LLM development, single-user desktop deployment, prototyping integrations against an OpenAI-compatible API without paying for OpenAI.

**Best used for.** Developer machines and single-user local deployments. Not appropriate for multi-tenant production (use vLLM instead).

**Where to find more info.** https://ollama.com and https://github.com/ollama/ollama

---

## Training infrastructure

### AdamW

**Description.** Standard LLM training optimizer (Loshchilov & Hutter, 2017). Adam with decoupled weight decay — weight decay is applied separately from the gradient-based update rather than mixed into the gradient as in the original Adam-with-L2-regularization formulation. The decoupling matters for transformers; original Adam-L2 produced systematically worse results. AdamW is the default optimizer for nearly all modern LLM training and fine-tuning workflows.

**Use cases.** SFT, LoRA training, RLHF policy optimization, virtually all gradient-based LLM training. Typical learning rates: 1e-4 to 1e-5 for LoRA, 5e-6 to 2e-5 for full fine-tuning.

**Best used for.** Default choice for almost any fine-tuning workflow. Use 8-bit AdamW (`bitsandbytes`) or paged AdamW for memory-constrained settings.

**Where to find more info.** https://arxiv.org/abs/1711.05101 (Loshchilov & Hutter, 2017)

---

### Gradient accumulation

**Description.** Technique to simulate larger batch sizes when GPU memory is limited. Process N micro-batches, accumulate gradients across them, then apply one optimizer update. Mathematically equivalent (under most conditions) to a single batch of size `N · micro_batch_size`. The trade-off is wall-clock time: accumulating over N micro-batches takes N times as long as one large batch would, if memory allowed.

**Use cases.** Universal in fine-tuning workflows where the desired effective batch size exceeds GPU memory limits. Especially common in LoRA training of large models.

**Best used for.** Reaching the effective batch size your training recipe calls for when single-batch memory is the bottleneck. Always tune in pairs with micro-batch size.

**Where to find more info.** https://huggingface.co/docs/transformers/perf_train_gpu_one#gradient-accumulation

---

### Gradient checkpointing

**Description.** Memory-saving technique trading compute for memory by recomputing activations during the backward pass instead of storing them. Reduces activation memory by ~√N where N is the depth of recomputation; doubles wall-clock time for the backward pass. Crucial for training large models on limited memory; MLX exposes this via `mx.checkpoint`, PyTorch via `torch.utils.checkpoint`.

**Use cases.** Training large models on limited memory, fine-tuning very long contexts where activation memory dominates, any setting where the model fits but activations don't.

**Best used for.** Almost universal in modern large-model fine-tuning. The compute overhead is generally acceptable given the memory savings.

**Where to find more info.** https://pytorch.org/docs/stable/checkpoint.html

---

### KL coefficient (β) / KL divergence

**Description.** In RLHF, DPO, and GRPO, the KL-divergence term penalizes the policy for moving too far from the reference (SFT-trained) model. The KL coefficient β (sometimes denoted `kl_coef`) controls the strength of this penalty. Higher β means more conservative policy updates — the policy stays closer to the reference model. Common values: 0.01–0.1. Too high stalls learning; too low causes the policy to drift far from the reference and lose general capabilities.

**Use cases.** Universal hyperparameter in RL-based fine-tuning. The single most-tuned RL fine-tuning knob after learning rate.

**Best used for.** Always tuned per task. Start with the recipe's recommended value; raise if the model loses general capabilities, lower if it doesn't move toward the reward.

**Where to find more info.** https://huggingface.co/blog/the_n_implementation_details_of_rlhf_with_ppo

---

### Reward model (RM)

**Description.** In RLHF, a separately trained model that scores responses for quality. Typically trained on pairwise preference data (response A vs response B, human picks one), producing a scalar score for any given response. Used by PPO during the policy-optimization stage. Replaced in DPO by the implicit reward derived directly from the preference data, eliminating the need for a separate RM.

**Use cases.** RLHF training, reward-based filtering of training data, evaluation of model outputs against learned quality preferences.

**Best used for.** Traditional RLHF pipelines. DPO and similar methods that absorb the reward model implicitly are now generally preferred.

**Where to find more info.** https://huggingface.co/docs/trl/en/reward_trainer

---

### Reference model

**Description.** In DPO/RLHF, the SFT-trained model used as the "anchor" against which the policy is compared via KL divergence. Held frozen during preference optimization. Identical to the policy at the start of preference training; the policy diverges over training while the reference stays fixed. Memory-expensive at scale because both policy and reference must be resident.

**Use cases.** Universal in RL-based fine-tuning. SimPO eliminates the reference-model requirement at training time.

**Best used for.** Standard component of DPO/RLHF/GRPO training. Use SimPO if memory pressure makes keeping the reference resident impractical.

**Where to find more info.** https://huggingface.co/docs/trl/en/dpo_trainer

---

### Rollout

**Description.** A complete generation from the policy under training. In GRPO, multiple rollouts per prompt are generated (typically 4–16) and compared against each other to estimate advantages. The rollout is the unit of RL fine-tuning — the policy generates rollouts, the reward function or grader scores them, and the policy is updated to favor higher-scoring rollouts.

**Use cases.** RL fine-tuning of LLMs (RLHF, GRPO, RFT). Always means "a sample from the policy" in this context.

**Best used for.** Conceptual unit in any RL fine-tuning workflow.

**Where to find more info.** https://arxiv.org/abs/2402.03300 (DeepSeekMath, GRPO)

---

### Advantage

**Description.** In policy-gradient RL, the difference between a rollout's reward and the expected reward. Positive advantages push the policy toward producing similar outputs; negative advantages push away. Estimating advantages well is the central technical problem in RL; PPO uses generalized advantage estimation (GAE) with a learned value function, while GRPO estimates advantages from group-relative reward differences (no value function needed).

**Use cases.** Universal in policy-gradient RL training.

**Best used for.** Conceptual unit of RL training. Different methods estimate advantages differently; the choice of estimator matters.

**Where to find more info.** https://arxiv.org/abs/1506.02438 (GAE, Schulman et al., 2015)

---

### Policy gradient

**Description.** The gradient of expected reward with respect to policy parameters. The fundamental object PPO, GRPO, and similar methods optimize. The policy gradient theorem gives an unbiased estimator: `∇θ E[R] = E[R · ∇θ log π(a|s)]`. In practice this estimator has high variance; methods like PPO and GRPO add variance-reduction tricks (clipping, advantages, KL penalties) that make policy-gradient training tractable.

**Use cases.** Foundational concept in RL fine-tuning of LLMs.

**Best used for.** Conceptual foundation. In practice you use PPO, GRPO, or similar; you rarely implement raw policy gradient.

**Where to find more info.** https://spinningup.openai.com/en/latest/algorithms/vpg.html

---

## Data and evaluation

### Preference pair

**Description.** In DPO/RLHF training data, a tuple `(prompt, preferred_response, rejected_response)`. The basic data unit for preference optimization. Preference pairs come from human ranking of model outputs, AI-generated rankings (typically by a stronger model), or rule-based comparison.

**Use cases.** DPO, RLHF reward-model training, ORPO, SimPO. The data format for any pairwise-preference fine-tuning.

**Best used for.** Standard data unit of preference learning. Volume needed depends on use case (thousands to tens of thousands typical).

**Where to find more info.** https://huggingface.co/docs/trl/en/dpo_trainer#expected-dataset-format

---

### HITL: Human-in-the-Loop

**Description.** Workflow pattern where humans provide labels, preferences, or evaluations during the training loop. HITL DPO uses human preference judgments to generate preference pairs. HITL RFT uses human-written graders. The "in-the-loop" framing distinguishes from offline-collected human data — HITL implies the human input shapes the next training iteration.

**Use cases.** Iterative alignment refinement, gathering high-quality preference data on edge cases, RL fine-tuning where the reward signal benefits from human judgment.

**Best used for.** High-stakes alignment work where data quality matters more than data volume. Less appropriate for bulk training data collection where AI-generated labels suffice.

**Where to find more info.** https://www.anthropic.com/research/constitutional-ai-harmlessness-from-ai-feedback (Bai et al., 2022)

---

### Synthetic data / synthetic preferences

**Description.** Training data generated by another model rather than collected from humans. Common cost-reduction strategy. Stronger teacher models (GPT-4, Claude, etc.) generate completions or preference rankings; the weaker student model trains on the synthesized data. Quality risks include teacher-model bias propagation, distribution shift, and reasoning shortcuts. The teacher's blind spots become the student's blind spots.

**Use cases.** SFT data generation at scale, preference-pair generation for DPO when human labelers are too expensive, distillation workflows.

**Best used for.** Bulk data generation when a strong teacher model is available and the task tolerates teacher-model quirks. Always combine with at least some human-evaluated quality checks.

**Where to find more info.** https://arxiv.org/abs/2306.02707 (Orca, Mukherjee et al., 2023)

---

### Distillation

**Description.** Training a smaller "student" model to imitate a larger "teacher" model's outputs. The student learns from teacher-generated completions (response distillation) or from teacher's output probability distributions (logit distillation). Often combined with SFT; can be combined with preference optimization (the teacher generates preference rankings).

**Use cases.** Producing fast, cheap models that approximate the behavior of larger ones. DeepSeek-R1's distilled variants (R1-Distill-Qwen-7B, etc.) are canonical examples.

**Best used for.** Production deployment where the teacher is too expensive but the student's capability ceiling is acceptable.

**Where to find more info.** https://arxiv.org/abs/1503.02531 (Hinton et al., 2015, original distillation paper)

---

### Top-k accuracy

**Description.** Evaluation metric: fraction of cases where the correct answer appears in the model's top-k predictions. Top-1 accuracy = answer appears first; top-5 = appears in top five. Common for grader-based evaluation and recommendation-style tasks.

**Use cases.** Evaluating RFT-fine-tuned models, ranking tasks, recommendation systems, retrieval evaluation.

**Best used for.** Tasks where the model produces a ranked list and the metric of interest is presence-in-top-k rather than exact match.

**Where to find more info.** https://en.wikipedia.org/wiki/Evaluation_measures_(information_retrieval)

---

### Calibration

**Description.** How well a model's confidence in its predictions matches actual accuracy. A well-calibrated model that says "I'm 80% confident" is right 80% of the time. RL fine-tuning often degrades calibration (the model becomes overconfident); SFT tends to preserve it better. Calibration is measured by expected calibration error (ECE) or by reliability diagrams.

**Use cases.** Production deployment in high-stakes domains (medical, legal, financial) where confidence claims matter, evaluation of post-RL models, deciding whether to trust model self-assessments.

**Best used for.** Evaluation criterion when downstream consumers need reliable confidence signals.

**Where to find more info.** https://arxiv.org/abs/1706.04599 (Guo et al., 2017, "On Calibration of Modern Neural Networks")

---

### Eval set

**Description.** Held-out evaluation dataset used to measure model quality without training contamination. Standard practice: split data into train/validation/test, with the test set never seen during training or hyperparameter tuning. Eval-set contamination (training data leaking into the eval set) is a persistent problem in LLM evaluation, especially for benchmarks that have been on the web long enough to appear in pretraining corpora.

**Use cases.** Universal in any fine-tuning workflow.

**Best used for.** Always set aside an eval set before training. Always.

**Where to find more info.** https://huggingface.co/docs/evaluate

---

## Model families and naming conventions

### Qwen / Qwen3

**Description.** Alibaba's open-weight LLM family. Qwen3-14B is a 14-billion-parameter variant; the "4-bit" suffix indicates 4-bit quantization. Qwen models have strong multilingual support, particularly for Chinese, and competitive performance on reasoning and code tasks. Released under the Tongyi Qianwen License (similar to Llama 2's license — permissive for most commercial use with some constraints on very large deployments).

**Use cases.** Multilingual LLM applications, open-weight alternatives to Llama for fine-tuning, models with strong code and math performance for their size.

**Best used for.** Multilingual settings (especially Chinese-English), or as a general open-weight alternative when Llama licensing is a concern.

**Where to find more info.** https://huggingface.co/Qwen and https://github.com/QwenLM

---

### Base / Instruct / Chat

**Description.** Common variants of an open-weight model. *Base* = pretrained-only, completes text without instruction-following capability. *Instruct* = SFT-aligned for instruction-following. *Chat* = SFT + preference-tuned for multi-turn conversation. The variants ship as separate model checkpoints; you fine-tune from the variant closest to your target use case.

**Use cases.** Choice of starting point for fine-tuning. Base for from-scratch instruction tuning, Instruct for further task-specific tuning, Chat for refining an already-aligned conversational model.

**Best used for.** Start fine-tuning from the variant closest to your target capability. Don't fine-tune Chat models if you'll re-do alignment from scratch.

**Where to find more info.** https://huggingface.co/blog/Llama2#why-i-need-llama-2

---

### Reasoning model (o-series, R1-style)

**Description.** Newer model family trained with extensive RL on verifiable rewards (math, code) producing extended chain-of-thought reasoning. OpenAI's o-series (o1, o3, o4-mini) and DeepSeek-R1 are the canonical examples. Distinguished from standard chat models by the volume of reasoning tokens produced before the final answer (often 1000–10000+ tokens of internal reasoning) and by training pipelines that use RL on programmatically-verifiable rewards rather than (or in addition to) human-preference RLHF.

**Use cases.** Math, code, scientific reasoning, multi-step planning, formal logic — any task where extended deliberation produces better answers than fast generation.

**Best used for.** Tasks where answer quality matters more than latency. Wasteful for simple-question-answering or short-form generation.

**Where to find more info.** https://openai.com/index/learning-to-reason-with-llms/ (o1) and https://arxiv.org/abs/2501.12948 (DeepSeek-R1)

---

## Project-specific terms (relevant to MDEMG context)

### UAITS: Universal AI Training Specification

**Description.** MDEMG-internal specification framework governing training data curation across paradigms (SFT, DPO, RAFT, curriculum). Project-specific term, not standard in the wider literature. Defined by `docs/specs/FRAMEWORK_GOVERNANCE.md` in the MDEMG repository.

**Use cases.** MDEMG-internal training data governance.

**Best used for.** N/A outside the MDEMG project.

**Where to find more info.** https://github.com/reh3376/mdemg

---

### M5 Max

**Description.** Apple Silicon hardware platform. The 128GB unified-memory variant is currently used for MLX-based local LoRA training in MDEMG. Subject to a Metal MTLResource cap (499K) that constrains MoE-on-MLX training and forced the MoE→dense pivot in the FT-LORA workstream. macOS 26 removed the user-space `iogpu.rsrc_limit` sysctl escape hatch, making the cap a hard architectural constraint.

**Use cases.** Local LLM training and inference on Apple Silicon at scale (up to ~30B-parameter models with QLoRA).

**Best used for.** Single-developer LLM fine-tuning workflows where CUDA hardware is not available. Constrained by the Metal MTLResource cap for MoE architectures.

**Where to find more info.** https://www.apple.com/shop/buy-mac and the MLX documentation linked above.

---

### RAFT: Retrieval-Augmented Fine-Tuning

**Description.** Training paradigm where the model is fine-tuned on retrieval-augmented prompts so it learns to use retrieved context effectively. The training data includes (query, retrieved-context, correct-answer) triples; the model learns to attend to the retrieved context and produce grounded answers. One of the four UAITS paradigms in MDEMG. Distinct from inference-time RAG (which adds retrieved context to prompts at runtime without training).

**Use cases.** Domain-specific knowledge integration, grounded question answering, fine-tuning models to reason over their own retrieval results, reducing hallucination on knowledge-intensive tasks.

**Best used for.** Production RAG systems where the retrieval layer is stable and the model needs to be tuned to use it well. Not a substitute for high-quality retrieval — RAFT improves how the model uses retrieved context, not what gets retrieved.

**Where to find more info.** https://arxiv.org/abs/2403.10131 (Zhang et al., 2024, "RAFT: Adapting Language Model to Domain Specific RAG")

---

## What this glossary deliberately omits

- Vision-language model specifics (CLIP, SigLIP, multimodal projectors).
- Tokenizer-level vocabulary (BPE, SentencePiece, Tiktoken).
- Pretraining-specific concepts (the focus here is fine-tuning).
- Most evaluation benchmarks (MMLU, GSM8K, HumanEval, etc.) — these are domain-specific.
- Most architecture variants (RoPE, FlashAttention, GQA, RMSNorm) — these are foundation-model concerns more than fine-tuning concerns.

If any of these would be useful, they are a separate document.
