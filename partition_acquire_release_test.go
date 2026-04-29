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
	"errors"
	"sync"
	"testing"
)

// TestPoolPartitionAcquireReleaseThroughLeaseRegistry verifies partition-owned lease flow.
func TestPoolPartitionAcquireReleaseThroughLeaseRegistry(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	if lease.IsZero() {
		t.Fatalf("Acquire returned zero lease")
	}
	if lease.RequestedSize() != SizeFromBytes(300) {
		t.Fatalf("lease requested size = %s, want 300 B", lease.RequestedSize())
	}
	if lease.AcquiredCapacity() == 0 {
		t.Fatalf("lease acquired capacity must be positive")
	}

	buffer := lease.Buffer()
	if len(buffer) != 300 {
		t.Fatalf("lease buffer len = %d, want 300", len(buffer))
	}
	if uint64(cap(buffer)) != lease.AcquiredCapacity() {
		t.Fatalf("buffer cap = %d, want acquired capacity %d", cap(buffer), lease.AcquiredCapacity())
	}

	requirePartitionNoError(t, partition.Release(lease, buffer))

	metrics := partition.Metrics()
	if metrics.LeaseAcquisitions != 1 {
		t.Fatalf("LeaseAcquisitions = %d, want 1", metrics.LeaseAcquisitions)
	}
	if metrics.LeaseReleases != 1 {
		t.Fatalf("LeaseReleases = %d, want 1", metrics.LeaseReleases)
	}
	if metrics.PoolReturnAttempts != 1 {
		t.Fatalf("PoolReturnAttempts = %d, want 1", metrics.PoolReturnAttempts)
	}
	if metrics.PoolReturnSuccesses != 1 {
		t.Fatalf("PoolReturnSuccesses = %d, want 1", metrics.PoolReturnSuccesses)
	}
	if metrics.ActiveLeases != 0 || metrics.CurrentActiveBytes != 0 {
		t.Fatalf("active lease gauges = leases:%d bytes:%d, want zero", metrics.ActiveLeases, metrics.CurrentActiveBytes)
	}
}

// TestPoolPartitionAcquireRejectsUnknownPool verifies named Pool lookup failures.
func TestPoolPartitionAcquireRejectsUnknownPool(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	_, err := partition.Acquire("missing", 300)
	requirePartitionErrorIs(t, err, ErrInvalidOptions)

	_, err = partition.AcquireSize("missing", SizeFromBytes(300))
	requirePartitionErrorIs(t, err, ErrInvalidOptions)
}

// TestPoolPartitionAcquireZeroSizeIsRejectedBeforePool verifies lease-layer validation.
func TestPoolPartitionAcquireZeroSizeIsRejectedBeforePool(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	before, ok := partition.PoolMetrics("primary")
	if !ok {
		t.Fatalf("missing primary pool metrics")
	}

	_, err := partition.Acquire("primary", 0)
	requirePartitionErrorIs(t, err, ErrInvalidSize)
	after, ok := partition.PoolMetrics("primary")
	if !ok {
		t.Fatalf("missing primary pool metrics after acquire")
	}

	if after.Gets != before.Gets {
		t.Fatalf("Pool Gets changed after zero-size lease acquire: before=%d after=%d", before.Gets, after.Gets)
	}
	if after.Misses != before.Misses || after.Hits != before.Hits {
		t.Fatalf("Pool reuse counters changed after zero-size lease acquire")
	}
}

// TestPoolPartitionReleaseForeignLeaseIsRejectedByPartitionRegistry verifies registry ownership.
func TestPoolPartitionReleaseForeignLeaseIsRejectedByPartitionRegistry(t *testing.T) {
	left := testNewPoolPartition(t, "primary")
	right := testNewPoolPartition(t, "primary")

	lease, err := left.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	buffer := lease.Buffer()

	err = right.Release(lease, buffer)
	if !errors.Is(err, ErrInvalidLease) {
		t.Fatalf("Release through wrong partition error = %v, want ErrInvalidLease", err)
	}

	leftMetrics := left.Metrics()
	if leftMetrics.ActiveLeases != 1 {
		t.Fatalf("left ActiveLeases = %d, want 1 after wrong-partition release", leftMetrics.ActiveLeases)
	}
	rightMetrics := right.Metrics()
	if rightMetrics.LeaseReleases != 0 {
		t.Fatalf("right LeaseReleases = %d, want 0", rightMetrics.LeaseReleases)
	}

	requirePartitionNoError(t, left.Release(lease, buffer))
}

