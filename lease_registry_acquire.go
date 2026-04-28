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
	// errLeaseAcquireNilPool is used when acquisition has no Pool data-plane
	// owner to allocate or reuse a buffer from.
	errLeaseAcquireNilPool = "bufferpool.LeaseRegistry.Acquire: pool must not be nil"

	// errLeaseAcquireZeroSize is used when lease acquisition requests no logical
	// bytes.
	errLeaseAcquireZeroSize = "bufferpool.LeaseRegistry.Acquire: requested size must be greater than zero"

	// errLeaseAcquireZeroCapacity is used when Pool returned a buffer that
	// cannot represent reusable owned capacity.
	errLeaseAcquireZeroCapacity = "bufferpool.LeaseRegistry.Acquire: acquired buffer has zero capacity"

	// errLeaseAcquireUnsupportedClass is used when Pool returned a capacity that
	// cannot be mapped back to one of its configured classes.
	errLeaseAcquireUnsupportedClass = "bufferpool.LeaseRegistry.Acquire: acquired capacity is not supported by pool classes"
)

// Acquire gets a positive-size buffer from pool and records a lease for it.
//
// Zero-size lease acquisition is rejected by the ownership layer before calling
// Pool. Lease ownership needs a reusable backing capacity to track; it must not
// inherit Pool's policy-specific zero-size request behavior.
func (r *LeaseRegistry) Acquire(pool *Pool, size int) (Lease, error) {
	if size < 0 {
		return Lease{}, newError(ErrInvalidSize, errPoolGetNegativeSize)
	}
	if size == 0 {
		return Lease{}, newError(ErrInvalidSize, errLeaseAcquireZeroSize)
	}

	return r.AcquireSize(pool, SizeFromInt(size))
}

// AcquireSize is the Size-typed form of Acquire.
//
// The method delegates allocation/reuse to Pool.GetSize only after lease-layer
// validation succeeds. That keeps ownership acquisition semantics independent
// from Pool zero-size request policy and keeps rejected lease acquisitions out
// of Pool counters.
func (r *LeaseRegistry) AcquireSize(pool *Pool, requestedSize Size) (Lease, error) {
	r.mustBeInitialized()

	if pool == nil {
		return Lease{}, newError(ErrInvalidOptions, errLeaseAcquireNilPool)
	}
	if requestedSize.IsZero() {
		return Lease{}, newError(ErrInvalidSize, errLeaseAcquireZeroSize)
	}
	if !r.lifecycle.AllowsWork() {
		return Lease{}, newError(ErrClosed, errLeaseRegistryClosed)
	}

	buffer, err := pool.GetSize(requestedSize)
	if err != nil {
		return Lease{}, err
	}
	if cap(buffer) == 0 {
		return Lease{}, newError(ErrZeroCapacity, errLeaseAcquireZeroCapacity)
	}

	originClass, ok := pool.originClassForAcquiredCapacity(uint64(cap(buffer)))
	if !ok {
		return Lease{}, newError(ErrUnsupportedClass, errLeaseAcquireUnsupportedClass)
	}

	record := &leaseRecord{
		id:               LeaseID(r.nextID.Add(1)),
		registry:         r,
		pool:             pool,
		state:            LeaseStateActive,
		buffer:           buffer,
		data:             bufferBackingData(buffer),
		requestedSize:    requestedSize,
		originClass:      originClass.Size(),
		acquiredCapacity: uint64(cap(buffer)),
		acquiredAt:       time.Now(),
	}

	r.mu.Lock()
	if !r.lifecycle.AllowsWork() {
		r.mu.Unlock()
		// No lease was created in this rollback path. Pool.GetSize succeeded,
		// but registry shutdown won the race before the record entered active
		// ownership. The Pool.Put call is a best-effort return of a buffer that
		// never became checked-out lease state, so it is intentionally not
		// counted as a lease pool-return handoff. Future PoolPartition shutdown
		// ordering should normally close registries before pools so this path is
		// rare and remains a simple rollback.
		_ = pool.Put(buffer)
		return Lease{}, newError(ErrClosed, errLeaseRegistryClosed)
	}
	r.active[record.id] = record
	r.counters.recordAcquire(requestedSize, record.acquiredCapacity)
	r.generation.Advance()
	r.mu.Unlock()

	return Lease{registry: r, record: record}, nil
}
