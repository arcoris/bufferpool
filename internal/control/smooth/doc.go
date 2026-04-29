// Package smooth provides control-plane smoothing primitives.
//
// EWMA state is intended to be updated from window rates or scores outside hot
// paths. The first observation initializes the state directly; later
// observations use alpha to move toward the new value. Alpha validation is
// explicit so callers can choose whether invalid configuration should be an
// error or whether defensive update behavior is enough.
//
// This package performs no policy mutation, runtime publication, scheduling, or
// background work. It does not import the root bufferpool package and must not
// be called from Pool.Get, Pool.Put, or lease-release hot paths.
package smooth
