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

// TestZeroLeaseMethods verifies that zero Lease handles are safe to inspect and
// fail closed on release.
func TestZeroLeaseMethods(t *testing.T) {
	t.Parallel()

	var lease Lease
	if !lease.IsZero() {
		t.Fatal("zero Lease did not report IsZero")
	}
	if !lease.ID().IsZero() {
		t.Fatalf("zero Lease ID = %s, want zero", lease.ID())
	}
	if lease.Buffer() != nil {
		t.Fatal("zero Lease Buffer() returned non-nil")
	}
	if lease.RequestedSize() != 0 {
		t.Fatalf("zero Lease RequestedSize() = %s, want 0", lease.RequestedSize())
	}
	if !lease.OriginClass().IsZero() {
		t.Fatalf("zero Lease OriginClass() = %s, want zero", lease.OriginClass())
	}
	if lease.AcquiredCapacity() != 0 {
		t.Fatalf("zero Lease AcquiredCapacity() = %d, want 0", lease.AcquiredCapacity())
	}
	if !lease.Snapshot().ID.IsZero() {
		t.Fatalf("zero Lease Snapshot ID = %s, want zero", lease.Snapshot().ID)
	}

	err := lease.Release(nil)
	if !errors.Is(err, ErrInvalidLease) {
		t.Fatalf("zero Lease Release error = %v, want ErrInvalidLease", err)
	}
}

// TestLeaseAcquireReleaseStrict verifies the ordinary strict ownership flow.
func TestLeaseAcquireReleaseStrict(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	lease, err := registry.Acquire(pool, 300)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}
	if lease.IsZero() {
		t.Fatal("Acquire() returned zero lease")
	}
	if lease.ID().IsZero() {
		t.Fatal("Acquire() returned zero lease ID")
	}
	if lease.RequestedSize() != SizeFromInt(300) {
		t.Fatalf("RequestedSize() = %s, want 300 B", lease.RequestedSize())
	}
	if lease.OriginClass() != ClassSizeFromBytes(512) {
		t.Fatalf("OriginClass() = %s, want 512 B", lease.OriginClass())
	}
	if lease.AcquiredCapacity() != 512 {
		t.Fatalf("AcquiredCapacity() = %d, want 512", lease.AcquiredCapacity())
	}

	buffer := lease.Buffer()
	if len(buffer) != 300 {
		t.Fatalf("len(Buffer()) = %d, want 300", len(buffer))
	}
	if cap(buffer) != 512 {
		t.Fatalf("cap(Buffer()) = %d, want 512", cap(buffer))
	}

	snapshot := registry.Snapshot()
	if snapshot.ActiveCount() != 1 {
		t.Fatalf("ActiveCount() = %d, want 1", snapshot.ActiveCount())
	}
	if snapshot.Counters.Acquisitions != 1 {
		t.Fatalf("Acquisitions = %d, want 1", snapshot.Counters.Acquisitions)
	}
	if snapshot.Counters.ActiveLeases != 1 {
		t.Fatalf("ActiveLeases = %d, want 1", snapshot.Counters.ActiveLeases)
	}
	if snapshot.Counters.ActiveBytes != 512 {
		t.Fatalf("ActiveBytes = %d, want 512", snapshot.Counters.ActiveBytes)
	}

	buffer[0] = 17
	if err := lease.Release(buffer); err != nil {
		t.Fatalf("Release() returned error: %v", err)
	}

	if lease.Buffer() != nil {
		t.Fatal("released Lease Buffer() returned non-nil")
	}

	after := registry.Snapshot()
	if after.ActiveCount() != 0 {
		t.Fatalf("ActiveCount() after release = %d, want 0", after.ActiveCount())
	}
	if after.Counters.Releases != 1 {
		t.Fatalf("Releases = %d, want 1", after.Counters.Releases)
	}
	if after.Counters.ActiveLeases != 0 {
		t.Fatalf("ActiveLeases after release = %d, want 0", after.Counters.ActiveLeases)
	}
	if after.Counters.ActiveBytes != 0 {
		t.Fatalf("ActiveBytes after release = %d, want 0", after.Counters.ActiveBytes)
	}
	if after.Counters.ReleasedBytes != 512 {
		t.Fatalf("ReleasedBytes = %d, want 512", after.Counters.ReleasedBytes)
	}
	if after.Counters.PoolReturnAttempts != 1 {
		t.Fatalf("PoolReturnAttempts = %d, want 1", after.Counters.PoolReturnAttempts)
	}
	if after.Counters.PoolReturnSuccesses != 1 {
		t.Fatalf("PoolReturnSuccesses = %d, want 1", after.Counters.PoolReturnSuccesses)
	}

	leaseSnapshot := lease.Snapshot()
	if !leaseSnapshot.IsReleased() {
		t.Fatalf("Lease snapshot state = %s, want released", leaseSnapshot.State)
	}
	if leaseSnapshot.ReturnedCapacity != 512 {
		t.Fatalf("ReturnedCapacity = %d, want 512", leaseSnapshot.ReturnedCapacity)
	}
}

