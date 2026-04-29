// Package stability provides anti-oscillation primitives.
//
// Hysteresis, deadband, cooldown, and consecutive-window guards are pure
// control-plane helpers. They are meant to sit above window rates and scores so
// future controllers do not mutate policy on every small signal movement.
// Validation rejects non-finite thresholds and inverted high-state hysteresis
// bands; cooldown and consecutive-window helpers handle reset-like window
// movement defensively.
//
// The package does not sample state, rank candidates, mutate policies, or
// import the root bufferpool package.
// Repository-level control boundaries and verification gates are documented in
// docs/design/control-algorithms.md.
package stability
