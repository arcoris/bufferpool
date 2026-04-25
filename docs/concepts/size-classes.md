<div align="center">

# Size Classes

**Capacity normalization model for size-aware byte-buffer reuse.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Retention](https://img.shields.io/badge/Core-Memory%20Retention-0F172A?style=flat)](./memory-retention.md)
[![Growth](https://img.shields.io/badge/Next-Buffer%20Growth-7C2D12?style=flat)](./buffer-growth.md)
[![Buckets](https://img.shields.io/badge/Storage-Shards%20%26%20Buckets-B45309?style=flat)](./shards-and-buckets.md)

[Docs](../index.md) · [Concepts](./index.md) · [Memory Retention](./memory-retention.md) · [Buffer Growth](./buffer-growth.md) · [Shards and Buckets](./shards-and-buckets.md) · [Policies](../policies/index.md)

Requested size · Class size · Actual capacity · Fragmentation · Class budget · Local storage

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Size terms](#size-terms) · [Class fit](#class-fit) · [Class budgets](#class-budgets) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns size-class concepts for `arcoris.dev/bufferpool`.

It explains requested size, class size, actual capacity, class mapping,
fragmentation, and class-level budget context. Returned-capacity growth belongs
to [Buffer Growth](./buffer-growth.md). Local storage belongs to
[Shards and Buckets](./shards-and-buckets.md).

## Core idea

Byte-buffer reuse is capacity-shaped.

A runtime that stores all capacities together can satisfy a small request with
a large buffer and accidentally retain too much memory. Size classes prevent
unrelated capacity shapes from competing in one undifferentiated store.

## Size terms

| Term | Meaning |
| --- | --- |
| Requested size | Number of bytes needed by caller code or workload. |
| Class size | Normalized capacity selected for a requested size. |
| Actual capacity | Real capacity of a checked-out, returned, or retained buffer. |
| Internal slack | Difference between class size and requested size. |
| Growth | Increase in actual capacity after caller use. |

## Class fit

A size-class model should keep compatible capacities together without making
class count explode.

Good class fit means:

- small requests do not retain very large capacity;
- large classes must prove enough usefulness to justify their cost;
- repeated requests map predictably;
- returned capacity can be checked against expected class relationship;
- class-level retention can be budgeted and observed.

The exact class table is a policy/design detail, not a concept-layer contract.

## Fragmentation trade-off

Class spacing creates a trade-off:

| Class spacing | Benefit | Cost |
| --- | --- | --- |
| Fine-grained | Less slack per retained buffer. | More metadata, counters, and control work. |
| Coarse-grained | Less metadata and simpler control. | More internal slack and possible waste. |

The concept requirement is not one exact mapping. The requirement is that size
normalization keeps retention bounded and explainable.

## Class budgets

Class budgets constrain how much capacity a class may retain within a workload
domain.

A class budget is local context for retention decisions. It does not replace
group or partition governance, and it does not define the bucket storage
mechanism.

Budget flow is introduced by [Memory Retention](./memory-retention.md).
Target redistribution is owned by
[Adaptive Retention](./adaptive-retention.md).

## Class sensitivity under pressure

Classes can respond differently to pressure because their memory cost differs.

Large or cold classes usually need stronger evidence before retaining capacity
under pressure. Small hot classes may remain useful longer because they provide
allocation relief at lower retained-memory cost.

Detailed pressure behavior belongs to
[Memory Pressure](./memory-pressure.md).

## Relationship to shards and buckets

Size classes decide what capacities are compatible. Shards and buckets decide
how retained capacity is stored and accessed locally.

This distinction matters:

- `SizeClass` is about capacity normalization and class-level accounting;
- `Shard` is about data-path contention reduction;
- `Bucket` is about local retained-buffer storage.

Storage details belong to [Shards and Buckets](./shards-and-buckets.md).

## Common mistakes

### Treating requested size and capacity as identical

Requested size describes demand. Actual capacity describes memory cost.

### Returning grown buffers without validation

A buffer requested for one class may grow beyond that class. Growth validation
belongs to [Buffer Growth](./buffer-growth.md).

### Creating public pools for every class

Size classes are internal capacity boundaries. They should not force users to
manage one public workload domain per class.

### Making class count too large

Too many classes increase metadata, counters, snapshots, and control work.

### Making class count too small

Too few classes increase slack and make large retained capacity easier to hide.

## Conceptual invariants

1. Retention is capacity-shaped.
2. Requested size, class size, and actual capacity are distinct.
3. Class mapping must be predictable.
4. Class budgets bound retained capacity at class granularity.
5. Class-level concepts do not define public API names.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Capacity shape determines retention cost | Normalize requests into size classes. |
| Class granularity has trade-offs | Balance slack against metadata and control overhead. |
| Returned buffers can grow | Link class fit to returned-capacity validation. |
| Classes need local storage | Pair class boundaries with shards and buckets beneath them. |

## Out of scope

This concept does not define exact class tables, growth thresholds, pressure
thresholds, bucket layout, public configuration names, or benchmark thresholds.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand retained capacity | [Memory Retention](./memory-retention.md) |
| Understand grown returned buffers | [Buffer Growth](./buffer-growth.md) |
| Understand local class storage | [Shards and Buckets](./shards-and-buckets.md) |
| Understand adaptive class targets | [Adaptive Retention](./adaptive-retention.md) |
| Understand pressure behavior | [Memory Pressure](./memory-pressure.md) |
| Understand exact policy rules | [Policies](../policies/index.md) |

## Summary

Size classes make buffer reuse size-aware. They protect retention from mixing
unrelated capacities while giving the control plane a useful class-level unit
for budgets, signals, and trim decisions.
