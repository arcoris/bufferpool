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

import (
	"math"
	"testing"
)

func TestPoolGroupSampleAggregatesPartitions(t *testing.T) {
	group := testNewPoolGroup(t, "alpha", "beta")
	sample := group.Sample()
	if sample.PartitionCount != 2 {
		t.Fatalf("PartitionCount = %d, want 2", sample.PartitionCount)
	}
	if len(sample.Partitions) != 2 {
		t.Fatalf("len(Partitions) = %d, want 2", len(sample.Partitions))
	}
	if sample.PoolCount != 2 || sample.Aggregate.PoolCount != 2 {
		t.Fatalf("PoolCount = %d aggregate = %d, want 2", sample.PoolCount, sample.Aggregate.PoolCount)
	}
	if sample.Partitions[0].Name != "alpha" || sample.Partitions[1].Name != "beta" {
		t.Fatalf("partition order = %#v", sample.Partitions)
	}
}

func TestPoolGroupSampleIntoReusesPartitionSlice(t *testing.T) {
	group := testNewPoolGroup(t, "alpha", "beta")
	dst := PoolGroupSample{Partitions: make([]PoolGroupPartitionSample, 0, 4)}
	beforeCap := cap(dst.Partitions)
	group.SampleInto(&dst)
	if cap(dst.Partitions) != beforeCap {
		t.Fatalf("SampleInto cap = %d, want %d", cap(dst.Partitions), beforeCap)
	}
	if len(dst.Partitions) != 2 {
		t.Fatalf("len(dst.Partitions) = %d, want 2", len(dst.Partitions))
	}
}

// TestPoolGroupSampleIntoReusesNestedPartitionSamples verifies nested slice reuse.
func TestPoolGroupSampleIntoReusesNestedPartitionSamples(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	dst := PoolGroupSample{
		Partitions: []PoolGroupPartitionSample{{
			Sample: PoolPartitionSample{Pools: make([]PoolPartitionPoolSample, 0, 4)},
		}},
	}
	beforeCap := cap(dst.Partitions[0].Sample.Pools)

	group.SampleInto(&dst)
	if cap(dst.Partitions[0].Sample.Pools) != beforeCap {
		t.Fatalf("nested Pools cap = %d, want %d", cap(dst.Partitions[0].Sample.Pools), beforeCap)
	}
	if len(dst.Partitions[0].Sample.Pools) != 1 {
		t.Fatalf("len nested Pools = %d, want 1", len(dst.Partitions[0].Sample.Pools))
	}
}

func TestPoolGroupSampleIntoNilDstNoop(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	group.SampleInto(nil)
}

func TestAddPartitionSampleToGroupAggregate(t *testing.T) {
	var aggregate PoolPartitionSample
	addPartitionSampleToGroupAggregate(&aggregate, PoolPartitionSample{
		TotalPoolCount:       1,
		SampledPoolCount:     1,
		PoolCount:            1,
		PoolCounters:         PoolCountersSnapshot{Gets: 2, Hits: 1, CurrentRetainedBytes: 64},
		LeaseCounters:        LeaseCountersSnapshot{Acquisitions: 3, ActiveLeases: 1, ActiveBytes: 32},
		ActiveLeases:         1,
		CurrentRetainedBytes: 64,
		CurrentActiveBytes:   32,
	})
	addPartitionSampleToGroupAggregate(&aggregate, PoolPartitionSample{
		TotalPoolCount:       2,
		SampledPoolCount:     2,
		PoolCount:            2,
		PoolCounters:         PoolCountersSnapshot{Gets: 4, Hits: 2, CurrentRetainedBytes: 128},
		LeaseCounters:        LeaseCountersSnapshot{Acquisitions: 5, ActiveLeases: 2, ActiveBytes: 96},
		ActiveLeases:         2,
		CurrentRetainedBytes: 128,
		CurrentActiveBytes:   96,
	})
	if aggregate.PoolCount != 3 || aggregate.PoolCounters.Gets != 6 || aggregate.LeaseCounters.Acquisitions != 8 {
		t.Fatalf("aggregate = %#v", aggregate)
	}
	if aggregate.CurrentOwnedBytes != 320 {
		t.Fatalf("CurrentOwnedBytes = %d, want 320", aggregate.CurrentOwnedBytes)
	}
}

