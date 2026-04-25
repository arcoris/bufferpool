<div align="center">

# arcoris.dev/bufferpool Docs

**Repository documentation entry point for the adaptive byte-buffer memory-retention runtime.**

[![README](https://img.shields.io/badge/Landing-README-0F766E?style=flat)](../README.md)
[![Overview](https://img.shields.io/badge/Start-Overview-1D4ED8?style=flat)](./overview.md)
[![Scope](https://img.shields.io/badge/Scope-Goals%20%26%20Non--goals-0F172A?style=flat)](./goals-and-non-goals.md)
[![Maturity](https://img.shields.io/badge/Readiness-Maturity%20Model-7C2D12?style=flat)](./maturity-model.md)
[![Roadmap](https://img.shields.io/badge/Plan-Roadmap-B45309?style=flat)](./roadmap.md)

[README](../README.md) · [Overview](./overview.md) · [Goals and Non-goals](./goals-and-non-goals.md) · [Terminology](./terminology.md) · [Maturity Model](./maturity-model.md) · [Roadmap](./roadmap.md) · [Architecture](./architecture/index.md) · [Policies](./policies/index.md) · [Operations](./operations/index.md) · [Reference](./reference/index.md)

Size-aware reuse · Bounded memory retention · Explicit ownership/accounting · Pool-based partitions · Workload-driven control

**Start:** [Navigation scope](#navigation-scope) · [Read by goal](#read-by-goal) · [Common reading paths](#common-reading-paths) · [Document map](#document-map) · [Source of truth](#source-of-truth)

</div>

## Navigation scope

This page is the repository-facing documentation entry point only.

It does not define project scope, runtime vocabulary, public API shape,
architecture, policy behavior, maturity status, or implementation design. Use
the links below to reach the document that owns each topic.

## Read by goal

| If you want to... | Read first | Then continue with |
| --- | --- | --- |
| evaluate the project quickly | [README](../README.md) | [Overview](./overview.md), [Goals and Non-goals](./goals-and-non-goals.md) |
| understand whether the runtime applies to a workload | [Overview](./overview.md) | [Concepts](./concepts/index.md), [Operations](./operations/index.md) |
| review scope before changing behavior | [Goals and Non-goals](./goals-and-non-goals.md) | [Rationale](./rationale/index.md), [ADRs](./adr/index.md) |
| use consistent project language | [Terminology](./terminology.md) | [Concepts](./concepts/index.md), [Architecture](./architecture/index.md) |
| understand runtime boundaries | [Architecture](./architecture/index.md) | [Policies](./policies/index.md), [Workload](./workload/index.md) |
| understand retention and pressure rules | [Policies](./policies/index.md) | [Workload](./workload/index.md), [Operations](./operations/index.md) |
| inspect readiness criteria | [Maturity Model](./maturity-model.md) | [Performance](./performance/index.md), [Operations](./operations/index.md) |
| inspect planned capability sequencing | [Roadmap](./roadmap.md) | [Maturity Model](./maturity-model.md), [ADRs](./adr/index.md) |
| operate or tune the runtime | [Operations](./operations/index.md) | [Reference](./reference/index.md), [Performance](./performance/index.md) |
| verify exact user-facing contracts | [Reference](./reference/index.md) | [Compatibility](./reference/compatibility.md), [API](./reference/api.md) |
| validate performance claims | [Performance](./performance/index.md) | [Methodology](./performance/methodology.md), [Interpretation Guide](./performance/interpretation-guide.md) |

## Common reading paths

| Goal | Recommended path | Outcome |
| --- | --- | --- |
| first-time evaluation | [README](../README.md) -> [Overview](./overview.md) -> [Goals and Non-goals](./goals-and-non-goals.md) -> [Terminology](./terminology.md) | understand purpose, applicability, scope, and vocabulary |
| architecture review | [Overview](./overview.md) -> [Architecture](./architecture/index.md) -> [Policies](./policies/index.md) -> [Rationale](./rationale/index.md) -> [ADRs](./adr/index.md) | inspect runtime boundaries, policy ownership, and durable decisions |
| behavior or policy change | [Goals and Non-goals](./goals-and-non-goals.md) -> [Terminology](./terminology.md) -> [Policies](./policies/index.md) -> [Workload](./workload/index.md) | keep changes aligned with scope, vocabulary, and adaptive behavior |
| readiness review | [Maturity Model](./maturity-model.md) -> [Performance](./performance/index.md) -> [Operations](./operations/index.md) -> [Reference](./reference/index.md) | decide whether claims are supported by evidence and contracts |
| roadmap planning | [Roadmap](./roadmap.md) -> [Maturity Model](./maturity-model.md) -> [ADRs](./adr/index.md) | sequence capability work by dependency and promotion gates |

## Document map

| Document | Role |
| --- | --- |
| [README](../README.md) | public landing page and quick orientation |
| [Overview](./overview.md) | project purpose, applicability, runtime responsibility, and high-level model |
| [Goals and Non-goals](./goals-and-non-goals.md) | scope boundaries, accepted trade-offs, and in/out-of-scope rules |
| [Terminology](./terminology.md) | canonical glossary and avoided vocabulary |
| [Maturity Model](./maturity-model.md) | readiness levels, evidence gates, and production-readiness criteria |
| [Roadmap](./roadmap.md) | capability phases, dependencies, release gates, and risks |
| [Concepts](./concepts/index.md) | conceptual foundations for allocation pressure, retention, ownership, and workload behavior |
| [Architecture](./architecture/index.md) | runtime component boundaries, lifecycle, control plane, data plane, and failure model |
| [Policies](./policies/index.md) | admission, retention, pressure, trim, ownership, zeroing, and policy update rules |
| [Workload](./workload/index.md) | workload observation, statistics, scoring, cadence, and shifts |
| [Design](./design/index.md) | component-level design beneath architecture and policy boundaries |
| [Operations](./operations/index.md) | configuration, sizing, tuning, observability, troubleshooting, and adoption guidance |
| [Reference](./reference/index.md) | public contracts, options, metrics, errors, drop reasons, and compatibility |
| [Performance](./performance/index.md) | benchmark methodology, workload profiles, reports, and interpretation rules |
| [Rationale](./rationale/index.md) | alternatives, prior art, rejected approaches, and trade-off analysis |
| [ADRs](./adr/index.md) | durable architectural decisions and consequences |
| [Diagrams](./diagrams/index.md) | maintained diagram sources and rendered architecture/runtime views |

## Source of truth

- [Overview](./overview.md) owns project purpose, applicability, and the high-level runtime model.
- [Goals and Non-goals](./goals-and-non-goals.md) owns project scope and proposal boundaries.
- [Terminology](./terminology.md) owns canonical vocabulary.
- [Maturity Model](./maturity-model.md) owns readiness labels, evidence gates, and production-readiness criteria.
- [Roadmap](./roadmap.md) owns capability sequencing, dependencies, gates, and roadmap risks.
- [Architecture](./architecture/index.md) owns runtime responsibility boundaries.
- [Policies](./policies/index.md) owns retention, admission, pressure, trim, ownership, and policy-update behavior.
- [Workload](./workload/index.md) owns observation, scoring, convergence, and workload-shift semantics.
- [Design](./design/index.md) owns component-level design beneath the architecture.
- [Operations](./operations/index.md) owns production configuration and diagnostic guidance.
- [Reference](./reference/index.md) owns exact user-facing contracts.
- [Performance](./performance/index.md) owns benchmark evidence and interpretation rules.
- [ADRs](./adr/index.md) own durable decisions that change or constrain the design.
