<div align="center">

# Buffer Pooling

**Umbrella concept for reusable byte-buffer capacity under explicit policy.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Problem](https://img.shields.io/badge/Problem-Space-0F172A?style=flat)](./problem-space.md)
[![Retention](https://img.shields.io/badge/Core-Memory%20Retention-7C2D12?style=flat)](./memory-retention.md)
[![sync.Pool](https://img.shields.io/badge/Alternative-sync.Pool-B45309?style=flat)](./sync-pool-limitations.md)

[Docs](../index.md) · [Concepts](./index.md) · [Problem Space](./problem-space.md) · [Allocation Pressure](./allocation-pressure.md) · [Memory Retention](./memory-retention.md) · [sync.Pool Limitations](./sync-pool-limitations.md)

Reusable capacity · Bounded admission · Size awareness · Ownership/accounting · Not a cache

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Project meaning](#project-meaning) · [What it does not own](#what-it-does-not-own) · [Reading guide](#reading-guide)

</div>

## Purpose

This document defines "buffer pooling" as an umbrella concept for this project.

It is intentionally a bridge page. Detailed problem framing, retention
mechanics, size mapping, growth validation, ownership, adaptation, pressure,
and `sync.Pool` comparison are owned by their dedicated concept pages.

## Core idea

Buffer pooling means retaining reusable byte-buffer capacity after one use so a
future operation may avoid allocation.

In this project, the retained value is not application data. The useful asset
is backing capacity.

That distinction changes the goal:

> buffer pooling is not keeping values alive because they are semantically
> important; it is retaining capacity because future allocation may be costly.

## Project meaning

`arcoris.dev/bufferpool` treats buffer pooling as policy-governed memory
retention.

The pooling model must be:

- buffer-specific;
- size-aware;
- bounded by explicit retained-memory policy;
- able to reject returned capacity;
- compatible with explicit ownership/accounting semantics;
- observable through retention, drop, pressure, trim, and controller signals;
- workload-aware without making the data path depend on fresh control work.

## Why naive pooling is insufficient

Simple pooling can reduce allocation, but it can also retain the wrong memory.

Typical failure modes are:

- one flat store mixes unrelated capacities;
- rare large buffers remain reachable after a burst;
- grown buffers return more capacity than the original demand justified;
- workload domains compete without isolation;
- there is no drop reason or trim story;
- pressure cannot make retention more conservative.

Each failure mode maps to a dedicated project concept.

## Concept links

| Concern | Source of truth |
| --- | --- |
| Why the project matters | [Problem Space](./problem-space.md) |
| How allocation pressure is measured | [Allocation Pressure](./allocation-pressure.md) |
| Retained capacity lifecycle | [Memory Retention](./memory-retention.md) |
| Requested size and class mapping | [Size Classes](./size-classes.md) |
| Grown returned capacity | [Buffer Growth](./buffer-growth.md) |
| Checked-out buffer semantics | [Explicit Ownership](./explicit-ownership.md) |
| Data/control path separation | [Hot Path and Cold Path](./hot-path-and-cold-path.md) |
| Workload-driven policy | [Adaptive Retention](./adaptive-retention.md) |
| Pressure-aware behavior | [Memory Pressure](./memory-pressure.md) |
| Why `sync.Pool` is not enough | [sync.Pool Limitations](./sync-pool-limitations.md) |

## What it does not own

This page does not define:

- allocation-pressure formulas;
- retained/in-use/reserved memory terms;
- size-class mapping;
- growth-ratio rules;
- ownership handle shape;
- controller algorithms;
- pressure thresholds;
- public API names;
- implementation layout.

## Conceptual invariants

1. Buffer pooling retains capacity, not application data.
2. Retained capacity must be bounded.
3. Returned capacity may be dropped.
4. Size, growth, ownership, workload, and pressure affect retention decisions.
5. Public API names remain outside the concept layer.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Capacity is the reusable asset | Model pooling as memory retention rather than cache eviction. |
| Naive pooling can retain the wrong memory | Route size, growth, pressure, and ownership details to specialized concepts. |
| Reuse must be explainable | Preserve drop, trim, and observability concepts across lower-level docs. |

## Out of scope

This bridge page does not define formulas, class mapping, growth validation,
ownership modes, adaptive scoring, pressure levels, storage layout, or public
API names.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand when pooling is worth considering | [Problem Space](./problem-space.md) |
| Quantify allocation cost | [Allocation Pressure](./allocation-pressure.md) |
| Understand retained capacity | [Memory Retention](./memory-retention.md) |
| Understand size-aware reuse | [Size Classes](./size-classes.md) |
| Understand why not `sync.Pool` | [sync.Pool Limitations](./sync-pool-limitations.md) |
| Understand scope limits | [Goals and Non-goals](../goals-and-non-goals.md) |

## Summary

Buffer pooling in this project means bounded, observable retention of reusable
byte-buffer capacity.

It is a gateway concept. The detailed rules belong to the specialized concept
pages linked above.
