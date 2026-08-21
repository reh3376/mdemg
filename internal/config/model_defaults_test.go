package config

import "testing"

// HOMEBREW-INSTALLER-QWEN-UPDATE-002 Phase C pins.
// defaultRamTiersForModel returns the model-appropriate default JSON for
// MDEMG_MODEL_RAM_TIERS. v1 (14B) and v2 (27B) have different memory footprints.

func TestDefaultRamTiersForModel_V1(t *testing.T) {
	got := defaultRamTiersForModel("mdemg-llm-v1")
	want := `{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}`
	if got != want {
		t.Errorf("v1 default:\n  got  %s\n  want %s", got, want)
	}
}

func TestDefaultRamTiersForModel_V2(t *testing.T) {
	got := defaultRamTiersForModel("mdemg-llm-v2")
	want := `{"<32":"Q4_K_M","<48":"Q5_K_M","default":"Q8_0"}`
	if got != want {
		t.Errorf("v2 default (27B math):\n  got  %s\n  want %s", got, want)
	}
}

func TestDefaultRamTiersForModel_EmptyAndUnknownDefaultToV1(t *testing.T) {
	// Empty ModelName + unknown ModelName both fall through to the v1 default.
	// Backward-compat: pre-Phase-C the v1 default was the only hardcoded value;
	// unknown ModelName must not silently receive v2 tiers (which would
	// mis-route quant selection on unrelated models).
	cases := []string{"", "mdemg-llm-v99-custom", "some-other-model"}
	want := `{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}`
	for _, name := range cases {
		t.Run("name="+name, func(t *testing.T) {
			got := defaultRamTiersForModel(name)
			if got != want {
				t.Errorf("fallback default for %q:\n  got  %s\n  want %s", name, got, want)
			}
		})
	}
}

func TestDefaultRamTiersForModel_CaseInsensitiveV2(t *testing.T) {
	// Match the LoadQuantManifest dispatch predicate — case-insensitive on ModelName.
	cases := []string{"MDEMG-LLM-V2", "Mdemg-Llm-V2", "  mdemg-llm-v2  "}
	want := `{"<32":"Q4_K_M","<48":"Q5_K_M","default":"Q8_0"}`
	for _, name := range cases {
		t.Run("name="+name, func(t *testing.T) {
			got := defaultRamTiersForModel(name)
			if got != want {
				t.Errorf("case-insensitive v2 dispatch failed for %q:\n  got  %s\n  want %s", name, got, want)
			}
		})
	}
}
