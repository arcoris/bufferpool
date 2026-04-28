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

import "time"

const (
	// errLeaseInvalid is used when a release handle is empty or malformed.
	errLeaseInvalid = "bufferpool.Lease: lease is invalid"

	// errLeaseForeignRegistry is used when a lease is released through a
	// registry other than the registry that created it.
	errLeaseForeignRegistry = "bufferpool.Lease: lease belongs to a different registry"
)

// Release completes ownership for lease and then attempts retained-storage
// handoff to the lease's origin Pool.
//
// Ownership validation failures leave the lease active when retry is safe.
// Successful validation consumes the lease before Pool.Put runs. A later
// Pool.Put failure is recorded as a pool-return diagnostic and Release still
// returns nil because ownership has already ended; retrying the same lease would
// be a double release.
func (r *LeaseRegistry) Release(lease Lease, buffer []byte) error {
	r.mustBeInitialized()
	if lease.registry != r {
		r.recordInvalidRelease(OwnershipViolationInvalidLease)
		return newError(ErrInvalidLease, errLeaseForeignRegistry)
	}

	record := lease.record
	if record == nil || record.registry != r {
		r.recordInvalidRelease(OwnershipViolationInvalidLease)
		return newError(ErrInvalidLease, errLeaseInvalid)
	}

	return r.releaseRecord(record, buffer)
}

// releaseRecord performs the locked ownership-state transition for a release.
//
// Lock order is registry first, then record. Snapshot uses the same order so
// concurrent snapshot/release cannot deadlock. Pool.Put is called only after
// both locks are released because Pool return admission may take shard locks and
// must not run while registry bookkeeping locks are held.
func (r *LeaseRegistry) releaseRecord(record *leaseRecord, buffer []byte) error {
	r.mu.Lock()
	record.mu.Lock()
	if record.state == LeaseStateReleased {
		record.mu.Unlock()
		r.mu.Unlock()
		r.recordInvalidRelease(OwnershipViolationDoubleRelease)
		return newError(ErrDoubleRelease, errLeaseDoubleRelease)
	}
	if violation, err := validateLeaseRelease(record, buffer, r.config.Ownership); err != nil {
		record.lastViolation = violation
		record.mu.Unlock()
		r.mu.Unlock()
		r.recordInvalidRelease(violation)
		return err
	}

	returnBuffer := canonicalLeaseReturnBuffer(record, buffer, r.config.Ownership)
	record.state = LeaseStateReleased
	record.returnedCapacity = uint64(cap(returnBuffer))
	record.releasedAt = time.Now()
	acquiredCapacity := record.acquiredCapacity
	returnedCapacity := record.returnedCapacity
	pool := record.pool
	delete(r.active, record.id)
	record.mu.Unlock()
	r.mu.Unlock()

	r.counters.recordRelease(acquiredCapacity, returnedCapacity)
	r.counters.recordPoolReturnAttempt()
	if err := pool.Put(returnBuffer); err != nil {
		r.counters.recordPoolReturnFailure(err)
		r.generation.Advance()
		return nil
	}

	r.counters.recordPoolReturnSuccess()
	r.generation.Advance()
	return nil
}

// recordInvalidRelease records a rejected ownership release and advances
// registry generation.
//
// Invalid release counters are snapshot-visible diagnostics, so generation moves
// even though active ownership state may remain unchanged.
func (r *LeaseRegistry) recordInvalidRelease(kind OwnershipViolationKind) {
	r.counters.recordInvalidRelease(kind)
	r.generation.Advance()
}

// canonicalLeaseReturnBuffer returns the slice header handed to Pool.Put after
// ownership validation succeeds.
//
// Strict mode canonicalizes to the originally acquired base and capacity. This
// lets callers release harmless clipped-capacity views without corrupting Pool's
// capacity-based class routing, while shifted subslices remain rejected during
// validation because their base pointer differs.
func canonicalLeaseReturnBuffer(record *leaseRecord, buffer []byte, policy OwnershipPolicy) []byte {
	if ownershipModeEnforcesStrictBufferIdentity(policy) {
		return record.buffer[:0]
	}

	return buffer
}