// TestPoolGroupSampleAggregatesRealPartitionActivity verifies real lease folding.
func TestPoolGroupSampleAggregatesRealPartitionActivity(t *testing.T) {
	group := testNewPoolGroup(t, "alpha", "beta")
	alphaLease, err := group.Acquire("alpha", "alpha-pool", 128)
	requireGroupNoError(t, err)
	betaLease, err := group.Acquire("beta", "beta-pool", 256)
	requireGroupNoError(t, err)
	betaReleased := false
	defer func() {
		if !betaReleased {
			requireGroupNoError(t, group.Release("beta", betaLease, betaLease.Buffer()))
		}
	}()

	requireGroupNoError(t, group.Release("alpha", alphaLease, alphaLease.Buffer()))
	sample := group.Sample()
	if sample.Aggregate.LeaseCounters.Acquisitions != 2 {
		t.Fatalf("Acquisitions = %d, want 2", sample.Aggregate.LeaseCounters.Acquisitions)
	}
	if sample.Aggregate.LeaseCounters.Releases != 1 {
		t.Fatalf("Releases = %d, want 1", sample.Aggregate.LeaseCounters.Releases)
	}
	if sample.Aggregate.ActiveLeases != 1 {
		t.Fatalf("ActiveLeases = %d, want 1", sample.Aggregate.ActiveLeases)
	}
	if sample.CurrentRetainedBytes == 0 || sample.CurrentActiveBytes == 0 {
		t.Fatalf("sample bytes retained=%d active=%d, want both non-zero", sample.CurrentRetainedBytes, sample.CurrentActiveBytes)
	}
	if sample.CurrentOwnedBytes != poolSaturatingAdd(sample.CurrentRetainedBytes, sample.CurrentActiveBytes) {
		t.Fatalf("CurrentOwnedBytes = %d, want retained+active", sample.CurrentOwnedBytes)
	}

	requireGroupNoError(t, group.Release("beta", betaLease, betaLease.Buffer()))
	betaReleased = true
}

// TestPoolGroupSampleSaturatesOwnedBytes verifies integer and byte saturation.
func TestPoolGroupSampleSaturatesOwnedBytes(t *testing.T) {
	var aggregate PoolPartitionSample
	addPartitionSampleToGroupAggregate(&aggregate, PoolPartitionSample{
		TotalPoolCount:       math.MaxInt,
		SampledPoolCount:     math.MaxInt,
		PoolCount:            math.MaxInt,
		CurrentRetainedBytes: math.MaxUint64 - 1,
	})
	addPartitionSampleToGroupAggregate(&aggregate, PoolPartitionSample{
		TotalPoolCount:     1,
		SampledPoolCount:   1,
		PoolCount:          1,
		CurrentActiveBytes: 8,
		LeaseCounters:      LeaseCountersSnapshot{ActiveBytes: 8},
	})
	if aggregate.TotalPoolCount != math.MaxInt || aggregate.SampledPoolCount != math.MaxInt || aggregate.PoolCount != math.MaxInt {
		t.Fatalf("pool counts did not saturate: total=%d sampled=%d pool=%d", aggregate.TotalPoolCount, aggregate.SampledPoolCount, aggregate.PoolCount)
	}
	if aggregate.CurrentOwnedBytes != math.MaxUint64 {
		t.Fatalf("CurrentOwnedBytes = %d, want MaxUint64", aggregate.CurrentOwnedBytes)
	}
}

// TestPoolGroupSamplePreservesDeterministicPartitionOrder verifies registry order.
func TestPoolGroupSamplePreservesDeterministicPartitionOrder(t *testing.T) {
	group := testNewPoolGroup(t, "gamma", "alpha", "beta")
	sample := group.Sample()
	got := []string{sample.Partitions[0].Name, sample.Partitions[1].Name, sample.Partitions[2].Name}
	want := []string{"gamma", "alpha", "beta"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("partition order = %#v, want %#v", got, want)
		}
	}
}
