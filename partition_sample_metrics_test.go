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

// TestPoolPartitionSampleIncludesPoolAndLeaseState verifies aggregate sample contents.
func TestPoolPartitionSampleIncludesPoolAndLeaseState(t *testing.T) {
	partition := testNewPoolPartition(t, "primary", "secondary")

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)

	sample := partition.Sample()
	if sample.PoolCount != 2 {
		t.Fatalf("PoolCount = %d, want 2", sample.PoolCount)
	}
	if sample.PolicyGeneration != InitialGeneration {
		t.Fatalf("PolicyGeneration = %s, want %s", sample.PolicyGeneration, InitialGeneration)
	}
	if len(sample.Pools) != 2 {
		t.Fatalf("len(Pools) = %d, want 2", len(sample.Pools))
	}
	if sample.ActiveLeases != 1 {
		t.Fatalf("ActiveLeases = %d, want 1", sample.ActiveLeases)
	}
	if sample.CurrentActiveBytes != lease.AcquiredCapacity() {
		t.Fatalf("CurrentActiveBytes = %d, want acquired capacity %d", sample.CurrentActiveBytes, lease.AcquiredCapacity())
	}
	wantOwned := poolSaturatingAdd(sample.CurrentRetainedBytes, sample.CurrentActiveBytes)
	if sample.CurrentOwnedBytes != wantOwned {
		t.Fatalf("owned bytes = %d, want retained + active = %d", sample.CurrentOwnedBytes, wantOwned)
	}
	if sample.LeaseCounters.Acquisitions != 1 {
		t.Fatalf("Lease acquisitions = %d, want 1", sample.LeaseCounters.Acquisitions)
	}
	if sample.PoolCounters.Gets != 1 {
		t.Fatalf("Pool Gets = %d, want 1", sample.PoolCounters.Gets)
	}

	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
}

// TestPoolPartitionInternalSampleAvoidsLeaseSnapshotAllocation verifies cheap lease sampling.
func TestPoolPartitionInternalSampleAvoidsLeaseSnapshotAllocation(t *testing.T) {
	partition := testNewPoolPartition(t, "primary", "secondary")

	leases := make([]Lease, 16)
	for index := range leases {
		lease, err := partition.Acquire("primary", 128)
		requirePartitionNoError(t, err)
		leases[index] = lease
	}
	defer func() {
		for _, lease := range leases {
			requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
		}
	}()

	dst := PoolPartitionSample{Pools: make([]PoolPartitionPoolSample, 0, len(partition.PoolNames()))}
	allocs := testing.AllocsPerRun(100, func() {
		partition.sample(&dst)
	})
	if allocs != 0 {
		t.Fatalf("partition.sample allocations = %v, want 0", allocs)
	}
	if dst.ActiveLeases != len(leases) {
		t.Fatalf("ActiveLeases = %d, want %d", dst.ActiveLeases, len(leases))
	}
}

// TestPoolPartitionSampleDoesNotSharePoolSliceWithCaller verifies sample copy boundaries.
func TestPoolPartitionSampleDoesNotSharePoolSliceWithCaller(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	first := partition.Sample()
	if len(first.Pools) != 1 {
		t.Fatalf("len(first.Pools) = %d, want 1", len(first.Pools))
	}
	first.Pools[0].Name = "mutated"

	second := partition.Sample()
	if second.Pools[0].Name != "primary" {
		t.Fatalf("Sample shared pool slice with caller; got %q", second.Pools[0].Name)
	}
}

