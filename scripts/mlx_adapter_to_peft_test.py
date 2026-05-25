#!/usr/bin/env python3
"""Tier 1 unit tests for scripts/mlx_adapter_to_peft.py."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import mlx_adapter_to_peft as m2p  # noqa: E402


class KeyTranslationTest(unittest.TestCase):
    def test_self_attn_q_proj_lora_a(self):
        out = m2p.mlx_to_peft_key("model.layers.0.self_attn.q_proj.lora_a")
        self.assertEqual(out, "base_model.model.model.layers.0.self_attn.q_proj.lora_A.weight")

    def test_self_attn_q_proj_lora_b(self):
        out = m2p.mlx_to_peft_key("model.layers.0.self_attn.q_proj.lora_b")
        self.assertEqual(out, "base_model.model.model.layers.0.self_attn.q_proj.lora_B.weight")

    def test_all_7_target_modules_per_layer(self):
        target_modules = [
            "self_attn.q_proj", "self_attn.k_proj", "self_attn.v_proj",
            "self_attn.o_proj", "mlp.gate_proj", "mlp.up_proj", "mlp.down_proj",
        ]
        for mod in target_modules:
            for direction in ("lora_a", "lora_b"):
                mlx_key = f"model.layers.5.{mod}.{direction}"
                peft_key = m2p.mlx_to_peft_key(mlx_key)
                self.assertIn(f".{mod}.", peft_key)
                expected_dir = "lora_A" if direction == "lora_a" else "lora_B"
                self.assertTrue(peft_key.endswith(f".{expected_dir}.weight"))

    def test_layer_index_preserved(self):
        for idx in (0, 1, 13, 39, 40):
            mlx_key = f"model.layers.{idx}.self_attn.v_proj.lora_a"
            peft_key = m2p.mlx_to_peft_key(mlx_key)
            self.assertIn(f"layers.{idx}.", peft_key)

    def test_invalid_key_raises(self):
        with self.assertRaises(ValueError):
            m2p.mlx_to_peft_key("not.an.mlx.key")
        with self.assertRaises(ValueError):
            m2p.mlx_to_peft_key("model.layers.0.self_attn.q_proj.weight")
        with self.assertRaises(ValueError):
            m2p.mlx_to_peft_key("")


class TensorTransposeTest(unittest.TestCase):
    def setUp(self):
        try:
            import torch  # noqa: F401
            self.torch_available = True
        except ImportError:
            self.torch_available = False

    def test_lora_a_shape_inversion(self):
        if not self.torch_available:
            self.skipTest("torch not installed")
        import torch
        mlx_a = torch.randn(5120, 32)
        peft_a = m2p.transpose_lora_tensor(mlx_a, "a")
        self.assertEqual(peft_a.shape, (32, 5120))

    def test_lora_b_shape_inversion(self):
        if not self.torch_available:
            self.skipTest("torch not installed")
        import torch
        mlx_b = torch.randn(32, 5120)
        peft_b = m2p.transpose_lora_tensor(mlx_b, "b")
        self.assertEqual(peft_b.shape, (5120, 32))

    def test_transpose_value_correctness(self):
        if not self.torch_available:
            self.skipTest("torch not installed")
        import torch
        original = torch.randn(5120, 32)
        transposed = m2p.transpose_lora_tensor(original, "a")
        self.assertTrue(torch.allclose(original[:, 0], transposed[0, :]))

    def test_non_2d_tensor_raises(self):
        if not self.torch_available:
            self.skipTest("torch not installed")
        import torch
        with self.assertRaises(ValueError):
            m2p.transpose_lora_tensor(torch.randn(5120), "a")
        with self.assertRaises(ValueError):
            m2p.transpose_lora_tensor(torch.randn(5120, 32, 4), "a")


class AdapterConfigSchemaTest(unittest.TestCase):
    def test_minimum_required_fields(self):
        mlx_cfg = {"lora_parameters": {"rank": 32, "alpha": 64, "dropout": 0.0, "keys": [
            "self_attn.q_proj", "self_attn.k_proj", "self_attn.v_proj",
            "self_attn.o_proj", "mlp.gate_proj", "mlp.up_proj", "mlp.down_proj",
        ]}}
        cfg = m2p.build_peft_adapter_config(mlx_cfg, "mlx-community/Qwen3-14B-4bit")
        self.assertEqual(cfg["peft_type"], "LORA")
        self.assertEqual(cfg["task_type"], "CAUSAL_LM")
        self.assertEqual(cfg["base_model_name_or_path"], "mlx-community/Qwen3-14B-4bit")
        self.assertEqual(cfg["r"], 32)
        self.assertEqual(cfg["lora_alpha"], 64)
        self.assertEqual(cfg["lora_dropout"], 0.0)
        self.assertFalse(cfg["fan_in_fan_out"])
        self.assertTrue(cfg["init_lora_weights"])

    def test_target_modules_leaves_only(self):
        mlx_cfg = {"lora_parameters": {"rank": 32, "alpha": 64, "keys": [
            "self_attn.q_proj", "self_attn.k_proj", "mlp.down_proj",
        ]}}
        cfg = m2p.build_peft_adapter_config(mlx_cfg, "x")
        for mod in cfg["target_modules"]:
            self.assertNotIn(".", mod, f"target_module {mod!r} contains a dot")

    def test_target_modules_dedup(self):
        mlx_cfg = {"lora_parameters": {"rank": 32, "alpha": 64, "keys": [
            "self_attn.q_proj", "other.q_proj", "mlp.down_proj",
        ]}}
        cfg = m2p.build_peft_adapter_config(mlx_cfg, "x")
        self.assertEqual(cfg["target_modules"].count("q_proj"), 1)

    def test_defaults_when_lora_parameters_missing(self):
        cfg = m2p.build_peft_adapter_config({}, "x")
        self.assertEqual(cfg["r"], 32)
        self.assertEqual(cfg["lora_alpha"], 64)
        self.assertEqual(cfg["target_modules"], [])

    def test_base_model_propagates(self):
        cfg = m2p.build_peft_adapter_config({"lora_parameters": {}}, "my/custom-base")
        self.assertEqual(cfg["base_model_name_or_path"], "my/custom-base")


def main() -> int:
    loader = unittest.TestLoader()
    suite = unittest.TestSuite()
    for cls in (KeyTranslationTest, TensorTransposeTest, AdapterConfigSchemaTest):
        suite.addTests(loader.loadTestsFromTestCase(cls))
    runner = unittest.TextTestRunner(verbosity=2)
    result = runner.run(suite)
    return 0 if result.wasSuccessful() else 1


if __name__ == "__main__":
    sys.exit(main())
