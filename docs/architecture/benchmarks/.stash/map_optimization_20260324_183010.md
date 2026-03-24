# Architecture Map Optimization Report

**Date**: 2026-03-24T22:30:10.826951Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 5  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 7.67 | 1 |
| 2 | 5.33 | 1 |
| 3 | 2.67 | 1 |
| 4 | 3.83 | 1 |
| 5 | 3.67 | 1 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| uxts_frameworks | UXTS | 166 | 89% | 3.7 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 5 **WEAK** | Which frameworks are merge-blocking in CI? | The agent correctly identified two merge-blocking frameworks but missed two others, resulting in an incomplete answer. |
| 2 | factual | 3 **WEAK** | How many UATS specs and variants exist? | The agent's numbers for specs and variants are incorrect and misses the test cases entirely, showing a fundamental misunderstanding. |
| 3 | relational | 7 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner as demoted due to grading issues but misses the specific GAP-04 designation and the implication of false passes from missing inline grading. |
| 4 | factual | 1 **WEAK** | Which framework has no runner at all? | The agent's answer is fundamentally wrong and unrelated to the correct framework "UAMS (GAP-21)". |
| 5 | factual | 1 **WEAK** | What is the UETS framework for? | The agent's answer is fundamentally incorrect and unrelated to the concept of evaluating LLM emergence concept-naming quality. |
| 6 | factual | 5 **WEAK** | Which frameworks use soft-fail CI gating? | The agent correctly identified two frameworks using soft-fail CI gating but missed two critical ones, resulting in an incomplete answer. |

## Map Evolution

### uxts_frameworks (Iteration 1)

Score before: 7.67/10  
Weak questions: 1  

```
UXTS|frameworks|v1  
UATS|HTTP contract|193 specs,378 variants|runner:active|CI:merge-blocking  
UPTS|parser conformance|27 specs|runner:active|CI:merge-blocking  
UDTS|gRPC contract|12 canonical,4 drafts|runner:active|CI:canonical-guard  
UBTS|benchmark regression|3 specs,3 profiles|runner:active|CI:soft-fail  
USTS|security|3 canonical,2 drafts|runner:active|CI:NONE(GAP-31)  
UAMS|auth methods|4 specs|runner:NONE|CI:NONE(GAP-21)  
UOBS|observability runtime|3 canonical,1 draft|runner:active|CI:NONE(GAP-31)  
UOTS|observability artifacts|5 specs|runner:active|CI:NONE(GAP-31)  
UVTS|semantic validation|1 canonical,1 draft|runner:demoted(GAP-04)|CI:NONE  
UNTS|hash verification|registry|runner:active|CI:partial(GAP-31)  
UETS|emergence concept-naming|8 specs|runner:active|CI:NONE(GAP-31)
```

### uxts_frameworks (Iteration 2)

Score before: 5.33/10  
Weak questions: 4  

```
UXTS|frameworks|v1  
UATS|HTTP|193c,378v|runner:active|CI:merge-block  
UPTS|parser|27|runner:active|CI:merge-block  
UDTS|gRPC|12c,4d|runner:active|CI:canonical-guard  
UBTS|bench|3,3p|runner:active|CI:soft-fail  
USTS|security|3c,2d|runner:active|CI:none  
UAMS|auth|4|runner:none|CI:none  
UOBS|obs-rt|3c,1d|runner:active|CI:none  
UOTS|obs-art|5|runner:active|CI:none  
UVTS|sem-val|1c,1d|runner:demoted|CI:none  
UNTS|hash|reg|runner:active|CI:partial  
UETS|emergence|8|runner:active|CI:none-soft-fail
```

### uxts_frameworks (Iteration 3)

Score before: 2.67/10  
Weak questions: 6  

```
UXTS|fw-reg|v1  
UATS|HTTP|193c,378v|runner:active|CI:merge-block  
UPTS|parser|27|runner:active|CI:merge-block  
UDTS|gRPC|12c,4d|runner:active|CI:canonical-guard  
UBTS|bench|3,3p|runner:active|CI:soft-fail  
USTS|sec|3c,2d|runner:active|CI:none  
UAMS|auth|4|runner:none|CI:none  
UOBS|obs-rt|3c,1d|runner:active|CI:none  
UOTS|obs-art|5|runner:active|CI:none  
UVTS|sem-val|1c,1d|runner:demoted|CI:none|demote:grading  
UNTS|hash|reg|runner:active|CI:partial  
UETS|emerg|8|runner:active|CI:soft-fail|eval:LLM-naming
```

### uxts_frameworks (Iteration 4)

Score before: 3.83/10  
Weak questions: 5  

```
UXTS|fw-reg|v2  
UATS|HTTP|193c,378v|runner:active|CI:merge-block  
UPTS|parser|27|runner:active|CI:merge-block  
UDTS|gRPC|12c,4d|runner:active|CI:canonical-guard  
UBTS|bench|3,3p|runner:active|CI:soft-fail  
USTS|sec|3c,2d|runner:active|CI:none  
UAMS|auth|4|runner:none|CI:none  
UOBS|obs-rt|3c,1d|runner:active|CI:none  
UOTS|obs-art|5|runner:active|CI:none  
UVTS|sem-val|1c,1d|runner:demoted|CI:none|demote:grading  
UNTS|hash|reg|runner:active|CI:partial  
UETS|emerg|8|runner:active|CI:soft-fail|eval:LLM-naming
```

### uxts_frameworks (Iteration 5)

Score before: 3.67/10  
Weak questions: 5  

```
UXTS|fw-reg|v3  
UATS|HTTP|193c,378v|runner:active|CI:merge-block|specs:193|vars:378  
UPTS|parser|27|runner:active|CI:merge-block|specs:27  
UDTS|gRPC|12c,4d|runner:active|CI:canon-guard|specs:12|drafts:4  
UBTS|bench|3,3p|runner:active|CI:soft-fail|specs:3|profiles:3  
USTS|sec|3c,2d|runner:active|CI:none|specs:3|drafts:2  
UAMS|auth|4|runner:none|CI:none|specs:4  
UOBS|obs-rt|3c,1d|runner:active|CI:none|specs:3|drafts:1  
UOTS|obs-art|5|runner:active|CI:none|specs:5  
UVTS|sem-val|1c,1d|runner:demoted|CI:none|specs:1|drafts:1|demote:grading  
UNTS|hash|reg|runner:active|CI:partial|covers:8fw  
UETS|emerg|8|runner:active|CI:soft-fail|specs:8|eval:LLM-naming
```

