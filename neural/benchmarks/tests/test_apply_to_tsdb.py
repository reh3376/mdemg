"""BENCH-SIDECAR-APPLY-001 — unit tests for persist.apply_to_tsdb.

Uses the _connect test seam (no real DB, no psycopg import needed on the
happy path)."""
import pytest

from neural.benchmarks.persist import apply_to_tsdb

REPORT = {
    "run_id": "testcuid0000000000000000",
    "started_at": "2026-07-21T00:00:00Z",
    "completed_at": "2026-07-21T00:01:00Z",
    "model_path": "test-model",
    "config_sha": "cafe",
    "n_runs_per_task": 1,
    "specs_total": 1,
    "specs_with_matched_rows": 1,
    "aggregate_weighted_score": 0.9,
    "stagnation_flag": False,
    "benchmark_results_rows": [
        {"task_id": "t", "run_idx": 0, "sampling_group": "T",
         "response_text": "x", "reward_vector": {"json_valid": 1.0},
         "judge_scores": {}, "judge_meta": {}, "latency_ms": 1},
    ],
}


class FakeCursor:
    def __init__(self, sink):
        self.sink = sink
    def execute(self, stmt):
        self.sink.append(stmt)
    def __enter__(self):
        return self
    def __exit__(self, *a):
        return False


class FakeConn:
    def __init__(self):
        self.stmts = []
        self.committed = False
        self.closed = False
    def cursor(self):
        return FakeCursor(self.stmts)
    def commit(self):
        self.committed = True
    def close(self):
        self.closed = True


def test_apply_executes_all_statements_and_commits():
    conn = FakeConn()
    n = apply_to_tsdb(REPORT, dsn="ignored", _connect=lambda dsn, **kw: conn)
    assert n == len(conn.stmts) == 2  # 1 runs INSERT + 1 results INSERT
    assert conn.stmts[0].startswith("INSERT INTO benchmark_runs")
    assert conn.stmts[1].startswith("INSERT INTO benchmark_results")
    assert conn.committed and conn.closed


def test_apply_connect_failure_raises_and_caller_decides():
    def boom(dsn, **kw):
        raise ConnectionError("no tsdb")
    with pytest.raises(ConnectionError):
        apply_to_tsdb(REPORT, dsn="ignored", _connect=boom)


def test_apply_execute_failure_still_closes_conn():
    class ExplodingConn(FakeConn):
        def cursor(self):
            raise RuntimeError("mid-tx failure")
    conn = ExplodingConn()
    with pytest.raises(RuntimeError):
        apply_to_tsdb(REPORT, dsn="ignored", _connect=lambda dsn, **kw: conn)
    assert conn.closed  # finally-close even on failure
