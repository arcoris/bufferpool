<div align="center">

# Memory Pressure

**Concept model for pressure-aware retention, contraction, and recovery.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Retention](https://img.shields.io/badge/Core-Memory%20Retention-0F172A?style=flat)](./memory-retention.md)
[![Adaptive](https://img.shields.io/badge/Adaptive-Retention-7C2D12?style=flat)](./adaptive-retention.md)
[![Operations](https://img.shields.io/badge/Operate-Operations-B45309?style=flat)](../operations/index.md)

[Docs](../index.md) · [Concepts](./index.md) · [Memory Retention](./memory-retention.md) · [Adaptive Retention](./adaptive-retention.md) · [Pool Groups](./pool-groups.md) · [Operations](../operations/index.md)

Pressure levels · Pressure sources · Conservative retention · Contraction · Recovery · Hysteresis

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Pressure sources](#pressure-sources) · [Pressure response](#pressure-response) · [Recovery](#recovery) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns the memory-pressure concept for `arcoris.dev/bufferpool`.

It explains pressure levels, pressure sources, contraction, recovery,
hysteresis, cooldown, and pressure-specific observability. It does not define
exact public pressure APIs, final enum names, or policy thresholds.

## Core idea

Buffer retention trades memory for lower future allocation pressure.

Memory pressure changes that trade-off. When memory becomes more expensive,
retention should become more conservative and retained capacity should move
toward lower targets.

Pressure does not make the runtime a process-wide memory manager. It only
changes how the buffer-retention runtime governs its own retained capacity.

## Pressure levels

A conceptual pressure model may use graded levels:

| Level | Meaning |
| --- | --- |
| Normal | Retention follows ordinary adaptive policy. |
| Medium | Large, cold, or low-value retention becomes more conservative. |
| High | Trim and admission strongly prefer useful small or hot retention. |
| Critical | Most non-essential retention may be disabled or aggressively contracted. |

Exact names and thresholds belong to policy/reference docs.

## Pressure sources

Pressure can come from several sources:

| Source | Meaning |
| --- | --- |
| External pressure | Application or environment indicates memory risk. |
| Internal pressure | Runtime detects retained memory above target or hard-limit risk. |
| Policy pressure | Configuration or policy update lowers retention targets. |
| Operational pressure | User or operations path requests lower retention or trim. |
| Deployment pressure | Container, host, or orchestration environment indicates memory risk. |

Runtime memory observation must not happen on ordinary data-path operations.

## Pressure ownership

At concept level, pressure context is coordinated by
[Pool Groups](./pool-groups.md) and applied locally by
[Pool Partitions](./pool-partitions.md).

Partitions should apply pressure to assigned pools without one global pressure
handler scanning every bucket as normal behavior.

## Pressure response

Pressure response has two parts:

| Response | Meaning |
| --- | --- |
| Immediate restriction | New retention becomes more conservative once pressure state is visible. |
| Bounded correction | Existing retained memory is reduced through bounded trim or contraction work. |

Admission may reject more returned capacity under pressure. Trim may prioritize
large, cold, or low-use retained memory.

Checked-out buffers must not be forcibly reclaimed. Ownership/accounting can
explain why reserved memory may remain high while retained memory contracts.
See [Explicit Ownership](./explicit-ownership.md).

## Contraction

Contraction is movement toward lower retained-memory targets after pressure or
budget shrink.

Conceptually:

1. lower targets are published;
2. admission becomes stricter;
3. trim removes retained capacity through bounded work;
4. metrics show backlog and progress;
5. recovery prevents immediate uncontrolled growth.

Contraction is part of the retention lifecycle described by
[Memory Retention](./memory-retention.md).

## Adaptive behavior under pressure

Pressure modifies adaptive retention.

A class that was previously useful may receive less target under pressure if
its memory cost is too high. Large classes may need stronger recent evidence
than small hot classes.

Adaptive scoring is owned by
[Adaptive Retention](./adaptive-retention.md).

## Recovery

When pressure clears, retention should not immediately jump back to previous
targets.

Recovery may use:

- hysteresis;
- cooldown;
- gradual target growth;
- stronger evidence requirements after severe pressure;
- continued trim backlog reporting.

Hysteresis avoids rapid enter/exit loops around one threshold. Cooldown avoids
re-growing retained memory immediately after contraction.

## Pressure observability

Pressure observability should answer:

- whether pressure is active;
- which source caused it;
- which level is active;
- when pressure changed;
- how admission and trim changed;
- how much trim backlog remains;
- which drops are pressure-related;
- whether controller lag delayed response.

Exact metric names belong to [Reference](../reference/index.md).

## Common mistakes

### Treating pressure as process-wide memory management

The runtime governs its retained buffers. It does not own the whole process.

### Computing pressure on the hot path

Pressure interpretation belongs to control work and published state.

### Assuming trim can reclaim checked-out buffers

Checked-out capacity can affect accounting, but it cannot be forcibly reclaimed
by retention policy.

### Dropping everything under any pressure

Moderate pressure may only require conservative large/cold retention. Pressure
should be graded and policy-driven.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Retained memory becomes more expensive | Make admission stricter and lower targets under pressure. |
| Checked-out buffers cannot be reclaimed | Report reserved/in-use context where accounting supports it. |
| Pressure can oscillate | Use hysteresis, cooldown, or bounded target growth during recovery. |
| Pressure should be diagnosable | Expose pressure level, source, drops, trim backlog, and lag. |

## Conceptual invariants

1. Pressure makes retention more conservative.
2. Pressure response must be bounded and observable.
3. New retention can be restricted before all old retention is trimmed.
4. Checked-out buffers are not forcibly reclaimed.
5. Recovery needs hysteresis or cooldown to avoid oscillation.
6. Pressure APIs and thresholds belong below the concept layer.

## Out of scope

This concept does not define public pressure APIs, pressure enum names, exact
thresholds, trim API names, emergency operations, or runtime memory-sampling
implementation.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand retained-memory lifecycle | [Memory Retention](./memory-retention.md) |
| Understand group pressure context | [Pool Groups](./pool-groups.md) |
| Understand partition-local pressure application | [Pool Partitions](./pool-partitions.md) |
| Understand adaptive target changes | [Adaptive Retention](./adaptive-retention.md) |
| Understand operation and tuning | [Operations](../operations/index.md) |
| Understand policy details | [Policies](../policies/index.md) |

## Summary

Memory pressure is the signal that retained byte-buffer capacity has become
more expensive. The runtime should respond by restricting new retention,
contracting existing retained memory through bounded work, and making pressure
state and progress observable.