// TestLeaseReleaseUnchanged verifies the convenience release path for callers
// that did not replace the acquired slice header.
func TestLeaseReleaseUnchanged(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}

	if err := lease.ReleaseUnchanged(); err != nil {
		t.Fatalf("ReleaseUnchanged() returned error: %v", err)
	}
}

// TestLeaseDoubleRelease verifies that copies of a Lease handle still point to
// the same record and double release is detected through shared record state.
func TestLeaseDoubleRelease(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}
	copyOfLease := lease

	if err := lease.ReleaseUnchanged(); err != nil {
		t.Fatalf("first ReleaseUnchanged() returned error: %v", err)
	}

	err = copyOfLease.ReleaseUnchanged()
	if !errors.Is(err, ErrDoubleRelease) {
		t.Fatalf("second ReleaseUnchanged() error = %v, want ErrDoubleRelease", err)
	}

	counters := registry.Snapshot().Counters
	if counters.DoubleReleases != 1 {
		t.Fatalf("DoubleReleases = %d, want 1", counters.DoubleReleases)
	}
	if counters.ReleaseAttempts() != 2 {
		t.Fatalf("ReleaseAttempts() = %d, want 2", counters.ReleaseAttempts())
	}
}

// TestLeaseReleaseForeignBufferStrict verifies that strict mode leaves the lease
// active after a failed release so the caller can retry with the correct buffer.
func TestLeaseReleaseForeignBufferStrict(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}

	foreign := make([]byte, 128, 512)
	err = lease.Release(foreign)
	if !errors.Is(err, ErrOwnershipViolation) {
		t.Fatalf("Release(foreign) error = %v, want ErrOwnershipViolation", err)
	}

	snapshot := registry.Snapshot()
	if snapshot.ActiveCount() != 1 {
		t.Fatalf("ActiveCount() after failed release = %d, want 1", snapshot.ActiveCount())
	}
	if snapshot.Counters.ForeignBufferReleases != 1 {
		t.Fatalf("ForeignBufferReleases = %d, want 1", snapshot.Counters.ForeignBufferReleases)
	}
	if snapshot.Active[0].LastViolation != OwnershipViolationForeignBuffer {
		t.Fatalf("LastViolation = %s, want %s", snapshot.Active[0].LastViolation, OwnershipViolationForeignBuffer)
	}

	if err := lease.ReleaseUnchanged(); err != nil {
		t.Fatalf("retry ReleaseUnchanged() returned error: %v", err)
	}
}

// TestLeaseReleaseShiftedSubsliceStrict verifies that strict mode requires the
// returned slice to preserve the acquired base pointer, not merely overlap the
// same backing allocation.
func TestLeaseReleaseShiftedSubsliceStrict(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}

	buffer := lease.Buffer()
	err = lease.Release(buffer[1:])
	if !errors.Is(err, ErrOwnershipViolation) {
		t.Fatalf("Release(shifted) error = %v, want ErrOwnershipViolation", err)
	}

	snapshot := registry.Snapshot()
	if snapshot.ActiveCount() != 1 {
		t.Fatalf("ActiveCount() after shifted release = %d, want 1", snapshot.ActiveCount())
	}
	if snapshot.Active[0].LastViolation != OwnershipViolationForeignBuffer {
		t.Fatalf("LastViolation = %s, want %s", snapshot.Active[0].LastViolation, OwnershipViolationForeignBuffer)
	}

	if err := lease.Release(buffer); err != nil {
		t.Fatalf("retry Release(original) returned error: %v", err)
	}
}

