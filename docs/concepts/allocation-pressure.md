<div align="center">

# Allocation Pressure

**Measurement model for temporary byte-buffer allocation cost.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Problem](https://img.shields.io/badge/Problem-Space-0F172A?style=flat)](./problem-space.md)
[![Retention](https://img.shields.io/badge/Model-Memory%20Retention-7C2D12?style=flat)](./memory-retention.md)
[![Performance](https://img.shields.io/badge/Evidence-Performance-B45309?style=flat)](../performance/index.md)

[Docs](../index.md) · [Concepts](./index.md) · [Problem Space](./problem-space.md) · [Buffer Pooling](./buffer-pooling.md) · [Memory Retention](./memory-retention.md) · [Performance](../performance/index.md)

Allocation throughput · Memory churn · GC pressure · Measurement signals · Benchmark evidence

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Formulas](#formulas) · [Measurement signals](#measurement-signals) · [Common mistakes](#common-mistakes) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns allocation-pressure concepts for `arcoris.dev/bufferpool`.

It explains how to reason about repeated temporary byte-buffer allocation,
which signals matter, and when retention is plausibly useful. It does not own
the retention lifecycle, size-class mapping, growth validation, or adaptive
control model.

## Core idea

Allocation pressure is the runtime cost created by repeated allocation of
temporary memory.

For this project, the relevant allocation pressure is byte-buffer shaped:
requested capacities repeat, buffers are short-lived, and allocation work
shows up in profiles, benchmarks, garbage collection behavior, or latency.

Buffer retention can reduce allocation pressure only by keeping some capacity
reachable. That means allocation reduction always has a memory-retention cost.

## Formulas

Request rate alone is not enough. A useful first approximation is:

$$
allocationThroughput =
requestsPerSecond \cdot allocatedBytesPerRequest
$$

Horizontal scaling changes the per-process value:

$$
allocationThroughputPerInstance =
\frac{requestsPerSecond \cdot allocatedBytesPerRequest}{instanceCount}
$$

These formulas are diagnostic tools, not policy algorithms. They help decide
whether custom retention is worth investigating.

## Measurement signals

Useful signals include:

- allocation bytes per operation;
- allocation count per operation;
- allocation rate over time;
- garbage collection frequency and pause impact;
- tail-latency changes around allocation-heavy paths;
- miss/allocation behavior under benchmarked workloads;
- retained memory required to reduce allocation pressure;
- workload stability and repeated capacity shapes.

The project should not claim allocation-pressure reduction without benchmark or
profile evidence. Evidence ownership belongs to
[Performance](../performance/index.md).

## What creates allocation pressure

Common sources include:

- repeated construction of request or response buffers;
- repeated encode/decode scratch capacity;
- compression and decompression windows;
- intermediate aggregation buffers;
- protocol framing buffers;
- burst workloads that allocate many similar temporary capacities.

Not every source should be pooled. Retention helps only when capacities repeat
and retained memory is cheaper than continued allocation.

## Relationship to memory retention

Allocation pressure and retained memory are the two sides of the trade-off.

| Concept | Meaning |
| --- | --- |
| Allocation pressure | Cost of repeatedly creating temporary capacity. |
| Retained memory | Cost of keeping reusable capacity reachable. |

If retained memory grows but allocation pressure does not improve, retention is
wasteful. If allocation pressure is high but retained capacity cannot be
bounded, retention is unsafe.

For the retained-capacity lifecycle, continue with
[Memory Retention](./memory-retention.md).

## Relationship to size and growth

Allocation pressure is capacity-shaped.

Size classes let the runtime group compatible capacity requests. Buffer growth
validation prevents a buffer that started small and grew large from silently
returning large capacity into a small retention decision.

Detailed ownership:

- size mapping belongs to [Size Classes](./size-classes.md);
- growth ratio and returned-capacity validation belong to
  [Buffer Growth](./buffer-growth.md).

## Common mistakes

### Treating high RPS as proof

High request rate matters only when it produces meaningful temporary allocation.

### Treating low RPS as disproof

A moderate-rate workload with large repeated temporary buffers may create more
allocation pressure than a high-rate workload with tiny buffers.

### Treating hit ratio as enough

A high reuse ratio is not sufficient if retained memory cost is too high or
large buffers dominate retention.

### Ignoring workload shape

One benchmark with one capacity shape does not prove general usefulness across
bursts, shifts, tenants, or size distributions.

## Conceptual invariants

1. Allocation pressure must be measured.
2. Retention is justified only when it reduces meaningful allocation cost.
3. Retention has a memory cost that must be bounded.
4. Size distribution matters as much as request count.
5. Performance claims require reproducible evidence.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Request rate is insufficient | Measure allocation bytes, allocation count, and capacity shape. |
| Retention has a cost | Connect allocation reduction to retained-memory behavior. |
| Workload shape matters | Benchmark stable, bursty, and shifting workloads separately. |
| Claims require evidence | Route performance conclusions through benchmark and profile documentation. |

## Out of scope

This concept does not define retention policy, size-class tables,
growth-ratio thresholds, benchmark thresholds, or public API behavior.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand the domain motivation | [Problem Space](./problem-space.md) |
| Understand the pooling concept | [Buffer Pooling](./buffer-pooling.md) |
| Understand retained memory cost | [Memory Retention](./memory-retention.md) |
| Understand size-shaped allocation | [Size Classes](./size-classes.md) |
| Understand grown returned capacity | [Buffer Growth](./buffer-growth.md) |
| Validate claims | [Performance](../performance/index.md) |

## Summary

Allocation pressure is the cost of repeated temporary allocation.

For this project, the important pressure is repeated byte-buffer capacity
allocation that can be reduced by bounded, observable retention.
