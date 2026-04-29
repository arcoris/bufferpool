# PoolGroup Readiness Gate

This document defines the implementation gate for starting PoolGroup work. It
does not implement PoolGroup, GroupCoordinator, policy mutation, background
control loops, physical trim, or adaptive budget redistribution.

## Current Stable Layers

Pool is the data-plane retained-storage owner. It owns retained buffers,
size-aware reuse, admission outcomes, retained-byte accounting, and the
`Get`/`Put` hot path.

LeaseRegistry is the checked-out ownership layer. It owns lease acquisition,
release, invalid release, double-release, ownership-violation, and active-lease
accounting.

PoolPartition is the partition-local ownership and control scope. It already
has partition samples, windows, rates, EWMA state, score evaluation, selected
sampling boundaries, active and dirty registry scaffolding, snapshots, metrics,
and controller evaluation projections.

`internal/control` is the shared pure algorithm layer. It owns generic numeric
helpers, counter and gauge movement, rates, smoothing, scoring, budget math,
pressure helpers, activity, risk, stability, ranking, and generic decisions. It
does not import the root package and does not know PoolPartition or future
PoolGroup domain types.

## Required Pre-Group Fixes

PoolGroup work may start only after these partition and control boundaries are
complete:

- Partition rates use saturated sums for fallback denominators and aggregate
  attempts, so extreme counter windows cannot wrap.
- `RetainRatio` and `DropRatio` denominator semantics are explicit: valid
  `Put` attempts are preferred when available, with retain+drop outcome totals
  used only as fallback.
- Partition rates expose real lease throughput fields based on LeaseRegistry
  counters, not pool get/put volume.
- Disabled pressure contributes zero severity even when a defensive snapshot
  carries a non-normal level.
- PoolPartitionScoreEvaluator owns usefulness, waste, activity, and risk
  scoring so partition scoring has one reusable domain adapter.
- Control coefficients have been audited, named, documented, and covered by
  tests for default, custom, zero, and non-finite behavior where applicable.
- Rank strategy has been benchmarked across small, medium, and large candidate
  sets. Full sorting remains stable and allocation-free for small and medium
  lists; top-k paths keep only the actionable subset and preserve `Into`
  zero-allocation behavior.

## Allowed First Group Scope

The first PoolGroup implementation is allowed to be observational only. It may
introduce:

- a partition registry;
- a group sample;
- a group window;
- group rates;
- group score values;
- a group snapshot;
- group metrics;
- a coordinator evaluation report.

The first group pass may aggregate partition-level observations and run pure
evaluation. It may not apply runtime decisions.

## Forbidden First Group Scope

The first PoolGroup implementation must not include:

- automatic policy mutation;
- background goroutines, timers, tickers, or controller loops;
- physical trim execution;
- adaptive budget redistribution that applies runtime policy;
- Pool hot-path changes;
- Pool awareness of PoolGroup or GroupCoordinator;
- LeaseRegistry ownership transfer to PoolGroup;
- root package imports from `internal/control`.

## Initial Group File Structure

Use a narrow file split so ownership, sampling, evaluation, and future
coordination boundaries remain reviewable:

- `group.go`
- `group_config.go`
- `group_policy.go`
- `group_lifecycle.go`
- `group_registry.go`
- `group_sample.go`
- `group_window.go`
- `group_rate.go`
- `group_score.go`
- `group_score_evaluator.go`
- `group_snapshot.go`
- `group_metrics.go`
- `group_coordinator.go`
- `group_controller_report.go`

The first version of `group_coordinator.go` should define evaluation and report
shape only. It must not start a background loop or mutate partition policies.

## Verification Gate

Run this gate before starting PoolGroup work:

```sh
gofmt -w .
go test ./...
go test -race ./...
go test ./internal/control/...
go test -race ./internal/control/...
go test -run '^$' -bench 'BenchmarkPool(GetHit|PutRetain|Metrics|SampleCounters)' -benchmem ./...
go test -run '^$' -bench 'BenchmarkPoolPartition(SampleInto|TickInto|ScoreEvaluator)' -benchmem ./...
git diff --check
git diff --staged --check
```

Expected results:

- all tests pass;
- Pool hot-path benchmarks remain `0 B/op`, `0 allocs/op`;
- `SampleInto`, `TickInto`, and `ScoreValues` controller paths remain
  allocation-conscious;
- static import-boundary tests pass;
- no staged or unstaged whitespace errors exist.

## GO / NO-GO Rule

Status is **GO for observational PoolGroup only** when every pre-Group fix in
this document is complete and the verification gate passes.

Status is **NO-GO** for automatic policy mutation, background coordination,
physical trim, and adaptive budget application until a later design explicitly
adds safety, hysteresis, cooldown, rollback, and benchmark gates for those
behaviors.
