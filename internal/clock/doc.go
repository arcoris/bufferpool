// Package clock provides real and manual clocks for runtime and tests.
//
// Production code uses the real clock when wall time is part of an explicit
// contract. Tests use the manual clock to make timeout, drain, and lifecycle
// behavior deterministic without sleeping unnecessarily.
//
// The package does not know about Pool, LeaseRegistry, PoolPartition, or policy
// types. It must remain a small time source abstraction rather than a scheduler
// or background loop implementation.
package clock
