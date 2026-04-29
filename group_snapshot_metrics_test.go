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

func TestPoolGroupSnapshot(t *testing.T) {
	group := testNewPoolGroup(t, "alpha", "beta")
	snapshot := group.Snapshot()
	if snapshot.Name != "test-group" {
		t.Fatalf("Name = %q, want test-group", snapshot.Name)
	}
	if snapshot.PartitionCount() != 2 {
		t.Fatalf("PartitionCount = %d, want 2", snapshot.PartitionCount())
	}
	if len(snapshot.Partitions) != 2 {
		t.Fatalf("len(Partitions) = %d, want 2", len(snapshot.Partitions))
	}
	if snapshot.Metrics.PartitionCount != 2 {
		t.Fatalf("Metrics.PartitionCount = %d, want 2", snapshot.Metrics.PartitionCount)
	}
}

func TestPoolGroupMetrics(t *testing.T) {
	sample := testGroupSampleWithCounters(Generation(1), PoolCountersSnapshot{
		Gets:                 10,
		Hits:                 7,
		Misses:               3,
		Puts:                 5,
		Retains:              4,
		Drops:                1,
		CurrentRetainedBytes: 80,
	}, LeaseCountersSnapshot{
		Acquisitions:        6,
		Releases:            2,
		PoolReturnAttempts:  5,
		PoolReturnFailures:  1,
		PoolReturnSuccesses: 4,
		ActiveLeases:        1,
		ActiveBytes:         20,
	})
	metrics := newPoolGroupMetrics("group", sample)
	if metrics.Name != "group" || metrics.Gets != 10 || metrics.LeaseAcquisitions != 6 {
		t.Fatalf("metrics = %#v", metrics)
	}
	requireGroupRatioClose(t, metrics.HitRatio, 0.7)
	requireGroupRatioClose(t, metrics.RetainRatio, 0.8)
	requireGroupRatioClose(t, metrics.DropRatio, 0.2)
	requireGroupRatioClose(t, metrics.ActiveMemoryRatio, 0.2)
	requireGroupRatioClose(t, metrics.RetainedMemoryRatio, 0.8)
}

func TestPoolGroupMetricsIsZero(t *testing.T) {
	if !(PoolGroupMetrics{}).IsZero() {
		t.Fatalf("zero metrics should be zero")
	}
	if (PoolGroupMetrics{Gets: 1}).IsZero() {
		t.Fatalf("metrics with Gets should not be zero")
	}
}
