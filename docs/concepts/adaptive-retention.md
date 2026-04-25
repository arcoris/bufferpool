<div align="center">

# Adaptive Retention

**Feedback-loop concept for workload-driven retained-memory targets.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Retention](https://img.shields.io/badge/Core-Memory%20Retention-0F172A?style=flat)](./memory-retention.md)
[![Planes](https://img.shields.io/badge/Planes-Hot%20%26%20Cold-7C2D12?style=flat)](./hot-path-and-cold-path.md)
[![Workload](https://img.shields.io/badge/Details-Workload-B45309?style=flat)](../workload/index.md)

[Docs](../index.md) · [Concepts](./index.md) · [Allocation Pressure](./allocation-pressure.md) · [Memory Retention](./memory-retention.md) · [Hot Path and Cold Path](./hot-path-and-cold-path.md) · [Memory Pressure](./memory-pressure.md) · [Workload](../workload/index.md)

Recent signals · Decay · Scoring · Target redistribution · Convergence · Workload shifts

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Feedback loop](#feedback-loop) · [Decayed signals](#decayed-signals) · [Convergence](#convergence) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns the adaptive-retention concept.

It explains feedback loops, decayed workload signals, scoring, target
redistribution, convergence, and workload shifts. Controller cadence belongs to
[Hot Path and Cold Path](./hot-path-and-cold-path.md). Detailed workload model
belongs to [Workload](../workload/index.md).

## Core idea

Static retention keeps fixed capacity whether or not recent workload still
needs it.

Adaptive retention adjusts retention targets based on recent observed memory
demand while staying inside explicit memory limits.

The goal is not to retain everything that was once useful. The goal is to
retain capacity that is likely to reduce future allocation cost under current
or recent workload.

## Feedback loop

The conceptual loop is:

1. data path records lightweight counters;
2. control path harvests recent deltas;
3. recent signals update scores;
4. scores influence retention targets;
5. targets are published through policy snapshots or equivalent state;
6. data path applies local targets and credits;
7. trim and admission move retained memory toward target.

The data path must remain safe if the feedback loop is delayed.

## Decayed signals

Adaptive retention should prefer recent workload over old workload.

One common conceptual model is exponential decay:

$$
\alpha =
2^{-\Delta t / H}
$$

Where:

- $\Delta t$ is elapsed time between samples;
- $H$ is the half-life;
- $\alpha$ controls how much previous signal remains.

The exact scoring formula belongs to workload/policy documentation. The concept
requirement is that old history decays.

## Signals

Useful adaptive signals may include:

- requests by class;
- hits and misses;
- allocation events;
- returned capacity;
- drops and drop reasons;
- retained bytes;
- trim activity;
- pressure state;
- controller lag and snapshot age.

Lifetime counters are useful for observability, but they should not directly
drive adaptive decisions forever.

## Target redistribution

Adaptive retention redistributes targets across governance levels.

It may increase targets for classes that show recent demand and useful reuse.
It may reduce targets for cold, wasteful, oversized, or pressure-sensitive
retention.

Budget context is introduced by [Memory Retention](./memory-retention.md).
Partition ownership is explained by
[Pool Partitions](./pool-partitions.md).

## Convergence

Under stable workload, adaptive retention should move toward a bounded
operating range.

Useful convergence means:

- scores stop drifting without new evidence;
- retained memory stays near targets;
- misses and allocation pressure improve enough to justify retention;
- target movement does not oscillate excessively;
- controller lag remains observable.

After workload shifts, old signals should decay and targets should move toward
the new workload shape.

## Pressure interaction

Pressure modifies the allocation-retention trade-off.

Under pressure, the same recent demand may receive less retained-memory target,
or require stronger evidence before retention grows. Pressure behavior is owned
by [Memory Pressure](./memory-pressure.md).

## Common mistakes

### Using lifetime counters as adaptive truth

Lifetime counters make old workload permanent. Adaptive decisions need recent
or decayed signals.

### Chasing every spike

Reacting too quickly can create oscillation. Bounded target movement matters.

### Ignoring memory limits

Adaptation redistributes bounded retention. It must not create unbounded memory
growth.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Recent workload should matter more than old workload | Use decayed signals rather than lifetime counters as direct truth. |
| Adaptation must stay bounded | Apply target movement inside explicit memory limits. |
| The data path must stay local | Publish targets or credits for local admission instead of scoring on every operation. |
| Workload shifts are expected | Preserve enough observability to explain convergence and re-convergence. |

## Conceptual invariants

1. Adaptive retention operates inside explicit memory limits.
2. Recent or decayed signals drive adaptation.
3. Lifetime counters are observability, not direct adaptive truth.
4. The data path applies published state; it does not compute global scores.
5. Stable workloads should converge to bounded operating ranges.
6. Workload shifts should cause controlled re-convergence.

## Out of scope

This concept does not define exact scoring algorithms, controller
implementation, policy thresholds, public policy APIs, or benchmark pass/fail
criteria.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand why allocation signals matter | [Allocation Pressure](./allocation-pressure.md) |
| Understand retained targets | [Memory Retention](./memory-retention.md) |
| Understand controller cadence | [Hot Path and Cold Path](./hot-path-and-cold-path.md) |
| Understand partition-local scoring | [Pool Partitions](./pool-partitions.md) |
| Understand pressure-modified targets | [Memory Pressure](./memory-pressure.md) |
| Understand detailed workload model | [Workload](../workload/index.md) |
| Understand adaptive policy rules | [Policies](../policies/index.md) |

## Summary

Adaptive retention is the feedback loop that moves retained-memory targets
toward recent useful demand while staying bounded, observable, and safe under
controller delay.
