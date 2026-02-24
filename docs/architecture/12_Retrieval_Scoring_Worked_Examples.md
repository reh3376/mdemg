# Retrieval Scoring — Worked Examples

This document gives **numeric, testable** examples of retrieval scoring that align with the project mantra:

- vector index = recall
- graph = reasoning
- runtime = activation physics
- DB writes = only learning deltas

The intent is to provide a set of “golden numbers” you can turn into regression tests.

---

## 1) Definitions

For each node *i* returned from vector recall:

- **Vector similarity**: `v_i ∈ [0,1]` (Neo4j returns `score` where larger is closer)
- **Activation**: `a_i ∈ [0,1]` computed in-memory via spreading activation
- **Recency**: `r_i ∈ [0,1]`, e.g. `r_i = exp(-ρ * age_days)`
- **Confidence**: `c_i ∈ [0,1]` from node property
- **Hub penalty**: `h_i ≥ 0`, e.g. `h_i = log(1 + degree_i)`
- **Redundancy penalty**: `d_i ≥ 0`, computed from near-duplicates (same abstraction parent, same path prefix, etc.)

Final score:

```
S_i = α*v_i + β*a_i + γ*r_i + δ*c_i - φ*h_i - κ*d_i
```

Suggested starting weights (tune later):

- `α=0.55` (vector)
- `β=0.30` (activation)
- `γ=0.10` (recency)
- `δ=0.05` (confidence)
- `φ=0.08` (hub penalty)
- `κ=0.12` (redundancy)

---

## 2) Toy graph

Nodes: A, B, C, D, E.

Vector recall (from query embedding):

- A: `v=0.90`
- B: `v=0.80`
- C: `v=0.40`
- D: `v=0.20`
- E: `v=0.10`

Edges (directed) with effective weights (after dimension mix + recency factor):

- A → C: `w=0.60`
- B → C: `w=0.30`
- C → D: `w=0.50`
- C → E: `w=0.20`
- D → E: `w=0.40`

Inhibitory edges (CONTRADICTS), treated as subtraction:

- B → D: `w_inhib=0.25`

Activation parameters:

- steps `T=2`
- step decay `λ=0.15`
- initial seeds: `a_i(0)=v_i` for the top-2 candidates (A,B), and 0 for others.

So:

- `a_A(0)=0.90`
- `a_B(0)=0.80`
- `a_C(0)=a_D(0)=a_E(0)=0`

---

## 3) Spreading activation math

Update rule (one of many valid variants):

```
a_j(t+1) = clamp( (1-λ)*a_j(t) + Σ_i a_i(t)*w(i→j) - Σ_k a_k(t)*w_inhib(k→j), 0, 1)
```

### Step 1 (t=0 → 1)

Compute C:

- incoming from A: `0.90 * 0.60 = 0.54`
- incoming from B: `0.80 * 0.30 = 0.24`
- base retention: `(1-0.15)*0 = 0`
So:
- `a_C(1) = clamp(0 + 0.54 + 0.24, 0, 1) = 0.78`

Compute D:

- incoming from C at t=0 is zero
- inhibitory from B: `0.80 * 0.25 = 0.20`
- retention: 0
So:
- `a_D(1) = clamp(0 - 0.20, 0, 1) = 0`

Compute E:

- no incoming at t=0 (C and D are 0)
So:
- `a_E(1)=0`

Also retain seeds (optional):

- `a_A(1) = clamp((1-0.15)*0.90,0,1) = 0.765`
- `a_B(1) = clamp((1-0.15)*0.80,0,1) = 0.680`

### Step 2 (t=1 → 2)

Compute D:

- incoming from C: `0.78 * 0.50 = 0.39`
- inhibitory from B: use `a_B(1)=0.68`: `0.68 * 0.25 = 0.17`
- retention: `(1-0.15)*0 = 0`
So:
- `a_D(2) = clamp(0 + 0.39 - 0.17, 0, 1) = 0.22`

Compute E:

- incoming from C: `0.78 * 0.20 = 0.156`
- incoming from D: `0 * 0.40 = 0`
- retention: 0
So:
- `a_E(2) = 0.156`

Final activations (use `a(2)`):

- A: 0.765
- B: 0.680
- C: 0.780
- D: 0.220
- E: 0.156

---

## 4) Final scoring with hub penalty

Assume:

- recency `r_i` all equal 0.9 (recent)
- confidence `c_i` all equal 0.7
- degrees: `deg(A)=10, deg(B)=2, deg(C)=25, deg(D)=3, deg(E)=1`
- redundancy penalty `d_i=0` for all (no duplicates in this toy)

Compute hub penalty `h_i = log(1+deg)` (natural log):

- `h_A = ln(11)=2.3979`
- `h_B = ln(3)=1.0986`
- `h_C = ln(26)=3.2581`
- `h_D = ln(4)=1.3863`
- `h_E = ln(2)=0.6931`

Now compute `S_i`:

A:

- `0.55*0.90 + 0.30*0.765 + 0.10*0.9 + 0.05*0.7 - 0.08*2.3979`
- `= 0.495 + 0.2295 + 0.09 + 0.035 - 0.1918`
- `= 0.6577`

B:

- `0.55*0.80 + 0.30*0.680 + 0.10*0.9 + 0.05*0.7 - 0.08*1.0986`
- `= 0.44 + 0.204 + 0.09 + 0.035 - 0.0879`
- `= 0.6811`

C:

- `0.55*0.40 + 0.30*0.780 + 0.10*0.9 + 0.05*0.7 - 0.08*3.2581`
- `= 0.22 + 0.234 + 0.09 + 0.035 - 0.2606`
- `= 0.3184`

D:

- `0.55*0.20 + 0.30*0.220 + 0.10*0.9 + 0.05*0.7 - 0.08*1.3863`
- `= 0.11 + 0.066 + 0.09 + 0.035 - 0.1109`
- `= 0.1901`

E:

- `0.55*0.10 + 0.30*0.156 + 0.10*0.9 + 0.05*0.7 - 0.08*0.6931`
- `= 0.055 + 0.0468 + 0.09 + 0.035 - 0.0554`
- `= 0.1714`

Ranking: **B > A >> C > D > E**.

Interpretation:

- C has strong activation but is punished as a hub. That’s intentional: hubs are often generic.
- B beats A because it’s “cleaner” (lower hub penalty) and still very relevant.

---

## 5) Learning deltas (bounded writeback)

After returning the top-K nodes, update only **local learning edges** (e.g., `CO_ACTIVATED_WITH`) among nodes that exceeded an activation threshold `a ≥ a_min`.

Example:

- top-K = {A,B,C}
- `a_min=0.20` so A,B,C all qualify

Hebbian update for each unordered pair (i,j):

```
Δw_ij = η * a_i * a_j - μ * w_ij
w_ij ← clamp(w_ij + Δw_ij, w_min, w_max)
```

Guardrails:

- Create at most `E_max` edges per request (cap on pairs).
- Cap per-edge absolute delta, e.g. `|Δw| ≤ 0.05`.
- Increment evidence count and timestamps.

This makes “emergence” a **result of repeated local reinforcement**, while preventing write amplification.
