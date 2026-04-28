/*
  Copyright 2026 The ARCORIS Authors

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

package bufferpool

import "time"

// LeaseSnapshot is an observational view of one lease.
//
// Snapshots do not expose the buffer itself. They report ownership metadata that
// helps future Partition/Group diagnostics understand in-use memory, release
// timing, and ownership violations without leaking mutable caller data.
type LeaseSnapshot struct {
	// ID is the registry-local lease identifier.
	ID LeaseID

	// State reports whether ownership is active or released.
	State LeaseState

	// RequestedSize is the logical size requested by the caller at acquisition.
	RequestedSize Size

	// OriginClass is the Pool class selected for the acquired buffer.
	OriginClass ClassSize

	// AcquiredCapacity is the backing capacity checked out from Pool.
	AcquiredCapacity uint64

	// ReturnedCapacity is the capacity handed back to Pool after successful
	// ownership release.
	//
	// Strict releases canonicalize this value to the acquired capacity even if
	// the caller returned a clipped-capacity slice view.
	ReturnedCapacity uint64

	// AcquiredAt is the time the registry created the lease.
	AcquiredAt time.Time

	// ReleasedAt is the time ownership release completed.
	//
	// It is zero while the lease remains active.
	ReleasedAt time.Time

	// HoldDuration is ReleasedAt-AcquiredAt for released leases and an
	// observational age for active leases.
	HoldDuration time.Duration

	// LastViolation records the most recent rejected release reason while the
	// lease was still active.
	LastViolation OwnershipViolationKind
}

// IsActive reports whether the lease is still checked out.
//
// Active means ownership has not completed. The Pool that produced the buffer
// may have a different lifecycle state; lease activity is owned by the
// LeaseRegistry.
func (s LeaseSnapshot) IsActive() bool { return s.State == LeaseStateActive }

// IsReleased reports whether the lease has completed release processing.
//
// Released means ownership accounting ended. It does not imply Pool retained
// the returned buffer; Pool return handoff failures are reported by registry
// counters.
func (s LeaseSnapshot) IsReleased() bool { return s.State == LeaseStateReleased }

// LeaseRegistrySnapshot is an observational view of a LeaseRegistry.
//
// The snapshot is intended for diagnostics and future controller integration. It
// is not a globally atomic transaction across all leases; each lease snapshot is
// locally consistent under that lease record's lock. Generation advances when
// registry-visible state changes, such as acquire, release, invalid release, and
// close. It is local to one registry and must not be compared with Pool
// generation streams as a global clock.
type LeaseRegistrySnapshot struct {
	// Lifecycle is the current registry lifecycle state.
	Lifecycle LifecycleState

	// Generation is the registry-local generation observed when the snapshot was
	// built.
	//
	// It advances on snapshot-visible state changes. It is not globally
	// comparable with Pool runtime generations or other registries.
	Generation Generation

	// Config is the normalized lease registry config.
	Config LeaseConfig

	// Counters is an observational atomic sample of registry counters.
	Counters LeaseCountersSnapshot

	// Active contains snapshots of leases that were active while the registry
	// lock was held.
	//
	// The slice is caller-owned; mutating it cannot affect registry state.
	Active []LeaseSnapshot
}

// ActiveCount returns the number of active lease snapshots.
//
// This is a convenience wrapper over len(Active). It counts the leases captured
// by this snapshot, not a live view of the registry.
func (s LeaseRegistrySnapshot) ActiveCount() int { return len(s.Active) }

// IsEmpty reports whether the registry has no counters and no active leases.
//
// Lifecycle and generation are ignored. A newly constructed registry may have an
// initial generation and active lifecycle while still being empty from an
// ownership-accounting perspective.
func (s LeaseRegistrySnapshot) IsEmpty() bool { return s.Counters.IsZero() && len(s.Active) == 0 }
