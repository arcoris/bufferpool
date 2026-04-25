<div align="center">

# Memory Retention

**Concept model for retained byte-buffer capacity, admission, drop, trim, and contraction.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Buffer Pooling](https://img.shields.io/badge/Concept-Buffer%20Pooling-0F172A?style=flat)](./buffer-pooling.md)
[![Pressure](https://img.shields.io/badge/Pressure-Memory%20Pressure-7C2D12?style=flat)](./memory-pressure.md)
[![Policies](https://img.shields.io/badge/Rules-Policies-B45309?style=flat)](../policies/index.md)

[Docs](../index.md) · [Concepts](./index.md) · [Buffer Pooling](./buffer-pooling.md) · [Size Classes](./size-classes.md) · [Buffer Growth](./buffer-growth.md) · [Explicit Ownership](./explicit-ownership.md) · [Adaptive Retention](./adaptive-retention.md) · [Policies](../policies/index.md)

Retained capacity · In-use memory · Reserved memory · Admission · Drop · Trim · Contraction

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Memory states](#memory-states) · [Retention lifecycle](#retention-lifecycle) · [Budget context](#budget-context) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns the memory-retention model for
`arcoris.dev/bufferpool`.

It defines retained capacity, retained/in-use/reserved memory, admission, drop,
trim, and contraction at the concept level. It does not own size mapping,
growth-ratio validation, ownership handle shape, adaptive scoring, or pressure
policy details.

## Core idea

Memory retention is keeping reusable byte-buffer capacity reachable after use
so future operations may avoid allocation.

Retained memory is useful only when:

- future demand is likely to reuse the retained capacity;
- the retained memory is within explicit bounds;
- the runtime can explain why capacity is retained, dropped, or trimmed;
- pressure or policy shrink can make retention more conservative.

## Memory states

| State | Meaning |
| --- | --- |
| Retained memory | Capacity currently held by the runtime for potential reuse. |
| In-use memory | Capacity currently checked out to caller code when accounting can track it. |
| Reserved memory | Retained memory plus in-use memory when both are known. |

The relationship is:

$$
reservedBytes =
retainedBytes + inUseBytes
$$

This relation is exact only when the configured accounting mode has enough
information. Ownership-aware accounting is explained by
[Explicit Ownership](./explicit-ownership.md).

## Retention lifecycle

The concept lifecycle is:

1. Demand appears for a requested size.
2. Size-aware logic maps demand to a compatible capacity class.
3. Retained capacity may serve the demand.
4. Caller code uses the checked-out buffer.
5. The buffer is returned to the runtime.
6. Admission decides whether returned capacity may be retained.
7. Retained capacity may later be trimmed or contracted.

This lifecycle is API-neutral. It intentionally does not name public
operations.

## Admission and drop

Admission is the decision made when returned capacity is evaluated for
retention.

A returned buffer may be dropped because:

- its capacity is unsupported;
- it grew beyond the expected class relationship;
- the relevant budget or local credit is exhausted;
- pressure made retention more conservative;
- the class or workload has low recent usefulness;
- lifecycle state disallows retention.

Drops are normal bounded-retention behavior. They are not automatically errors.

Returned-capacity growth is owned by
[Buffer Growth](./buffer-growth.md). Size-class fit is owned by
[Size Classes](./size-classes.md).

## Trim and contraction

Drop and trim are different.

| Action | Meaning |
| --- | --- |
| Drop | Reject returned capacity before it becomes retained memory. |
| Trim | Remove capacity that is already retained. |
| Contraction | Move retained memory toward a lower target after pressure or policy shrink. |

Contraction should restrict new retention promptly and reduce existing retained
memory through bounded work. Checked-out buffers must not be forcibly reclaimed.

Pressure-specific behavior is owned by
[Memory Pressure](./memory-pressure.md).

## Budget context

Retention is bounded by targets and budgets. The general budget context is:

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

- $B_{group}$ is the group budget;
- $T_k$ is the target for partition $k$;
- $T_p$ is the target for pool $p$;
- $T_{p,c}$ is the target for class $c$ in pool $p$;
- $C_{p,c,s}$ is local shard credit or equivalent allowance.

Group and partition ownership are explained by
[Pool Groups](./pool-groups.md) and [Pool Partitions](./pool-partitions.md).
Adaptive redistribution is explained by
[Adaptive Retention](./adaptive-retention.md).

## Concept-specific observability

Retention observability should answer:

- how much memory is retained;
- how much memory is checked out when accounting supports it;
- which returned buffers are dropped and why;
- whether retained memory is over target;
- whether trim or contraction is backlogged;
- whether pressure is affecting retention.

Exact public metric names belong to [Reference](../reference/index.md).

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Retention reduces allocation by keeping memory reachable | Bound retained memory explicitly. |
| Returned capacity can be unsuitable | Treat admission and drop as normal retention behavior. |
| Existing retained memory may become too expensive | Support bounded trim and contraction. |
| Local storage must enforce targets cheaply | Use size classes, shards, buckets, and credits beneath published policy. |

## Conceptual invariants

1. Retained capacity is memory that remains reachable for possible reuse.
2. Retention must be bounded by explicit policy.
3. Returned capacity must pass admission before it becomes retained memory.
4. Drop and trim are different operations.
5. Contraction must not forcibly reclaim checked-out buffers.
6. Public acquisition, return, pressure, and trim APIs are outside this concept.

## Out of scope

This concept does not define:

- final public API shape;
- exact class mapping;
- growth-ratio thresholds;
- adaptive scoring formulas;
- pressure levels or thresholds;
- bucket storage design;
- benchmark thresholds.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand why retention exists | [Buffer Pooling](./buffer-pooling.md) |
| Understand size-aware retention | [Size Classes](./size-classes.md) |
| Understand returned-capacity validation | [Buffer Growth](./buffer-growth.md) |
| Understand in-use and reserved accounting | [Explicit Ownership](./explicit-ownership.md) |
| Understand workload-driven targets | [Adaptive Retention](./adaptive-retention.md) |
| Understand pressure contraction | [Memory Pressure](./memory-pressure.md) |
| Understand local retained storage | [Shards and Buckets](./shards-and-buckets.md) |
| Understand policy rules | [Policies](../policies/index.md) |

## Summary

Memory retention is the core trade-off of the project: keep reusable
byte-buffer capacity to reduce future allocation, but keep that memory bounded,
observable, and safe to contract.