// TestPoolPartitionConcurrentAcquireRelease verifies concurrent partition operations.
func TestPoolPartitionConcurrentAcquireRelease(t *testing.T) {
	partition := testNewPoolPartition(t, "primary", "secondary")
	poolNames := []string{"primary", "secondary"}

	const workers = 8
	const iterations = 64

	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			poolName := poolNames[worker%len(poolNames)]
			for iteration := 0; iteration < iterations; iteration++ {
				lease, err := partition.Acquire(poolName, 300)
				if err != nil {
					t.Errorf("Acquire(%q) error = %v", poolName, err)
					return
				}
				buffer := lease.Buffer()
				if err := partition.Release(lease, buffer); err != nil {
					t.Errorf("Release(%q) error = %v", poolName, err)
					return
				}
			}
		}()
	}
	wg.Wait()

	metrics := partition.Metrics()
	want := uint64(workers * iterations)
	if metrics.LeaseAcquisitions != want {
		t.Fatalf("LeaseAcquisitions = %d, want %d", metrics.LeaseAcquisitions, want)
	}
	if metrics.LeaseReleases != want {
		t.Fatalf("LeaseReleases = %d, want %d", metrics.LeaseReleases, want)
	}
	if metrics.ActiveLeases != 0 || metrics.CurrentActiveBytes != 0 {
		t.Fatalf("active gauges after concurrent release = leases:%d bytes:%d, want zero", metrics.ActiveLeases, metrics.CurrentActiveBytes)
	}
}

// TestPoolPartitionConcurrentSampleWhileAcquireRelease verifies sampling during ownership churn.
func TestPoolPartitionConcurrentSampleWhileAcquireRelease(t *testing.T) {
	partition := testNewPoolPartition(t, "primary", "secondary")
	poolNames := []string{"primary", "secondary"}

	const workers = 4
	const iterations = 64

	var sampleWG sync.WaitGroup
	done := make(chan struct{})
	sampleWG.Add(1)
	go func() {
		defer sampleWG.Done()
		for {
			select {
			case <-done:
				return
			default:
				sample := partition.Sample()
				if sample.PolicyGeneration.IsZero() {
					t.Errorf("sample policy generation is zero")
					return
				}
			}
		}
	}()

	var workerWG sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			poolName := poolNames[worker%len(poolNames)]
			for iteration := 0; iteration < iterations; iteration++ {
				lease, err := partition.Acquire(poolName, 300)
				if err != nil {
					t.Errorf("Acquire(%q) error = %v", poolName, err)
					return
				}
				if err := partition.Release(lease, lease.Buffer()); err != nil {
					t.Errorf("Release(%q) error = %v", poolName, err)
					return
				}
			}
		}()
	}
	workerWG.Wait()
	close(done)
	sampleWG.Wait()

	metrics := partition.Metrics()
	if metrics.ActiveLeases != 0 || metrics.CurrentActiveBytes != 0 {
		t.Fatalf("active gauges after sampled concurrent release = leases:%d bytes:%d, want zero", metrics.ActiveLeases, metrics.CurrentActiveBytes)
	}
}

// TestPoolPartitionConcurrentSnapshotWhileAcquireRelease verifies diagnostic snapshots under ownership churn.
func TestPoolPartitionConcurrentSnapshotWhileAcquireRelease(t *testing.T) {
	partition := testNewPoolPartition(t, "primary", "secondary")
	poolNames := []string{"primary", "secondary"}

	const workers = 4
	const iterations = 64

	var snapshotWG sync.WaitGroup
	done := make(chan struct{})
	snapshotWG.Add(1)
	go func() {
		defer snapshotWG.Done()
		for {
			select {
			case <-done:
				return
			default:
				snapshot := partition.Snapshot()
				if snapshot.PolicyGeneration.IsZero() {
					t.Errorf("snapshot policy generation is zero")
					return
				}
			}
		}
	}()

	var workerWG sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			poolName := poolNames[worker%len(poolNames)]
			for iteration := 0; iteration < iterations; iteration++ {
				lease, err := partition.Acquire(poolName, 300)
				if err != nil {
					t.Errorf("Acquire(%q) error = %v", poolName, err)
					return
				}
				if err := partition.Release(lease, lease.Buffer()); err != nil {
					t.Errorf("Release(%q) error = %v", poolName, err)
					return
				}
			}
		}()
	}
	workerWG.Wait()
	close(done)
	snapshotWG.Wait()

	metrics := partition.Metrics()
	if metrics.ActiveLeases != 0 || metrics.CurrentActiveBytes != 0 {
		t.Fatalf("active gauges after snapshot concurrent release = leases:%d bytes:%d, want zero", metrics.ActiveLeases, metrics.CurrentActiveBytes)
	}
}
