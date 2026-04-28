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

// TestLeaseCountersSnapshotZero verifies the public zero predicate.
func TestLeaseCountersSnapshotZero(t *testing.T) {
	t.Parallel()

	var snapshot LeaseCountersSnapshot
	if !snapshot.IsZero() {
		t.Fatalf("zero LeaseCountersSnapshot did not report IsZero: %#v", snapshot)
	}
	if snapshot.ReleaseAttempts() != 0 {
		t.Fatalf("zero ReleaseAttempts() = %d, want 0", snapshot.ReleaseAttempts())
	}

	snapshot.ActiveLeases = 1
	if snapshot.IsZero() {
		t.Fatal("non-zero LeaseCountersSnapshot reported IsZero")
	}
}

// TestLeaseCountersAcquireRelease verifies lifetime counters and active gauges.
func TestLeaseCountersAcquireRelease(t *testing.T) {
	t.Parallel()

	var counters leaseCounters
	counters.recordAcquire(SizeFromBytes(300), 512)
	counters.recordAcquire(SizeFromBytes(700), 1024)

	snapshot := counters.snapshot()
	if snapshot.Acquisitions != 2 {
		t.Fatalf("Acquisitions = %d, want 2", snapshot.Acquisitions)
	}
	if snapshot.RequestedBytes != 1000 {
		t.Fatalf("RequestedBytes = %d, want 1000", snapshot.RequestedBytes)
	}
	if snapshot.AcquiredBytes != 1536 {
		t.Fatalf("AcquiredBytes = %d, want 1536", snapshot.AcquiredBytes)
	}
	if snapshot.ActiveLeases != 2 {
		t.Fatalf("ActiveLeases = %d, want 2", snapshot.ActiveLeases)
	}
	if snapshot.ActiveBytes != 1536 {
		t.Fatalf("ActiveBytes = %d, want 1536", snapshot.ActiveBytes)
	}

	counters.recordRelease(512, 512)
	counters.recordRelease(1024, 2048)

	snapshot = counters.snapshot()
	if snapshot.Releases != 2 {
		t.Fatalf("Releases = %d, want 2", snapshot.Releases)
	}
	if snapshot.ReleasedBytes != 2560 {
		t.Fatalf("ReleasedBytes = %d, want 2560", snapshot.ReleasedBytes)
	}
	if snapshot.ActiveLeases != 0 {
		t.Fatalf("ActiveLeases = %d, want 0", snapshot.ActiveLeases)
	}
	if snapshot.ActiveBytes != 0 {
		t.Fatalf("ActiveBytes = %d, want 0", snapshot.ActiveBytes)
	}
	if snapshot.ReleaseAttempts() != 2 {
		t.Fatalf("ReleaseAttempts() = %d, want 2", snapshot.ReleaseAttempts())
	}

	counters.recordPoolReturnAttempt()
	counters.recordPoolReturnSuccess()
	counters.recordPoolReturnAttempt()
	counters.recordPoolReturnFailure(ErrClosed)
	counters.recordPoolReturnAttempt()
	counters.recordPoolReturnFailure(ErrRetentionStorageFull)

	snapshot = counters.snapshot()
	if snapshot.PoolReturnAttempts != 3 {
		t.Fatalf("PoolReturnAttempts = %d, want 3", snapshot.PoolReturnAttempts)
	}
	if snapshot.PoolReturnSuccesses != 1 {
		t.Fatalf("PoolReturnSuccesses = %d, want 1", snapshot.PoolReturnSuccesses)
	}
	if snapshot.PoolReturnFailures != 2 {
		t.Fatalf("PoolReturnFailures = %d, want 2", snapshot.PoolReturnFailures)
	}
	if snapshot.PoolReturnClosedFailures != 1 {
		t.Fatalf("PoolReturnClosedFailures = %d, want 1", snapshot.PoolReturnClosedFailures)
	}
	if snapshot.PoolReturnAdmissionFailures != 1 {
		t.Fatalf("PoolReturnAdmissionFailures = %d, want 1", snapshot.PoolReturnAdmissionFailures)
	}
}

// TestLeaseCountersInvalidReleaseKinds verifies violation-specific counters.
func TestLeaseCountersInvalidReleaseKinds(t *testing.T) {
	t.Parallel()

	var counters leaseCounters
	counters.recordInvalidRelease(OwnershipViolationInvalidLease)
	counters.recordInvalidRelease(OwnershipViolationNilBuffer)
	counters.recordInvalidRelease(OwnershipViolationZeroCapacity)
	counters.recordInvalidRelease(OwnershipViolationDoubleRelease)
	counters.recordInvalidRelease(OwnershipViolationForeignBuffer)
	counters.recordInvalidRelease(OwnershipViolationCapacityGrowth)

	snapshot := counters.snapshot()
	if snapshot.InvalidReleases != 3 {
		t.Fatalf("InvalidReleases = %d, want 3", snapshot.InvalidReleases)
	}
	if snapshot.DoubleReleases != 1 {
		t.Fatalf("DoubleReleases = %d, want 1", snapshot.DoubleReleases)
	}
	if snapshot.OwnershipViolations != 2 {
		t.Fatalf("OwnershipViolations = %d, want 2", snapshot.OwnershipViolations)
	}
	if snapshot.ForeignBufferReleases != 1 {
		t.Fatalf("ForeignBufferReleases = %d, want 1", snapshot.ForeignBufferReleases)
	}
	if snapshot.CapacityGrowthViolations != 1 {
		t.Fatalf("CapacityGrowthViolations = %d, want 1", snapshot.CapacityGrowthViolations)
	}
	if snapshot.ReleaseAttempts() != 6 {
		t.Fatalf("ReleaseAttempts() = %d, want 6", snapshot.ReleaseAttempts())
	}
}
