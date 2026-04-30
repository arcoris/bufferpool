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
	"sync"
	"time"
)

// leaseRecord is the mutable ownership state behind every Lease value.
//
// Lease values are copyable handles; the record is where single-release state,
// acquired identity, active accounting metadata, and last validation diagnostics
// live. The record belongs to exactly one LeaseRegistry and is removed from that
// registry's active map when ownership release succeeds.
type leaseRecord struct {
	// mu protects state, buffer metadata, release timestamps, and violation
	// diagnostics.
	mu sync.Mutex

	// id is the registry-local lease identifier.
	id LeaseID

	// registry is the owning LeaseRegistry.
	registry *LeaseRegistry

	// pool is the retained-storage owner that produced the acquired buffer.
	pool *Pool

	// state records whether ownership is still active or already released.
	state LeaseState

	// buffer is the originally acquired slice header.
	//
	// Strict release canonicalizes Pool handoff back to this base and
	// capacity after validation succeeds. The registry stores the header for
	// identity and handoff; it does not serialize caller reads or writes to the
	// backing array while the lease is active.
	buffer []byte

	// data is the acquired slice base used for strict identity validation.
	data *byte

	// requestedSize is the logical byte length requested by the caller.
	requestedSize Size

	// originClass is the class capacity selected by Pool acquisition.
	originClass ClassSize

	// acquiredCapacity is cap(buffer) at acquisition time.
	acquiredCapacity uint64

	// returnedCapacity is the capacity handed to Pool after successful
	// ownership release.
	returnedCapacity uint64

	// acquiredAt records when the lease was created.
	acquiredAt time.Time

	// releasedAt records when ownership release succeeded.
	releasedAt time.Time

	// lastViolation records the most recent rejected release reason while the
	// lease remains active.
	lastViolation OwnershipViolationKind
}

// snapshot returns a locally consistent observational view of the record.
//
// The snapshot does not include the buffer itself. It is safe for diagnostics
// and future controller sampling without exposing mutable caller-owned memory.
func (r *leaseRecord) snapshot() LeaseSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	var hold time.Duration
	switch {
	case !r.releasedAt.IsZero():
		hold = r.releasedAt.Sub(r.acquiredAt)
	case !r.acquiredAt.IsZero():
		hold = time.Since(r.acquiredAt)
	}

	return LeaseSnapshot{
		ID:               r.id,
		State:            r.state,
		RequestedSize:    r.requestedSize,
		OriginClass:      r.originClass,
		AcquiredCapacity: r.acquiredCapacity,
		ReturnedCapacity: r.returnedCapacity,
		AcquiredAt:       r.acquiredAt,
		ReleasedAt:       r.releasedAt,
		HoldDuration:     hold,
		LastViolation:    r.lastViolation,
	}
}