// TestLeaseReleaseClippedCapacityStrictCanonicalizesPoolReturn verifies that a
// strict release with the same base pointer but clipped capacity cannot make
// Pool classify the return by the clipped capacity.
func TestLeaseReleaseClippedCapacityStrictCanonicalizesPoolReturn(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	policy.Admission.UnsupportedClass = AdmissionActionError

	pool := MustNew(PoolConfig{Policy: policy})
	defer closePoolForTest(t, pool)

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}

	buffer := lease.Buffer()
	clipped := buffer[:0:1]
	if err := lease.Release(clipped); err != nil {
		t.Fatalf("Release(clipped) returned error: %v", err)
	}

	snapshot := registry.Snapshot()
	if snapshot.Counters.ReleasedBytes != lease.AcquiredCapacity() {
		t.Fatalf("ReleasedBytes = %d, want acquired capacity %d", snapshot.Counters.ReleasedBytes, lease.AcquiredCapacity())
	}
	if snapshot.Counters.PoolReturnSuccesses != 1 {
		t.Fatalf("PoolReturnSuccesses = %d, want 1", snapshot.Counters.PoolReturnSuccesses)
	}

	reused, err := pool.Get(128)
	if err != nil {
		t.Fatalf("Pool.Get() after clipped release returned error: %v", err)
	}
	if cap(reused) != int(lease.AcquiredCapacity()) {
		t.Fatalf("reused cap = %d, want original acquired capacity %d", cap(reused), lease.AcquiredCapacity())
	}
}

// TestLeaseReleaseAppendBeyondCapacityStrictRejectsReallocation verifies that
// ordinary Go slice growth past capacity is rejected as a foreign strict base.
func TestLeaseReleaseAppendBeyondCapacityStrictRejectsReallocation(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}

	buffer := lease.Buffer()
	grown := append(buffer[:cap(buffer)], 1)
	err = lease.Release(grown)
	if !errors.Is(err, ErrOwnershipViolation) {
		t.Fatalf("Release(grown) error = %v, want ErrOwnershipViolation", err)
	}

	if err := lease.Release(buffer); err != nil {
		t.Fatalf("retry Release(original) returned error: %v", err)
	}
}

// TestLeaseReleaseAppendWithinCapacityStrictSucceeds verifies that strict mode
// accepts ordinary length changes that keep the acquired base pointer.
func TestLeaseReleaseAppendWithinCapacityStrictSucceeds(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}

	buffer := append(lease.Buffer(), 1, 2, 3)
	if err := lease.Release(buffer); err != nil {
		t.Fatalf("Release(append within cap) returned error: %v", err)
	}
}

// TestLeaseReleaseForeignBufferAccounting verifies that accounting mode records
// ownership lifecycle without enforcing strict backing-array identity.
func TestLeaseReleaseForeignBufferAccounting(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	registry := MustNewLeaseRegistry(LeaseConfigFromOwnershipPolicy(AccountingOwnershipPolicy()))
	defer closeLeaseRegistryForTest(t, registry)

	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}

	foreign := make([]byte, 128, 512)
	if err := lease.Release(foreign); err != nil {
		t.Fatalf("accounting Release(foreign) returned error: %v", err)
	}

	counters := registry.Snapshot().Counters
	if counters.Releases != 1 {
		t.Fatalf("Releases = %d, want 1", counters.Releases)
	}
	if counters.OwnershipViolations != 0 {
		t.Fatalf("OwnershipViolations = %d, want 0", counters.OwnershipViolations)
	}
	if counters.PoolReturnSuccesses != 1 {
		t.Fatalf("PoolReturnSuccesses = %d, want 1", counters.PoolReturnSuccesses)
	}
}

// TestLeaseReleaseNilAndZeroCapacity verifies release input validation and
// active-lease preservation after failed validation.
func TestLeaseReleaseNilAndZeroCapacity(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}

	if err := lease.Release(nil); !errors.Is(err, ErrNilBuffer) {
		t.Fatalf("Release(nil) error = %v, want ErrNilBuffer", err)
	}
	if err := lease.Release(make([]byte, 0)); !errors.Is(err, ErrZeroCapacity) {
		t.Fatalf("Release(zero-capacity) error = %v, want ErrZeroCapacity", err)
	}

	snapshot := registry.Snapshot()
	if snapshot.ActiveCount() != 1 {
		t.Fatalf("ActiveCount() after failed releases = %d, want 1", snapshot.ActiveCount())
	}
	if snapshot.Counters.InvalidReleases != 2 {
		t.Fatalf("InvalidReleases = %d, want 2", snapshot.Counters.InvalidReleases)
	}

	if err := lease.ReleaseUnchanged(); err != nil {
		t.Fatalf("ReleaseUnchanged() after failed releases returned error: %v", err)
	}
}

