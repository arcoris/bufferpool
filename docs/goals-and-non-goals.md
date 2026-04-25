<div align="center">

# Goals and Non-goals

**Scope boundary for the adaptive byte-buffer memory-retention runtime.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](./index.md)
[![Overview](https://img.shields.io/badge/Overview-overview.md-1D4ED8?style=flat)](./overview.md)
[![Terminology](https://img.shields.io/badge/Terms-terminology.md-0F172A?style=flat)](./terminology.md)
[![Maturity](https://img.shields.io/badge/Readiness-Maturity%20Model-7C2D12?style=flat)](./maturity-model.md)
[![Roadmap](https://img.shields.io/badge/Plan-Roadmap-B45309?style=flat)](./roadmap.md)

[Docs](./index.md) · [Overview](./overview.md) · [Terminology](./terminology.md) · [Architecture](./architecture/index.md) · [Policies](./policies/index.md) · [Workload](./workload/index.md) · [Rationale](./rationale/index.md) · [Reference](./reference/index.md)

Buffer-specific scope · Bounded retention · Explicit trade-offs · API-neutral boundaries · Evidence-backed claims

**Start:** [Purpose](#purpose) · [Scope statement](#scope-statement) · [Goals](#goals) · [Non-goals](#non-goals) · [Accepted trade-offs](#accepted-trade-offs) · [Boundary rules](#boundary-rules)

</div>

## Purpose

This document owns the project scope boundary for `arcoris.dev/bufferpool`.

It defines what the project must provide, what it must not become, and how
proposals should be judged before they move into architecture, policy, design,
reference, or implementation work.

This document is intentionally API-neutral. It does not define concrete public
method names for buffer acquisition, buffer return, ownership handles, or
lifecycle operations.

## Scope statement

`arcoris.dev/bufferpool` is an adaptive byte-buffer memory-retention runtime for
allocation-heavy Go systems.

It is in scope to retain reusable `[]byte` capacity when doing so reduces
future allocation cost and the retained memory remains explicit, bounded,
observable, and responsive to workload changes.

It is out of scope to turn the project into a generic object pool, ordinary
cache, Go allocator replacement, process-wide memory manager, or hidden
background framework.

## Goals

### 1. Reuse byte buffers by capacity shape

The project must be size-aware. It should group compatible requested sizes so
small requests do not accidentally retain large backing capacity and large
classes must justify their retention cost.

The goal is useful reuse, not retention of every returned buffer.

### 2. Bound retained memory explicitly

Retained byte-buffer capacity must be limited by policy. Designs that allow
unbounded retained memory by default are outside scope.

Bounds may be expressed at multiple governance levels, but the top-level
requirement is simple: retention must remain intentional and explainable.

### 3. Treat buffer pooling as memory-retention governance

A retained buffer has no application-data value. It is reusable capacity.

Policy should therefore reason about capacity, workload, pressure, budget, and
recent usefulness rather than ordinary cache entry value.

### 4. Target measured allocation pressure

The project is for workloads where temporary byte-buffer allocation is visible
in profiles, memory behavior, garbage collection, throughput, or latency.

High request rate alone is not enough. The workload must have allocation cost
that retention can plausibly reduce.

### 5. Keep the data path narrow and predictable

Ordinary buffer lifecycle operations must not depend on per-operation global
coordination, fresh controller execution, runtime memory sampling, or full
runtime scans.

The data path may use local state and previously published policy state. The
control path owns observation and adaptation.

### 6. Separate data-plane and control-plane responsibilities

The data plane serves buffers and local retained storage. The control plane
observes workload, computes retention targets, publishes policy state, handles
pressure, and performs bounded trim work.

Controller delay may reduce adaptation quality, but it must not break ordinary
runtime safety.

### 7. Use pool-based partitions for control-plane scaling

Pool-based partitions are the baseline control-plane ownership model.

A partition owns a subset of pools and the adaptive state for those pools.
Partitioning by size class or shard is not the baseline because it splits a
workload domain across controllers.

### 8. Adapt to recent workload

Adaptive decisions must be driven by recent or decayed workload signals.
Lifetime counters may support observability, but old workload must not dominate
retention decisions forever.

The target behavior is controlled convergence under stable workload and
controlled re-adaptation after workload shifts.

### 9. Support explicit ownership/accounting semantics

The runtime must be able to account for retained memory and, in configured
modes, checked-out and reserved memory.

The scope requires explicit semantics, not a specific public handle type or
method shape.

### 10. Contract safely under pressure or lower budgets

When pressure rises or policy lowers targets, the runtime must restrict new
retention promptly and reduce existing retained memory through bounded trim
work.

Checked-out buffers must not be forcibly reclaimed.

### 11. Make behavior observable

Users should be able to explain retained memory, misses, drops, trim activity,
pressure state, controller lag, and snapshot freshness through documented
metrics or snapshots.

Observability is part of the product boundary, not an optional afterthought.

### 12. Require evidence for performance claims

Claims about performance, allocation reduction, overhead, or production
readiness must point to reproducible tests, benchmarks, reports, or operational
evidence.

## Non-goals

### 1. Replacing the Go allocator

The project does not provide a general allocator, arena system, manual memory
manager, or deterministic object lifetime model.

### 2. Managing process-wide memory policy

Applications remain responsible for process, container, and deployment memory
limits. The package may respond to pressure signals, but it must not secretly
own process-wide memory policy.

### 3. Providing generic object pooling

The project is specific to byte-buffer capacity retention. Generic object
pooling has different safety, lifecycle, and accounting constraints.

### 4. Using opportunistic runtime storage as deterministic retention

Opportunistic temporary reuse mechanisms may be useful as background context or
benchmark baselines, but deterministic retained-buffer storage must be bounded,
observable, and governed by project policy.

### 5. Retaining every returned buffer

Dropping a returned buffer is normal bounded-retention behavior. The runtime
must be able to reject returned capacity when policy, pressure, lifecycle, or
size rules make retention undesirable.

### 6. Guaranteeing zero allocations

Allocation can still occur during cold start, misses, workload shifts, pressure
contraction, unsupported sizes, or strict accounting modes. The project reduces
useful allocation pressure; it does not promise zero allocation behavior.

### 7. Implementing cache replacement as the core model

Per-buffer cache policies such as per-buffer LRU, LFU, or timestamp tracking are
not the baseline retention model. They optimize for semantic cache value, not
reusable byte-buffer capacity.

### 8. Scaling control work through unbounded goroutines

The project must not require a goroutine per pool, class, shard, or timer.
Background work must be bounded, explicit, measurable, and lifecycle-owned.

### 9. Perfectly balancing shards on every operation

Shards reduce contention. They are not global balancing units. Some local skew
is acceptable when avoiding expensive hot-path coordination.

### 10. Dynamically routing every operation across pools

Pool selection should remain explicit and stable for workload ownership.
Adaptive behavior should change retention policy, not turn every operation into
a routing decision.

### 11. Preallocating all possible capacity

Retained memory should arise from real workload and policy-governed returns.
Broad speculative preallocation is not the baseline model.

### 12. Defining public API shape in scope documents

Scope documents must not invent public operation names or lifecycle handle
shapes. Exact contracts belong to package and reference documentation.

## Accepted trade-offs

| Trade-off | Accepted position |
| --- | --- |
| Retained memory vs allocation pressure | Retain memory only when bounded, useful, and observable. |
| Adaptation vs stability | Prefer decayed recent signals and bounded target movement over aggressive reaction. |
| Fast path vs exact global accounting | Keep the baseline data path local; stricter accounting may be mode-dependent. |
| Local shard skew vs perfect balance | Accept bounded skew to avoid hot-path scans and coordination. |
| Explicit lifecycle vs hidden convenience | Background control work must be owned, stoppable, and diagnosable. |
| Early API stability vs behavioral learning | Final public shape should follow stable runtime behavior and evidence. |

## Boundary rules

1. The project remains buffer-specific.
2. Retained memory must be bounded by explicit policy.
3. Returned-buffer admission is mandatory.
4. Data-path safety must not depend on control-path freshness.
5. Pool-based partitions are the control-plane scaling baseline.
6. Shards are contention boundaries, not control-plane partitions.
7. Adaptive decisions must decay old workload history.
8. Pressure or policy shrink must contract retention without reclaiming checked-out buffers.
9. Expensive monitoring and full scans must stay off the ordinary data path.
10. Public API names belong to package and reference documents.
11. Performance and readiness claims require evidence.

## Decision guide

| Question | Expected decision |
| --- | --- |
| Does it improve bounded byte-buffer retention? | Consider in scope. |
| Does it generalize the project into object pooling? | Reject or require a scope-changing ADR. |
| Does it require unbounded retained memory? | Reject. |
| Does it put global coordination on every data-path operation? | Redesign. |
| Does it preserve pool-based control-plane ownership? | Continue analysis. |
| Does it define public API shape from a scope document? | Move to package/reference design. |
| Can users observe and benchmark the behavior? | Continue analysis if evidence can be supplied. |

## Source-of-truth boundary

| Topic | Owner |
| --- | --- |
| Purpose and applicability | [Overview](./overview.md) |
| Vocabulary | [Terminology](./terminology.md) |
| Scope and non-scope | This document |
| Runtime responsibility boundaries | [Architecture](./architecture/index.md) |
| Behavioral policy rules | [Policies](./policies/index.md) |
| Public contracts | [Reference](./reference/index.md) |
| Readiness claims | [Maturity Model](./maturity-model.md) |
| Capability sequencing | [Roadmap](./roadmap.md) |
