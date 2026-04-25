<div align="center">

# Roadmap

**Capability roadmap for the adaptive byte-buffer memory-retention runtime.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](./index.md)
[![Overview](https://img.shields.io/badge/Overview-overview.md-1D4ED8?style=flat)](./overview.md)
[![Scope](https://img.shields.io/badge/Scope-Goals%20%26%20Non--goals-0F172A?style=flat)](./goals-and-non-goals.md)
[![Maturity](https://img.shields.io/badge/Readiness-Maturity%20Model-7C2D12?style=flat)](./maturity-model.md)
[![Performance](https://img.shields.io/badge/Evidence-Performance-B45309?style=flat)](./performance/index.md)

[Docs](./index.md) · [Overview](./overview.md) · [Goals and Non-goals](./goals-and-non-goals.md) · [Terminology](./terminology.md) · [Maturity Model](./maturity-model.md) · [Architecture](./architecture/index.md) · [Policies](./policies/index.md) · [Workload](./workload/index.md) · [Operations](./operations/index.md) · [Reference](./reference/index.md)

Bounded retention first · Partitioned control · Observable adaptation · Evidence gates · Production readiness

**Start:** [Purpose](#purpose) · [Roadmap principles](#roadmap-principles) · [Tracks](#tracks) · [Phases](#phases) · [Dependencies](#dependencies) · [Risk controls](#risk-controls)

</div>

## Purpose

This document owns capability sequencing for `arcoris.dev/bufferpool`.

The roadmap describes the order in which the project should mature from
documented scope to a production-ready adaptive byte-buffer memory-retention
runtime. It is capability-based, not date-based.

This document does not define public API names, implementation layout, concrete
algorithms, benchmark thresholds, or maturity criteria. Maturity levels and
promotion evidence are owned by [Maturity Model](./maturity-model.md).

## Roadmap principles

1. Build bounded retention before adaptive behavior.
2. Keep public API shape unsettled until runtime behavior and evidence are stable.
3. Treat observability as part of the capability, not a later decoration.
4. Require reproducible evidence before performance or production claims.
5. Add operational guidance before declaring production readiness.
6. Record major irreversible decisions in rationale or ADRs.

## Status model

Roadmap items use the maturity levels defined by
[Maturity Model](./maturity-model.md):

| Level | Meaning |
| ---: | --- |
| M0 | Concept |
| M1 | Design baseline |
| M2 | Prototype |
| M3 | Experimental |
| M4 | Stable |
| M5 | Production-ready |

A capability may advance independently of the whole repository. For example,
bounded retained storage can be experimental while workload adaptation is still
a design baseline.

## Tracks

| Track | Roadmap ownership |
| --- | --- |
| Documentation | Keep top-level purpose, scope, vocabulary, maturity, and roadmap aligned. |
| Core retention runtime | Provide size-aware bounded retained-buffer storage. |
| Ownership/accounting | Account for retained, checked-out, and reserved memory where configured. |
| Pool-based partitions | Establish control-plane ownership by pool partitions. |
| Control plane | Publish policy state, harvest workload signals, and perform bounded control work. |
| Workload adaptation | Score recent workload and redistribute retention targets. |
| Policy and pressure | Govern admission, retention limits, trimming, pressure, and contraction. |
| Observability | Explain runtime behavior through metrics, snapshots, and drop reasons. |
| Performance evidence | Measure allocation pressure, overhead, contention, trim, and controller cost. |
| Operations and reference | Provide production guidance and precise public contracts. |

## Current baseline

The accepted baseline for future work is:

- buffer-specific memory retention for allocation-heavy Go systems;
- size-aware reuse under explicit retained-memory bounds;
- explicit ownership/accounting semantics where configured;
- pool-based partitions as the control-plane scaling model;
- workload-driven control based on recent or decayed signals;
- observable drops, pressure, trim, retained memory, and controller behavior;
- no concrete public buffer operation names in top-level planning documents.

Scope constraints are owned by
[Goals and Non-goals](./goals-and-non-goals.md).

## Phases

### Phase 0: Top-level documentation baseline

Establish the repository narrative before lower-level work hardens accidental
contracts.

| Outcome | Gate |
| --- | --- |
| Purpose, scope, vocabulary, maturity, and roadmap pages have distinct roles. | No top-level page duplicates another page's source-of-truth responsibility. |
| Top-level documents stay API-neutral. | No concrete public names for buffer acquisition, return, ownership handles, or lifecycle operations. |
| Navigation is consistent. | Each top-level page has a centered header, badges, link row, compact tagline, and local start row. |
| Cross-links are stable. | Readers can move from purpose to scope, vocabulary, readiness, and roadmap without file-tree scanning. |

### Phase 1: Static bounded retention core

Build a safe non-adaptive retention foundation.

| Capability | Target |
| --- | --- |
| Size-aware capacity normalization | M2 |
| Bounded retained storage | M2 |
| Returned-buffer admission | M2 |
| Retained-memory accounting | M2 |
| Lifecycle basics | M2 |
| Core storage and admission tests | M2 |
| Allocation and overhead smoke benchmarks | M2 |

Exit gate:

- retained memory cannot grow without explicit limits;
- returned buffers can be rejected by policy;
- retained storage can be emptied safely;
- behavior is testable without adaptive control;
- benchmark smoke tests compare against meaningful simple baselines.

### Phase 2: Ownership and accounting model

Add accounting strength without forcing every workload into the strictest mode.

| Capability | Target |
| --- | --- |
| Retained memory accounting | M3 |
| Checked-out memory accounting where configured | M2 |
| Reserved memory reporting where supported | M2 |
| Returned-capacity validation | M2 |
| Strict checking semantics where supported | M2 |
| Accounting tests and metrics definitions | M2 |

Exit gate:

- retained, checked-out, and reserved memory terms are unambiguous;
- accounting modes have documented cost and guarantees;
- returned-capacity behavior is tested;
- top-level docs remain API-neutral while reference/API design owns concrete names.

### Phase 3: Pool-based memory-governance hierarchy

Introduce group and partition ownership boundaries.

| Capability | Target |
| --- | --- |
| `PoolGroup` lifecycle model | M2 |
| `PoolPartition` ownership model | M2 |
| Stable pool-to-partition assignment | M2 |
| Partition-local state isolation | M2 |
| Group and partition snapshots | M2 |
| Partition lifecycle tests | M2 |

Exit gate:

- each pool has exactly one partition owner;
- partition state can be inspected without crossing ownership boundaries;
- shutdown is deterministic;
- architecture rejects class-based and shard-based control-plane partitioning as the baseline.

### Phase 4: Control-plane skeleton

Create bounded control infrastructure before adding advanced scoring.

| Capability | Target |
| --- | --- |
| Partition controller lifecycle | M2 |
| Counter harvesting | M2 |
| Active-entry tracking | M2 |
| Policy snapshot publication | M2 |
| Controller lag and snapshot age metrics | M2 |
| Bounded work per control cycle | M2 |

Exit gate:

- the data path remains safe while controllers are delayed;
- control cycles account for elapsed time;
- normal control work avoids full scans;
- policy state can be published and observed;
- controller work has explicit lifecycle ownership.

### Phase 5: Workload-driven adaptation

Redistribute retention targets according to recent memory-demand signals.

| Capability | Target |
| --- | --- |
| Windowed workload signals | M3 |
| Decayed scoring model | M3 |
| Class, pool, and partition scores | M3 |
| Budget redistribution | M3 |
| Shard credits or equivalent local allowances | M3 |
| Convergence and workload-shift tests | M3 |
| Adaptive benchmark reports | M3 |

Exit gate:

- stable workloads converge to bounded operating ranges;
- workload shifts cause controlled re-adaptation;
- old workload does not dominate decisions indefinitely;
- target movement avoids expected oscillation;
- benchmarks reproduce the adaptive behavior being claimed.

### Phase 6: Pressure, contraction, and trim

Make retained-memory reduction predictable when budgets shrink or pressure
rises.

| Capability | Target |
| --- | --- |
| Manual trim behavior | M3 |
| Pressure levels | M3 |
| Policy contraction | M3 |
| Trim prioritization | M3 |
| Trim backlog observability | M3 |
| Pressure and trim overhead benchmarks | M3 |

Exit gate:

- lower targets restrict new retention promptly;
- existing retained memory moves toward lower targets through bounded work;
- checked-out buffers are not forcibly reclaimed;
- users can explain why memory may not shrink immediately;
- pressure behavior is tested and documented.

### Phase 7: Observability and reference contracts

Turn runtime behavior into diagnosable public or documented contracts.

| Capability | Target |
| --- | --- |
| Metrics model | M4 |
| Drop reasons | M4 |
| Runtime snapshots | M4 |
| Pressure, trim, and controller diagnostics | M4 |
| Public reference documentation | M4 |
| Compatibility language | M4 |

Exit gate:

- users can explain retained memory, drops, trim, pressure, and controller lag;
- metrics state whether values are exact, approximate, or best-effort;
- public contracts are precise enough for stable use;
- diagnostics do not require source-code reading.

### Phase 8: Performance evidence

Provide reproducible evidence for allocation, memory, and control-plane claims.

| Capability | Target |
| --- | --- |
| Benchmark methodology | M4 |
| Workload matrix | M4 |
| Baseline comparisons | M4 |
| Retention behavior reports | M4 |
| Controller and trim overhead reports | M4 |
| Interpretation guide | M4 |

Required benchmark dimensions include buffer sizes, workload stability,
burstiness, workload shifts, pool count, partition count, class count, shard
count, accounting mode, trim behavior, and pressure behavior.

Exit gate:

- benchmark commands are reproducible;
- reports include environment assumptions and interpretation limits;
- claims point to reports rather than intuition;
- allocation, retained memory, latency where applicable, and controller cost are visible.

### Phase 9: Operations and stable public contract

Make the runtime usable under documented constraints.

| Capability | Target |
| --- | --- |
| Configuration profiles | M4 |
| Capacity planning | M4 |
| Partition sizing guidance | M4 |
| Tuning guide | M4 |
| Troubleshooting guide | M4 |
| Production checklist | M4 |
| Stable package/reference contract | M4 |

Exit gate:

- users can choose an initial configuration profile;
- operators can interpret metrics and drop reasons;
- retained-memory behavior and pressure response are documented;
- public behavior is stable and compatibility expectations are clear.

### Phase 10: Production-ready release

Reach M5 for documented allocation-heavy byte-buffer workloads.

Production-ready release requires:

1. all stable release criteria are met for core capabilities;
2. production profiles and capacity planning exist;
3. troubleshooting and failure-mode documentation exist;
4. benchmark reports cover representative workloads and baselines;
5. public contracts and compatibility expectations are current;
6. known limitations are visible;
7. release criteria are repeatable.

## Dependencies

| Dependency | Reason |
| --- | --- |
| Scope before design | Design must not expand the project boundary accidentally. |
| Terminology before architecture | Architecture needs stable vocabulary. |
| Bounded core before adaptation | Adaptive control is unsafe without bounded retention. |
| Accounting before reserved-memory claims | Reserved memory requires enough ownership/accounting information. |
| Pool-based partitions before adaptive budgets | Budget redistribution needs ownership boundaries. |
| Control skeleton before scoring | Scores need a safe publication and cadence model. |
| Metrics before operations | Operators need signals before tuning guidance is useful. |
| Benchmark methodology before claims | Performance claims require reproducible measurement. |
| Reference before stable API | Stability requires precise public contracts. |
| ADRs before irreversible scope changes | Durable decisions need recorded context and consequences. |

## Release gates

| Gate | Minimum requirements |
| --- | --- |
| Experimental | Bounded retention, lifecycle tests, basic observability, benchmark smoke tests, documented caveats. |
| Stable | Stable public contract, documented configuration, metrics, drop reasons, lifecycle behavior, pressure/contraction behavior, reproducible benchmarks, operations guide, compatibility expectations. |
| Production-ready | Stable gate plus production profiles, capacity planning, troubleshooting, failure-mode documentation, benchmark interpretation guide, known limitations, release checklist, and ADR coverage for major decisions. |

Release gates refine the maturity criteria. They do not replace
[Maturity Model](./maturity-model.md).

## Risk controls

| Risk | Control |
| --- | --- |
| Scope expands into generic object pooling | Enforce [Goals and Non-goals](./goals-and-non-goals.md); require ADR for scope changes. |
| Public API freezes too early | Keep top-level planning API-neutral until behavior and evidence stabilize. |
| Adaptive policy hides unsafe retention | Complete bounded retention before adaptation. |
| Controller work leaks into the data path | Test and benchmark data/control separation. |
| Metrics become too high-cardinality | Define default aggregation and optional detail levels. |
| Benchmarks overfit one workload | Maintain a workload matrix and interpretation guide. |
| Retained memory fails to shrink predictably | Test contraction, pressure behavior, trim backlog, and checked-out memory explanations. |
| Partition imbalance hurts control latency | Provide partition sizing and assignment guidance. |
| Shard or bucket metadata becomes too expensive | Benchmark overhead across class and shard counts. |
| Pressure behavior surprises operators | Document pressure levels, drop reasons, trim behavior, and expected lag. |
| Implementation contradicts architecture | Update architecture and ADRs before merging contradictory behavior. |

## Deferred work

Deferred work may be revisited after the core runtime and evidence model are
stable.

| Area | Condition for reconsideration |
| --- | --- |
| Profile-guided warm start | Must not let stale workload dominate current decisions. |
| External pressure integrations | Must not turn the package into a process-wide memory manager. |
| Advanced affinity strategies | Must remain explainable and avoid private runtime dependencies for correctness. |
| Alternative storage policies | Must outperform or simplify the baseline under reproducible benchmarks. |
| Richer workload classification | Must remain observable and operationally useful. |

## Out-of-roadmap

The following are not planned unless project scope changes through ADR:

- generic object pooling;
- Go allocator replacement;
- process-wide memory manager behavior;
- ordinary cache replacement as the core model;
- goroutine per pool, class, shard, or timer;
- correctness based on private Go runtime internals;
- zero-allocation guarantees;
- automatic dynamic routing of every operation across pools;
- unbounded retained-memory modes.

## Review checklist

Before promoting a roadmap item, verify:

- it fits project scope;
- canonical terminology exists;
- architecture boundary exists;
- major trade-offs have rationale or ADR coverage;
- policy behavior is documented;
- expected and failure paths are tested;
- metrics explain the behavior;
- benchmarks cover relevant overhead;
- operations guidance exists if users configure or diagnose it;
- reference documentation exists if behavior is public;
- known limitations are documented.

## Source-of-truth boundary

| Topic | Owner |
| --- | --- |
| Capability sequencing, dependencies, and roadmap risks | This document |
| Readiness levels and promotion evidence | [Maturity Model](./maturity-model.md) |
| Scope boundaries | [Goals and Non-goals](./goals-and-non-goals.md) |
| Vocabulary | [Terminology](./terminology.md) |
| Runtime architecture | [Architecture](./architecture/index.md) |
| Policy behavior | [Policies](./policies/index.md) |
| Workload behavior | [Workload](./workload/index.md) |
| Operations guidance | [Operations](./operations/index.md) |
| Public contracts | [Reference](./reference/index.md) |
| Performance evidence | [Performance](./performance/index.md) |
