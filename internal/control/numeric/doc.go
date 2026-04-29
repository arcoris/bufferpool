// Package numeric contains leaf numeric helpers for control-plane algorithms.
//
// The package is pure computation. It does not import the root bufferpool
// package, does not perform policy decisions, and does not mutate runtime
// state. It is safe to use from partition and future group control adapters
// because every helper is deterministic and works only with scalar values.
//
// numeric deliberately distinguishes strict runtime math from forgiving control
// projections. Strict primitives such as generic clamping and strict ratios
// live in internal/mathx and may panic on impossible internal invariants.
// numeric integrates those primitives behind safer helpers that normalize
// control-plane edge cases: zero denominators return zero, non-finite values are
// converted to zero, and normalized scores are clamped into [0, 1].
//
// This package must remain dependency-light. It may depend on generic internal
// math primitives, but it must not depend on Pool, LeaseRegistry,
// PoolPartition, PoolGroup, policy snapshots, metrics snapshots, or any root
// domain type. Domain-specific meaning belongs in the root package adapters.
// Repository-level control boundaries and verification gates are documented in
// docs/design/control-algorithms.md.
package numeric
