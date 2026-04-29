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

// PoolGroupSample is an aggregate group sample for foreground observations.
//
// The group sample is observational across partitions. It is not a global
// transaction: each partition sample is race-safe but may represent a nearby
// different instant under concurrent activity.
type PoolGroupSample struct {
	// Generation is the group state/event generation.
	Generation Generation

	// PolicyGeneration is the published group runtime-policy generation.
	PolicyGeneration Generation

	// Lifecycle is the observed group lifecycle state.
	Lifecycle LifecycleState

	// PartitionCount is the number of partitions owned by the group.
	PartitionCount int

	// PoolCount is the aggregate number of Pools represented by partitions.
	PoolCount int

	// Partitions contains per-partition samples in deterministic registry order.
	Partitions []PoolGroupPartitionSample

	// Aggregate is a partition-shaped aggregate across all group partitions.
	//
	// It intentionally contains no per-Pool records. It exists so group window,
	// rate, metrics, budget, pressure, and score projection can reuse the already
	// hardened partition aggregate algorithms without copying policy-application
	// behavior into PoolGroup.
	Aggregate PoolPartitionSample

	// CurrentRetainedBytes is retained storage currently held by all partitions.
	CurrentRetainedBytes uint64

	// CurrentActiveBytes is checked-out capacity currently tracked by all partitions.
	CurrentActiveBytes uint64

	// CurrentOwnedBytes is retained plus active bytes, with saturating addition.
	CurrentOwnedBytes uint64
}

// PoolGroupPartitionSample is one partition's sample inside a group sample.
type PoolGroupPartitionSample struct {
	// Name is the group-local partition name.
	Name string

	// Generation is the observed partition state/event generation.
	Generation Generation

	// PolicyGeneration is the observed partition runtime-policy generation.
	PolicyGeneration Generation

	// Lifecycle is the observed partition lifecycle state.
	Lifecycle LifecycleState

	// PoolCount is the number of Pools represented by the partition sample.
	PoolCount int

	// Sample is the partition sample captured for this group observation.
	Sample PoolPartitionSample
}

// Sample returns an observational group sample.
//
// Sample returns a value and may allocate per-partition and nested per-Pool
// sample storage. Frequent coordinator-style callers should use SampleInto with
// reusable destination capacity. Sampling is diagnostic and remains available
// after group close so callers can inspect final counters and ownership state.
func (g *PoolGroup) Sample() PoolGroupSample {
	var sample PoolGroupSample
	g.SampleInto(&sample)
	return sample
}

// SampleInto writes an observational group sample into dst.
//
// dst.Partitions capacity is reused. Nested partition samples also reuse their
// existing Pool slice capacity where possible. A nil dst is a no-op after
// receiver validation. dst must not be shared by concurrent callers without
// external synchronization.
func (g *PoolGroup) SampleInto(dst *PoolGroupSample) {
	g.mustBeInitialized()
	if dst == nil {
		return
	}
	g.sample(dst)
}

// sample writes a detailed group sample into dst.
func (g *PoolGroup) sample(dst *PoolGroupSample) {
	g.sampleWithRuntimeAndGeneration(dst, g.currentRuntimeSnapshot(), g.generation.Load())
}

// sampleWithRuntimeAndGeneration samples all group partitions against a fixed
// group generation and runtime-policy view.
//
// The fixed group values keep group-level metadata internally consistent inside
// one sample. Child partition samples are still independent observations, so a
// concurrent workload may be reflected at slightly different instants across
// partitions.
func (g *PoolGroup) sampleWithRuntimeAndGeneration(dst *PoolGroupSample, runtime *groupRuntimeSnapshot, generation Generation) {
	if dst == nil {
		return
	}
	partitions := dst.Partitions[:0]
	if cap(partitions) < g.registry.len() {
		partitions = make([]PoolGroupPartitionSample, 0, g.registry.len())
	}
	previous := dst.Partitions
	*dst = PoolGroupSample{
		Generation:       generation,
		PolicyGeneration: runtime.Generation,
		Lifecycle:        g.lifecycle.Load(),
		PartitionCount:   g.registry.len(),
		Partitions:       partitions,
		Aggregate: PoolPartitionSample{
			Generation:       generation,
			PolicyGeneration: runtime.Generation,
			Lifecycle:        g.lifecycle.Load(),
			Scope:            PoolPartitionSampleScopePartition,
		},
	}
	for index, entry := range g.registry.entries {
		partitionSample := reusablePartitionSample(previous, index)
		entry.partition.SampleInto(&partitionSample)
		dst.Partitions = append(dst.Partitions, PoolGroupPartitionSample{
			Name:             entry.name,
			Generation:       partitionSample.Generation,
			PolicyGeneration: partitionSample.PolicyGeneration,
			Lifecycle:        partitionSample.Lifecycle,
			PoolCount:        partitionSample.PoolCount,
			Sample:           partitionSample,
		})
		addPartitionSampleToGroupAggregate(&dst.Aggregate, partitionSample)
	}
	dst.PoolCount = dst.Aggregate.PoolCount
	dst.CurrentRetainedBytes = dst.Aggregate.CurrentRetainedBytes
	dst.CurrentActiveBytes = dst.Aggregate.CurrentActiveBytes
	dst.CurrentOwnedBytes = dst.Aggregate.CurrentOwnedBytes
}

