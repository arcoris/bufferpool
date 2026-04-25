<div align="center">

# Hot Path and Cold Path

**Concept model for separating fast buffer operations from adaptive control work.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Partitions](https://img.shields.io/badge/Control-Pool%20Partitions-0F172A?style=flat)](./pool-partitions.md)
[![Adaptive](https://img.shields.io/badge/Adaptive-Retention-7C2D12?style=flat)](./adaptive-retention.md)
[![Architecture](https://img.shields.io/badge/Model-Architecture-B45309?style=flat)](../architecture/index.md)

[Docs](../index.md) · [Concepts](./index.md) · [Pool Partitions](./pool-partitions.md) · [Shards and Buckets](./shards-and-buckets.md) · [Adaptive Retention](./adaptive-retention.md) · [Architecture](../architecture/index.md)

Data path · Control path · Policy snapshots · Controller lag · Active registries · Bounded work

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Data path](#data-path) · [Control path](#control-path) · [Snapshots and lag](#snapshots-and-lag) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns the concept of separating hot data-path work from cold
control-path work.

It explains snapshot semantics, controller lag, coalesced ticks, and active
registries at the concept level. It does not define controller algorithms,
threading implementation, or public lifecycle APIs.

## Core idea

Fast buffer lifecycle operations must remain local and predictable.

Adaptive retention needs observation, scoring, budget movement, pressure
handling, snapshot publication, and trim. That work should happen on the
control path, not inside every ordinary buffer operation.

The separation is:

| Plane | Owns |
| --- | --- |
| Data path | Local buffer lifecycle operations, local admission checks, local retained storage access. |
| Control path | Workload observation, scoring, budgets, pressure interpretation, snapshots, and trim scheduling. |

## Data path

The data path should use:

- local size-class and shard state;
- local retained storage;
- local counters;
- published policy snapshots or derived local views;
- cheap admission checks;
- bounded fallback behavior.

The data path should avoid:

- full runtime scans;
- runtime memory sampling;
- per-operation global coordination;
- waiting for a controller tick;
- recomputing adaptive scores;
- directly moving budgets between partitions.

## Control path

The control path may:

- harvest counters;
- compute window deltas;
- update decayed workload signals;
- redistribute retention targets;
- publish policy snapshots;
- interpret pressure;
- schedule bounded trim work;
- report controller lag and snapshot age.

Adaptive scoring belongs to [Adaptive Retention](./adaptive-retention.md).
Partition ownership belongs to [Pool Partitions](./pool-partitions.md).

## Snapshots and lag

The data path reads published state. It should remain safe even when that state
is not perfectly fresh.

Controller lag may reduce adaptation quality, but it must not make ordinary
buffer operations unsafe. A delayed controller cycle should account for elapsed
time rather than pretending every missed interval ran exactly on schedule.

Coalesced ticks mean:

- elapsed time is considered;
- missed intervals are not replayed one by one as unbounded catch-up work;
- lag is observable;
- policy snapshots are replaced atomically or through an equivalent published
  state model.

## Active registries

An active registry is a control-path helper for prioritizing recently active
pools, classes, or shards.

Its purpose is to avoid full scans on every control cycle. Full scans may still
exist for maintenance or safety, but they should be bounded and not the normal
cadence path.

## Relationship to shards and buckets

Shards and buckets are data-path structures. They reduce contention and store
retained capacity locally.

They should not become control-plane partitions. That contrast is explained by
[Pool Partitions](./pool-partitions.md) and
[Shards and Buckets](./shards-and-buckets.md).

## Conceptual invariants

1. Data-path safety must not depend on control-path punctuality.
2. The data path uses published state; it does not compute global policy.
3. Controller lag must be observable.
4. Control work should be bounded per cycle.
5. Active scans are preferred over routine full scans.
6. Public lifecycle operation names are outside concept docs.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Ordinary operations must stay predictable | Keep global scans, memory sampling, and scoring off the data path. |
| Control work may lag | Publish observable snapshot age and controller lag. |
| Catch-up work must be bounded | Use elapsed-time semantics rather than unbounded replay of missed ticks. |
| Active workload is sparse | Prefer active registries over routine full scans. |

## Out of scope

This concept does not define goroutine structure, exact scheduler behavior,
snapshot data structures, controller algorithms, or public lifecycle APIs.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand partitioned control ownership | [Pool Partitions](./pool-partitions.md) |
| Understand local data-path storage | [Shards and Buckets](./shards-and-buckets.md) |
| Understand decayed scoring | [Adaptive Retention](./adaptive-retention.md) |
| Understand pressure propagation | [Memory Pressure](./memory-pressure.md) |
| Understand architecture details | [Architecture](../architecture/index.md) |
| Understand workload cadence details | [Workload](../workload/index.md) |

## Summary

The hot path serves ordinary buffer lifecycle operations with local state and
published policy. The cold path observes, scores, redistributes, publishes, and
trims. This separation keeps adaptive behavior from making every operation a
global coordination point.
