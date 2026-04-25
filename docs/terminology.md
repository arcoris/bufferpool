<div align="center">

# Terminology

**Canonical vocabulary for the adaptive byte-buffer memory-retention runtime.**

[![Docs](https://img.shields.io/badge/Docs-index.md-0F766E?style=flat)](./index.md)
[![Overview](https://img.shields.io/badge/Overview-overview.md-1D4ED8?style=flat)](./overview.md)
[![Scope](https://img.shields.io/badge/Scope-Goals%20%26%20Non--goals-0F172A?style=flat)](./goals-and-non-goals.md)
[![Architecture](https://img.shields.io/badge/Model-Architecture-7C2D12?style=flat)](./architecture/index.md)
[![Reference](https://img.shields.io/badge/Contracts-Reference-4C1D95?style=flat)](./reference/index.md)

[Docs](./index.md) · [Overview](./overview.md) · [Goals and Non-goals](./goals-and-non-goals.md) · [Architecture](./architecture/index.md) · [Policies](./policies/index.md) · [Workload](./workload/index.md) · [Reference](./reference/index.md)

Memory retention · Allocation pressure · Pool partitions · Size classes · Ownership/accounting · Workload control

**Start:** [Purpose](#purpose) · [Vocabulary rules](#vocabulary-rules) · [Glossary](#glossary) · [API-neutral terms](#api-neutral-terms) · [Avoided terms](#avoided-terms) · [Symbols](#symbols)

</div>

## Purpose

This document is the terminology source of truth for `arcoris.dev/bufferpool`.

It defines canonical words used by documentation, code comments, issues,
reviews, architecture notes, policy documents, and ADRs. Other documents may
apply these terms in context, but they should not redefine them.

This page is a glossary only. It does not define scope, runtime architecture,
policy algorithms, public API names, implementation design, or maturity status.

## Vocabulary rules

Use the canonical term when one exists.

Avoid near-synonyms for core concepts unless a new term has a distinct
responsibility. In particular:

- use `memory retention`, not generic `cache eviction`, for retained
  byte-buffer capacity;
- use `PoolPartition`, not `controller shard`, for pool-based control-plane
  ownership;
- use `Shard`, not `partition`, for hot-path striping;
- use `Bucket`, not `pool`, for local retained-buffer storage.

Keep data-plane and control-plane terms separate. A `Shard` is not a
`PoolPartition`; a local bucket is not a workload pool.

Do not use this document to freeze concrete public method names, ownership
handle names, option names, or lifecycle operation names.

## Glossary

### Core terms

| Term | Definition |
| --- | --- |
| `arcoris.dev/bufferpool` | Go package for adaptive, size-aware reuse of temporary byte buffers under bounded memory-retention policy. |
| Buffer | A `[]byte` value together with the backing capacity relevant to the runtime. |
| Byte buffer | Reusable byte slice capacity. Usually equivalent to buffer unless slice value and capacity need separate emphasis. |
| Buffer capacity | Capacity of a byte buffer. Retention policy is primarily concerned with capacity because capacity determines retained memory cost. |
| Requested size | Byte count requested by caller code or workload before normalization. |
| Class size | Normalized capacity selected for a requested size. |
| Actual capacity | Real capacity of a returned or retained buffer. |
| Returned capacity | Capacity observed when a buffer is returned to the runtime. |
| Reusable capacity | Byte-buffer capacity that may reduce future allocation if retained. |
| Allocation-heavy workload | Workload where repeated temporary allocation is large or frequent enough to affect allocation pressure, GC pressure, memory behavior, throughput, or latency. |
| Allocation pressure | Runtime cost caused by repeated allocation and later garbage collection. |
| Memory retention | Keeping reusable byte-buffer capacity after use so future operations may avoid allocation. |
| Memory-retention policy | Rules that decide which capacities are retained, where, how much, and for how long. |
| Bounded retention | Retention constrained by explicit memory limits. |
| Unbounded retention | Retention without effective limits. This is outside project scope. |

### Runtime hierarchy

| Term | Definition |
| --- | --- |
| `PoolGroup` | Top-level memory-governance and lifecycle domain. |
| `GroupCoordinator` | Group-level control component that coordinates aggregate policy, pressure, and budget assignment. |
| `PoolPartition` | Control-plane ownership unit for a subset of pools. |
| `PartitionController` | Partition-local control component responsible for observation, target calculation, policy publication, and trim scheduling within its partition. |
| `Pool` | Workload, component, tenant, or usage-domain isolation unit. |
| `SizeClass` | Normalized capacity class used to group compatible buffer sizes for reuse and accounting. |
| `Shard` | Hot-path contention-reduction unit within a size class. |
| `Bucket` | Local retained-buffer storage for one shard and size class. |

### Memory and accounting terms

| Term | Definition |
| --- | --- |
| Retained memory | Memory currently held by the runtime as reusable buffer capacity. |
| Retained bytes | Byte count of retained memory. |
| Retained buffers | Count of buffers currently retained. |
| Checked-out buffer | Buffer currently held by caller code and not yet returned to the runtime. |
| In-use memory | Memory represented by checked-out buffers when accounting can track it. |
| In-use bytes | Byte count of in-use memory when supported by the configured accounting mode. |
| Reserved memory | Retained memory plus in-use memory when the runtime has enough information to account for both. |
| Reserved bytes | `retainedBytes + inUseBytes` when supported. |
| Target bytes | Desired retained byte count for a governance level. |
| Target buffers | Desired retained buffer count, usually secondary to byte targets. |
| Hard limit | Bound that policy must not intentionally exceed. |
| Soft limit | Bound that may trigger more conservative retention before a hard limit is reached. |
| High watermark | Threshold that starts stronger contraction or trim behavior. |
| Low watermark | Threshold that relaxes contraction or trim behavior to avoid oscillation. |
| Over-budget | State where retained memory exceeds the current target or budget. |
| Under-target | State where retained memory is below the current target. |
| Idle retained memory | Retained memory that has not recently contributed useful reuse. |
| Cold retained memory | Retained memory associated with low recent usefulness or activity. |

### Policy terms

| Term | Definition |
| --- | --- |
| Policy | Configuration and behavioral rules governing retention, admission, trimming, pressure, ownership/accounting, and observability. |
| Policy snapshot | Immutable published control state used by runtime paths. |
| Policy generation | Monotonic version attached to published policy state. |
| Policy update | Validated policy change that may change limits, targets, pressure behavior, or contraction state. |
| Admission | Decision made when a returned buffer is evaluated for retention. |
| Admission policy | Rules that decide whether returned capacity is retained or dropped. |
| Drop | Decision not to retain returned capacity. Drops are normal bounded-retention behavior. |
| Drop reason | Classified explanation for a drop. |
| Retention policy | Rules that govern retained memory budgets and targets. |
| Trim | Physical removal of already retained buffers from runtime storage. |
| Trim policy | Rules that decide when and how retained buffers are removed. |
| Contraction | Transition toward lower retained memory after pressure or budget shrink. |
| Pressure | Signal that retention should become more conservative. |
| Pressure policy | Rules that reshape retention under pressure. |
| Zeroing policy | Rules that decide whether buffer contents should be cleared for safety or security. |
| Oversize policy | Rules for requested or returned capacity outside supported retention ranges. |
| Drop-on-return | Principle that unsuitable returned buffers may be rejected immediately instead of retained and trimmed later. |
| Ownership/accounting mode | Runtime mode that determines how strictly checked-out, returned, retained, in-use, or reserved memory is tracked. |

### Workload terms

| Term | Definition |
| --- | --- |
| Workload | Observed memory-demand behavior, including requested sizes, reuse, misses, allocation, drops, retained bytes, pressure, and timing. |
| Workload signal | Measured input used by the control plane. |
| Workload shift | Meaningful change in observed memory-demand shape. |
| Lifetime counters | Counters accumulated over runtime lifetime. Useful for observability, not direct adaptive truth. |
| Window counters | Counters collected for a recent interval. |
| Window delta | Difference between two counter snapshots over a sampling interval. |
| EWMA | Exponentially weighted moving average used to smooth recent signals. |
| Half-life | Interval after which older observations have half their previous influence in a decayed model. |
| Activity score | Smoothed signal representing recent demand or usefulness. |
| Class score | Score for one size class within a pool. |
| Pool score | Aggregate score for a pool. |
| Partition score | Aggregate score for a pool partition. |
| Demand boost | Score adjustment for recent misses, allocation pressure, or starvation. |
| Retention efficiency | Useful reuse relative to retained memory cost. |
| Size penalty | Score adjustment that makes larger capacity prove usefulness. |
| Hot class | Class with high recent demand or useful reuse. |
| Warm class | Class with moderate useful activity. |
| Cold class | Class with low recent usefulness or activity. |
| Starved class | Class with demand but insufficient retained capacity. |
| Wasteful class | Class retaining memory without enough useful reuse. |
| Bursty class | Class with intermittent but repeated demand bursts. |
| Convergence | Movement toward a stable operating range under stable workload. |
| Re-convergence | Movement toward a new stable range after workload shape changes. |
| Controller lag | Delay between expected and actual control-cycle execution. |

### Budget terms

| Term | Definition |
| --- | --- |
| Group budget | Memory-retention budget owned by `PoolGroup`. |
| Partition budget | Budget assigned to one `PoolPartition`. |
| Pool budget | Budget assigned to one `Pool`. |
| Class budget | Budget assigned to one `SizeClass` within a pool. |
| Shard credit | Local retention allowance assigned to one shard. |
| Adaptive budget | Budget portion distributed according to recent workload scores. |
| Base budget | Budget portion reserved independently of current score. |
| Minimum budget | Lower bound for a target. |
| Maximum budget | Upper bound for a target. |
| Budget redistribution | Reallocation of targets across governance levels. |
| Budget shrink | Reduction of one or more memory targets. |
| Budget growth | Increase of one or more memory targets. |
| Credit exhaustion | State where local retention has reached or exceeded assigned credit. |

Budget flow is written as:

$$
B_{group}
\rightarrow
T_k
\rightarrow
T_p
\rightarrow
T_{p,c}
\rightarrow
C_{p,c,s}
$$

### Plane and lifecycle terms

| Term | Definition |
| --- | --- |
| Data path | Runtime path that serves ordinary buffer lifecycle operations and local storage access. |
| Hot path | Same concept as data path when emphasizing performance sensitivity. |
| Control path | Runtime path that observes workload and adjusts retention behavior. |
| Cold path | Same concept as control path when emphasizing lower-frequency work. |
| Local operation | Operation that can complete using local state and published policy state. |
| Global coordination | Control-plane work involving aggregate or group-level state. |
| Per-operation global coordination | Global coordination on every ordinary buffer operation. Outside the baseline design. |
| Controller tick | Scheduled control-path cycle. A target cadence, not a realtime guarantee. |
| Coalesced tick | Control cycle that accounts for elapsed time without replaying every missed interval. |
| Full scan | Inspection of all registered runtime entries. Full scans should be rare and bounded. |
| Active scan | Inspection of recently active entries only. |

### Observability terms

| Term | Definition |
| --- | --- |
| Metrics snapshot | Point-in-time view of runtime metrics. |
| Runtime snapshot | Point-in-time view of policy generation, budgets, pressure, retained memory, and related state. |
| Hit | Operation served from retained storage instead of allocation. |
| Miss | Operation not served from retained storage. |
| Allocation | Creation of new backing capacity because retained storage could not or should not serve demand. |
| Drop count | Number of returned buffers not retained. |
| Trim count | Number of trim actions or buffers removed, depending on metric definition. |
| Trim backlog | Retained memory or trim candidates still waiting for trim work. |
| Snapshot age | Time since a policy or metrics snapshot was published. |
| Controller cycle duration | Time spent in a control-path cycle. |
| Retained target error | Distance between current retained memory and the current target. |

Retained target error may be expressed as:

$$
error_{p,c} =
\frac{|retainedBytes_{p,c} - T_{p,c}|}{T_{p,c}}
$$

## API-neutral terms

| API-neutral term | Meaning |
| --- | --- |
| Buffer acquisition | Any public operation that obtains a buffer from the runtime. Concrete method names are not defined here. |
| Buffer return | Any public operation that gives a buffer back to the runtime for admission or discard. Concrete method names are not defined here. |
| Ownership handle | Possible public or internal representation of explicit ownership. The concrete name and shape are not defined here. |
| Lifecycle operation | Public or internal operation that affects runtime startup, shutdown, ownership, or policy state. Concrete names are not defined here. |
| Repeated return | Returning the same checked-out buffer more than once when strict checking can detect it. |
| Ownership-aware mode | Mode capable of tracking additional lifecycle or accounting state for checked-out buffers. |
| Strict checking | Stronger validation that may detect ownership violations when supported. |

## Avoided terms

| Avoided term | Prefer | Reason |
| --- | --- | --- |
| Cache eviction | Memory-retention policy | Buffers are reusable capacity, not semantic cache entries. |
| Cache entry | Retained buffer | Retained buffers have no key/value application meaning. |
| Object pool | Byte-buffer retention runtime | The project is buffer-specific. |
| Controller shard | `PoolPartition` | Partitions are pool-based control-plane ownership units. |
| Shard partition | `Shard` | Shards are data-plane contention boundaries. |
| Global pool | `PoolGroup` | The top-level concept is a governance domain, not one flat store. |
| Free list | `Bucket` | Bucket semantics include bounded retention and accounting. |
| Exact realtime controller | Controller target cadence | Controller timing is best-effort and elapsed-time based. |
| Always retain | Admission policy | Retaining every returned buffer violates bounded retention. |
| Zero allocation guarantee | Allocation-pressure reduction | The project does not promise zero allocations. |

## Symbols

| Symbol | Meaning |
| --- | --- |
| $B_{group}$ | Group memory budget. |
| $T_k$ | Target budget for partition $k$. |
| $T_p$ | Target budget for pool $p$. |
| $T_{p,c}$ | Target budget for class $c$ in pool $p$. |
| $C_{p,c,s}$ | Credit or local target for shard $s$ in class $c$ of pool $p$. |
| $S_k$ | Score for partition $k$. |
| $S_p$ | Score for pool $p$. |
| $S_{p,c}$ | Score for class $c$ in pool $p$. |
| $A_c(t)$ | EWMA activity for class $c$ at time $t$. |
| $R_c(t)$ | Recent activity signal for class $c$ at time $t$. |
| $\alpha$ | EWMA decay coefficient. |
| $H$ | Half-life. |
| $\Delta t$ | Elapsed time between samples. |
| $P_a$ | Number of active pools. |
| $C_a$ | Average active classes per active pool. |
| $S_a$ | Average active shards per active class. |
| $K$ | Number of pool partitions. |
| $E_{target}$ | Target number of active entries per partition. |

## Source-of-truth boundary

Terminology owns names and definitions only.

| Need | Owner |
| --- | --- |
| Purpose and applicability | [Overview](./overview.md) |
| Scope and proposal limits | [Goals and Non-goals](./goals-and-non-goals.md) |
| Runtime responsibilities | [Architecture](./architecture/index.md) |
| Policy behavior | [Policies](./policies/index.md) |
| Workload scoring details | [Workload](./workload/index.md) |
| Public contracts | [Reference](./reference/index.md) |
