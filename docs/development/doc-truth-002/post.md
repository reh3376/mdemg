# DOC-TRUTH-002 — Sprint Post

Closed: 2026-06-11 · docs-only · executed from the operator prompt
(`PROMPT_doc_truth_002_moe_cleanup.md`) **with the five modifications
from the file:line-verified sub-agent review**:

1. Embedding-table fix applied to the GENERATIVE column only (the prompt
   misread the table; column 3 is Phase D's placeholder, untouched).
2. Scope extended to the two same-class files the prompt missed:
   `docs/tests/uaits/README.md` and a superseded banner on
   `docs/operations/vllm-mlx-setup.md` (banner-only; body preserved).
3. Within scope file 1, the vllm-mlx Serve references corrected to
   llama-server :8102 under the prompt's "fix consistently" clause.
4. Validation via the real validator: `mdemg data validate --spec
   docs/tests/uaits/specs/mdemg.uaits.json` (live, exit 0) — the
   prompt's cited checkers don't cover uaits.
5. This minimal 12-section plan + post (DOC-TRUTH-001 precedent).

Rejected prompt claims (disclosed): "EMBED-WIRE-001 wired an OpenAI
embedder by default" — false, the default is Ollama `qwen3-embedding:8b`.

Verification: Tier 2 grep sweep — the six as-current sites are gone;
historical references untouched (vllm-mlx-setup.md's body refs now sit
under the superseded banner). Tier 3 — live `mdemg data validate` exit 0;
UxTS drift checker green (uaits count unchanged).
