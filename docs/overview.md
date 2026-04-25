<div align="center">

# arcoris.dev/bufferpool Overview

**Project overview for the adaptive byte-buffer memory-retention runtime.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](./index.md)
[![Reference](https://img.shields.io/badge/Contracts-Reference-1D4ED8?style=flat)](./reference/index.md)
[![Scope](https://img.shields.io/badge/Scope-Goals%20%26%20Non--goals-0F172A?style=flat)](./goals-and-non-goals.md)
[![Architecture](https://img.shields.io/badge/Model-Architecture-7C2D12?style=flat)](./architecture/index.md)
[![Performance](https://img.shields.io/badge/Evidence-Performance-B45309?style=flat)](./performance/index.md)

[Docs](./index.md) · [Reference](./reference/index.md) · [Goals and Non-goals](./goals-and-non-goals.md) · [Terminology](./terminology.md) · [Architecture](./architecture/index.md) · [Policies](./policies/index.md) · [Workload](./workload/index.md) · [Operations](./operations/index.md)

Allocation-heavy Go systems · Size-aware reuse · Bounded memory retention · Explicit accounting · Workload-driven control

**Start:** [Purpose](#purpose) · [Applicability](#applicability) · [Runtime responsibility](#runtime-responsibility) · [High-level model](#high-level-model) · [Next reading](#next-reading)

</div>

## Purpose

`arcoris.dev/bufferpool` is an adaptive byte-buffer memory-retention runtime for
allocation-heavy Go systems.

The project exists for systems where temporary `[]byte` allocations are large
or frequent enough to affect allocation pressure, garbage collection pressure,
retained memory, throughput, or tail latency. Its responsibility is not merely
to reuse buffers. Its responsibility is to decide how much reusable
byte-buffer capacity should be retained, where it should be retained, and when
it should be discarded.

The central model is:

> retain reusable byte-buffer capacity when it reduces future allocation cost,
> while keeping retained memory explicit, bounded, observable, and responsive
> to workload changes.

This overview is API-neutral. It does not define concrete public method names
for buffer acquisition, buffer return, ownership handles, or lifecycle
operations. Public contracts belong to reference documentation.

## Applicability

The runtime is intended for workloads where temporary byte-buffer allocation is
a material part of system cost.

Representative areas include:

- proxies and gateways;
- serialization, deserialization, compression, and transformation paths;
- telemetry, log, and event ingestion;
- stream processing and protocol handling;
- storage, cache, and batching layers;
- multi-tenant systems that need memory-retention isolation.

The relevant signal is allocation throughput, not request rate alone:

$$
allocationThroughput =
requestsPerSecond \cdot allocatedBytesPerRequest
$$

A high-rate service with tiny temporary buffers may not need a dedicated
retention runtime. A moderate-rate service that repeatedly allocates large
temporary buffers may benefit substantially.

### Good fit

Use this project when:

- temporary byte-buffer allocation is visible in profiles or memory telemetry;
- requested capacities repeat enough for reuse to matter;
- retained memory must be bounded by explicit policy;
- different workload domains need separate retention behavior;
- memory pressure should reduce retention predictably;
- runtime behavior must be explainable through metrics, snapshots, and drop reasons.

### Poor fit

The project is probably unnecessary when:

- byte-buffer allocation is not a measurable cost;
- temporary buffers are small, rare, or not repeatedly shaped;
- streaming or caller-owned buffers remove the allocation pattern;
- opportunistic temporary reuse is sufficient;
- the real bottleneck is outside allocation and retention behavior.

Scope exclusions and proposal boundaries are owned by
[Goals and Non-goals](./goals-and-non-goals.md).

## Runtime responsibility

The runtime treats pooling as memory-retention governance.

| Responsibility | Overview-level meaning |
| --- | --- |
| Size-aware reuse | Similar requested sizes are normalized into reusable capacity groups. |
| Bounded retention | Retained capacity is constrained by explicit memory policy. |
| Admission control | Returned buffers are retained only when policy allows retention. |
| Explicit ownership/accounting | Configured modes can account for retained, checked-out, and reserved memory. |
| Pool-based isolation | Workload domains are separated so retention decisions do not collapse into one flat store. |
| Workload-driven control | Recent demand can influence retention targets without making old history permanent truth. |
| Pressure-aware contraction | Lower targets or pressure can reduce future retention and trigger bounded trimming. |
| Observability | Retention, drops, pressure, trim, and controller state should be explainable. |
| Lifecycle control | Background control work must have explicit ownership and shutdown behavior. |

The runtime is not a Go allocator replacement, a process-wide memory manager,
an ordinary cache, or a generic object-pooling framework. Those boundaries are
defined normatively in [Goals and Non-goals](./goals-and-non-goals.md).

## High-level model

At the top level, the runtime separates memory governance into layered domains:

```text
PoolGroup
└── PoolPartition
    └── Pool
        └── SizeClass
            └── Shard
                └── Bucket
```

| Concept | Overview-level role |
| --- | --- |
| `PoolGroup` | Top-level memory-governance and lifecycle domain. |
| `PoolPartition` | Control-plane ownership unit for a subset of pools. |
| `Pool` | Workload, component, tenant, or usage-domain isolation unit. |
| `SizeClass` | Capacity normalization boundary for compatible buffer sizes. |
| `Shard` | Contention-reduction boundary on the fast path. |
| `Bucket` | Local storage for retained reusable capacity. |

This hierarchy gives the runtime places to answer different questions:

- how much memory may the whole runtime retain;
- how retention budget is divided across partitioned control work;
- which workload domains deserve separate retention behavior;
- which capacity ranges are worth retaining;
- how hot-path contention is reduced without global coordination.

Detailed component responsibilities belong to
[Architecture](./architecture/index.md). Policy rules belong to
[Policies](./policies/index.md). Vocabulary belongs to
[Terminology](./terminology.md).

## Data path and control path

The runtime separates immediate buffer operations from adaptive management.

The data path serves ordinary buffer lifecycle operations using local state and
the latest published policy state. It should remain narrow, predictable, and
free of per-operation global coordination.

The control path observes workload, interprets recent signals, redistributes
retention targets, handles pressure policy, publishes policy state, and
performs bounded trim work.

The data path must remain safe if the control path is delayed. Delayed control
work may reduce adaptation quality, but ordinary runtime safety must not depend
on perfect controller punctuality.

## Workload stance

The runtime is workload-driven rather than static.

Under a stable workload, it should move from cold start through warm-up toward
a bounded steady operating range. Under workload shifts, old observations must
decay so retention policy can re-adapt.

The goal is not to retain all buffers or eliminate all allocations. The goal is
to reduce useful allocation pressure while keeping memory retention bounded and
diagnosable.

## Next reading

| Need | Continue with |
| --- | --- |
| Scope boundaries and accepted trade-offs | [Goals and Non-goals](./goals-and-non-goals.md) |
| Canonical vocabulary | [Terminology](./terminology.md) |
| Runtime component boundaries | [Architecture](./architecture/index.md) |
| Retention, admission, pressure, and trim rules | [Policies](./policies/index.md) |
| Workload observation and scoring | [Workload](./workload/index.md) |
| Production configuration and diagnostics | [Operations](./operations/index.md) |
| Public contracts | [Reference](./reference/index.md) |
| Evidence for performance claims | [Performance](./performance/index.md) |
| Capability sequencing | [Roadmap](./roadmap.md) |
