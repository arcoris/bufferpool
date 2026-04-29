// Package series computes movement between control-plane observations.
//
// The core invariant is that monotonic counters and gauges are different
// signals. Counter deltas avoid uint64 underflow and mark backwards movement as
// reset or reconstruction. Gauge deltas report directional movement instead of
// pretending gauges are monotonic. This keeps future controller windows honest:
// bytes currently retained, active leases, and active bytes are gauges, while
// hits, misses, drops, releases, and return failures are counters.
//
// series does not import the root bufferpool package and does not compute EWMA,
// scores, pressure decisions, trim plans, or budget redistribution. It only
// defines generic movement and sample-scope metadata that root adapters can map
// onto PoolPartition and future PoolGroup observations.
package series
