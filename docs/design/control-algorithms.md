# Control Algorithms

This document defines the shared control-algorithm boundary used by
`bufferpool` before PoolGroup work. The goal is to keep workload analysis
reusable while preserving strict separation between data-plane ownership and
control-plane interpretation.

## Architecture

`Pool` is the data-plane retained-storage owner. It owns size-aware retained
buffers, admission outcomes, drops, and retained-byte accounting. Control
algorithms must not become part of `Pool.Get` or `Pool.Put`.

`LeaseRegistry` is the checked-out ownership layer. It records lease
acquisition, release, invalid release, double release, and ownership-violation
signals. Control algorithms may score those signals after sampling, but they do
not enforce ownership.

`PoolPartition` is the partition-local ownership and control scope. It adapts
partition samples, windows, rates, EWMA state, scores, and recommendations into
domain-specific values. It remains the root package boundary where generic
control outputs acquire bufferpool meaning.

`internal/control` is the shared pure algorithm layer. It computes scalar
numeric projections, counter and gauge movement, rates, smoothing, scores,
pressure, budget math, risk, ranking, and generic recommendations. It must not
import the root `bufferpool` package and must not know about `Pool`,
`LeaseRegistry`, `PoolPartition`, `PoolGroup`, policies, snapshots, or reports.

The root package interprets `internal/control` outputs. It maps partition
samples into generic algorithm inputs and maps generic algorithm outputs back
into partition diagnostics, recommendations, or future policy proposals.

Future `PoolGroup` will be an aggregate control scope over partitions. It may
reuse the same `internal/control` algorithms, but it must provide its own
domain adapter rather than moving group-specific meaning into `internal/control`.

## Signal Flow

The intended control signal chain is:

```text
Sample
-> Window
-> Rates
-> EWMA
-> Scores
-> Recommendations
-> future policy decisions
```

`Sample` captures observed counters and gauges from the domain. A sample may be
full-partition or selected-pool scoped, and scope must be explicit.

`Window` compares two samples. Monotonic counters produce bounded deltas.
Gauges remain gauges and must not be treated as cumulative counters.

`Rates` project window deltas into ratios and throughput. They use bounded
window movement rather than lifetime counters, so recent workload behavior can
be evaluated without letting old history dominate.

`EWMA` smooths noisy window rates or scores. It dampens one-window spikes while
still moving toward workload changes. EWMA is not a predictor, is not a policy
decision, and must not be driven directly from lifetime metrics.

`Scores` project domain-adapted rates and smoothed signals into normalized
usefulness, waste, activity, risk, budget, and pressure values. Scores explain
signals; they do not mutate policy.

`Recommendations` are non-mutating proposals. They describe what a controller
might consider, but they do not execute actions, publish runtime snapshots, or
trim memory.

Future policy decisions must sit above recommendations and add domain-specific
guards such as hysteresis, cooldown, safety checks, and bounded change.

## Counters And Gauges

Counters are monotonic event totals. Examples include hits, misses, gets, puts,
allocations, returns, drops, lease acquisitions, releases, invalid releases,
double releases, and ownership violations. A counter delta treats
`current < previous` as reset or reconstruction, not uint64 wraparound. This is
appropriate for controller windows because sampled sources may be rebuilt.

Gauges are current-state measurements. Examples include retained bytes, active
bytes, owned bytes, and active lease count. Gauges can increase or decrease, so
they must be modeled as directional movement or current pressure, not as
monotonic deltas.

Lifetime counters remain diagnostic. They can explain total process behavior,
but adaptive control must use bounded windows and derived rates so old workload
history does not dominate current decisions.

## Algorithm Roles

Usefulness measures whether retained memory is producing reuse value. Hit ratio
and allocation avoidance are strong signals because they show successful reuse
and reduced allocation pressure. Retain ratio and activity support the score,
while current-window drops can suppress usefulness when admission pressure is
immediate. Usefulness is not a memory-pressure score and does not imply that a
policy should grow automatically.

Waste measures retained capacity that appears cold or inefficient. Low hit
ratio, retained pressure, low activity, and drops can raise waste. Waste is not
a trim command and does not identify an exact number of bytes to release.

Activity and hotness measure workload intensity. Gets, puts, bytes, and lease
operations describe demand and ownership churn. Hotness is useful for active-set
and idle-candidate decisions, but it is not the same as usefulness or pressure.

Risk measures safety and caller-misuse signals. Ownership violations, double
releases, invalid releases, and return failures are kept separate so diagnostics
can explain the source of risk. Risk can suppress unsafe recommendations, but it
does not mutate ownership state.

Budget utilization measures current usage, headroom, deficit, and proportional
targets. Budget helpers are pure math. They do not apply policies and do not
perform redistribution by themselves.

Pressure severity maps generic pressure levels or thresholds into normalized
severity. Root adapters decide how domain pressure levels map into this generic
projection. Disabled pressure contributes zero severity.

Ranking orders candidates deterministically by score and tie-break. It is a
selection primitive, not a policy engine. Into helpers reuse caller-owned
storage for controller loops, while owning helpers allocate by design.

Recommendations are generic or domain-specific proposals. None and observe are
non-actionable. Grow, shrink, trim, or investigate recommendations still require
a future domain controller before any action can happen.

## Non-Goals

`internal/control` does not start background goroutines, timers, tickers, or
controller loops.

`internal/control` does not mutate runtime policy, publish runtime snapshots,
change retention budgets, execute trim, close Pools, or update ownership state.

Scoring does not physically trim retained buffers. Physical trim requires a
Pool-level retained-storage trim API that can report exact removed bytes and
buffers.

`internal/control` must not import the root `bufferpool` package. The
dependency direction is root package to `internal/control`, never the reverse.

The control layer is not an LRU/LFU cache policy and not a sync.Pool wrapper.
Buffers are reusable capacity, not semantic cached values.

## Performance Policy

`Pool.Get`, `Pool.Put`, Pool metrics, and Pool sample-counter hot paths must
remain `0 B/op` and `0 allocs/op` in their benchmark gates.

Diagnostic snapshots and explainable score reports may allocate by design
because they copy component details for observation.

`ScoreValues`, `Into`, prepared evaluator, and prepared scorer paths are for
controller loops. They should reuse caller-owned storage or stable configuration
where practical.

`Scores`, `Snapshot`, and owning top-k helpers are diagnostic or convenience
APIs. They may allocate when they need defensive copies.

Verification gates should keep import boundaries explicit:

- `internal/control` must not import the root `bufferpool` package.
- hot data-plane files must not import score, risk, activity, or decision
  packages.
- `internal/testutil` must only be imported from `_test.go` files.
