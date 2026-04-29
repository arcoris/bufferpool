// Package activity computes generic activity and idle-candidate signals.
//
// Activity scoring is control-plane only. It must be driven from sampled window
// rates, not updated in Pool.Get or Pool.Put. Hotness normalizes enabled
// throughput dimensions against caller-provided high-water marks; zero
// thresholds disable a dimension. Idle helpers use quiet-window counts rather
// than wall-clock timers so callers can decide the sampling cadence.
// HotnessScorer is preferred for repeated evaluation because thresholds and
// weights are normalized once. Hotness measures volume and intensity, not reuse
// usefulness or memory pressure.
//
// The package does not mutate active registries or policies by itself. It does
// not import the root bufferpool package and is not a hot-path dependency.
// Repository-level control boundaries and verification gates are documented in
// docs/design/control-algorithms.md.
package activity
