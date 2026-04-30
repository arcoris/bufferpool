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

	// errPoolUnsupportedReturnFallbackShards is returned when direct Pool
	// construction is asked to probe additional shards on Put.
	errPoolUnsupportedReturnFallbackShards = "bufferpool.Pool.New: return fallback shards are not supported by standalone Pool"
)

// validatePoolSupportedPolicy validates the subset of Policy implemented by a
// directly constructed Pool.
//
// Policy intentionally describes the broader target architecture. Some fields
// belong to future PoolPartition, PoolGroup, or ownership-aware lease layers
// rather than the current bare []byte Pool data plane. Generic Policy.Validate
// can accept those fields because they are valid policy model concepts, but New
// must reject them for standalone Pool construction so callers do not receive
// false guarantees.
//
// Direct Pool currently supports capacity-based Get/Put admission, bounded
// class/shard storage, lifecycle safety, counters, snapshots, metrics, and
// immutable runtime snapshot consumption. It does not own checked-out buffer
// identity, in-use registries, origin-class growth checks, double-release
// detection, or return fallback probing. Those features require additional API
// or controller layers and must not be silently treated as active.
//
// OwnershipModeUnset is allowed only as "no ownership policy configured"; it is
// treated the same as OwnershipModeNone when no active tracking flags are set.
// MaxReturnedCapacityGrowth may carry the normalized default value, but direct
// Pool does not enforce that ratio until a lease-aware ownership API exists.
func validatePoolSupportedPolicy(policy Policy) error {
	var err error

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

	if policy.Shards.ReturnFallbackShards > 0 {
		multierr.AppendInto(&err, newError(ErrInvalidPolicy, errPoolUnsupportedReturnFallbackShards))
	}

	return err
}
