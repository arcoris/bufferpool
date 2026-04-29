// Package activity computes generic activity and idle-candidate signals.
//
// Activity scoring is control-plane only. It must be driven from sampled window
// rates, not updated in Pool.Get or Pool.Put. Hotness normalizes enabled
// throughput dimensions against caller-provided high-water marks; zero
// thresholds disable a dimension. Idle helpers use quiet-window counts rather
// than wall-clock timers so callers can decide the sampling cadence.
//
// The package does not mutate active registries or policies by itself. It does
// not import the root bufferpool package and is not a hot-path dependency.
package activity
