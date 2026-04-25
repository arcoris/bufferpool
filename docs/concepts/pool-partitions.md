<div align="center">

# Pool Partitions

**Pool-based control-plane partitioning for scalable adaptive retention.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Groups](https://img.shields.io/badge/Group-Pool%20Groups-0F172A?style=flat)](./pool-groups.md)
[![Planes](https://img.shields.io/badge/Planes-Hot%20%26%20Cold-7C2D12?style=flat)](./hot-path-and-cold-path.md)
[![Buckets](https://img.shields.io/badge/Contrast-Shards%20%26%20Buckets-B45309?style=flat)](./shards-and-buckets.md)

[Docs](../index.md) · [Concepts](./index.md) · [Pool Groups](./pool-groups.md) · [Hot Path and Cold Path](./hot-path-and-cold-path.md) · [Adaptive Retention](./adaptive-retention.md) · [Shards and Buckets](./shards-and-buckets.md)

Pool assignment · Partition controller · Local adaptive state · Snapshot publication · Not shard partitioning

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Partition ownership](#partition-ownership) · [Controller scope](#controller-scope) · [Shards are different](#shards-are-different) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns pool-based control-plane partitioning.

It explains why partitions are based on pools, what partition controllers own,
and why shards are not partitions.

## Core idea

A `PoolPartition` owns a subset of pools and the adaptive state for those
pools.

Pool-based partitioning preserves workload ownership. A workload domain should
not be split across unrelated controllers just because it has multiple classes
or shards.

## Partition ownership

At the concept level, a partition owns:

- assigned pools;
- partition-local workload counters and windows;
- local score and target calculation;
- partition-local policy snapshot publication;
- trim candidates for assigned pools;
- pressure application for assigned pools;
- partition-level observability.

Pool assignment should be stable enough that control state is meaningful.

## Controller scope

A partition controller performs cold-path work for assigned pools.

It may:

- harvest local counters;
- update recent workload signals;
- compute pool and class targets;
- publish partition-local policy state;
- schedule bounded trim work;
- report controller lag and snapshot age.

Cadence, lag, and snapshot semantics are owned by
[Hot Path and Cold Path](./hot-path-and-cold-path.md). Workload scoring is
owned by [Adaptive Retention](./adaptive-retention.md).

## Single partition still follows the model

Even if there is only one partition, the conceptual model remains partitioned.

That avoids two architectures: one for "small" runtimes and another for
"scaled" runtimes. A single partition is just the degenerate case of the same
ownership model.

## Shards are different

Shards are data-path contention boundaries. Partitions are control-plane
ownership boundaries.

| Concept | Plane | Owns |
| --- | --- | --- |
| `PoolPartition` | Control path | Pools and adaptive state for those pools. |
| `Shard` | Data path | Local retained storage access for one size class. |

Shard and bucket concepts belong to
[Shards and Buckets](./shards-and-buckets.md).

## Common mistakes

### Partitioning by size class

Class-based partitions split one workload domain across multiple controllers.

### Partitioning by shard

Shard-based partitions confuse contention reduction with control ownership.

### Routing every operation across partitions

Partitioning should not make ordinary buffer operations perform global routing
or coordination.

## Conceptual invariants

1. Pool partitions are control-plane ownership units.
2. A pool belongs to exactly one partition at a time.
3. Partition controllers own adaptive state for assigned pools.
4. Shards are not partitions.
5. Data-path safety must not depend on a fresh partition tick.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Workload ownership should remain coherent | Partition by pools rather than classes or shards. |
| Control work must scale | Keep adaptive state partition-local where possible. |
| Data-path operations should stay local | Avoid per-operation cross-partition routing. |
| Partitions apply group pressure locally | Propagate pressure context without global bucket scans. |

## Out of scope

This concept does not define exact assignment algorithms, controller
implementation, partition count defaults, public creation APIs, or balancing
thresholds.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand group ownership | [Pool Groups](./pool-groups.md) |
| Understand controller cadence and snapshots | [Hot Path and Cold Path](./hot-path-and-cold-path.md) |
| Understand local storage and shards | [Shards and Buckets](./shards-and-buckets.md) |
| Understand adaptive target movement | [Adaptive Retention](./adaptive-retention.md) |
| Understand architecture details | [Architecture](../architecture/index.md) |

## Summary

Pool partitions scale the control plane by assigning pools and their adaptive
state to bounded ownership units. They are not data-path shards and should not
turn ordinary operations into cross-partition routing.
