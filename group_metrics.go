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

// PoolGroupMetrics combines retained-storage Pool counters with partition lease
// counters across all group-owned partitions.
//
// Ratios are lifetime-derived diagnostics. Future adaptive group coordination
// should use PoolGroupWindow and PoolGroupWindowRates rather than treating these
// lifetime ratios as workload scores.
type PoolGroupMetrics struct {
	// Name is diagnostic group metadata.
	Name string

	// Lifecycle is the observed group lifecycle state.
	Lifecycle LifecycleState

	// Generation is the group state/event generation.
	Generation Generation

	// PolicyGeneration is the group runtime-policy generation.
	PolicyGeneration Generation

	// PartitionCount is the number of partitions owned by the group.
	PartitionCount int

	// PoolCount is the aggregate number of Pools owned by group partitions.
	PoolCount int

	// ActiveLeases is the number of currently checked-out leases across partitions.
	ActiveLeases int

	// CurrentRetainedBytes is retained storage currently held by all partitions.
	CurrentRetainedBytes uint64

	// CurrentActiveBytes is checked-out backing capacity tracked by all leases.
	CurrentActiveBytes uint64

	// CurrentOwnedBytes is retained plus active bytes with saturating addition.
	CurrentOwnedBytes uint64

	// Gets is the aggregate Pool acquisition count.
	Gets uint64

	// Hits is the aggregate retained-storage reuse count.
	Hits uint64

	// Misses is the aggregate miss/allocation count.
	Misses uint64

	// Puts is the aggregate valid Pool return attempt count.
	Puts uint64

	// Retains is the aggregate retained return count.
	Retains uint64

	// Drops is the aggregate dropped return count.
	Drops uint64

	// LeaseAcquisitions is the aggregate successful lease acquisition count.
	LeaseAcquisitions uint64

	// LeaseReleases is the aggregate successful ownership release count.
	LeaseReleases uint64

	// PoolReturnAttempts counts Pool handoffs after ownership completion.
	PoolReturnAttempts uint64

	// PoolReturnSuccesses counts post-release Pool handoffs that returned nil.
	PoolReturnSuccesses uint64

	// PoolReturnFailures counts post-release Pool handoffs that failed.
	PoolReturnFailures uint64

	// HitRatio is Hits divided by Hits plus Misses.
	HitRatio PolicyRatio

	// RetainRatio is Retains divided by Retains plus Drops.
	RetainRatio PolicyRatio

	// DropRatio is Drops divided by Retains plus Drops.
	DropRatio PolicyRatio

	// PoolReturnFailureRatio is PoolReturnFailures divided by PoolReturnAttempts.
	PoolReturnFailureRatio PolicyRatio

	// ActiveMemoryRatio is CurrentActiveBytes divided by CurrentOwnedBytes.
	ActiveMemoryRatio PolicyRatio

	// RetainedMemoryRatio is CurrentRetainedBytes divided by CurrentOwnedBytes.
	RetainedMemoryRatio PolicyRatio
}

// Metrics returns an aggregate group metrics projection.
func (g *PoolGroup) Metrics() PoolGroupMetrics {
	g.mustBeInitialized()
	var sample PoolGroupSample
	g.sampleWithRuntimeAndGeneration(&sample, g.currentRuntimeSnapshot(), g.generation.Load())
	return newPoolGroupMetrics(g.name, sample)
}

// newPoolGroupMetrics derives lifetime ratios from one aggregate group sample.
func newPoolGroupMetrics(name string, sample PoolGroupSample) PoolGroupMetrics {
	reuseAttempts := sample.Aggregate.PoolCounters.ReuseAttempts()
	putOutcomes := sample.Aggregate.PoolCounters.PutOutcomes()
	ownedBytes := partitionOwnedBytesFromSample(sample.Aggregate)
	return PoolGroupMetrics{
		Name:                   name,
		Lifecycle:              sample.Lifecycle,
		Generation:             sample.Generation,
		PolicyGeneration:       sample.PolicyGeneration,
		PartitionCount:         sample.PartitionCount,
		PoolCount:              sample.PoolCount,
		ActiveLeases:           sample.Aggregate.ActiveLeases,
		CurrentRetainedBytes:   sample.CurrentRetainedBytes,
		CurrentActiveBytes:     sample.CurrentActiveBytes,
		CurrentOwnedBytes:      ownedBytes,
		Gets:                   sample.Aggregate.PoolCounters.Gets,
		Hits:                   sample.Aggregate.PoolCounters.Hits,
		Misses:                 sample.Aggregate.PoolCounters.Misses,
		Puts:                   sample.Aggregate.PoolCounters.Puts,
		Retains:                sample.Aggregate.PoolCounters.Retains,
		Drops:                  sample.Aggregate.PoolCounters.Drops,
		LeaseAcquisitions:      sample.Aggregate.LeaseCounters.Acquisitions,
		LeaseReleases:          sample.Aggregate.LeaseCounters.Releases,
		PoolReturnAttempts:     sample.Aggregate.LeaseCounters.PoolReturnAttempts,
		PoolReturnSuccesses:    sample.Aggregate.LeaseCounters.PoolReturnSuccesses,
		PoolReturnFailures:     sample.Aggregate.LeaseCounters.PoolReturnFailures,
		HitRatio:               poolRatio(sample.Aggregate.PoolCounters.Hits, reuseAttempts),
		RetainRatio:            poolRatio(sample.Aggregate.PoolCounters.Retains, putOutcomes),
		DropRatio:              poolRatio(sample.Aggregate.PoolCounters.Drops, putOutcomes),
		PoolReturnFailureRatio: poolRatio(sample.Aggregate.LeaseCounters.PoolReturnFailures, sample.Aggregate.LeaseCounters.PoolReturnAttempts),
		ActiveMemoryRatio:      poolRatio(sample.Aggregate.CurrentActiveBytes, ownedBytes),
		RetainedMemoryRatio:    poolRatio(sample.Aggregate.CurrentRetainedBytes, ownedBytes),
	}
}

// IsZero reports whether m contains no activity and no owned memory.
func (m PoolGroupMetrics) IsZero() bool {
	return m.CurrentRetainedBytes == 0 &&
		m.CurrentActiveBytes == 0 &&
		m.CurrentOwnedBytes == 0 &&
		m.Gets == 0 &&
		m.Hits == 0 &&
		m.Misses == 0 &&
		m.Puts == 0 &&
		m.Retains == 0 &&
		m.Drops == 0 &&
		m.LeaseAcquisitions == 0 &&
		m.LeaseReleases == 0 &&
		m.PoolReturnAttempts == 0 &&
		m.PoolReturnSuccesses == 0 &&
		m.PoolReturnFailures == 0
}
