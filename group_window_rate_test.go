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
	"testing"
	"time"
)

func TestPoolGroupWindowUsesAggregatePartitionWindow(t *testing.T) {
	previous := testGroupSampleWithCounters(Generation(1), PoolCountersSnapshot{Hits: 1, Misses: 1}, LeaseCountersSnapshot{Acquisitions: 1})
	current := testGroupSampleWithCounters(Generation(2), PoolCountersSnapshot{Hits: 4, Misses: 3}, LeaseCountersSnapshot{Acquisitions: 4})
	window := NewPoolGroupWindow(previous, current)
	if window.Generation != Generation(2) {
		t.Fatalf("Generation = %v, want 2", window.Generation)
	}
	if window.Delta.Aggregate.Hits != 3 || window.Delta.Aggregate.Misses != 2 || window.Delta.Aggregate.LeaseAcquisitions != 3 {
		t.Fatalf("Delta = %#v", window.Delta.Aggregate)
	}

	current.Aggregate.PoolCounters.Hits = 100
	if window.Current.Aggregate.PoolCounters.Hits == 100 {
		t.Fatalf("window retained caller-owned sample storage")
	}
}

func TestPoolGroupWindowResetReusesPartitionSlice(t *testing.T) {
	previous := PoolGroupSample{Partitions: []PoolGroupPartitionSample{{Name: "alpha"}}}
	current := PoolGroupSample{Partitions: []PoolGroupPartitionSample{{Name: "alpha"}, {Name: "beta"}}}
	window := PoolGroupWindow{Previous: PoolGroupSample{Partitions: make([]PoolGroupPartitionSample, 0, 4)}, Current: PoolGroupSample{Partitions: make([]PoolGroupPartitionSample, 0, 4)}}
	window.Reset(previous, current)
	if cap(window.Current.Partitions) != 4 {
		t.Fatalf("Current partition cap = %d, want 4", cap(window.Current.Partitions))
	}
	if len(window.Current.Partitions) != 2 {
		t.Fatalf("len(Current.Partitions) = %d, want 2", len(window.Current.Partitions))
	}
}

func TestPoolGroupTimedRatesMatchAggregatePartitionRates(t *testing.T) {
	previous := testGroupSampleWithCounters(Generation(1), PoolCountersSnapshot{Gets: 10, Hits: 6, Misses: 4, Puts: 5, Retains: 3, Drops: 2}, LeaseCountersSnapshot{Acquisitions: 2, Releases: 1})
	current := testGroupSampleWithCounters(Generation(2), PoolCountersSnapshot{Gets: 20, Hits: 14, Misses: 6, Puts: 9, Retains: 6, Drops: 3}, LeaseCountersSnapshot{Acquisitions: 5, Releases: 3})
	window := NewPoolGroupWindow(previous, current)
	rates := NewPoolGroupTimedWindowRates(window, 2*time.Second)
	partitionRates := NewPoolPartitionTimedWindowRates(NewPoolPartitionWindow(previous.Aggregate, current.Aggregate), 2*time.Second)
	if rates.Aggregate.HitRatio != partitionRates.HitRatio || rates.Aggregate.RetainRatio != partitionRates.RetainRatio {
		t.Fatalf("rates = %#v partitionRates = %#v", rates.Aggregate, partitionRates)
	}
	if rates.Aggregate.LeaseOpsPerSecond != 2.5 {
		t.Fatalf("LeaseOpsPerSecond = %v, want 2.5", rates.Aggregate.LeaseOpsPerSecond)
	}
}
