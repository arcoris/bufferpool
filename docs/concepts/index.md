<div align="center">

# arcoris.dev/bufferpool Concepts

**Conceptual foundation for the adaptive byte-buffer memory-retention runtime.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Problem Space](https://img.shields.io/badge/Start-Problem%20Space-1D4ED8?style=flat)](./problem-space.md)
[![Retention](https://img.shields.io/badge/Core-Memory%20Retention-0F172A?style=flat)](./memory-retention.md)
[![Control](https://img.shields.io/badge/Planes-Hot%20%26%20Cold%20Path-7C2D12?style=flat)](./hot-path-and-cold-path.md)
[![Adaptation](https://img.shields.io/badge/Feedback-Adaptive%20Retention-B45309?style=flat)](./adaptive-retention.md)

[Docs](../index.md) · [Overview](../overview.md) · [Goals and Non-goals](../goals-and-non-goals.md) · [Terminology](../terminology.md) · [Architecture](../architecture/index.md) · [Policies](../policies/index.md) · [Workload](../workload/index.md) · [Rationale](../rationale/index.md) · [Reference](../reference/index.md)

Allocation pressure · Memory retention · Size classes · Explicit ownership/accounting · Pool-based partitions · Workload-driven control

**Start:** [Concept scope](#concept-scope) · [Read by goal](#read-by-goal) · [Common reading paths](#common-reading-paths) · [Concept map](#concept-map) · [Source of truth](#source-of-truth)

</div>

## Concept scope

This page is the entry point for the `docs/concepts` layer.

The concept layer explains the mental model behind `arcoris.dev/bufferpool`: why
the project exists, what problem it solves, how buffer pooling is modeled as
bounded memory retention, and how the major runtime concepts relate to each
other.

Use this section to understand the project before reading architecture, policy,
workload, design, operations, reference, or performance documents.

Concept documents do not define public API names, implementation layout, exact
policy algorithms, benchmark thresholds, or production defaults. Those belong to
the lower-level documentation areas linked below.

Concept pages stay API-neutral. They use terms such as buffer acquisition,
buffer return, checked-out buffer, returned capacity, retained capacity,
ownership-aware mode, pressure signaling, and trim request instead of concrete
public method names.

## Read by goal

| If you want to... | Read first | Then continue with |
| --- | --- | --- |
| understand why the project exists | [Problem Space](./problem-space.md) | [Allocation Pressure](./allocation-pressure.md), [Buffer Pooling](./buffer-pooling.md) |
| decide whether a workload may benefit | [Allocation Pressure](./allocation-pressure.md) | [Memory Retention](./memory-retention.md), [Operations](../operations/index.md) |
| understand what buffer pooling means here | [Buffer Pooling](./buffer-pooling.md) | [Memory Retention](./memory-retention.md), [sync.Pool Limitations](./sync-pool-limitations.md) |
| understand why this is memory retention, not cache eviction | [Memory Retention](./memory-retention.md) | [Size Classes](./size-classes.md), [Memory Pressure](./memory-pressure.md) |
| understand size-aware reuse | [Size Classes](./size-classes.md) | [Buffer Growth](./buffer-growth.md), [Shards and Buckets](./shards-and-buckets.md) |
| understand why returned buffers may be rejected | [Buffer Growth](./buffer-growth.md) | [Explicit Ownership](./explicit-ownership.md), [Policies](../policies/index.md) |
| understand ownership and accounting | [Explicit Ownership](./explicit-ownership.md) | [Memory Retention](./memory-retention.md), [Reference](../reference/index.md) |
| understand the fast path and control path split | [Hot Path and Cold Path](./hot-path-and-cold-path.md) | [Pool Partitions](./pool-partitions.md), [Adaptive Retention](./adaptive-retention.md) |
| understand top-level memory governance | [Pool Groups](./pool-groups.md) | [Pool Partitions](./pool-partitions.md), [Architecture](../architecture/index.md) |
| understand control-plane scaling | [Pool Partitions](./pool-partitions.md) | [Adaptive Retention](./adaptive-retention.md), [Workload](../workload/index.md) |
| understand local contention and storage | [Shards and Buckets](./shards-and-buckets.md) | [Size Classes](./size-classes.md), [Design](../design/index.md) |
| understand workload-driven adaptation | [Adaptive Retention](./adaptive-retention.md) | [Workload](../workload/index.md), [Policies](../policies/index.md) |
| understand behavior under memory pressure | [Memory Pressure](./memory-pressure.md) | [Policies](../policies/index.md), [Operations](../operations/index.md) |
| understand why `sync.Pool` is not enough | [sync.Pool Limitations](./sync-pool-limitations.md) | [Rationale](../rationale/index.md), [Performance](../performance/index.md) |

## Common reading paths

| Goal | Recommended path | Outcome |
| --- | --- | --- |
| first-time concept pass | [Problem Space](./problem-space.md) -> [Allocation Pressure](./allocation-pressure.md) -> [Buffer Pooling](./buffer-pooling.md) -> [Memory Retention](./memory-retention.md) | understand the problem, applicability, and central retention model |
| memory-safety review | [Memory Retention](./memory-retention.md) -> [Size Classes](./size-classes.md) -> [Buffer Growth](./buffer-growth.md) -> [Explicit Ownership](./explicit-ownership.md) | understand how retained capacity, growth, and accounting stay bounded |
| runtime structure review | [Hot Path and Cold Path](./hot-path-and-cold-path.md) -> [Pool Groups](./pool-groups.md) -> [Pool Partitions](./pool-partitions.md) -> [Shards and Buckets](./shards-and-buckets.md) | understand the hierarchy and the separation between control-plane ownership and data-path storage |
| adaptive behavior review | [Adaptive Retention](./adaptive-retention.md) -> [Memory Pressure](./memory-pressure.md) -> [Workload](../workload/index.md) -> [Policies](../policies/index.md) | understand feedback control, target redistribution, pressure response, and policy ownership |
| alternative analysis | [Buffer Pooling](./buffer-pooling.md) -> [sync.Pool Limitations](./sync-pool-limitations.md) -> [Rationale](../rationale/index.md) -> [Performance](../performance/index.md) | understand why the project is not a thin `sync.Pool` wrapper and how claims should be validated |
| implementation preparation | [Concepts](./index.md) -> [Architecture](../architecture/index.md) -> [Policies](../policies/index.md) -> [Design](../design/index.md) -> [Reference](../reference/index.md) | move from mental model to component boundaries, behavior rules, design details, and user-facing contracts |

## Concept map

| Concept | Role |
| --- | --- |
| [Problem Space](./problem-space.md) | explains the domain, workload shapes, and why reusable byte-buffer capacity can matter |
| [Allocation Pressure](./allocation-pressure.md) | owns allocation throughput, RPS vs allocation pressure, measurement signals, and benchmark indicators |
| [Buffer Pooling](./buffer-pooling.md) | defines buffer pooling as this project's umbrella concept and routes readers to specialized concepts |
| [Memory Retention](./memory-retention.md) | owns retained capacity, retained/in-use/reserved memory, admission, drop, trim, and contraction concepts |
| [Size Classes](./size-classes.md) | owns requested size, class size, actual capacity, class mapping, fragmentation, and class-level retention context |
| [Buffer Growth](./buffer-growth.md) | owns length/capacity distinction, grown buffers, growth ratio, and returned-capacity validation |
| [Explicit Ownership](./explicit-ownership.md) | owns checked-out buffer lifecycle, ownership-aware accounting, misuse model, and exact/approximate accounting boundaries |
| [Hot Path and Cold Path](./hot-path-and-cold-path.md) | owns data/control separation, snapshot semantics, controller lag, active registries, and bounded control work |
| [Pool Groups](./pool-groups.md) | owns the top-level memory-governance and lifecycle domain concept |
| [Pool Partitions](./pool-partitions.md) | owns pool-based control-plane partitioning, partition controller scope, assignment, and partition-local state |
| [Shards and Buckets](./shards-and-buckets.md) | owns data-path striping, segmented bounded LIFO bucket storage, local credits, bounded fallback, and slot clearing |
| [Adaptive Retention](./adaptive-retention.md) | owns feedback loops, decayed workload signals, scoring concepts, target redistribution, convergence, and workload shifts |
| [Memory Pressure](./memory-pressure.md) | owns pressure levels, pressure sources, conservative retention, contraction, recovery, and pressure observability |
| [sync.Pool Limitations](./sync-pool-limitations.md) | alternatives-style concept page explaining why `sync.Pool` is not the deterministic core retained-storage model |

## Source of truth

| Topic | Owner |
| --- | --- |
| Concept navigation and reading order | this document |
| Project problem domain and applicability | [Problem Space](./problem-space.md) |
| Allocation throughput and measurement signals | [Allocation Pressure](./allocation-pressure.md) |
| Project-specific meaning of buffer pooling | [Buffer Pooling](./buffer-pooling.md) |
| Memory-retention lifecycle | [Memory Retention](./memory-retention.md) |
| Retained, in-use, and reserved memory concepts | [Memory Retention](./memory-retention.md) |
| Requested size, class size, and actual capacity | [Size Classes](./size-classes.md) |
| Capacity growth and returned-capacity validation | [Buffer Growth](./buffer-growth.md) |
| Ownership/accounting semantics | [Explicit Ownership](./explicit-ownership.md) |
| Data path and control path separation | [Hot Path and Cold Path](./hot-path-and-cold-path.md) |
| Group-level memory governance | [Pool Groups](./pool-groups.md) |
| Pool-based control-plane ownership | [Pool Partitions](./pool-partitions.md) |
| Shard and bucket storage concepts | [Shards and Buckets](./shards-and-buckets.md) |
| Adaptive scoring and target redistribution concepts | [Adaptive Retention](./adaptive-retention.md) |
| Pressure response and recovery concepts | [Memory Pressure](./memory-pressure.md) |
| `sync.Pool` alternative analysis | [sync.Pool Limitations](./sync-pool-limitations.md) |

## Lower-level ownership

| Need | Continue with |
| --- | --- |
| exact runtime component boundaries | [Architecture](../architecture/index.md) |
| behavioral retention, admission, pressure, trim, ownership, and policy-update rules | [Policies](../policies/index.md) |
| workload observation, statistics, scoring, cadence, convergence, and shifts | [Workload](../workload/index.md) |
| alternatives, prior art, rejected approaches, and trade-off analysis | [Rationale](../rationale/index.md) |
| component-level design beneath architecture and policy boundaries | [Design](../design/index.md) |
| production configuration, sizing, tuning, observability, and troubleshooting | [Operations](../operations/index.md) |
| public contracts, options, metrics, errors, drop reasons, and compatibility | [Reference](../reference/index.md) |
| benchmark methodology, workload profiles, reports, and interpretation rules | [Performance](../performance/index.md) |

## Concept boundaries

Concept pages may introduce the mental model, responsibility boundaries, and
stable vocabulary for a topic.

Concept pages must not define:

- final public API names;
- concrete method signatures;
- exact Go structs or fields;
- exact goroutine implementation;
- exact source-code file layout;
- exact benchmark thresholds;
- final production defaults;
- implementation-specific algorithms beyond conceptual formulas.

When a concept needs lower-level detail, link to the owning documentation area
instead of duplicating it.
