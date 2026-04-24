"""Phase 10 automated benchmark framework.

Reward-signal factory for GRPO/DPO (Phase 11) unblock. See
docs/development/ft-lora/sprint_plan_ft_lora_phase10.md for the sprint-level
contract.

Modules:
    preflight       — ULTS spec completeness assertion
    carve_golden    — deterministic validation holdout carve
    sampling_policy — T/C/J group → MLX kwargs resolver
    llm_judge       — gpt-5.4-mini judge (temp=0, seed=run_idx) for subjective metrics
    variance        — per-task reward mean/stddev aggregator (Phase 11 advantage norm)
    run_benchmark   — the runner itself
    _ids            — CUIDv2 wrapper (MEMORY: never UUID)
"""
