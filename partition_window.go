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

// PoolPartitionCounterDelta is the raw counter movement between two samples.
//
// Delta values are controller inputs. Lifetime metrics remain diagnostic, while
// future EWMA or adaptive scoring should be built from bounded windows like
// this one. This type stores raw counter deltas only; it does not compute rates,
// scores, or policy decisions.
type PoolPartitionCounterDelta struct {
	// Gets is the Pool acquisition-attempt delta.
	Gets uint64

	// Hits is the retained-storage reuse delta.
	Hits uint64

	// Misses is the miss/allocation-path delta.
	Misses uint64

	// Allocations is the backing-allocation delta.
	Allocations uint64

	// Puts is the Pool return-attempt delta.
	Puts uint64

	// Retains is the retained-return delta.
	Retains uint64

	// Drops is the dropped-return delta.
	Drops uint64

	// LeaseAcquisitions is the lease-acquisition delta.
	LeaseAcquisitions uint64

	// LeaseReleases is the successful ownership-release delta.
	LeaseReleases uint64

	// LeaseInvalidReleases is the malformed release-attempt delta.
	LeaseInvalidReleases uint64

	// LeaseDoubleReleases is the double-release delta.
	LeaseDoubleReleases uint64

	// LeaseOwnershipViolations is the strict ownership violation delta.
	LeaseOwnershipViolations uint64

	// LeasePoolReturnAttempts is the post-release Pool handoff attempt delta.
	LeasePoolReturnAttempts uint64

	// LeasePoolReturnSuccesses is the successful post-release Pool handoff delta.
	LeasePoolReturnSuccesses uint64

	// LeasePoolReturnFailures is the failed post-release Pool handoff delta.
	LeasePoolReturnFailures uint64
}

// IsZero reports whether d contains no observed counter movement.
func (d PoolPartitionCounterDelta) IsZero() bool {
	return d.Gets == 0 &&
		d.Hits == 0 &&
		d.Misses == 0 &&
		d.Allocations == 0 &&
		d.Puts == 0 &&
		d.Retains == 0 &&
		d.Drops == 0 &&
		d.LeaseAcquisitions == 0 &&
		d.LeaseReleases == 0 &&
		d.LeaseInvalidReleases == 0 &&
		d.LeaseDoubleReleases == 0 &&
		d.LeaseOwnershipViolations == 0 &&
		d.LeasePoolReturnAttempts == 0 &&
		d.LeasePoolReturnSuccesses == 0 &&
		d.LeasePoolReturnFailures == 0
}

// PoolPartitionWindow captures two partition samples and their counter delta.
//
// Generation and PolicyGeneration come from Current. Window construction does
// not mutate Pool, LeaseRegistry, or PoolPartition state. If a counter appears
// to move backwards, the delta treats Current as the post-reset value. That
// makes reset or reconstruction visible as a fresh baseline instead of wrapping
// unsigned counters.
type PoolPartitionWindow struct {
	// Generation is the current sample partition generation.
	Generation Generation

	// PolicyGeneration is the current sample runtime-policy generation.
	PolicyGeneration Generation

	// Previous is the older sample used for delta computation.
	Previous PoolPartitionSample

	// Current is the newer sample used for delta computation.
	Current PoolPartitionSample

	// Delta is the raw counter movement from Previous to Current.
	Delta PoolPartitionCounterDelta
}

// NewPoolPartitionWindow returns a window from previous to current.
func NewPoolPartitionWindow(previous, current PoolPartitionSample) PoolPartitionWindow {
	return PoolPartitionWindow{
		Generation:       current.Generation,
		PolicyGeneration: current.PolicyGeneration,
		Previous:         clonePoolPartitionSample(previous),
		Current:          clonePoolPartitionSample(current),
		Delta:            newPoolPartitionCounterDelta(previous, current),
	}
}

// newPoolPartitionCounterDelta computes raw counter movement between samples.
func newPoolPartitionCounterDelta(previous, current PoolPartitionSample) PoolPartitionCounterDelta {
	return PoolPartitionCounterDelta{
		Gets:                     partitionCounterDelta(previous.PoolCounters.Gets, current.PoolCounters.Gets),
		Hits:                     partitionCounterDelta(previous.PoolCounters.Hits, current.PoolCounters.Hits),
		Misses:                   partitionCounterDelta(previous.PoolCounters.Misses, current.PoolCounters.Misses),
		Allocations:              partitionCounterDelta(previous.PoolCounters.Allocations, current.PoolCounters.Allocations),
		Puts:                     partitionCounterDelta(previous.PoolCounters.Puts, current.PoolCounters.Puts),
		Retains:                  partitionCounterDelta(previous.PoolCounters.Retains, current.PoolCounters.Retains),
		Drops:                    partitionCounterDelta(previous.PoolCounters.Drops, current.PoolCounters.Drops),
		LeaseAcquisitions:        partitionCounterDelta(previous.LeaseCounters.Acquisitions, current.LeaseCounters.Acquisitions),
		LeaseReleases:            partitionCounterDelta(previous.LeaseCounters.Releases, current.LeaseCounters.Releases),
		LeaseInvalidReleases:     partitionCounterDelta(previous.LeaseCounters.InvalidReleases, current.LeaseCounters.InvalidReleases),
		LeaseDoubleReleases:      partitionCounterDelta(previous.LeaseCounters.DoubleReleases, current.LeaseCounters.DoubleReleases),
		LeaseOwnershipViolations: partitionCounterDelta(previous.LeaseCounters.OwnershipViolations, current.LeaseCounters.OwnershipViolations),
		LeasePoolReturnAttempts:  partitionCounterDelta(previous.LeaseCounters.PoolReturnAttempts, current.LeaseCounters.PoolReturnAttempts),
		LeasePoolReturnSuccesses: partitionCounterDelta(previous.LeaseCounters.PoolReturnSuccesses, current.LeaseCounters.PoolReturnSuccesses),
		LeasePoolReturnFailures:  partitionCounterDelta(previous.LeaseCounters.PoolReturnFailures, current.LeaseCounters.PoolReturnFailures),
	}
}

// partitionCounterDelta returns monotonic counter movement.
func partitionCounterDelta(previous, current uint64) uint64 {
	if current >= previous {
		return current - previous
	}
	return current
}

// clonePoolPartitionSample returns a sample copy with owned per-Pool slice storage.
func clonePoolPartitionSample(sample PoolPartitionSample) PoolPartitionSample {
	if sample.Pools != nil {
		sample.Pools = append([]PoolPartitionPoolSample(nil), sample.Pools...)
	}
	return sample
}
