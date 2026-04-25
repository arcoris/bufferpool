<div align="center">

# Problem Space

**Why allocation-heavy Go systems need bounded byte-buffer retention.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Overview](https://img.shields.io/badge/Start-Overview-0F172A?style=flat)](../overview.md)
[![Scope](https://img.shields.io/badge/Scope-Goals%20%26%20Non--goals-7C2D12?style=flat)](../goals-and-non-goals.md)
[![Allocation](https://img.shields.io/badge/Next-Allocation%20Pressure-B45309?style=flat)](./allocation-pressure.md)

[Docs](../index.md) · [Concepts](./index.md) · [Overview](../overview.md) · [Goals and Non-goals](../goals-and-non-goals.md) · [Terminology](../terminology.md) · [Architecture](../architecture/index.md)

Workload shape · Temporary buffers · Allocation cost · Retention risk · Applicability

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Where the problem appears](#where-the-problem-appears) · [When it matters](#when-it-matters) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns the high-level problem domain for
`arcoris.dev/bufferpool`.

It explains when byte-buffer retention is worth considering and when it is
probably unnecessary. Measurement details belong to
[Allocation Pressure](./allocation-pressure.md). Retention mechanics belong to
[Memory Retention](./memory-retention.md).

## Core idea

Many Go systems create temporary `[]byte` capacity while moving, decoding,
encoding, compressing, batching, or transforming data.

Those buffers are often short-lived. Repeated allocation can create allocator
work, garbage collection work, memory churn, and latency variance. Reusing
capacity can reduce that cost, but keeping capacity reachable also consumes
memory.

The project exists for the trade-off:

> retain reusable byte-buffer capacity only when it reduces meaningful
> allocation cost and the retained memory remains bounded and explainable.

## Where the problem appears

The problem is common in systems such as:

- reverse proxies and gateways;
- protocol servers;
- RPC and API gateways;
- telemetry, log, and event ingestion;
- serialization and deserialization paths;
- compression and decompression services;
- stream processing and batching;
- storage or cache layers that allocate temporary transfer buffers;
- multi-tenant services that need retention isolation between workload domains.

The common shape is not "high traffic" by itself. The common shape is repeated
temporary byte-buffer allocation.

## When it matters

Buffer retention is worth considering when:

- allocation shows up in profiles or benchmark output;
- temporary byte buffers are large or frequent enough to affect throughput,
  garbage collection, memory churn, or tail latency;
- requested capacities repeat;
- retained capacity can be bounded;
- different workload domains should not compete through one flat retained store;
- operators need to understand why memory is retained, dropped, or trimmed.

For measurement signals and formulas, continue with
[Allocation Pressure](./allocation-pressure.md).

## When it probably does not matter

The project may be unnecessary when:

- temporary buffer allocation is not visible in measurement;
- buffers are tiny, rare, or not repeated by capacity shape;
- streaming or caller-owned buffers remove the temporary allocation pattern;
- workload cost is dominated by database, disk, network, or downstream latency;
- opportunistic reuse is enough and bounded retained-memory behavior is not
  required.

## What this project is not solving

This project is not a generic object-pooling framework, ordinary cache,
process-wide memory manager, Go allocator replacement, or thin `sync.Pool`
wrapper.

Those boundaries are defined by
[Goals and Non-goals](../goals-and-non-goals.md). This page only explains the
problem that motivates the concept layer.

## Conceptual invariants

1. Request rate alone does not justify buffer retention.
2. The relevant workload signal is temporary byte-buffer allocation cost.
3. Retention is only useful when capacity shapes repeat.
4. Retention must stay bounded because retained capacity is still memory.
5. A workload-specific decision requires measurement, not intuition.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| The project targets allocation-heavy systems | Avoid generic "high-load" positioning. |
| Retention has cost | Pair allocation-pressure reduction with bounded retained memory. |
| Workload domains can differ | Preserve concepts for pools, partitions, and workload-aware control. |
| Measurement matters | Route claims to profiles, benchmarks, and performance evidence. |

## Out of scope

This concept does not define allocation formulas, retention policy, public API
shape, implementation structure, benchmark thresholds, or operations profiles.

## Reading guide

| Need | Continue with |
| --- | --- |
| Quantify the problem | [Allocation Pressure](./allocation-pressure.md) |
| Understand the pooling idea | [Buffer Pooling](./buffer-pooling.md) |
| Understand the retention trade-off | [Memory Retention](./memory-retention.md) |
| Understand scope boundaries | [Goals and Non-goals](../goals-and-non-goals.md) |
| Understand runtime structure | [Architecture](../architecture/index.md) |

## Summary

`arcoris.dev/bufferpool` matters when repeated temporary byte-buffer allocation
is a measurable cost and retained capacity can be governed safely.

The problem is allocation-heavy buffer churn, not traffic volume by itself.
