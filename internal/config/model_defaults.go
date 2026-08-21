package config

import "strings"

// defaultRamTiersForModel returns the model-appropriate default JSON for
// MDEMG_MODEL_RAM_TIERS. Operator override via env still wins in FromEnv.
//
// v1 (Qwen3-14B) fits `Q4_K_M` (~10 GB RSS) on 16 GB machines and `Q8_0`
// (~16 GB RSS) on 32 GB+. v2 (Qwen3.8-27B) is roughly 2× larger — Q4_K_M
// (~18 GB RSS) needs 24 GB minimum, Q8_0 (~32 GB RSS) needs 48 GB
// recommended. See docs/features/local-model-distribution.md for the
// per-quant RAM math.
//
// HOMEBREW-INSTALLER-QWEN-UPDATE-002 Phase C (2026-08-20).
func defaultRamTiersForModel(modelName string) string {
	switch strings.ToLower(strings.TrimSpace(modelName)) {
	case "mdemg-llm-v2":
		return `{"<32":"Q4_K_M","<48":"Q5_K_M","default":"Q8_0"}`
	default:
		// v1 default (also for empty/unknown ModelName — matches the pre-Phase-C
		// hardcoded default so existing operators see byte-identical behavior).
		return `{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}`
	}
}
