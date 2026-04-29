// Package rank provides stable candidate ranking helpers.
//
// Ranking is deterministic and generic. Candidates carry caller-owned indexes,
// scores, and tie-break keys. Sort helpers mutate caller-provided slices by
// design; TopK helpers return copied sorted slices and never mutate input.
// Ties use the lower tie-break value first so callers can preserve registry or
// construction order explicitly.
// The current sort uses allocation-free insertion sort because controller
// candidate sets are expected to be small or medium. If a future
// GroupCoordinator ranks thousands of candidates per tick, benchmark data
// should drive a hybrid or heap-based implementation rather than changing the
// algorithm preemptively.
//
// The package does not compute scores, mutate policies, attach domain meaning
// to candidate indexes, or import the root bufferpool package.
package rank
