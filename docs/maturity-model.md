<div align="center">

# Maturity Model

**Readiness model for the adaptive byte-buffer memory-retention runtime.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](./index.md)
[![Overview](https://img.shields.io/badge/Overview-overview.md-1D4ED8?style=flat)](./overview.md)
[![Scope](https://img.shields.io/badge/Scope-Goals%20%26%20Non--goals-0F172A?style=flat)](./goals-and-non-goals.md)
[![Terminology](https://img.shields.io/badge/Terms-terminology.md-7C2D12?style=flat)](./terminology.md)
[![Performance](https://img.shields.io/badge/Evidence-Performance-B45309?style=flat)](./performance/index.md)

[Docs](./index.md) · [Overview](./overview.md) · [Goals and Non-goals](./goals-and-non-goals.md) · [Terminology](./terminology.md) · [Roadmap](./roadmap.md) · [Architecture](./architecture/index.md) · [Operations](./operations/index.md) · [Reference](./reference/index.md) · [Performance](./performance/index.md)

Evidence gates · Runtime safety · Public contracts · Operational readiness · Production criteria

**Start:** [Purpose](#purpose) · [Maturity levels](#maturity-levels) · [Readiness dimensions](#readiness-dimensions) · [Promotion gates](#promotion-gates) · [Production-ready criteria](#production-ready-criteria)

</div>

## Purpose

This document defines how readiness is judged for `arcoris.dev/bufferpool`.

It owns maturity levels, evidence gates, stability labels, and
production-readiness criteria. It does not define roadmap sequencing, public API
names, internal implementation design, benchmark thresholds, or release
mechanics.

The model exists so terms such as `stable`, `production-ready`,
`high-performance`, and `safe` are used only when evidence supports them.

## Maturity principles

1. Claims require evidence.
2. Scope and terminology must be stable before behavior is called stable.
3. Public contract stability is separate from internal implementation maturity.
4. Performance maturity is workload-specific.
5. Production readiness includes operability, not only tests.
6. Adaptive behavior must show convergence and controlled response to workload shifts.
7. A capability is only as mature as its weakest critical dimension.

## Maturity levels

| Level | Name | Meaning |
| ---: | --- | --- |
| M0 | Concept | Problem, vocabulary, and target scope are being shaped. |
| M1 | Design baseline | Responsibility boundaries and major decisions are documented well enough to guide implementation. |
| M2 | Prototype | Core behavior exists in executable form and can be validated locally. |
| M3 | Experimental | Behavior can be evaluated under controlled conditions with explicit caveats. |
| M4 | Stable | Public behavior is documented, tested, benchmarked, and suitable for normal use under documented constraints. |
| M5 | Production-ready | Stable behavior plus operational guidance, failure evidence, compatibility expectations, and release discipline. |

A maturity level applies to a capability area as well as to the repository as a
whole. Documentation, core retention, observability, performance evidence, and
operations may mature at different speeds.

## Level criteria

### M0: Concept

M0 requires:

- problem statement;
- intended audience;
- initial vocabulary;
- initial scope;
- known risks;
- relationship to similar approaches.

M0 must not claim stable public behavior, production readiness, or benchmark
superiority.

### M1: Design baseline

M1 requires:

- documented scope and non-scope;
- stable draft terminology;
- high-level runtime model;
- data-plane and control-plane boundary;
- memory-retention model;
- accepted and rejected alternatives;
- lifecycle expectations.

At M1, implementation may be incomplete and public API names may remain
undecided.

M1 must preserve buffer-specific scope, bounded retention, pool-based
partitions, data/control separation, API-neutral top-level documents, and the
rejection of generic object-pool storage as the core model.

### M2: Prototype

M2 requires executable behavior for the capability under review.

Evidence should include:

- tests for core invariants;
- tests for boundary conditions;
- basic lifecycle validation;
- early benchmark or smoke measurement where performance is relevant;
- documented limitations.

M2 must not introduce unbounded retention by default, hidden lifecycle work, or
per-operation global coordination as a required baseline.

### M3: Experimental

M3 means controlled evaluation is possible.

Evidence should include:

- coherent public contract draft where the capability is user-facing;
- documented configuration semantics or caveats;
- metrics or snapshots sufficient for debugging;
- benchmark methodology draft for performance claims;
- documented known limitations and failure modes.

M3 must not advertise general production readiness or present benchmark results
without baselines and interpretation limits.

### M4: Stable

M4 means normal use is acceptable under documented constraints.

Evidence must include:

- documented public behavior;
- stable configuration semantics;
- tested expected and failure paths;
- documented lifecycle behavior;
- documented metrics, drop reasons, and relevant errors;
- reproducible benchmarks for supported workloads;
- documented compatibility expectations;
- operational guidance.

M4 must not leave public behavior, lifecycle ownership, drop behavior, retained
memory limits, or major failure modes undocumented.

### M5: Production-ready

M5 means production use is supported for documented allocation-heavy
byte-buffer workloads and supported configuration profiles.

Evidence must include all M4 evidence plus:

- production configuration guidance;
- capacity-planning guidance;
- partition-sizing guidance where partitioning is user-visible;
- pressure and contraction guidance;
- troubleshooting guidance;
- failure-mode documentation;
- repeatable benchmark reports with meaningful baselines;
- compatibility and upgrade expectations;
- release checklist or equivalent release criteria;
- documented known limitations.

M5 does not mean the runtime is appropriate for every Go service.

## Readiness dimensions

| Dimension | Readiness question |
| --- | --- |
| Scope | Is the capability within [Goals and Non-goals](./goals-and-non-goals.md)? |
| Terminology | Are canonical terms defined and used consistently? |
| Architecture | Is the responsibility boundary documented? |
| Rationale | Are alternatives and trade-offs recorded? |
| Policy | Are behavior rules documented? |
| Workload | Are observation, scoring, convergence, and shifts described where relevant? |
| Implementation | Does executable behavior match the documented model? |
| Testing | Are expected paths, boundaries, concurrency, lifecycle, and failure cases covered? |
| Performance | Are claims supported by reproducible methodology and baselines? |
| Observability | Can users explain runtime behavior from documented signals? |
| Operations | Can users configure, size, tune, and diagnose the capability? |
| Reference | Are public contracts precise? |
| Compatibility | Are stability and change expectations clear? |

The maturity of a capability should not exceed the lowest maturity of its
critical dimensions.

## Promotion gates

| Promotion | Gate |
| --- | --- |
| M0 -> M1 | Scope, vocabulary, main risks, alternatives, and responsibility boundary are documented. |
| M1 -> M2 | Prototype exists, key invariants are testable, and obvious scope violations are absent. |
| M2 -> M3 | Controlled evaluation is possible, caveats are documented, and basic observability exists. |
| M3 -> M4 | Public behavior, tests, metrics, lifecycle, configuration, and reproducible benchmarks are stable enough for normal use. |
| M4 -> M5 | Operational guidance, production profiles, capacity planning, troubleshooting, compatibility expectations, and production evidence exist. |

Promotion is evidence-based. A capability does not advance because code exists;
it advances when its documented behavior and evidence support the next level.

## Evidence requirements

| Area | Evidence required for stability |
| --- | --- |
| Scope | Current goals/non-goals and ADRs for scope-changing decisions. |
| Terminology | Stable glossary and consistent usage across top-level docs. |
| Architecture | Responsibility boundaries, lifecycle model, invariants, and failure model. |
| Rationale | Alternatives, rejected approaches, and trade-off analysis. |
| Policy | Admission, retention, trim, pressure, ownership/accounting, and update rules. |
| Workload | Signals, windows, scoring, cadence, convergence, and workload-shift behavior. |
| Testing | Unit, concurrency, lifecycle, pressure, policy, and failure tests as applicable. |
| Performance | Reproducible methodology, baselines, reports, and interpretation limits. |
| Observability | Metrics, snapshots, drop reasons, pressure, trim, and controller state. |
| Operations | Configuration, tuning, capacity planning, pressure handling, and troubleshooting. |
| Reference | API, options, metrics, errors, drop reasons, ownership modes, and compatibility. |

## Production-ready criteria

The project may be described as production-ready only when all of the following
are true for the supported workload and configuration scope:

1. Project scope is stable and current.
2. Public behavior is documented.
3. Retained memory is bounded by explicit policy.
4. Data-path safety does not depend on control-path punctuality.
5. Pool-based partitioning is documented and tested where enabled.
6. Pressure and contraction behavior are documented and tested.
7. Metrics explain retention, drops, trim, pressure, and controller behavior.
8. Benchmark reports compare against meaningful baselines.
9. Operational guidance explains configuration, sizing, tuning, and troubleshooting.
10. Compatibility and upgrade expectations are documented.
11. Major design decisions have rationale or ADR coverage.
12. Known limitations are visible to users.

Correct claim:

> Production-ready for documented allocation-heavy byte-buffer workloads under
> supported configuration profiles.

Incorrect claim:

> Production-ready for all Go services.

## Stability labels

| Label | Meaning |
| --- | --- |
| `draft` | Content or behavior is being shaped and may change substantially. |
| `experimental` | Usable for controlled evaluation, but not stable. |
| `stable` | Public behavior is documented, tested, and expected to remain compatible. |
| `production-ready` | Stable plus operational guidance, evidence, and release discipline. |
| `deprecated` | Still documented, but not recommended for new work. |
| `removed` | No longer supported except as historical context. |

A document label must not exceed the maturity of the behavior it describes.

## Capability tracking

Track maturity by capability area rather than assuming the whole repository has
one uniform status.

| Capability area | Typical evidence before M4 |
| --- | --- |
| Scope and positioning | Stable overview, goals/non-goals, terminology, and ADR alignment. |
| Core retention runtime | Tests for bounded retention, admission, accounting, lifecycle, and failure paths. |
| Ownership/accounting | Documented modes, accounting semantics, tests, and metrics. |
| Pool-based partitions | Documented ownership model, lifecycle tests, metrics, and operational guidance. |
| Workload adaptation | Signal definitions, decayed scoring, convergence evidence, and shift behavior. |
| Pressure and contraction | Documented pressure levels, shrink behavior, trim backlog, and failure cases. |
| Observability | Metrics and snapshots that explain runtime behavior without source reading. |
| Performance evidence | Reproducible benchmark matrix, baselines, reports, and interpretation guide. |
| Operations | Configuration profiles, capacity planning, troubleshooting, and production checklist. |
| Public contracts | Stable reference docs, compatibility policy, and public behavior tests. |

## Regression rules

A capability may regress in maturity if:

- public behavior changes without documentation;
- tests no longer cover critical invariants;
- benchmark evidence becomes stale or unreproducible;
- implementation contradicts documented architecture;
- retained memory can grow without policy bounds;
- observability no longer explains important behavior;
- lifecycle ownership becomes ambiguous;
- known limitations are removed without resolution.

Regressions must be visible in project tracking, release notes, or relevant
documentation.

## Source-of-truth boundary

| Topic | Owner |
| --- | --- |
| Capability sequencing | [Roadmap](./roadmap.md) |
| Readiness levels and gates | This document |
| Scope constraints | [Goals and Non-goals](./goals-and-non-goals.md) |
| Runtime architecture | [Architecture](./architecture/index.md) |
| Public contracts | [Reference](./reference/index.md) |
| Performance evidence | [Performance](./performance/index.md) |
| Production operation | [Operations](./operations/index.md) |
