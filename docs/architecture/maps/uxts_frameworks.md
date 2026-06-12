UXTS|frameworks|v2|unified test specification frameworks (source of truth: docs/development/UXTS_FRAMEWORK_MATRIX.md — regenerate this map from it)
#name|scope|specs|runner|CI (post UXTS-CI-001, 2026-06-11)
UATS|HTTP contract|214 canonical,7 drafts|runner:active|CI:merge-blocking
UPTS|parser conformance|28|runner:active|CI:merge-blocking
UDTS|gRPC contract|15 canonical,4 drafts|runner:active|CI:canonical-guard
UBTS|benchmark regression|3 specs,3 profiles|runner:active|CI:soft-fail
USTS|security|5 canonical,2 drafts|runner:active|CI:merge-blocking
UOBS|observability runtime|11 canonical,1 draft|runner:active|CI:soft-fail
UOTS|observability artifacts|11|runner:active|CI:merge-blocking
UNTS|hash verification|registry|runner:active|CI:merge-blocking
UETS|emergence eval:LLM concept-naming quality|8|runner:active|CI:soft-fail
UITS|iterative-improvement:T1 encoding comprehension|11|runner:active|CI:soft-fail
UVTS|semantic validation|3 canonical,1 draft|runner:active|CI:none (live-gated via make test-uvts-*; stub-embedder CI step deleted)
UAMS|auth methods|4|spec-only,no runner|CI:none
ULTS|LLM task contracts:prompts,schemas,quality,training config|17|runner:active|CI:merge-blocking (--verify-hashes prompt-drift gate)
UTDS|training-data export manifests,privacy gates|3|runner:active|CI:none
UAITS|training-data curation governance (SFT/DPO/RAFT/curriculum)|1 (4 datasets)|runner:active|CI:none
UBENCH|aggregate LLM eval (wraps Phase 10)|1 (108 rows/17 tasks)|runner:active|CI:merge-blocking (contract)