// TestLeaseReleaseThroughWrongRegistry verifies registry-local lease ownership.
func TestLeaseReleaseThroughWrongRegistry(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	owner := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, owner)
	other := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, other)

	lease, err := owner.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}

	err = other.Release(lease, lease.Buffer())
	if !errors.Is(err, ErrInvalidLease) {
		t.Fatalf("foreign registry Release() error = %v, want ErrInvalidLease", err)
	}
	if other.Snapshot().Counters.InvalidReleases != 1 {
		t.Fatalf("other InvalidReleases = %d, want 1", other.Snapshot().Counters.InvalidReleases)
	}
	if owner.Snapshot().ActiveCount() != 1 {
		t.Fatalf("owner ActiveCount() = %d, want 1", owner.Snapshot().ActiveCount())
	}

	if err := lease.ReleaseUnchanged(); err != nil {
		t.Fatalf("owner release returned error: %v", err)
	}
}

// TestLeaseAcquireRejectsInvalidInputs verifies public acquisition validation.
func TestLeaseAcquireRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	if _, err := registry.Acquire(nil, 128); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Acquire(nil pool) error = %v, want ErrInvalidOptions", err)
	}
	if _, err := registry.Acquire(nil, -1); !errors.Is(err, ErrInvalidSize) {
		t.Fatalf("Acquire(negative size) error = %v, want ErrInvalidSize", err)
	}
}

// TestLeaseAcquireZeroSizeRejectedByLeaseLayer verifies that Pool's zero-size
// policy cannot influence lease acquisition semantics.
func TestLeaseAcquireZeroSizeRejectedByLeaseLayer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy ZeroSizeRequestPolicy
	}{
		{name: "empty buffer", policy: ZeroSizeRequestEmptyBuffer},
		{name: "smallest class", policy: ZeroSizeRequestSmallestClass},
		{name: "reject", policy: ZeroSizeRequestReject},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy := poolTestSmallSingleShardPolicy()
			policy.Admission.ZeroSizeRequests = tt.policy

			pool := MustNew(PoolConfig{Policy: policy})
			defer closePoolForTest(t, pool)
			registry := MustNewLeaseRegistry(DefaultLeaseConfig())
			defer closeLeaseRegistryForTest(t, registry)

			_, err := registry.Acquire(pool, 0)
			if !errors.Is(err, ErrInvalidSize) {
				t.Fatalf("Acquire(0) error = %v, want ErrInvalidSize", err)
			}

			_, err = registry.AcquireSize(pool, 0)
			if !errors.Is(err, ErrInvalidSize) {
				t.Fatalf("AcquireSize(0) error = %v, want ErrInvalidSize", err)
			}

			snapshot := pool.Snapshot()
			if snapshot.Counters.Gets != 0 {
				t.Fatalf("Pool Gets = %d, want 0 for lease-layer rejection", snapshot.Counters.Gets)
			}
			if registry.Snapshot().Counters.Acquisitions != 0 {
				t.Fatalf("registry acquisitions = %d, want 0", registry.Snapshot().Counters.Acquisitions)
			}
		})
	}
}

// TestLeaseReleaseAfterPoolCloseCompletesOwnership verifies that Pool.Put
// failure after successful ownership release is diagnostic only.
func TestLeaseReleaseAfterPoolCloseCompletesOwnership(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}
	buffer := lease.Buffer()

	closePoolForTest(t, pool)

	if err := lease.Release(buffer); err != nil {
		t.Fatalf("Release() after Pool.Close returned error: %v", err)
	}

	snapshot := registry.Snapshot()
	if snapshot.ActiveCount() != 0 {
		t.Fatalf("ActiveCount() = %d, want 0", snapshot.ActiveCount())
	}
	if snapshot.Counters.Releases != 1 {
		t.Fatalf("Releases = %d, want 1", snapshot.Counters.Releases)
	}
	if snapshot.Counters.PoolReturnAttempts != 1 {
		t.Fatalf("PoolReturnAttempts = %d, want 1", snapshot.Counters.PoolReturnAttempts)
	}
	if snapshot.Counters.PoolReturnFailures != 1 {
		t.Fatalf("PoolReturnFailures = %d, want 1", snapshot.Counters.PoolReturnFailures)
	}
	if snapshot.Counters.PoolReturnClosedFailures != 1 {
		t.Fatalf("PoolReturnClosedFailures = %d, want 1", snapshot.Counters.PoolReturnClosedFailures)
	}

	err = lease.Release(buffer)
	if !errors.Is(err, ErrDoubleRelease) {
		t.Fatalf("second Release() error = %v, want ErrDoubleRelease", err)
	}
}