// TestPoolPartitionMetricsProjectsRatiosFromRawCounters verifies lifetime ratio projection.
func TestPoolPartitionMetricsProjectsRatiosFromRawCounters(t *testing.T) {
	sample := PoolPartitionSample{
		Lifecycle:            LifecycleActive,
		Generation:           InitialGeneration,
		PolicyGeneration:     Generation(7),
		PoolCount:            2,
		CurrentRetainedBytes: 75,
		CurrentActiveBytes:   25,
		CurrentOwnedBytes:    100,
		PoolCounters: PoolCountersSnapshot{
			Hits:    3,
			Misses:  1,
			Puts:    4,
			Retains: 2,
			Drops:   1,
		},
		LeaseCounters: LeaseCountersSnapshot{
			Acquisitions:       4,
			Releases:           3,
			PoolReturnAttempts: 4,
			PoolReturnFailures: 1,
		},
		ActiveLeases: 1,
	}

	metrics := newPoolPartitionMetrics("partition", sample)

	if metrics.Name != "partition" || metrics.PoolCount != 2 || metrics.ActiveLeases != 1 {
		t.Fatalf("metrics identity fields not projected correctly: %+v", metrics)
	}
	if metrics.Generation != InitialGeneration || metrics.PolicyGeneration != Generation(7) {
		t.Fatalf("metrics generations = %s/%s, want %s/7", metrics.Generation, metrics.PolicyGeneration, InitialGeneration)
	}
	if metrics.HitRatio != PolicyRatioFromBasisPoints(7500) {
		t.Fatalf("HitRatio = %d, want 7500", metrics.HitRatio)
	}
	if metrics.RetainRatio != PolicyRatioFromBasisPoints(6666) {
		t.Fatalf("RetainRatio = %d, want 6666", metrics.RetainRatio)
	}
	if metrics.DropRatio != PolicyRatioFromBasisPoints(3333) {
		t.Fatalf("DropRatio = %d, want 3333", metrics.DropRatio)
	}
	if metrics.PoolReturnFailureRatio != PolicyRatioFromBasisPoints(2500) {
		t.Fatalf("PoolReturnFailureRatio = %d, want 2500", metrics.PoolReturnFailureRatio)
	}
	if metrics.ActiveMemoryRatio != PolicyRatioFromBasisPoints(2500) {
		t.Fatalf("ActiveMemoryRatio = %d, want 2500", metrics.ActiveMemoryRatio)
	}
	if metrics.RetainedMemoryRatio != PolicyRatioFromBasisPoints(7500) {
		t.Fatalf("RetainedMemoryRatio = %d, want 7500", metrics.RetainedMemoryRatio)
	}
}

// TestPoolPartitionOwnedBytesProjectionUsesSaturatingAdd verifies owned-byte overflow safety.
func TestPoolPartitionOwnedBytesProjectionUsesSaturatingAdd(t *testing.T) {
	sample := PoolPartitionSample{CurrentRetainedBytes: ^uint64(0), CurrentActiveBytes: 1}

	if got := partitionOwnedBytesFromSample(sample); got != ^uint64(0) {
		t.Fatalf("owned bytes = %d, want saturated max", got)
	}

	metrics := newPoolPartitionMetrics("partition", sample)
	if metrics.CurrentOwnedBytes != ^uint64(0) {
		t.Fatalf("metrics owned bytes = %d, want saturated max", metrics.CurrentOwnedBytes)
	}

	budget := newPartitionBudgetSnapshot(PartitionBudgetPolicy{MaxOwnedBytes: SizeFromBytes(^uint64(0) - 1)}, sample)
	if budget.CurrentOwnedBytes != ^uint64(0) || !budget.OwnedOverBudget {
		t.Fatalf("budget owned projection = %+v, want saturated over-budget", budget)
	}

	pressure := newPartitionPressureSnapshot(PartitionPressurePolicy{Enabled: true, CriticalOwnedBytes: SizeFromBytes(^uint64(0) - 1)}, sample)
	if pressure.CurrentOwnedBytes != ^uint64(0) || pressure.Level != PressureLevelCritical {
		t.Fatalf("pressure owned projection = %+v, want saturated critical", pressure)
	}
}

// TestPoolPartitionMetricsIsZero verifies zero-value metrics behavior.
func TestPoolPartitionMetricsIsZero(t *testing.T) {
	if !(PoolPartitionMetrics{}).IsZero() {
		t.Fatalf("zero metrics should be zero")
	}
	if (PoolPartitionMetrics{CurrentOwnedBytes: 1}).IsZero() {
		t.Fatalf("metrics with owned bytes should not be zero")
	}
	if (PoolPartitionMetrics{LeaseAcquisitions: 1}).IsZero() {
		t.Fatalf("metrics with lease acquisitions should not be zero")
	}
}
