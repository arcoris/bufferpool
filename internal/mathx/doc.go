// Package mathx contains strict internal numeric primitives.
//
// mathx is the fail-fast math layer for bufferpool internals. Helpers such as
// Clamp, Ratio, UnitRatio, and NextPowerOfTwo panic when callers violate
// invariants that should have been validated earlier. This is intentional:
// invalid bounds, impossible ratios, and overflowing powers of two are
// programming errors inside the runtime, not ordinary workload events.
//
// Forgiving control-plane adapters live elsewhere, for example
// internal/control/numeric. Those adapters may turn empty windows into zero
// ratios or sanitize non-finite values before delegating to mathx. Keeping the
// strict and forgiving layers separate prevents hot-path runtime math from
// silently hiding broken invariants.
//
// mathx must not import the root bufferpool package and must not attach domain
// meaning to numeric values. Callers provide domain context in their own error
// handling and comments.
package mathx
