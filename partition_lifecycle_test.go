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
	"testing"
)

// TestPoolPartitionLifecycleStartsActiveAndCloseIsIdempotent verifies lifecycle basics.
func TestPoolPartitionLifecycleStartsActiveAndCloseIsIdempotent(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	if got := partition.Lifecycle(); got != LifecycleActive {
		t.Fatalf("Lifecycle() = %s, want active", got)
	}
	if partition.IsClosed() {
		t.Fatalf("new partition reports closed")
	}

	requirePartitionNoError(t, partition.Close())
	if !partition.IsClosed() {
		t.Fatalf("partition should be closed after Close")
	}
	requirePartitionNoError(t, partition.Close())
}

// TestPoolPartitionAcquireAfterCloseIsRejected verifies hard-close acquisition gating.
func TestPoolPartitionAcquireAfterCloseIsRejected(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Close())

	_, err = partition.Acquire("primary", 300)
	requirePartitionErrorIs(t, err, ErrClosed)

	_, err = partition.AcquireSize("primary", SizeFromBytes(300))
	requirePartitionErrorIs(t, err, ErrClosed)
}

// TestPoolPartitionCloseAllowsActiveLeaseRelease verifies late release diagnostics.
func TestPoolPartitionCloseAllowsActiveLeaseRelease(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	buffer := lease.Buffer()

	requirePartitionNoError(t, partition.Close())
	requirePartitionNoError(t, partition.Release(lease, buffer))

	metrics := partition.Metrics()
	if metrics.ActiveLeases != 0 {
		t.Fatalf("ActiveLeases = %d, want 0", metrics.ActiveLeases)
	}
	if metrics.CurrentActiveBytes != 0 {
		t.Fatalf("CurrentActiveBytes = %d, want 0", metrics.CurrentActiveBytes)
	}
	if metrics.PoolReturnFailures != 1 {
		t.Fatalf("PoolReturnFailures = %d, want 1 after pools are closed by partition Close", metrics.PoolReturnFailures)
	}
	if metrics.PoolReturnSuccesses != 0 {
		t.Fatalf("PoolReturnSuccesses = %d, want 0 after closed-pool handoff", metrics.PoolReturnSuccesses)
	}

	err = partition.Release(lease, buffer)
	if !errors.Is(err, ErrDoubleRelease) {
		t.Fatalf("second Release error = %v, want ErrDoubleRelease", err)
	}
}

// TestPoolPartitionNilReceiverPanics verifies zero-value receiver invariants.
func TestPoolPartitionNilReceiverPanics(t *testing.T) {
	var partition *PoolPartition
	requirePartitionPanic(t, func() { _ = partition.Name() })
	requirePartitionPanic(t, func() { _ = partition.Metrics() })
	requirePartitionPanic(t, func() { _ = partition.Snapshot() })
}
