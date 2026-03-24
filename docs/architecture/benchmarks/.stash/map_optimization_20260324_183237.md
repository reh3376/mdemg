# Architecture Map Optimization Report

**Date**: 2026-03-24T22:32:37.680054Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 3  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 7.83 | 1 |
| 2 | 7.17 | 1 |
| 3 | 7.00 | 1 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| uxts_frameworks | UXTS | 236 | 84% | 7.0 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated purposes without missing any critical information. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly provided the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 8 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner is demoted and CI status is NONE but misses mentioning the missing inline grading and potential for false passes. |
| 4 | factual | 2 **WEAK** | Which framework has no runner at all? | The agent's answer is fundamentally incorrect and irrelevant, failing to identify UAMS (GAP-21) as the correct framework. |
| 5 | factual | 5 **WEAK** | What is the UETS framework for? | The agent mentions the UETS framework and emergence evaluation but omits the key concept of evaluating LLM emergence concept-naming quality and lacks detail on the 8 specs. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all four frameworks with accurate descriptions, matching the ground truth completely. |

## Map Evolution

### uxts_frameworks (Iteration 1)

Score before: 7.83/10  
Weak questions: 1  

```
UXTS|frameworks|v1  
UATS|HTTP contract testing|purpose:API endpoint validation|195 specs,224 variants,318 test cases|runner:active|CI:merge-blocking  
UPTS|parser conformance|purpose:parser output validation|27 specs|runner:active|CI:merge-blocking  
UDTS|gRPC contract testing|purpose:protobuf RPC validation|12 canonical,4 drafts|runner:active|CI:canonical-guard  
UBTS|benchmark regression|purpose:performance regression detection|3 specs,3 profiles|runner:active|CI:soft-fail  
USTS|security testing|purpose:security behavior validation|3 canonical,2 drafts|runner:active|CI:merge-blocking  
UAMS|auth methods|purpose:auth contract definition|4 specs|runner:NONE|CI:NONE(GAP-21)  
UOBS|observability runtime|purpose:runtime observability validation|3 canonical,1 draft|runner:active|CI:soft-fail  
UOTS|observability artifacts|purpose:static artifact validation|5 specs|runner:active|CI:soft-fail  
UVTS|semantic validation|purpose:API semantic quality check|1 canonical,1 draft|runner:demoted(GAP-04)|CI:NONE  
UNTS|hash verification|purpose:spec integrity registry|registry|runner:active|CI:merge-blocking  
UETS|emergence eval|purpose:evaluate LLM emergence concept-naming quality|8 specs|runner:active|CI:soft-fail
```

### uxts_frameworks (Iteration 2)

Score before: 7.17/10  
Weak questions: 1  

```
UXTS|frameworks|v1
UATS|HTTP contract|195 specs,224 variants,318 test cases|runner:active|runner.exists:yes|CI:merge-blocking
UPTS|parser conformance|27 specs|runner:active|runner.exists:yes|CI:merge-blocking
UDTS|gRPC contract|12 canonical,4 drafts|runner:active|runner.exists:yes|CI:canonical-guard
UBTS|benchmark regression|3 specs,3 profiles|runner:active|runner.exists:yes|CI:soft-fail
USTS|security|3 canonical,2 drafts|runner:active|runner.exists:yes|CI:merge-blocking
UAMS|auth methods|4 specs|runner:NONE|runner.exists:NO|CI:NONE(GAP-21)
UOBS|observability runtime|3 canonical,1 draft|runner:active|runner.exists:yes|CI:soft-fail
UOTS|observability artifacts|5 specs|runner:active|runner.exists:yes|CI:soft-fail
UVTS|semantic validation|1 canonical,1 draft|runner:demoted(GAP-04)|runner.exists:yes (demoted)|CI:NONE
UNTS|hash verification|registry|runner:active|runner.exists:yes|CI:merge-blocking
UETS|emergence eval|8 specs|runner:active|runner.exists:yes|CI:soft-fail
```

### uxts_frameworks (Iteration 3)

Score before: 7.0/10  
Weak questions: 2  

```
UXTS|frameworks|v1
UATS|HTTP contract|195 specs,224 variants,318 test cases|runner:active|CI:merge-blocking
UPTS|parser conformance|27 specs|runner:active|CI:merge-blocking
UDTS|gRPC contract|12 canonical,4 drafts|runner:active|CI:canonical-guard
UBTS|benchmark regression|3 specs,3 profiles|runner:active|CI:soft-fail
USTS|security|3 canonical,2 drafts|runner:active|CI:merge-blocking
UAMS|auth methods|4 specs (API key, JWT, SAML, no-auth)|runner:NONE (no runner implemented, spec-only, GAP-21)|CI:NONE
UOBS|observability runtime|3 canonical,1 draft|runner:active|CI:soft-fail
UOTS|observability artifacts|5 specs|runner:active|CI:soft-fail
UVTS|semantic validation|1 canonical,1 draft|runner:demoted (spec-only, runner exists but disabled, GAP-04)|CI:NONE
UNTS|hash verification|registry|runner:active|CI:merge-blocking
UETS|LLM emergence concept-naming eval|8 specs (evaluates LLM emergence concept-naming quality)|runner:active|CI:soft-fail
```

