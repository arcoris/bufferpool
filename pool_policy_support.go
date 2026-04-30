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

import "arcoris.dev/bufferpool/internal/multierr"

const (
	// errPoolUnsupportedOwnershipMode is returned when direct Pool construction
	// receives a lease-dependent ownership mode.
	errPoolUnsupportedOwnershipMode = "bufferpool.Pool.New: ownership mode requires a lease-aware ownership layer"

	// errPoolUnsupportedTrackInUseBytes is returned when direct Pool
	// construction is asked to expose checked-out byte accounting.
	errPoolUnsupportedTrackInUseBytes = "bufferpool.Pool.New: in-use byte tracking requires a lease-aware ownership layer"

	// errPoolUnsupportedTrackInUseBuffers is returned when direct Pool
	// construction is asked to expose checked-out buffer accounting.
	errPoolUnsupportedTrackInUseBuffers = "bufferpool.Pool.New: in-use buffer tracking requires a lease-aware ownership layer"

	// errPoolUnsupportedDoubleReleaseDetection is returned when direct Pool
	// construction is asked to detect repeated returns without ownership state.
	errPoolUnsupportedDoubleReleaseDetection = "bufferpool.Pool.New: double-release detection requires a lease-aware ownership layer"

	// errPoolUnsupportedReturnFallbackShards is returned when Pool construction
	// is asked to probe additional shards on Put.
	errPoolUnsupportedReturnFallbackShards = "bufferpool.Pool.New: return fallback shards are not supported by standalone Pool"
)

// poolConstructionMode identifies which owner is constructing a Pool.
//
// Standalone Pool construction exposes only the raw []byte Get/Put data plane,
// so it cannot claim lease-backed ownership guarantees. Partition-owned Pool
// construction is different: PoolPartition wraps the Pool with LeaseRegistry
// acquisition/release and can therefore accept ownership-aware policy metadata
// while still keeping Pool.Get and Pool.Put simple.
type poolConstructionMode uint8

const (
	// poolConstructionModeStandalone builds a Pool for direct public use.
	poolConstructionModeStandalone poolConstructionMode = iota

	// poolConstructionModePartitionOwned builds a Pool that is reachable through
	// PoolPartition or PoolGroup managed acquisition and release.
	poolConstructionModePartitionOwned
)

// validatePoolSupportedPolicy validates the subset of Policy implemented by a
// Pool in the requested construction mode.
//
// Policy intentionally describes the broader target architecture. Some fields
// belong to PoolPartition, PoolGroup, or ownership-aware lease layers rather
// than the bare []byte Pool data plane. Generic Policy.Validate can accept those
// fields because they are valid policy model concepts; construction support
// validation decides whether the selected owner can actually implement them.
//
// Direct Pool currently supports capacity-based Get/Put admission, bounded
// class/shard storage, lifecycle safety, counters, snapshots, metrics, and
// immutable runtime snapshot consumption. It does not own checked-out buffer
// identity, in-use registries, origin-class growth checks, double-release
// detection, or return fallback probing. Standalone mode rejects those
// ownership-only fields so callers do not receive false guarantees.
//
// Partition-owned mode allows accounting and strict ownership metadata because
// LeaseRegistry owns the managed acquisition/release boundary. Pool itself still
// does not perform lease validation in Get or Put; it only provides the
// retained-storage return primitive used after LeaseRegistry consumes a lease.
//
// OwnershipModeUnset is allowed only as "no ownership policy configured"; it is
// treated the same as OwnershipModeNone when no active tracking flags are set.
// MaxReturnedCapacityGrowth may carry the normalized default value.
func validatePoolSupportedPolicy(policy Policy, mode poolConstructionMode) error {
	var err error

	if mode == poolConstructionModeStandalone {
		if policy.Ownership.Mode != OwnershipModeUnset && policy.Ownership.Mode != OwnershipModeNone {
			multierr.AppendInto(&err, newError(ErrInvalidPolicy, errPoolUnsupportedOwnershipMode))
		}

		if policy.Ownership.TrackInUseBytes {
			multierr.AppendInto(&err, newError(ErrInvalidPolicy, errPoolUnsupportedTrackInUseBytes))
		}

		if policy.Ownership.TrackInUseBuffers {
			multierr.AppendInto(&err, newError(ErrInvalidPolicy, errPoolUnsupportedTrackInUseBuffers))
		}

		if policy.Ownership.DetectDoubleRelease {
			multierr.AppendInto(&err, newError(ErrInvalidPolicy, errPoolUnsupportedDoubleReleaseDetection))
		}
	}

	if policy.Shards.ReturnFallbackShards > 0 {
		multierr.AppendInto(&err, newError(ErrInvalidPolicy, errPoolUnsupportedReturnFallbackShards))
	}

	return err
}
