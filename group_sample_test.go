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

import "testing"

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
