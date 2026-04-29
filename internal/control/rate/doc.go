// Package rate computes ratios and throughput from bounded counter deltas.
//
// Rates are control-plane projections over window movement, not lifetime
// counters. A zero denominator means the window had no observations for that
// denominator, so ratio helpers return zero. Throughput helpers require an
// elapsed duration; non-positive durations return zero instead of dividing by
// zero or reading wall-clock time.
//
// The package performs no EWMA, scoring, policy mutation, or runtime decisions.
// It does not import the root bufferpool package. Root adapters choose which
// deltas represent gets, puts, lease attempts, or bytes, then pass scalar
// values here for pure calculation.
// Repository-level control boundaries and verification gates are documented in
// docs/design/control-algorithms.md.
package rate
