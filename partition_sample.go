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

import controlseries "arcoris.dev/bufferpool/internal/control/series"

// PoolPartitionSampleScope describes which Pool set contributed retained-storage
// counters to a partition sample.
type PoolPartitionSampleScope = controlseries.SampleScope

const (
	// PoolPartitionSampleScopeUnset is the zero value for hand-built samples.
	PoolPartitionSampleScopeUnset PoolPartitionSampleScope = controlseries.SampleScopeUnset

	// PoolPartitionSampleScopePartition means the sample represents every
	// Pool owned by the partition.
	PoolPartitionSampleScopePartition PoolPartitionSampleScope = controlseries.SampleScopeFull

	// PoolPartitionSampleScopeSelectedPools means retained-storage counters
	// were collected from a selected Pool index set. LeaseRegistry counters are
	// still partition-wide until per-Pool lease accounting exists.
	PoolPartitionSampleScopeSelectedPools PoolPartitionSampleScope = controlseries.SampleScopeSelected
)

// PoolPartitionSample is an aggregate partition sample for explicit controller
// ticks.
//
// Generation is the partition state/event generation. PolicyGeneration is the
// currently published partition runtime-policy generation. Keeping them separate
// prevents controller reports from mixing partition events with policy
// publication versions.
type PoolPartitionSample struct {
	// Generation is the partition state/event generation.
	Generation Generation

	// PolicyGeneration is the published partition runtime-policy generation.
	PolicyGeneration Generation

	// Lifecycle is the observed partition lifecycle state.
	Lifecycle LifecycleState

	// Scope identifies whether retained-storage counters came from the full
	// partition or from selected Pool indexes.
	Scope PoolPartitionSampleScope

	// TotalPoolCount is the number of Pools owned by the partition.
	TotalPoolCount int

	// SampledPoolCount is the number of Pools represented in PoolCounters and
	// Pools. For full partition samples it equals TotalPoolCount.
	SampledPoolCount int

	// PoolCount is the number of Pools represented by this sample. Public
	// partition samples represent every owned Pool; selected-index controller
	// samples may represent only a subset. It is retained as a compatibility
	// alias for SampledPoolCount.
	PoolCount int

	// Pools contains per-Pool counter samples.
	Pools []PoolPartitionPoolSample

	// PoolCounters aggregates retained-storage counters across owned Pools.
	PoolCounters PoolCountersSnapshot

	// LeaseLifecycle is the observed partition LeaseRegistry lifecycle state.
	LeaseLifecycle LifecycleState

	// LeaseGeneration is the observed partition LeaseRegistry generation.
	LeaseGeneration Generation

	// LeaseCounters contains ownership-layer counters from the partition
	// LeaseRegistry.
	LeaseCounters LeaseCountersSnapshot

	// ActiveLeases is the current active lease count.
	ActiveLeases int

	// CurrentRetainedBytes is retained storage currently owned by sampled Pools.
	CurrentRetainedBytes uint64

	// CurrentActiveBytes is checked-out capacity currently owned by all
	// partition leases. Selected-Pool samples cannot scope this value until
	// per-Pool lease accounting exists.
	CurrentActiveBytes uint64

	// CurrentOwnedBytes is retained plus active bytes, with saturating addition.
	// For selected-Pool samples this combines selected retained bytes with
	// partition-wide active bytes, so it is not a selected-Pool ownership total.
	CurrentOwnedBytes uint64
}

// PoolPartitionPoolSample is one Pool's aggregate sample inside a partition.
type PoolPartitionPoolSample struct {
	// Name is the partition-local Pool name.
	Name string

	// Generation is the Pool runtime-policy generation.
	Generation Generation

	// Lifecycle is the Pool lifecycle state.
	Lifecycle LifecycleState

	// ClassCount is the number of configured Pool classes.
	ClassCount int

	// ShardCount is the aggregate shard count across Pool classes.
	ShardCount int

	// Counters is the Pool's aggregate counter sample.
	Counters PoolCountersSnapshot
}

// Sample returns an observational partition sample.
//
// Sample returns a value and may allocate per-Pool sample storage. Frequent
// controller-style callers should use SampleInto with a preallocated Pools
// slice to reuse storage.
func (p *PoolPartition) Sample() PoolPartitionSample {
	var sample PoolPartitionSample
	p.SampleInto(&sample)
	return sample
}

// SampleInto writes an observational partition sample into dst.
//
// PoolPartition owns the sampling boundary above Pool and LeaseRegistry:
// retained-storage counters come from Pool samples, while active ownership
// counters come from the partition-owned LeaseRegistry. dst.Pools capacity is
// reused, so callers can avoid per-call allocation by preallocating capacity for
// PoolCount entries. A nil dst is a no-op after receiver validation.
func (p *PoolPartition) SampleInto(dst *PoolPartitionSample) {
	p.mustBeInitialized()
	if dst == nil {
		return
	}
	p.sample(dst)
}

// sample writes a detailed controller-facing sample into dst.
func (p *PoolPartition) sample(dst *PoolPartitionSample) {
	p.sampleWithRuntimeAndGeneration(dst, p.currentRuntimeSnapshot(), p.generation.Load(), true)
}

