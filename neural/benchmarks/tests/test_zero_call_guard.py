"""EVAL-INTEGRITY-001: zero-call hard-fail guard.

A benchmark run that makes zero successful calls (or where no task
contributes a score) previously reported aggregate 0.0 — indistinguishable
from a genuine low score, the false-0.0 class behind FT-CLASSIFY-002's
four silent failures. The guard flags such runs as status=error.
"""
from neural.benchmarks.run_benchmark import _count_zero_call_specs


class _StubAgg:
    def __init__(self, ns):
        self._ns = ns  # task_name -> aggregate n

    def aggregate(self, task):
        if task not in self._ns:
            raise KeyError(task)
        class _A:
            pass
        a = _A()
        a.n = self._ns[task]
        return a


class _Spec:
    def __init__(self, name):
        self.task_name = name


def test_zero_call_specs_counted():
    specs = [_Spec("a"), _Spec("b"), _Spec("c"), _Spec("d")]
    per_spec = {
        "a": {"matched_golden_rows": 20},            # n>0 → ok
        "b": {"matched_golden_rows": 20},            # n==0 → zero-call
        "c": {"matched_golden_rows": 0},             # no rows → skip
        "d": {"matched_golden_rows": 20, "error": "boom"},  # errored → skip
    }
    agg = _StubAgg({"a": 18, "b": 0})  # b matched rows but scored nothing
    # b is the only zero-call spec (a scored, c had no rows, d errored)
    assert _count_zero_call_specs(specs, per_spec, agg) == 1


def test_no_zero_call_when_all_score():
    specs = [_Spec("a"), _Spec("b")]
    per_spec = {"a": {"matched_golden_rows": 20}, "b": {"matched_golden_rows": 20}}
    agg = _StubAgg({"a": 18, "b": 20})
    assert _count_zero_call_specs(specs, per_spec, agg) == 0
