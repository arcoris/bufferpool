<div align="center">

# System Overview

**Architecture overview for the adaptive byte-buffer memory-retention runtime.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Architecture](https://img.shields.io/badge/Architecture-index.md-1D4ED8?style=flat)](./index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-0F172A?style=flat)](../concepts/index.md)
[![Policies](https://img.shields.io/badge/Rules-Policies-7C2D12?style=flat)](../policies/index.md)
[![Workload](https://img.shields.io/badge/Model-Workload-B45309?style=flat)](../workload/index.md)

[Docs](../index.md) · [Overview](../overview.md) · [Goals and Non-goals](../goals-and-non-goals.md) · [Terminology](../terminology.md) · [Concepts](../concepts/index.md) · [Architecture](./index.md) · [Policies](../policies/index.md) · [Workload](../workload/index.md) · [Design](../design/index.md) · [Reference](../reference/index.md)

PoolGroup · PoolPartition · Pool · SizeClass · Shard · Bucket · Data path · Control path

**Start:** [Architecture scope](#architecture-scope) · [System model](#system-model) · [Runtime hierarchy](#runtime-hierarchy) · [Plane separation](#plane-separation) · [Responsibility map](#responsibility-map) · [Source of truth](#source-of-truth)

</div>

## Architecture scope

This document is the architecture entry point for the runtime system model of
`arcoris.dev/bufferpool`.

It explains the runtime hierarchy, primary component responsibilities, data-path
and control-path separation, memory-governance boundaries, policy publication
model, and failure-safety expectations.

This page does not define public API names, exact configuration fields,
implementation files, concrete algorithms, benchmark thresholds, or production
profiles. Those belong to reference, design, workload, performance, and
operations documentation.

Architecture documents should answer:

- which runtime component owns each responsibility;
- which boundaries must not be crossed;
- how control-plane work is separated from data-path work;
- how memory governance flows from the group to local retained storage;
- how policy updates and pressure change runtime behavior safely;
- which details are delegated to policies, workload, design, and reference.

## System model

`arcoris.dev/bufferpool` is a buffer-specific memory-retention runtime for
allocation-heavy Go systems.

The runtime retains reusable byte-buffer capacity when doing so can reduce
future allocation pressure. It must retain that capacity only within explicit
memory bounds, expose enough state for diagnosis, and adapt retention to recent
workload without making ordinary buffer operations depend on global control work.

The system model is:

```text
workload demand
→ size-aware buffer acquisition
→ local retained storage or allocation
→ checked-out buffer
→ buffer return
→ admission
→ retained capacity or drop
→ control-path observation
→ target redistribution
→ bounded trim and contraction
```

The central architecture invariant is:

> The data path must remain local and predictable; the control path may be
> adaptive and analytical, but it must not become a dependency for every ordinary
> buffer operation.

## Runtime hierarchy

The accepted runtime hierarchy is:

```text
PoolGroup
├── GroupCoordinator
└── PoolPartition
    ├── PartitionController
    └── Pool
        └── SizeClass
            └── Shard
                └── Bucket
```

| Component | Architecture role |
| --- | --- |
| `PoolGroup` | Top-level memory-governance and lifecycle domain. |
| `GroupCoordinator` | Group-level control component that coordinates aggregate budgets, pressure, generations, and partition-level intent. |
| `PoolPartition` | Control-plane ownership unit for a subset of pools. |
| `PartitionController` | Partition-local control component for observation, scoring, target refinement, policy publication, and trim scheduling. |
| `Pool` | Workload, component, tenant, or usage-domain isolation unit. |
| `SizeClass` | Capacity-normalization boundary for compatible requested sizes. |
| `Shard` | Data-path contention-reduction boundary inside one size class. |
| `Bucket` | Local retained-buffer storage for one shard and one size class. |

The hierarchy is intentionally layered. Upper layers constrain lower layers.
Lower layers must not redefine ownership of higher-level responsibilities.

## Plane separation

The runtime separates the data path from the control path.

### Data path

The data path serves ordinary buffer lifecycle operations.

It may perform:

- requested-size normalization;
- local pool/class/shard selection;
- local bucket access;
- local admission checks;
- cheap local counter updates;
- reads of published policy state;
- drop decision recording;
- ownership/accounting checks when the configured mode requires them.

The data path must not perform:

- group-level budget redistribution;
- EWMA or adaptive score computation;
- full pool/class/shard scans;
- runtime memory sampling;
- per-operation coordinator calls;
- per-operation channel event delivery;
- global trim;
- waits for controller freshness.

### Control path

The control path observes, interprets, and corrects.

It may perform:

- counter harvesting;
- window delta construction;
- workload scoring;
- pool/class/partition classification;
- target redistribution;
- pressure interpretation;
- policy snapshot publication;
- bounded trim;
- contraction after policy shrink;
- aggregate metrics and snapshot construction.

Controller delay may reduce adaptation quality. It must not break data-path
safety.

## Runtime responsibility map

| Responsibility | Owner | Notes |
| --- | --- | --- |
| Top-level memory-governance boundary | `PoolGroup` | Owns group budget, group lifecycle, pressure state, and partitions. |
| Aggregate budget coordination | `GroupCoordinator` | Coordinates partition targets and group generations; does not scan buckets directly on every cycle. |
| Pool ownership for control work | `PoolPartition` | A pool belongs to exactly one partition. |
| Partition-local adaptation | `PartitionController` | Computes local pool/class scores, local targets, shard credits, and trim candidates. |
| Workload isolation | `Pool` | Separates usage domains that need independent retention behavior. |
| Capacity compatibility | `SizeClass` | Groups compatible requested sizes and owns class-level retention context. |
| Hot-path contention reduction | `Shard` | Spreads local class access across bounded stripes. |
| Local retained storage | `Bucket` | Stores retained buffers using bounded, trim-safe local storage. |
| Admission enforcement | Data path using local policy state | Prevents unsuitable returned capacity from entering retained storage. |
| Trim correction | Partition-local control path | Removes retained memory that no longer fits targets or pressure state. |
| Public contracts | Reference documentation | Architecture does not freeze public API names or signatures. |

## Memory-governance flow

Memory governance flows from broad budgets to local credits.

```text
PoolGroup budget
→ Partition target
→ Pool target
→ SizeClass target
→ Shard credit
→ Bucket admission and retained storage
```

The conceptual budget flow is:

$$
B_{group}
\rightarrow
T_k
\rightarrow
T_p
\rightarrow
T_{p,c}
\rightarrow
C_{p,c,s}
$$

Where:

- $B_{group}$ is the group memory budget;
- $T_k$ is the target for partition $k$;
- $T_p$ is the target for pool $p$;
- $T_{p,c}$ is the target for class $c$ in pool $p$;
- $C_{p,c,s}$ is the local credit or target for shard $s`.

The exact formulas for scoring and budget allocation belong to workload and
policy documents. This architecture document defines ownership and direction.

## Policy publication model

The runtime should apply policy through published control state rather than
per-operation global decisions.

Conceptually:

```text
control path
→ computes targets and pressure view
→ publishes policy generation
→ lower layers observe local policy state
→ data path applies local admission and storage decisions
```

A data-path operation may use a valid older policy view. Snapshot freshness may
affect optimization quality, but data-path safety must not depend on perfect
controller punctuality.

Policy shrink is handled as contraction:

```text
validate new policy
→ publish lower targets
→ restrict new retention
→ schedule bounded trim
→ observe progress
```

Checked-out buffers must not be forcibly reclaimed.

## Data-path flow

A normal data-path interaction is conceptualized as:

```text
requested size
→ size class
→ pool-local class state
→ selected shard
→ local bucket reuse or allocation
→ checked-out buffer
```

For returned capacity:

```text
returned buffer
→ lifecycle and ownership/accounting checks
→ capacity and class-fit checks
→ local budget or credit checks
→ retain in bucket or drop with reason
```

The exact public operations are intentionally unspecified here. This document
defines architectural flow, not public API shape.

## Control-path flow

A normal control-path cycle is conceptualized as:

```text
harvest local counters
→ compute window deltas
→ update decayed workload state
→ classify pools/classes
→ refine targets and shard credits
→ publish partition-local policy state
→ schedule bounded trim
→ report aggregate state to group
```

At group level:

```text
receive partition reports
→ compute or constrain partition targets
→ coordinate pressure and contraction intent
→ publish group generation
```

The group should work with aggregate partition state. Detailed pool/class
scoring belongs to partitions.

## Component boundaries

### `PoolGroup`

`PoolGroup` owns the global memory-governance and lifecycle boundary.

It owns:

- group policy validation boundary;
- group budget;
- group pressure state;
- group lifecycle;
- group coordinator;
- pool partitions;
- aggregate runtime snapshots.

It does not own:

- per-class workload score state;
- shard-local bucket storage;
- per-buffer data-path decisions;
- process-wide memory management.

### `GroupCoordinator`

`GroupCoordinator` coordinates aggregate control.

It owns:

- group policy generation;
- partition target assignment;
- pressure propagation;
- contraction intent;
- aggregate partition report processing.

It does not own:

- direct bucket trim;
- class-level EWMA;
- ordinary data-path admission;
- per-buffer retention decisions.

### `PoolPartition`

`PoolPartition` owns control-plane state for assigned pools.

It owns:

- assigned pool registry;
- partition-local workload state;
- partition-local policy snapshots;
- partition-local trim candidates;
- partition-level controller state.

A pool belongs to exactly one partition at a time.

### `PartitionController`

`PartitionController` is the partition-local control worker.

It owns:

- local counter harvesting;
- window deltas;
- local scoring;
- pool/class target refinement;
- shard credit calculation;
- local trim scheduling;
- partition lag and snapshot-state reporting.

It should not be required for every data-path operation.

### `Pool`

`Pool` is a workload-domain isolation unit.

A pool should exist when a workload domain needs separate retention behavior,
budgeting, lifecycle, pressure behavior, security posture, or observability.

A pool should not be created for every request, goroutine, function, or size.

### `SizeClass`

`SizeClass` is the capacity compatibility boundary.

It owns:

- normalized capacity range;
- class-level retained-memory context;
- class-level counters and target view;
- class-scoped shard set.

A size class must not be used as a control-plane partition.

### `Shard`

`Shard` is a data-path contention boundary.

It owns:

- one local bucket for one size class;
- local retained counters;
- local credit view;
- local fast-path synchronization boundary.

A shard must not own pool-level workload policy or partition-level control.

### `Bucket`

`Bucket` is local retained-buffer storage.

It owns:

- retained buffer slots;
- local push/pop behavior;
- local trim primitives;
- slot clearing after pop or trim.

A bucket must not compute adaptive policy, pressure levels, or workload scores.

## Failure-safety model

The architecture must preserve safety when control work is delayed, stale, or
under pressure.

| Condition | Required behavior |
| --- | --- |
| Controller delayed | Data path continues using latest valid local policy state; lag is observable. |
| Snapshot stale | Optimization may degrade; admission still enforces visible policy. |
| Policy budget shrinks | New retention becomes more conservative; excess retained memory is trimmed in bounded work. |
| Memory pressure rises | Retention becomes more conservative through published pressure state and bounded trim. |
| Checked-out memory is high | Runtime reports in-use or reserved memory when accounting supports it; trim does not reclaim checked-out buffers. |
| Bucket contains cold retained capacity | Partition-local trim can remove it according to policy. |
| Shard is locally full | Returned capacity may be dropped instead of searching globally. |
| Partition is overloaded | Lag, active entries, and trim backlog should expose the condition. |
| Group closes | Lower layers stop accepting new retained memory according to lifecycle policy. |

## Architecture invariants

1. `PoolGroup` is the top-level memory-governance and lifecycle domain.
2. `PoolPartition` is the control-plane ownership unit.
3. Partitioning is by pools, not by size classes or shards.
4. `Shard` is a data-path contention boundary, not a control-plane partition.
5. `Bucket` is specialized retained-buffer storage, not a generic pool.
6. The data path must not depend on per-operation global coordination.
7. The control path must be bounded, observable, and lifecycle-owned.
8. Policy state is applied through published snapshots or equivalent local views.
9. Policy shrink causes contraction, not immediate global rebuild.
10. Checked-out buffers are not forcibly reclaimed.
11. Concepts define mental models; architecture defines responsibility boundaries.
12. Public API names are not defined by architecture documents.

## Out of scope

This document does not define:

- public method names;
- public option structs;
- concrete Go source layout;
- exact class table defaults;
- exact shard selection implementation;
- exact bucket segment layout;
- exact controller goroutine model;
- exact EWMA or scoring formulas;
- exact pressure thresholds;
- exact benchmark acceptance thresholds;
- production configuration profiles.

Use:

- [Concepts](../concepts/index.md) for mental models;
- [Policies](../policies/index.md) for behavioral rules;
- [Workload](../workload/index.md) for scoring and cadence details;
- [Design](../design/index.md) for component-level implementation design;
- [Operations](../operations/index.md) for production use and tuning;
- [Reference](../reference/index.md) for public contracts;
- [Performance](../performance/index.md) for benchmark evidence.

## Source of truth

| Topic | Owner |
| --- | --- |
| Project purpose and applicability | [Overview](../overview.md) |
| Scope and non-scope | [Goals and Non-goals](../goals-and-non-goals.md) |
| Vocabulary | [Terminology](../terminology.md) |
| Conceptual mental model | [Concepts](../concepts/index.md) |
| Runtime responsibility boundaries | This document and the architecture section |
| Behavioral policy rules | [Policies](../policies/index.md) |
| Workload observation, scoring, and convergence | [Workload](../workload/index.md) |
| Component-level implementation design | [Design](../design/index.md) |
| Public user-facing contracts | [Reference](../reference/index.md) |
| Operational tuning and diagnostics | [Operations](../operations/index.md) |
| Benchmark evidence and interpretation | [Performance](../performance/index.md) |

## Next reading

| Need | Continue with |
| --- | --- |
| Conceptual foundation before architecture | [Concepts](../concepts/index.md) |
| Exact vocabulary | [Terminology](../terminology.md) |
| Retention, admission, pressure, and trim behavior | [Policies](../policies/index.md) |
| Workload signals, scoring, and cadence | [Workload](../workload/index.md) |
| Component-level design details | [Design](../design/index.md) |
| Runtime contracts and metrics | [Reference](../reference/index.md) |
| Production sizing and troubleshooting | [Operations](../operations/index.md) |
| Evidence for performance claims | [Performance](../performance/index.md) |
