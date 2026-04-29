// Package smooth provides control-plane smoothing primitives.
//
// EWMA state is intended to be updated from window rates or scores outside hot
// paths. The first observation initializes the state directly; later
// observations use alpha to move toward the new value. Alpha validation is
// explicit so callers can choose whether invalid configuration should be an
// error or whether defensive update behavior is enough.
// EWMASmoother prepares alpha but does not store moving state; controllers own
// EWMA values and pass them through Update explicitly. A zero EWMASmoother is
// disabled and returns state unchanged, avoiding hidden default configuration.
//
// This package performs no policy mutation, runtime publication, scheduling, or
// background work. It does not import the root bufferpool package and must not
// be called from Pool.Get, Pool.Put, or lease-release hot paths.
// Repository-level control boundaries and verification gates are documented in
// docs/design/control-algorithms.md.
package smooth
