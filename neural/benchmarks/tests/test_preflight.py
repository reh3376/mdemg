"""Unit tests for preflight.

Covers _probe_mlx_group behavior and run_preflight's mlx_probes integration.
The on-disk spec check is exercised indirectly by the existing sampling/runner
suites; this file focuses on the new live-probe path.
"""
from __future__ import annotations

from unittest import mock

from neural.benchmarks import preflight


def test_probe_mlx_group_ok():
    resp = mock.MagicMock()
    resp.__enter__.return_value = resp
    resp.read.return_value = b'{"choices":[{"message":{"content":"ok"}}]}'
    with mock.patch("urllib.request.urlopen", return_value=resp):
        ok, detail = preflight._probe_mlx_group(
            "http://127.0.0.1:8101/v1", "m", {"temperature": 0.0}
        )
    assert ok is True
    assert detail == "ok"


def test_probe_mlx_group_http_error():
    import urllib.error

    err = urllib.error.HTTPError(
        "http://x", 400, "bad", {}, fp=mock.MagicMock(read=lambda: b"top_k bogus"),
    )
    with mock.patch("urllib.request.urlopen", side_effect=err):
        ok, detail = preflight._probe_mlx_group(
            "http://127.0.0.1:8101/v1", "m", {"top_k": -1}
        )
    assert ok is False
    assert "http 400" in detail


def test_probe_mlx_group_remote_disconnect():
    from http.client import RemoteDisconnected

    with mock.patch("urllib.request.urlopen", side_effect=RemoteDisconnected("closed")):
        ok, detail = preflight._probe_mlx_group(
            "http://127.0.0.1:8101/v1", "m", {"top_k": -1}
        )
    assert ok is False
    assert "RemoteDisconnected" in detail


def test_run_preflight_probe_mlx_failures_force_fail(tmp_path):
    """If any live probe returns not-ok, overall status must be 'fail'."""
    config_path = tmp_path / "cfg.yaml"
    config_path.write_text(
        """
ults:
  specs_dir: docs/tests/ults/specs
  allowed_sampling_groups: [T, C, J]
  required_spec_fields: [sampling_group]
output:
  preflight_report: /tmp/pf.json
sampling_groups:
  T: {weight: 0.5, temperature: 0.6, top_p: 0.95, top_k: 20}
  C: {weight: 0.35, temperature: 0.7, top_p: 0.8, top_k: 20}
  J: {weight: 0.15, temperature: 0.0, top_p: 1.0, top_k: -1, presence_penalty: 1.5}
"""
    )
    with mock.patch.object(preflight, "_probe_mlx_group",
                           side_effect=[(True, "ok"), (True, "ok"), (False, "boom")]):
        report = preflight.run_preflight(
            config_path, probe_mlx=True,
            mlx_base_url="http://127.0.0.1:8101/v1",
            mlx_model_name="m",
        )
    assert report["status"] == "fail"
    assert any(not p["ok"] for p in report["mlx_probes"].values())
