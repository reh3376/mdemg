"""Early-Stop Subprocess Monitor for mlx_lm.lora (Sprint FT-LORA-E Epic 4).

Wraps a streaming stdout feed from `python -m mlx_lm.lora --train …` and
fires SIGTERM when the FT-OAI-001 overfitting-prevention policy triggers.

Policy (from `docs/development/ft-lora/00_README_v2.md` v5.x + memory
`feedback_epoch_cap`):
  * SFT mode: fire when `val_loss > best_val_loss × early_stop_ratio`
              for `patience` consecutive evals. Default ratio 1.05, patience 2.
  * RL  mode: fire when `val_reward < best_val_reward × early_stop_ratio`
              for `patience` consecutive evals. Default ratio 0.95, patience 2.
              (RL path wired but unvalidated in Sprint E — Phase 5 validates.)

mlx_lm 0.31.2 prints validation lines at `tuner/trainer.py:302-307`:

    Iter 100: Val loss 0.531, Val took 4.217s

Regex in this file is pinned to that format. A script-level version check
warns if `mlx_lm.__version__` drifts from 0.31.2 so a silently-broken regex
(no lines match → no fire → policy silently disabled) gets surfaced.

Checkpoint-behavior note (decision-doc territory for Sprint E):
  If mlx_lm.lora writes *current-state* adapters only (not best-so-far),
  early-stop on patience=2 saves a worse-than-best state. The Sprint E
  decision doc records the empirical behavior and the response:
    case (a) best+current both written → record best pointer in
             `<adapter-path>.earlystop.json`.
    case (b) current only              → patience=1 OR adapter snapshot
             rollback; default is snapshot rollback.
    case (c) other                      → case-by-case in decision doc.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Literal


# Pinned to mlx_lm 0.31.2 trainer.py:302-307 output format. If the upstream
# format changes, this regex no longer matches and the monitor silently
# never fires — update here when bumping mlx_lm.
SFT_VAL_LOSS_RE = re.compile(r"Iter\s+(\d+):\s+Val\s+loss\s+(\d+(?:\.\d+)?)")
RL_VAL_REWARD_RE = re.compile(r"Iter\s+(\d+):\s+Val\s+reward\s+(-?\d+(?:\.\d+)?)")


@dataclass
class EarlyStopMonitor:
    """Streaming stdout monitor that fires on overfit trigger.

    Usage:
        monitor = EarlyStopMonitor(mode="sft", ratio=1.05, patience=2)
        for line in subprocess.Popen(...).stdout:
            decoded = line.decode() if isinstance(line, bytes) else line
            print(decoded, end="")
            if monitor.on_line(decoded):
                proc.send_signal(signal.SIGTERM)
                break
    """

    mode: Literal["sft", "rl"] = "sft"
    ratio: float = 1.05
    patience: int = 2

    # Internal state
    best: float | None = field(default=None, init=False)
    _best_iter: int | None = field(default=None, init=False)
    consecutive_bad: int = field(default=0, init=False)
    history: list[tuple[int, float]] = field(default_factory=list, init=False)
    fired: bool = field(default=False, init=False)
    fired_reason: str | None = field(default=None, init=False)

    def __post_init__(self) -> None:
        if self.mode not in ("sft", "rl"):
            raise ValueError(f"mode must be 'sft' or 'rl', got {self.mode!r}")
        if self.patience < 1:
            raise ValueError(f"patience must be >= 1, got {self.patience}")
        if self.mode == "sft" and self.ratio <= 1.0:
            raise ValueError(
                f"SFT ratio must be > 1.0 (e.g. 1.05), got {self.ratio}"
            )
        if self.mode == "rl" and not (0.0 < self.ratio < 1.0):
            raise ValueError(
                f"RL ratio must be in (0, 1) (e.g. 0.95), got {self.ratio}"
            )

    def on_line(self, line: str) -> bool:
        """Parse one stdout line; return True iff SIGTERM should fire.

        Idempotent after firing — once fired, subsequent calls return True
        without mutating state (avoids double-fire if caller is slow to
        react to the first True).
        """
        if self.fired:
            return True

        m = self._match(line)
        if m is None:
            return False
        iter_num, value = m
        self.history.append((iter_num, value))

        # Establish baseline on first reading.
        if self.best is None:
            self.best = value
            self._best_iter = iter_num
            return False

        if self.mode == "sft":
            # SFT: fire when val_loss > best × ratio (worse = higher).
            threshold = self.best * self.ratio
            is_bad = value > threshold
            if value < self.best:
                self.best = value
                self._best_iter = iter_num
                self.consecutive_bad = 0
                return False
        else:
            # RL: fire when val_reward < best × ratio (worse = lower).
            threshold = self.best * self.ratio
            is_bad = value < threshold
            if value > self.best:
                self.best = value
                self._best_iter = iter_num
                self.consecutive_bad = 0
                return False

        if is_bad:
            self.consecutive_bad += 1
        else:
            self.consecutive_bad = 0

        if self.consecutive_bad >= self.patience:
            self.fired = True
            metric_name = "val_loss" if self.mode == "sft" else "val_reward"
            self.fired_reason = (
                f"{self.mode.upper()} early-stop fired at iter {iter_num}: "
                f"{metric_name}={value:.4f} crossed threshold "
                f"({self.best:.4f} × {self.ratio} = {threshold:.4f}) "
                f"for {self.consecutive_bad} consecutive evals (patience={self.patience}). "
                f"Best was iter {self._best_iter} at {self.best:.4f}."
            )
            return True
        return False

    def _match(self, line: str) -> tuple[int, float] | None:
        pat = SFT_VAL_LOSS_RE if self.mode == "sft" else RL_VAL_REWARD_RE
        m = pat.search(line)
        if not m:
            return None
        return int(m.group(1)), float(m.group(2))

    def sidecar_record(self) -> dict:
        """Snapshot the monitor's state for the .earlystop.json sidecar."""
        return {
            "mode": self.mode,
            "ratio": self.ratio,
            "patience": self.patience,
            "fired": self.fired,
            "fired_reason": self.fired_reason,
            "best_value": self.best,
            "best_iter": self._best_iter,
            "final_consecutive_bad": self.consecutive_bad,
            "history": [
                {"iter": it, "value": v} for it, v in self.history
            ],
        }
