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
	"sync/atomic"
)

const (
	// errNilLeaseRegistry is used when a LeaseRegistry method is called on a nil
	// receiver or on a zero-value registry that was not constructed by
	// NewLeaseRegistry.
	errNilLeaseRegistry = "bufferpool.LeaseRegistry: receiver must not be nil"
)

// LeaseRegistry owns checked-out lease records for one ownership-aware runtime
// scope.
//
// The registry is deliberately separate from Pool. Pool remains the local
// retained-storage data plane. LeaseRegistry can be owned by a future
// PoolPartition or PoolGroup layer and can acquire from one or more Pools without
// forcing bare []byte Put to pretend it has ownership guarantees.
//
// LeaseRegistry owns checked-out identity, not retained storage. Pool owns
// retained storage, not checked-out ownership. Release completes ownership first
// and then attempts a best-effort Pool.Put handoff so a retained-storage failure
// cannot make an already released lease active again.
//
// Multiple registries may acquire from the same Pool, but their active lease and
// in-use accounting is independent. A future PoolPartition should provide the
// single registry for its owned pool set when coherent ownership accounting is
// required.
//
// Concurrency:
//
// LeaseRegistry is safe for concurrent Acquire, Release, Snapshot, and Close.
// Close prevents new acquisitions but does not invalidate active leases; release
// remains allowed so checked-out buffers can return during shutdown.
type LeaseRegistry struct {
	// lifecycle gates new acquisitions and exposes registry shutdown state.
	//
	// Release deliberately does not require the registry to be Active because
	// closing a registry should stop new ownership while allowing outstanding
	// ownership to complete.
	lifecycle AtomicLifecycle

	// generation advances whenever snapshot-visible registry state changes.
	//
	// The stream is registry-local. It is not comparable to Pool runtime
	// generations or to other LeaseRegistry instances as a global clock.
	generation AtomicGeneration

	// config is the normalized construction config.
	//
	// LeaseConfig is value-shaped today, so returning it from Config does not
	// expose mutable internal storage.
	config LeaseConfig

	// nextID allocates registry-local lease identifiers.
	nextID atomic.Uint64

	// mu protects active.
	//
	// releaseRecord locks mu before a lease record lock. Snapshot follows the
	// same order to avoid registry/record lock inversion.
	mu sync.Mutex

	// active contains currently checked-out leases by registry-local id.
	active map[LeaseID]*leaseRecord

	// counters records ownership lifetime, active gauges, validation failures,
	// and best-effort Pool handoff diagnostics.
	counters leaseCounters
}

// NewLeaseRegistry constructs and activates a lease registry.
//
// A zero LeaseConfig normalizes to strict ownership. The registry starts Active
// immediately and owns no background goroutines or controller loops.
func NewLeaseRegistry(config LeaseConfig) (*LeaseRegistry, error) {
	normalized := config.Normalize()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}

	registry := &LeaseRegistry{config: normalized, active: make(map[LeaseID]*leaseRecord)}
	registry.generation.Store(InitialGeneration)
	registry.lifecycle.Activate()
	return registry, nil
}

// MustNewLeaseRegistry constructs a LeaseRegistry and panics on invalid config.
//
// Use this helper in tests and static setup paths where invalid ownership
// configuration is a programmer error. User-facing construction should prefer
// NewLeaseRegistry and handle the returned error.
func MustNewLeaseRegistry(config LeaseConfig) *LeaseRegistry {
	registry, err := NewLeaseRegistry(config)
	if err != nil {
		panic(err)
	}
	return registry
}

// Config returns a copy of the normalized registry config.
//
// LeaseConfig currently contains no mutable slice or map fields. Returning it by
// value is still treated as a defensive boundary so future config fields can
// preserve the same accessor contract.
func (r *LeaseRegistry) Config() LeaseConfig {
	r.mustBeInitialized()

	return r.config
}

// OwnershipPolicy returns the normalized ownership policy used by this registry.
//
// The returned policy is value-shaped. It describes the registry's ownership
// semantics; mutating the returned value cannot change live registry behavior.
func (r *LeaseRegistry) OwnershipPolicy() OwnershipPolicy {
	r.mustBeInitialized()

	return r.config.Ownership
}

// mustBeInitialized verifies that r is a constructed LeaseRegistry.
//
// The zero value is intentionally not usable because it lacks the active map and
// normalized configuration required for safe ownership accounting.
func (r *LeaseRegistry) mustBeInitialized() {
	if r == nil {
		panic(errNilLeaseRegistry)
	}
	if r.active == nil {
		panic(errNilLeaseRegistry)
	}
}
