"""Sprint FT-LORA-PHASE11 — automated RL post-training.

This package implements the GRPO trainer, reward sampler, advantage estimator,
and regression harness that consume Phase 10 benchmark output (reward_vector +
per-task stddev) to produce an RL-refined LoRA adapter on top of the Phase 5
dense Qwen3-14B SFT model.

Modules:
  preflight       — Epic 0: gate assertions before training.
  grpo_loss       — Epic 1: clipped surrogate + KL penalty.
  advantage       — Epic 1: advantage estimator with 3 zero-stddev policies.
  reward_sampler  — Epic 1: TSDB reader for benchmark_results.
  trainer         — Epic 2: GRPOTrainer orchestrating the RL step loop.
  regression      — Epic 5: dual regression gate (5a vs Phase 5, 5b vs fresh).

Out of scope (deferred to Phase 12+): DPO training loop, nightly RL scheduler,
`mdemg finetune rl` CLI, distillation.
"""
