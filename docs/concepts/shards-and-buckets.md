<div align="center">

# Shards and Buckets

**Data-path striping and local retained-buffer storage concepts.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Size Classes](https://img.shields.io/badge/Size-Classes-0F172A?style=flat)](./size-classes.md)
[![Partitions](https://img.shields.io/badge/Contrast-Pool%20Partitions-7C2D12?style=flat)](./pool-partitions.md)
[![Design](https://img.shields.io/badge/Details-Design-B45309?style=flat)](../design/index.md)

[Docs](../index.md) · [Concepts](./index.md) · [Size Classes](./size-classes.md) · [Hot Path and Cold Path](./hot-path-and-cold-path.md) · [Pool Partitions](./pool-partitions.md) · [Design](../design/index.md)

Data-path striping · Local buckets · Segmented bounded LIFO · Local credits · Slot clearing

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Shards](#shards) · [Buckets](#buckets) · [Local credits](#local-credits) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns the concepts of shards and buckets.

Shards are data-path contention boundaries. Buckets are local retained-buffer
storage for a shard and size class. This page stays above implementation
details but explains why these concepts exist.

## Core idea

Size classes decide which capacities are compatible. Shards and buckets decide
how compatible retained capacity is accessed locally.

The goals are:

- reduce data-path contention;
- keep ordinary operations local;
- bound local retained storage;
- make local retention explainable through credits and counters;
- avoid confusing storage striping with control-plane partitioning.

## Shards

A shard is a hot-path striping unit inside a size class.

It exists to reduce contention when multiple goroutines use the same class.
Shard selection should be cheap and stable enough for locality, without
requiring private Go runtime details for correctness.

A shard is not a control-plane partition. It does not own pool assignment,
workload scoring, or global budget redistribution.

## Buckets

A bucket is local retained-buffer storage for one shard and size class.

Conceptually, bucket storage should be:

- bounded by local credit or equivalent allowance;
- cheap on the data path;
- able to drop capacity when local limits are exhausted;
- able to clear slots so trimmed buffers can become unreachable;
- observable enough to diagnose local retention and trim behavior.

The accepted project direction is specialized retained-buffer storage, not a
generic object pool backend.

## Segmented bounded LIFO

The baseline bucket concept is segmented bounded LIFO storage.

| Property | Conceptual reason |
| --- | --- |
| Segmented | Avoid large fixed metadata allocation when storage is sparse. |
| Bounded | Prevent local retained memory from growing without policy allowance. |
| LIFO | Prefer recently returned capacity, which is often cache-warm and recently useful. |

Exact segment layout belongs to [Design](../design/index.md).

## Local credits

Local credits let the data path make cheap admission decisions without
consulting global state on every operation.

A credit is not a global budget. It is a local allowance derived from published
policy state. When credit is exhausted, returned capacity may be dropped.

Budget and target flow is introduced by
[Memory Retention](./memory-retention.md). Adaptive redistribution is owned by
[Adaptive Retention](./adaptive-retention.md).

## Slot clearing

When a buffer is removed from a bucket by reuse or trim, the storage slot should
stop keeping the reference reachable.

Slot clearing is conceptually important because retained-buffer storage should
not accidentally extend object reachability after capacity has been removed
from retention.

## Observability for local storage

Shard and bucket observability should answer:

- how much capacity is retained locally;
- whether local credit is exhausted;
- whether drops are caused by local limits;
- whether trim removed local retained capacity;
- whether a shard is significantly hotter than others.

Exact metric names belong to [Reference](../reference/index.md).

## Conceptual invariants

1. Shards are data-path contention boundaries.
2. Buckets are specialized retained-buffer storage.
3. Shards are not control-plane partitions.
4. Local credits keep admission cheap.
5. Bucket storage must not retain references after slots are cleared.
6. Exact storage layout belongs below the concept layer.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Data-path contention must be reduced locally | Stripe retained storage by shards within size classes. |
| Storage must stay bounded | Apply local credits or equivalent allowances to bucket admission. |
| Retained references must be releasable | Clear slots when buffers are removed or trimmed. |
| Storage is buffer-specific | Avoid generic object-pool storage as the authoritative retained-buffer backend. |

## Out of scope

This concept does not define segment layout, exact shard-selection algorithms,
bucket structs, lock strategy, benchmark thresholds, or public API names.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand class boundaries | [Size Classes](./size-classes.md) |
| Understand data/control separation | [Hot Path and Cold Path](./hot-path-and-cold-path.md) |
| Understand partition contrast | [Pool Partitions](./pool-partitions.md) |
| Understand local storage design | [Design](../design/index.md) |
| Understand metrics contracts | [Reference](../reference/index.md) |

## Summary

Shards and buckets keep retained-buffer storage local and contention-aware.
They support fast data-path reuse while leaving adaptive policy and partition
ownership to the control path.
