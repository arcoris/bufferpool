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

// Lease is an ownership-aware checked-out buffer handle.
//
// Lease is intentionally a small value handle. It may be copied by callers; all
// copies point at the same internal lease record. Double release is detected by
// the shared record, not by value identity.
//
// Callers that may grow the returned slice should release the final slice:
//
//	lease, err := registry.Acquire(pool, 1024)
//	if err != nil { ... }
//
//	buffer := lease.Buffer()
//	buffer = append(buffer, payload...)
//
//	err = lease.Release(buffer)
//
// ReleaseUnchanged is available when the caller did not replace the slice header.
//
// Lease does not make Pool a controller. A future PoolPartition can own a
// LeaseRegistry and use it to provide ownership-aware acquisition/release while
// Pool remains the local retained-storage data plane.
type Lease struct {
	registry *LeaseRegistry
	record   *leaseRecord
}

// IsZero reports whether lease is empty or invalid.
//
// A zero lease has no registry and no record, so it cannot be released. This is
// a value-handle check only; an already released non-zero lease is not zero and
// will be rejected by Release as a double release.
func (l Lease) IsZero() bool { return l.registry == nil || l.record == nil }

// ID returns the registry-local lease id.
//
// Zero leases return id 0. The id is useful for diagnostics but is not enough
// to release a buffer by itself; release still requires the Lease handle and its
// owning registry.
func (l Lease) ID() LeaseID {
	if l.record == nil {
		return 0
	}
	return l.record.id
}

// Buffer returns the currently recorded acquired buffer.
//
// The returned slice is a copy of the slice header. Mutating the slice contents
// mutates the checked-out buffer. If the caller appends and receives a new slice
// header, the final header should be passed to Release.
func (l Lease) Buffer() []byte {
	if l.record == nil {
		return nil
	}
	l.record.mu.Lock()
	defer l.record.mu.Unlock()
	if l.record.state != LeaseStateActive {
		return nil
	}
	return l.record.buffer
}

// RequestedSize returns the logical size requested at acquisition time.
//
// This value is independent from the backing capacity selected by Pool. It is
// retained for diagnostics and future controller sampling of requested-vs-held
// memory.
func (l Lease) RequestedSize() Size {
	if l.record == nil {
		return 0
	}
	return l.record.requestedSize
}

// OriginClass returns the class capacity selected at acquisition time.
//
// Strict release uses this class as the capacity identity of the checked-out
// buffer. It also feeds capacity-growth validation when that secondary guard is
// enabled.
func (l Lease) OriginClass() ClassSize {
	if l.record == nil {
		return 0
	}
	return l.record.originClass
}

// AcquiredCapacity returns the backing capacity acquired by the lease.
//
// Active byte gauges are incremented and decremented by this value so in-use
// accounting reflects checked-out backing memory, not the caller's current
// slice length.
func (l Lease) AcquiredCapacity() uint64 {
	if l.record == nil {
		return 0
	}
	return l.record.acquiredCapacity
}

// Snapshot returns an observational snapshot of this lease.
//
// The snapshot is safe to retain after the lease changes state. It does not
// expose the mutable buffer and does not synchronize with Pool storage.
func (l Lease) Snapshot() LeaseSnapshot {
	if l.record == nil {
		return LeaseSnapshot{}
	}
	return l.record.snapshot()
}

// Release releases the lease using buffer as the final returned slice.
//
// Release validates the lease token first. In strict mode it also validates that
// buffer preserves the acquired slice base identity. A shifted subslice is not a
// valid strict release even if it overlaps the same backing allocation.
//
// Release is an ownership operation, not a promise that retained storage will
// accept the returned buffer. Once ownership validation succeeds, the lease is
// marked released and active accounting is decremented. Pool.Put then runs as a
// best-effort retained-storage handoff. If that handoff fails, Release still
// returns nil because ownership has completed and retrying the same lease would
// be a double release; the registry records the handoff failure in counters.
func (l Lease) Release(buffer []byte) error {
	if l.registry == nil {
		return newError(ErrInvalidLease, errLeaseInvalid)
	}
	return l.registry.Release(l, buffer)
}

// ReleaseUnchanged releases the lease with the originally acquired slice header.
//
// Use Release when caller code may append to the buffer and receive a different
// slice header.
func (l Lease) ReleaseUnchanged() error { return l.Release(l.Buffer()) }
