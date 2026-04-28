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

// PoolPartitionMetrics combines retained-storage Pool counters with lease
// counters.
//
// Ratios are lifetime-derived diagnostics. Future adaptive controller scoring
// should use delta samples and windowed rates rather than treating these
// lifetime ratios as workload scores. Old history must not directly drive
// budget publication or trim decisions; controller code should compare bounded
// samples from explicit windows. Raw counters stay in lower layers; derived
// metrics are computed from samples.
type PoolPartitionMetrics struct {
	// Name is diagnostic partition metadata.
	Name string

	// Lifecycle is the observed partition lifecycle state.
	Lifecycle LifecycleState

	// Generation is the partition state/event generation.
	Generation Generation

	// PolicyGeneration is the partition runtime-policy generation.
	PolicyGeneration Generation

	// PoolCount is the number of Pools owned by the partition.
	PoolCount int

	// ActiveLeases is the number of currently checked-out leases.
	ActiveLeases int

	// CurrentRetainedBytes is retained storage currently held by Pools.
	CurrentRetainedBytes uint64

	// CurrentActiveBytes is checked-out backing capacity tracked by leases.
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

// Metrics returns an aggregate partition metrics projection.
func (p *PoolPartition) Metrics() PoolPartitionMetrics {
	p.mustBeInitialized()
	var sample PoolPartitionSample
	p.sampleSummary(&sample)
	return newPoolPartitionMetrics(p.name, sample)
}

// newPoolPartitionMetrics derives lifetime ratios from one aggregate sample.
func newPoolPartitionMetrics(name string, sample PoolPartitionSample) PoolPartitionMetrics {
	reuseAttempts := sample.PoolCounters.ReuseAttempts()
	putOutcomes := sample.PoolCounters.PutOutcomes()
	ownedBytes := partitionOwnedBytesFromSample(sample)
	return PoolPartitionMetrics{
		Name:                   name,
		Lifecycle:              sample.Lifecycle,
		Generation:             sample.Generation,
		PolicyGeneration:       sample.PolicyGeneration,
		PoolCount:              sample.PoolCount,
		ActiveLeases:           sample.ActiveLeases,
		CurrentRetainedBytes:   sample.CurrentRetainedBytes,
		CurrentActiveBytes:     sample.CurrentActiveBytes,
		CurrentOwnedBytes:      ownedBytes,
		Gets:                   sample.PoolCounters.Gets,
		Hits:                   sample.PoolCounters.Hits,
		Misses:                 sample.PoolCounters.Misses,
		Puts:                   sample.PoolCounters.Puts,
		Retains:                sample.PoolCounters.Retains,
		Drops:                  sample.PoolCounters.Drops,
		LeaseAcquisitions:      sample.LeaseCounters.Acquisitions,
		LeaseReleases:          sample.LeaseCounters.Releases,
		PoolReturnAttempts:     sample.LeaseCounters.PoolReturnAttempts,
		PoolReturnSuccesses:    sample.LeaseCounters.PoolReturnSuccesses,
		PoolReturnFailures:     sample.LeaseCounters.PoolReturnFailures,
		HitRatio:               poolRatio(sample.PoolCounters.Hits, reuseAttempts),
		RetainRatio:            poolRatio(sample.PoolCounters.Retains, putOutcomes),
		DropRatio:              poolRatio(sample.PoolCounters.Drops, putOutcomes),
		PoolReturnFailureRatio: poolRatio(sample.LeaseCounters.PoolReturnFailures, sample.LeaseCounters.PoolReturnAttempts),
		ActiveMemoryRatio:      poolRatio(sample.CurrentActiveBytes, ownedBytes),
		RetainedMemoryRatio:    poolRatio(sample.CurrentRetainedBytes, ownedBytes),
	}
}

// IsZero reports whether m contains no activity and no owned memory.
func (m PoolPartitionMetrics) IsZero() bool {
	return m.CurrentRetainedBytes == 0 && m.CurrentActiveBytes == 0 && m.CurrentOwnedBytes == 0 && m.Gets == 0 && m.Hits == 0 && m.Misses == 0 && m.Puts == 0 && m.Retains == 0 && m.Drops == 0 && m.LeaseAcquisitions == 0 && m.LeaseReleases == 0 && m.PoolReturnAttempts == 0 && m.PoolReturnSuccesses == 0 && m.PoolReturnFailures == 0
}
