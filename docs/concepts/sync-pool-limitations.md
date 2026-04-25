<div align="center">

# sync.Pool Limitations

**Alternatives-style concept page for why `sync.Pool` is not the deterministic core model.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Buffer Pooling](https://img.shields.io/badge/Concept-Buffer%20Pooling-0F172A?style=flat)](./buffer-pooling.md)
[![Retention](https://img.shields.io/badge/Core-Memory%20Retention-7C2D12?style=flat)](./memory-retention.md)
[![Rationale](https://img.shields.io/badge/Context-Rationale-B45309?style=flat)](../rationale/index.md)

[Docs](../index.md) · [Concepts](./index.md) · [Buffer Pooling](./buffer-pooling.md) · [Memory Retention](./memory-retention.md) · [Adaptive Retention](./adaptive-retention.md) · [Memory Pressure](./memory-pressure.md) · [Rationale](../rationale/index.md)

Opportunistic reuse · Weak retention guarantees · No byte budget · No pressure model · Useful baseline

**Start:** [Purpose](#purpose) · [What sync.Pool is good for](#what-syncpool-is-good-for) · [Core limitations](#core-limitations) · [Project consequence](#project-consequence) · [Reading guide](#reading-guide)

</div>

## Purpose

This document explains why `sync.Pool` is not sufficient as the deterministic
core retained-storage model for `arcoris.dev/bufferpool`.

It lives in the concept layer as an alternatives/rationale-style page. It
is the source of truth for the concept-level comparison. Rationale material may
link here when alternatives are discussed, but should not redefine this model.

## What sync.Pool is good for

`sync.Pool` is useful for opportunistic temporary-object reuse in Go.

It is simple, concurrency-safe, runtime-managed, and appropriate when a program
does not need deterministic retained-byte bounds, class-level trim, explicit
drop reasons, pressure-aware contraction, or workload-driven target movement.

Reference: [`sync.Pool` documentation](https://pkg.go.dev/sync#Pool).

## Core limitations

For this project, the important limitations are:

| Limitation | Why it matters |
| --- | --- |
| No deterministic retained-byte budget | The runtime must bound retained capacity explicitly. |
| No byte-aware accounting | The project must reason about retained, in-use, and reserved memory. |
| No built-in size-class ownership | Buffer retention is capacity-shaped. |
| No returned-capacity admission policy | Grown or oversized returned buffers must be rejectable. |
| No explicit drop reasons | Operators need to understand why retention did or did not happen. |
| No workload scoring | Adaptive retention needs recent signals and target redistribution. |
| No pressure-aware contraction model | Pressure must make retention more conservative and observable. |
| No trim-to-target | Policy shrink requires bounded movement toward lower retained memory. |
| No pool-based control ownership | The project uses pool partitions for adaptive control. |

These are not flaws in `sync.Pool`; they are outside its intended contract.

## Project consequence

`arcoris.dev/bufferpool` may compare against `sync.Pool` as a baseline and may
learn from its simplicity.

The project must not depend on `sync.Pool` as authoritative storage for:

- retained-byte accounting;
- class-level budget enforcement;
- pressure contraction;
- deterministic trim;
- drop reasons;
- workload-driven targets;
- ownership-aware accounting.

Using `sync.Pool` for non-authoritative internal scratch would require a design
that preserves the project's explicit memory-governance semantics.

## Comparison matrix

| Capability | `sync.Pool` | `arcoris.dev/bufferpool` target |
| --- | --- | --- |
| Opportunistic reuse | Yes | Yes, under explicit policy. |
| Deterministic retained-byte bounds | No | Required. |
| Size-aware retention | Manual and external | Core concept. |
| Returned-capacity validation | No built-in model | Required. |
| Drop reasons | No | Required for observability. |
| Pressure-aware contraction | No | Required. |
| Workload-driven targets | No | Required for adaptive retention. |
| Pool-based control partitions | No | Baseline control-plane model. |

## Common mistakes

### Treating sync.Pool as a bounded buffer pool

It is not a deterministic retained-byte storage system.

### Building adaptive policy on hidden contents

Adaptive policy needs visible retained state, counters, and targets.

### Assuming pooled values survive until needed

`sync.Pool` intentionally does not provide that guarantee.

### Hiding drop behavior

The project needs explainable drops. Hidden runtime decisions are not enough.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| `sync.Pool` hides retained contents | Do not use it as authoritative retained-byte storage. |
| It has no size-aware admission model | Keep returned-capacity validation in project-owned policy. |
| It has no pressure contraction contract | Implement pressure and trim semantics outside `sync.Pool`. |
| It remains a useful comparison point | Benchmark against it where the workload makes that comparison meaningful. |

## Conceptual invariants

1. `sync.Pool` is useful opportunistic reuse.
2. `sync.Pool` is not deterministic memory governance.
3. The project must own retained-byte accounting and admission semantics.
4. `sync.Pool` can be a benchmark baseline.
5. The project must not be documented as a thin `sync.Pool` wrapper.

## Out of scope

This concept does not define exact internal use of `sync.Pool`, benchmark
results, public API shape, or detailed alternative decision records. Durable
rationale records belong under [Rationale](../rationale/index.md) and should
link back here for the concept model.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand the project pooling model | [Buffer Pooling](./buffer-pooling.md) |
| Understand retained capacity | [Memory Retention](./memory-retention.md) |
| Understand adaptive behavior | [Adaptive Retention](./adaptive-retention.md) |
| Understand pressure behavior | [Memory Pressure](./memory-pressure.md) |
| Understand broader alternatives | [Rationale](../rationale/index.md) |

## Summary

`sync.Pool` is a valuable Go primitive and benchmark baseline, but it does not
provide the deterministic byte-aware, size-aware, pressure-aware, observable,
workload-driven retention model this project requires.
