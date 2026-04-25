<div align="center">

# Explicit Ownership

**Concept model for checked-out buffers, ownership-aware accounting, and misuse boundaries.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Retention](https://img.shields.io/badge/Core-Memory%20Retention-0F172A?style=flat)](./memory-retention.md)
[![Growth](https://img.shields.io/badge/Growth-Validation-7C2D12?style=flat)](./buffer-growth.md)
[![Reference](https://img.shields.io/badge/Contracts-Reference-B45309?style=flat)](../reference/index.md)

[Docs](../index.md) · [Concepts](./index.md) · [Memory Retention](./memory-retention.md) · [Buffer Growth](./buffer-growth.md) · [Terminology](../terminology.md) · [Reference](../reference/index.md)

Checked-out buffers · In-use memory · Reserved memory · Origin context · Exact vs approximate accounting

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Ownership states](#ownership-states) · [Accounting precision](#accounting-precision) · [Misuse model](#misuse-model) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns explicit ownership/accounting concepts for
`arcoris.dev/bufferpool`.

It explains checked-out buffers, in-use and reserved memory, origin context,
misuse boundaries, and exact versus approximate accounting. It does not define
public ownership handle names or lifecycle method names.

## Core idea

Retained memory is only part of the memory picture.

When a buffer is checked out to caller code, the runtime may no longer hold it
in retained storage, but the backing capacity can still matter for accounting.
Ownership-aware modes let the runtime reason about checked-out capacity and
returned-capacity validation more precisely.

## Ownership states

| State | Meaning |
| --- | --- |
| Retained | Capacity is held by runtime storage for future reuse. |
| Checked out | Capacity is held by caller code and not currently retained. |
| Returned | Capacity has been given back for admission or drop. |
| Dropped | Returned capacity was not retained. |

The concept layer intentionally avoids concrete public operation names.

## Accounting precision

Accounting can be exact or approximate depending on configured mode and
available ownership context.

| Mode shape | Conceptual capability | Cost |
| --- | --- | --- |
| Retained-only | Tracks memory held in runtime storage. | Lowest accounting overhead. |
| Ownership-aware | Tracks checked-out capacity and origin context where configured. | More bookkeeping. |
| Strict checking | May detect repeated returns or ownership misuse where supported. | Highest validation cost. |

Exact mode names and public contracts belong to
[Reference](../reference/index.md).

## Retained, in-use, and reserved memory

Memory terms are defined by [Memory Retention](./memory-retention.md):

- retained memory is capacity in runtime storage;
- in-use memory is checked-out capacity when accounting can track it;
- reserved memory is retained plus in-use memory when both are known.

Ownership-aware accounting makes in-use and reserved reporting possible. It
does not mean the runtime can reclaim checked-out buffers.

## Origin context

Origin context is information associated with a checked-out buffer that may help
the runtime evaluate it later.

Examples of conceptual origin context:

- expected class;
- capacity at checkout;
- accounting generation;
- ownership state;
- policy context needed for validation.

This context can improve [Buffer Growth](./buffer-growth.md) validation without
turning concept docs into API design.

## Misuse model

Explicit ownership clarifies misuse boundaries.

Conceptual misuse includes:

- returning the same checked-out buffer more than once;
- using a buffer after ownership has been transferred back;
- returning capacity that no longer matches the expected origin relationship;
- attempting to treat retained capacity as caller-owned;
- assuming strict validation is free in all modes.

The runtime may detect some misuse only in stricter modes. Concept pages should
not promise detection that the reference contract does not define.

## Relationship to pressure

Checked-out buffers are not retained storage and must not be forcibly reclaimed
under pressure.

Ownership-aware accounting can explain why retained memory shrinks while
reserved memory remains high: capacity may still be checked out. Pressure
behavior is owned by [Memory Pressure](./memory-pressure.md).

## Conceptual invariants

1. A checked-out buffer is not available for retained reuse.
2. Retained accounting and in-use accounting are separate concepts.
3. Reserved memory requires enough ownership/accounting information.
4. Ownership context improves returned-capacity validation.
5. Strict misuse detection is mode-dependent, not a universal baseline.
6. Public handle and method names belong to reference/API docs.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Checked-out capacity may matter | Support accounting modes that can represent in-use and reserved memory. |
| Origin context improves validation | Preserve enough context in ownership-aware modes to validate returned capacity. |
| Strict checking has cost | Keep stricter misuse detection mode-dependent rather than mandatory. |
| Public contracts must be precise | Leave handle names and exact guarantees to reference/API documentation. |

## Out of scope

This concept does not define ownership handle types, public lifecycle method
names, exact error behavior, internal tracking structures, or strict-mode
performance thresholds.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand retained/in-use/reserved terms | [Memory Retention](./memory-retention.md) |
| Understand grown returned capacity | [Buffer Growth](./buffer-growth.md) |
| Understand pressure and checked-out buffers | [Memory Pressure](./memory-pressure.md) |
| Understand policy-level ownership rules | [Policies](../policies/index.md) |
| Understand public contracts | [Reference](../reference/index.md) |

## Summary

Explicit ownership/accounting lets the runtime reason about checked-out
capacity, reserved memory, and returned-capacity validation when configured.

It is a semantic boundary, not a place to define public API names.