// reusablePartitionSample returns a previous nested partition sample when its
// storage can be reused by the next group sample call.
func reusablePartitionSample(samples []PoolGroupPartitionSample, index int) PoolPartitionSample {
	if index < 0 || index >= len(samples) {
		return PoolPartitionSample{}
	}
	return samples[index].Sample
}

// addPartitionSampleToGroupAggregate folds one partition sample into dst.
//
// The aggregate is deliberately partition-shaped because the hardened
// partition window, rate, budget, pressure, and score code already understands
// that representation. The aggregate never carries per-Pool records.
func addPartitionSampleToGroupAggregate(dst *PoolPartitionSample, src PoolPartitionSample) {
	if dst == nil {
		return
	}
	dst.TotalPoolCount = groupSaturatingIntAdd(dst.TotalPoolCount, src.TotalPoolCount)
	dst.SampledPoolCount = groupSaturatingIntAdd(dst.SampledPoolCount, src.SampledPoolCount)
	dst.PoolCount = groupSaturatingIntAdd(dst.PoolCount, src.PoolCount)
	poolCountersAdd(&dst.PoolCounters, src.PoolCounters)
	addLeaseCountersSnapshot(&dst.LeaseCounters, src.LeaseCounters)
	dst.ActiveLeases = groupSaturatingIntAdd(dst.ActiveLeases, src.ActiveLeases)
	dst.CurrentRetainedBytes = poolSaturatingAdd(dst.CurrentRetainedBytes, src.CurrentRetainedBytes)
	dst.CurrentActiveBytes = poolSaturatingAdd(dst.CurrentActiveBytes, src.CurrentActiveBytes)
	dst.CurrentOwnedBytes = poolSaturatingAdd(dst.CurrentRetainedBytes, dst.CurrentActiveBytes)
}

// addLeaseCountersSnapshot adds src into dst with saturating arithmetic.
func addLeaseCountersSnapshot(dst *LeaseCountersSnapshot, src LeaseCountersSnapshot) {
	dst.Acquisitions = poolSaturatingAdd(dst.Acquisitions, src.Acquisitions)
	dst.RequestedBytes = poolSaturatingAdd(dst.RequestedBytes, src.RequestedBytes)
	dst.AcquiredBytes = poolSaturatingAdd(dst.AcquiredBytes, src.AcquiredBytes)
	dst.Releases = poolSaturatingAdd(dst.Releases, src.Releases)
	dst.ReleasedBytes = poolSaturatingAdd(dst.ReleasedBytes, src.ReleasedBytes)
	dst.ActiveLeases = poolSaturatingAdd(dst.ActiveLeases, src.ActiveLeases)
	dst.ActiveBytes = poolSaturatingAdd(dst.ActiveBytes, src.ActiveBytes)
	dst.InvalidReleases = poolSaturatingAdd(dst.InvalidReleases, src.InvalidReleases)
	dst.DoubleReleases = poolSaturatingAdd(dst.DoubleReleases, src.DoubleReleases)
	dst.OwnershipViolations = poolSaturatingAdd(dst.OwnershipViolations, src.OwnershipViolations)
	dst.ForeignBufferReleases = poolSaturatingAdd(dst.ForeignBufferReleases, src.ForeignBufferReleases)
	dst.CapacityGrowthViolations = poolSaturatingAdd(dst.CapacityGrowthViolations, src.CapacityGrowthViolations)
	dst.PoolReturnAttempts = poolSaturatingAdd(dst.PoolReturnAttempts, src.PoolReturnAttempts)
	dst.PoolReturnSuccesses = poolSaturatingAdd(dst.PoolReturnSuccesses, src.PoolReturnSuccesses)
	dst.PoolReturnFailures = poolSaturatingAdd(dst.PoolReturnFailures, src.PoolReturnFailures)
	dst.PoolReturnClosedFailures = poolSaturatingAdd(dst.PoolReturnClosedFailures, src.PoolReturnClosedFailures)
	dst.PoolReturnAdmissionFailures = poolSaturatingAdd(dst.PoolReturnAdmissionFailures, src.PoolReturnAdmissionFailures)
}

// groupSaturatingIntAdd returns left + right capped at the platform int max.
func groupSaturatingIntAdd(left int, right int) int {
	const maxInt = int(^uint(0) >> 1)
	if right > maxInt-left {
		return maxInt
	}
	return left + right
}
