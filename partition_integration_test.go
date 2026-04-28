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

// TestPoolPartitionMultipleRegistriesOverSamePoolKeepIndependentAccounting verifies registry isolation.
func TestPoolPartitionMultipleRegistriesOverSamePoolKeepIndependentAccounting(t *testing.T) {
	pool := MustNew(PoolConfig{Name: "shared"})
	t.Cleanup(func() { requirePartitionNoError(t, pool.Close()) })

	left := MustNewLeaseRegistry(DefaultLeaseConfig())
	right := MustNewLeaseRegistry(DefaultLeaseConfig())
	t.Cleanup(func() { requirePartitionNoError(t, left.Close()) })
	t.Cleanup(func() { requirePartitionNoError(t, right.Close()) })

	leftLease, err := left.Acquire(pool, 300)
	requirePartitionNoError(t, err)
	rightLease, err := right.Acquire(pool, 300)
	requirePartitionNoError(t, err)

	if left.Snapshot().ActiveCount() != 1 || right.Snapshot().ActiveCount() != 1 {
		t.Fatalf("each registry should see only its own active lease")
	}

	err = right.Release(leftLease, leftLease.Buffer())
	requirePartitionErrorIs(t, err, ErrInvalidLease)
	if left.Snapshot().ActiveCount() != 1 {
		t.Fatalf("wrong-registry release must not complete left lease")
	}

	requirePartitionNoError(t, left.Release(leftLease, leftLease.Buffer()))
	requirePartitionNoError(t, right.Release(rightLease, rightLease.Buffer()))
}

// TestPoolPartitionCloseOrderRegistryBeforePool verifies shutdown ordering behavior.
func TestPoolPartitionCloseOrderRegistryBeforePool(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	buffer := lease.Buffer()

	// Directly close the partition-owned registry to model the first phase of a
	// future Partition shutdown. Releasing before Pool close should still hand the
	// buffer back successfully.
	requirePartitionNoError(t, partition.leases.Close())
	requirePartitionNoError(t, partition.Release(lease, buffer))

	metrics := partition.Metrics()
	if metrics.PoolReturnSuccesses != 1 {
		t.Fatalf("PoolReturnSuccesses = %d, want 1 before Pool close", metrics.PoolReturnSuccesses)
	}
	if metrics.PoolReturnFailures != 0 {
		t.Fatalf("PoolReturnFailures = %d, want 0 before Pool close", metrics.PoolReturnFailures)
	}

	requirePartitionNoError(t, partition.Close())
}

// TestPoolPartitionPoolHandoffAdmissionFailureIsDiagnostic verifies release ownership completion.
func TestPoolPartitionPoolHandoffAdmissionFailureIsDiagnostic(t *testing.T) {
	policy := DefaultConfigPolicy()
	policy.Admission.ReturnedBuffers = ReturnedBufferPolicyDrop
	policy.Admission.OversizedReturn = AdmissionActionError

	config := testPartitionConfig("primary")
	config.Pools[0].Config.Policy = policy
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))

	metrics := partition.Metrics()
	if metrics.LeaseReleases != 1 {
		t.Fatalf("LeaseReleases = %d, want 1", metrics.LeaseReleases)
	}
	if metrics.PoolReturnAttempts != 1 {
		t.Fatalf("PoolReturnAttempts = %d, want 1", metrics.PoolReturnAttempts)
	}
	// ReturnedBufferPolicyDrop is a successful Pool handoff with no retained
	// storage, not an ownership failure and not a Pool.Put error.
	if metrics.PoolReturnFailures != 0 {
		t.Fatalf("PoolReturnFailures = %d, want 0", metrics.PoolReturnFailures)
	}
	if metrics.PoolReturnSuccesses != 1 {
		t.Fatalf("PoolReturnSuccesses = %d, want 1", metrics.PoolReturnSuccesses)
	}
}