// sampleSummary writes aggregate counters without per-Pool sample records.
func (p *PoolPartition) sampleSummary(dst *PoolPartitionSample) {
	p.sampleWithRuntimeAndGeneration(dst, p.currentRuntimeSnapshot(), p.generation.Load(), false)
}

// sampleWithRuntimeAndGeneration samples all Pools and leases against a fixed
// partition generation and runtime-policy view.
func (p *PoolPartition) sampleWithRuntimeAndGeneration(dst *PoolPartitionSample, runtime *partitionRuntimeSnapshot, generation Generation, includePools bool) {
	if dst == nil {
		return
	}
	pools := dst.Pools[:0]
	if includePools && cap(pools) < p.registry.len() {
		pools = make([]PoolPartitionPoolSample, 0, p.registry.len())
	}
	if !includePools {
		pools = nil
	}
	*dst = PoolPartitionSample{
		Generation:       generation,
		PolicyGeneration: runtime.Generation,
		Lifecycle:        p.lifecycle.Load(),
		Scope:            PoolPartitionSampleScopePartition,
		TotalPoolCount:   p.registry.len(),
		SampledPoolCount: p.registry.len(),
		PoolCount:        p.registry.len(),
		Pools:            pools,
	}
	for _, entry := range p.registry.entries {
		var poolSample poolCounterSample
		entry.pool.sampleCounters(&poolSample)
		if includePools {
			dst.Pools = append(dst.Pools, PoolPartitionPoolSample{
				Name:       entry.name,
				Generation: poolSample.Generation,
				Lifecycle:  poolSample.Lifecycle,
				ClassCount: poolSample.ClassCount,
				ShardCount: poolSample.ShardCount,
				Counters:   poolSample.Counters,
			})
		}
		poolCountersAdd(&dst.PoolCounters, poolSample.Counters)
	}
	p.finishSampleWithLeaseCounters(dst)
}

// sampleIndexesWithRuntimeAndGeneration samples selected Pool indexes.
//
// This is the controller boundary for future active-only sampling. The current
// active registry still marks all Pools active, so ordinary TickInto remains an
// all-Pool detailed scan. Tests and future controller code can use this helper
// to verify that aggregation no longer assumes "every registry entry" as the
// only possible sampling shape.
//
// indexes must come from partition registry order, usually activeRegistry. An
// invalid index is an internal caller bug. LeaseRegistry counters in the result
// are partition-wide, not scoped to the selected Pools.
func (p *PoolPartition) sampleIndexesWithRuntimeAndGeneration(dst *PoolPartitionSample, runtime *partitionRuntimeSnapshot, generation Generation, indexes []int, includePools bool) {
	if dst == nil {
		return
	}
	pools := dst.Pools[:0]
	if includePools && cap(pools) < len(indexes) {
		pools = make([]PoolPartitionPoolSample, 0, len(indexes))
	}
	if !includePools {
		pools = nil
	}
	*dst = PoolPartitionSample{
		Generation:       generation,
		PolicyGeneration: runtime.Generation,
		Lifecycle:        p.lifecycle.Load(),
		Scope:            PoolPartitionSampleScopeSelectedPools,
		TotalPoolCount:   p.registry.len(),
		SampledPoolCount: len(indexes),
		PoolCount:        len(indexes),
		Pools:            pools,
	}
	for _, index := range indexes {
		entry := p.registry.entries[index]
		var poolSample poolCounterSample
		entry.pool.sampleCounters(&poolSample)
		if includePools {
			dst.Pools = append(dst.Pools, PoolPartitionPoolSample{
				Name:       entry.name,
				Generation: poolSample.Generation,
				Lifecycle:  poolSample.Lifecycle,
				ClassCount: poolSample.ClassCount,
				ShardCount: poolSample.ShardCount,
				Counters:   poolSample.Counters,
			})
		}
		poolCountersAdd(&dst.PoolCounters, poolSample.Counters)
	}
	p.finishSampleWithLeaseCounters(dst)
}

// finishSampleWithLeaseCounters adds LeaseRegistry counters and owned-byte gauges.
func (p *PoolPartition) finishSampleWithLeaseCounters(dst *PoolPartitionSample) {
	var leaseSample leaseCounterSample
	p.leases.sampleCounters(&leaseSample)
	dst.LeaseLifecycle = leaseSample.Lifecycle
	dst.LeaseGeneration = leaseSample.Generation
	dst.LeaseCounters = leaseSample.Counters
	dst.ActiveLeases = leaseSample.ActiveCount
	dst.CurrentRetainedBytes = dst.PoolCounters.CurrentRetainedBytes
	dst.CurrentActiveBytes = dst.LeaseCounters.ActiveBytes
	dst.CurrentOwnedBytes = poolSaturatingAdd(dst.CurrentRetainedBytes, dst.CurrentActiveBytes)
}

// partitionOwnedBytesFromSample returns retained plus active bytes safely.
func partitionOwnedBytesFromSample(sample PoolPartitionSample) uint64 {
	// Hand-built tests may set only CurrentOwnedBytes. Real samples always carry
	// retained and active gauges, and those are the authoritative inputs.
	ownedBytes := poolSaturatingAdd(sample.CurrentRetainedBytes, sample.CurrentActiveBytes)
	if ownedBytes == 0 && sample.CurrentOwnedBytes != 0 {
		return sample.CurrentOwnedBytes
	}
	return ownedBytes
}
