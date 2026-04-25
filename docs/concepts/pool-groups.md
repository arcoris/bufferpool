<div align="center">

# Pool Groups

**Top-level memory-governance and lifecycle domain for buffer retention.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Retention](https://img.shields.io/badge/Core-Memory%20Retention-0F172A?style=flat)](./memory-retention.md)
[![Partitions](https://img.shields.io/badge/Next-Pool%20Partitions-7C2D12?style=flat)](./pool-partitions.md)
[![Architecture](https://img.shields.io/badge/Model-Architecture-B45309?style=flat)](../architecture/index.md)

[Docs](../index.md) · [Concepts](./index.md) · [Memory Retention](./memory-retention.md) · [Pool Partitions](./pool-partitions.md) · [Memory Pressure](./memory-pressure.md) · [Architecture](../architecture/index.md)

Group budget · Lifecycle ownership · Pressure context · Partition ownership · Aggregate snapshots

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Group responsibilities](#group-responsibilities) · [What groups do not own](#what-groups-do-not-own) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns the `PoolGroup` concept.

A pool group is the top-level memory-governance and lifecycle domain. It is the
conceptual place where aggregate retention policy, partition ownership,
pressure context, and lifecycle ownership meet.

## Core idea

The runtime should not be one flat global bucket.

A pool group provides a top-level boundary for:

- aggregate retained-memory limits;
- partition lifecycle;
- pressure propagation;
- policy generation;
- high-level snapshots;
- coordination between workload domains.

Detailed runtime architecture belongs to
[Architecture](../architecture/index.md).

## Group responsibilities

At the concept level, a pool group owns:

| Responsibility | Meaning |
| --- | --- |
| Memory-governance domain | The aggregate boundary for retained-memory policy. |
| Partition set | The collection of pool partitions that perform local control work. |
| Lifecycle domain | Start, stop, and shutdown ownership for group-level and partition-level control work. |
| Pressure context | Group-visible pressure state that partitions can apply locally. |
| Aggregate snapshots | Summary state across partitions, pools, and retention behavior. |

The budget hierarchy is introduced by
[Memory Retention](./memory-retention.md). Partition-level control is owned by
[Pool Partitions](./pool-partitions.md).

## Pressure context

Memory pressure may be coordinated at the group level because pressure usually
affects aggregate retention policy.

The group should not directly scan every bucket as normal pressure behavior.
Instead, it publishes or coordinates pressure context so partitions can apply
pressure to their assigned pools.

Pressure rules are owned by [Memory Pressure](./memory-pressure.md).

## Lifecycle boundary

Background control work must have explicit ownership.

At the concept level, the group is the natural lifecycle boundary for:

- partition controllers;
- policy publication;
- pressure state;
- aggregate snapshot state;
- shutdown coordination.

Public lifecycle operation names are not defined here.

## What groups do not own

A pool group does not own:

- per-buffer lifecycle operations;
- shard-local storage internals;
- class-level scoring details;
- exact pressure thresholds;
- public API names;
- implementation file layout.

Those responsibilities belong to lower-level concept, architecture, policy, or
reference documents.

## Conceptual invariants

1. A pool group is a governance domain, not a flat buffer store.
2. Pool partitions belong to a group.
3. Group-level pressure context is applied locally by partitions.
4. Group lifecycle owns background control work at the aggregate boundary.
5. Group concepts do not define public lifecycle method names.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Retention needs an aggregate boundary | Keep group-level budget and pressure context above partitions. |
| Control work needs lifecycle ownership | Make partition control work subordinate to group lifecycle. |
| Pressure should scale | Coordinate pressure at the group and apply it through partitions. |
| Group state should be explainable | Provide aggregate snapshots without making the group a flat bucket. |

## Out of scope

This concept does not define public lifecycle method names, exact coordinator
algorithms, partition assignment implementation, pressure APIs, or file layout.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand retained-memory budgets | [Memory Retention](./memory-retention.md) |
| Understand partition ownership | [Pool Partitions](./pool-partitions.md) |
| Understand pressure propagation | [Memory Pressure](./memory-pressure.md) |
| Understand control/data separation | [Hot Path and Cold Path](./hot-path-and-cold-path.md) |
| Understand architecture details | [Architecture](../architecture/index.md) |

## Summary

`PoolGroup` is the top-level memory-governance and lifecycle domain. It
coordinates aggregate policy and pressure context while leaving partition-local
adaptation and data-path storage to lower levels.
