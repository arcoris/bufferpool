// Package risk scores ownership, caller-misuse, and return-failure safety signals.
//
// Risk values explain safety pressure; they are not usefulness, retention, or
// memory-pressure scores. Return-failure, ownership, and misuse components stay
// separate so root adapters can show why a risk score moved and can suppress
// unsafe recommendations without mutating ownership state.
//
// Double release is intentionally visible in both ownership and misuse
// components. It is an ownership-boundary signal because a checked-out lease is
// released more than once, and it is caller API misuse because the release
// contract was violated. Invalid release is misuse only because it does not
// identify a valid ownership record. Closed-resource handoff failures carry
// lower default severity than admission/runtime failures because shutdown can
// legitimately produce closed-Pool return diagnostics.
//
// All coefficients are conservative initial defaults. They are named and
// documented so controller adapters can tune them later; they are not
// mathematically optimal workload-specific values.
// Scorer is preferred for repeated evaluation because all weight groups are
// normalized once. Root controllers may use risk to suppress unsafe
// recommendations or raise diagnostics, but risk never mutates ownership state.
//
// The package does not directly mutate policies, publish runtime snapshots, or
// import the root bufferpool package. It is control-plane only.
package risk
