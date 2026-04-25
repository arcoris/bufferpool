<div align="center">

# Buffer Growth

**Concept model for length, capacity, growth ratio, and returned-capacity validation.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](../index.md)
[![Concepts](https://img.shields.io/badge/Concepts-index.md-1D4ED8?style=flat)](./index.md)
[![Size Classes](https://img.shields.io/badge/Size-Classes-0F172A?style=flat)](./size-classes.md)
[![Ownership](https://img.shields.io/badge/Ownership-Accounting-7C2D12?style=flat)](./explicit-ownership.md)
[![Retention](https://img.shields.io/badge/Core-Memory%20Retention-B45309?style=flat)](./memory-retention.md)

[Docs](../index.md) · [Concepts](./index.md) · [Size Classes](./size-classes.md) · [Memory Retention](./memory-retention.md) · [Explicit Ownership](./explicit-ownership.md) · [Policies](../policies/index.md)

Length vs capacity · Actual capacity · Growth ratio · Returned-capacity validation · Oversize drops

**Start:** [Purpose](#purpose) · [Core idea](#core-idea) · [Length and capacity](#length-and-capacity) · [Growth ratio](#growth-ratio) · [Validation](#validation) · [Reading guide](#reading-guide)

</div>

## Purpose

This document owns the buffer-growth concept for `arcoris.dev/bufferpool`.

It explains why returned capacity may differ from original demand, how growth
ratio is reasoned about, and why returned-capacity validation is required
before retention.

## Core idea

A buffer can be requested for one capacity shape and later grow during caller
use.

If the runtime retained the grown capacity without validation, a small logical
request could cause a large backing array to remain reachable. That would hide
memory cost and weaken size-aware retention.

## Length and capacity

For byte buffers, length and capacity are different concerns.

| Property | Meaning |
| --- | --- |
| Length | Bytes currently considered part of the visible slice. |
| Capacity | Backing array capacity that determines retained memory cost. |

Retention policy cares primarily about capacity because capacity determines how
much memory remains reachable.

## Growth ratio

Growth ratio compares returned capacity to the class or origin capacity that
made retention reasonable.

One conceptual form is:

$$
growthRatio =
\frac{returnedCapacity}{expectedClassCapacity}
$$

The exact threshold is a policy decision. The concept requirement is that a
runtime can detect when returned capacity is too large for the original
retention decision.

## Validation

Returned-capacity validation protects retention boundaries.

A returned buffer may be rejected when:

- returned capacity exceeds supported limits;
- returned capacity is too large for the expected class;
- the growth ratio is too high;
- the destination class would retain disproportionate memory;
- pressure or policy state makes large returned capacity unacceptable;
- ownership-aware accounting cannot validate the necessary origin context.

The decision is an admission decision, not a public API shape.

## Relationship to ownership

Ownership-aware accounting can preserve origin context such as expected class,
observed capacity, or checked-out state.

That context can make returned-capacity validation stronger. Without it, the
runtime may need more conservative approximate checks.

The ownership model is owned by
[Explicit Ownership](./explicit-ownership.md).

## Relationship to size classes

Size classes define compatible capacity groups. Buffer growth decides whether a
returned buffer still fits the expected group.

If a buffer grew too far, it may be:

- dropped;
- admitted to a larger compatible class if policy allows;
- counted as oversize or unsupported;
- retained only under stricter modes or budgets.

Exact behavior belongs to [Policies](../policies/index.md).

## Common mistakes

### Judging by length

A returned buffer with small length may still have large capacity.

### Assuming origin demand still describes returned capacity

Caller use may grow the backing array. Returned capacity must be checked.

### Hiding growth inside retention

Keeping grown buffers without classification can make retained memory look
smaller than it really is.

## Conceptual invariants

1. Capacity, not length, drives retained-memory cost.
2. Returned capacity must be validated before retention.
3. Growth validation protects size-class boundaries.
4. Ownership context can improve validation precision.
5. Growth thresholds belong to policy/reference layers, not concept pages.

## Design consequences

| Concept pressure | Consequence |
| --- | --- |
| Returned capacity may differ from original demand | Validate actual capacity before retention. |
| Growth can break class fit | Preserve or reconstruct enough context to compare returned capacity with expected class capacity. |
| Large grown buffers are expensive | Allow admission to drop or reclassify capacity according to policy. |

## Out of scope

This concept does not define exact growth thresholds, public ownership handles,
copy/shrink behavior, policy option names, or internal validation data
structures.

## Reading guide

| Need | Continue with |
| --- | --- |
| Understand capacity classes | [Size Classes](./size-classes.md) |
| Understand admission and drops | [Memory Retention](./memory-retention.md) |
| Understand origin context and accounting | [Explicit Ownership](./explicit-ownership.md) |
| Understand pressure treatment of large capacity | [Memory Pressure](./memory-pressure.md) |
| Understand exact rules | [Policies](../policies/index.md) |

## Summary

Buffer growth is why returned capacity cannot be blindly retained. A
size-aware runtime must validate the actual returned capacity before admitting
it back into retained storage.
