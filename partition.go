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
	// errNilPoolPartition reports nil or zero-value PoolPartition receivers.
	errNilPoolPartition = "bufferpool.PoolPartition: receiver must not be nil"

	// errPartitionPoolMissing reports lookup failure for a partition-local Pool.
	errPartitionPoolMissing = "bufferpool.PoolPartition: pool not found"
)

// PoolPartition owns a local set of Pools and one LeaseRegistry.
//
// PoolPartition is the first owner above Pool. It coordinates named Pools,
// checked-out lease accounting, aggregate samples, metrics, snapshots, and
// explicit controller ticks. It deliberately does not implement PoolGroup,
// global coordination, background goroutines, EWMA scoring, or adaptive loops.
type PoolPartition struct {
	// lifecycle gates partition-level work and hard close.
	lifecycle AtomicLifecycle

	// closeMu serializes hard cleanup across Close and the successful
	// CloseGracefully completion path. It lets hard Close finish a previous
	// graceful timeout while preventing duplicate LeaseRegistry/Pool cleanup.
	closeMu sync.Mutex

	// generation tracks partition-visible state and controller events.
	generation AtomicGeneration

	// name is diagnostic partition metadata.
	name string

	// config is the normalized construction config kept for diagnostics.
	config PoolPartitionConfig

	// runtimeSnapshot publishes immutable partition policy views.
	runtimeSnapshot atomic.Pointer[partitionRuntimeSnapshot]

	// registry owns the partition's immutable named Pool set.
	registry partitionRegistry

	// activeRegistry tracks partition-local active/dirty Pool indexes for
	// future controller sampling boundaries without adding Pool hot-path hooks.
	activeRegistry *partitionActiveRegistry

	// leases owns checked-out ownership records for partition acquisitions.
	leases *LeaseRegistry
}

// NewPoolPartition constructs and activates a PoolPartition.
func NewPoolPartition(config PoolPartitionConfig) (*PoolPartition, error) {
	normalized := config.Normalize()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	leases, err := NewLeaseRegistry(normalized.Lease)
	if err != nil {
		return nil, err
	}
	registry, err := newPartitionRegistry(normalized.Pools)
	if err != nil {
		_ = leases.Close()
		return nil, err
	}
	partition := &PoolPartition{
		name:           normalized.Name,
		config:         clonePartitionConfig(normalized),
		registry:       registry,
		activeRegistry: newPartitionActiveRegistry(registry.names),
		leases:         leases,
	}
	partition.generation.Store(InitialGeneration)
	partition.publishRuntimeSnapshot(newPartitionRuntimeSnapshot(InitialGeneration, normalized.Policy))
	partition.lifecycle.Activate()
	return partition, nil
}

// MustNewPoolPartition constructs a PoolPartition and panics on failure.
func MustNewPoolPartition(config PoolPartitionConfig) *PoolPartition {
	partition, err := NewPoolPartition(config)
	if err != nil {
		panic(err)
	}
	return partition
}

// Name returns the diagnostic partition name.
func (p *PoolPartition) Name() string { p.mustBeInitialized(); return p.name }

// Config returns a defensive copy of the normalized construction config.
func (p *PoolPartition) Config() PoolPartitionConfig {
	p.mustBeInitialized()
	return clonePartitionConfig(p.config)
}

// Policy returns the currently published partition policy.
func (p *PoolPartition) Policy() PartitionPolicy {
	p.mustBeInitialized()
	return p.currentRuntimeSnapshot().Policy
}

// PoolSnapshot returns a diagnostic snapshot for one partition-owned Pool.
//
// The partition does not expose raw *Pool access through its public API because
// direct Pool.Get/Put would bypass the partition-owned LeaseRegistry and break
// active ownership accounting. Diagnostics should use this accessor or
// PoolMetrics instead.
func (p *PoolPartition) PoolSnapshot(name string) (PoolSnapshot, bool) {
	p.mustBeInitialized()
	pool, ok := p.pool(name)
	if !ok {
		return PoolSnapshot{}, false
	}

	return pool.Snapshot(), true
}

// PoolMetrics returns diagnostic metrics for one partition-owned Pool.
func (p *PoolPartition) PoolMetrics(name string) (PoolMetrics, bool) {
	p.mustBeInitialized()
	pool, ok := p.pool(name)
	if !ok {
		return PoolMetrics{}, false
	}

	return pool.Metrics(), true
}

// PoolNames returns the deterministic list of partition-local Pool names.
func (p *PoolPartition) PoolNames() []string { p.mustBeInitialized(); return p.registry.namesCopy() }

// Acquire obtains an ownership lease from the named Pool.
func (p *PoolPartition) Acquire(poolName string, size int) (Lease, error) {
	p.mustBeInitialized()
	if !p.lifecycle.AllowsWork() {
		return Lease{}, newError(ErrClosed, errPartitionClosed)
	}
	pool, ok := p.pool(poolName)
	if !ok {
		return Lease{}, newError(ErrInvalidOptions, errPartitionPoolMissing+": "+poolName)
	}
	index, _ := p.registry.poolIndex(poolName)
	lease, err := p.leases.Acquire(pool, size)
	if err != nil {
		return Lease{}, err
	}
	p.markPoolActiveAndDirty(index)
	return lease, nil
}

// AcquireSize is the Size-typed form of Acquire.
func (p *PoolPartition) AcquireSize(poolName string, size Size) (Lease, error) {
	p.mustBeInitialized()
	if !p.lifecycle.AllowsWork() {
		return Lease{}, newError(ErrClosed, errPartitionClosed)
	}
	pool, ok := p.pool(poolName)
	if !ok {
		return Lease{}, newError(ErrInvalidOptions, errPartitionPoolMissing+": "+poolName)
	}
	index, _ := p.registry.poolIndex(poolName)
	lease, err := p.leases.AcquireSize(pool, size)
	if err != nil {
		return Lease{}, err
	}
	p.markPoolActiveAndDirty(index)
	return lease, nil
}

// Release releases a lease through the partition-owned LeaseRegistry.
func (p *PoolPartition) Release(lease Lease, buffer []byte) error {
	p.mustBeInitialized()
	if err := p.leases.Release(lease, buffer); err != nil {
		return err
	}
	p.markReleasedLeasePoolDirty(lease)
	return nil
}

// pool returns a raw partition-owned Pool for internal partition code.
//
// This helper is deliberately unexported. Public callers should acquire leases
// through PoolPartition so checked-out ownership remains visible to the
// partition's LeaseRegistry.
func (p *PoolPartition) pool(name string) (*Pool, bool) {
	return p.registry.pool(name)
}

// markPoolActiveAndDirty records partition-owned activity outside Pool hot paths.
func (p *PoolPartition) markPoolActiveAndDirty(index int) {
	_ = p.activeRegistry.markActiveIndex(index)
	_ = p.activeRegistry.markDirtyIndex(index)
}

// markReleasedLeasePoolDirty records successful release activity for an owned Pool.
//
// Lease currently carries package-internal record metadata that identifies the
// Pool used for acquisition. This helper isolates that direct record access so a
// future LeaseRegistry release outcome can replace it without spreading lease
// internals through PoolPartition. It runs only after ownership release
// succeeds; invalid, wrong-registry, and double-release attempts must not dirty
// partition control state.
func (p *PoolPartition) markReleasedLeasePoolDirty(lease Lease) {
	if lease.record == nil {
		return
	}
	if index, ok := p.registry.poolIndexForPool(lease.record.pool); ok {
		p.markPoolActiveAndDirty(index)
	}
}

// mustBeInitialized verifies that p was constructed by NewPoolPartition.
func (p *PoolPartition) mustBeInitialized() {
	if p == nil || p.leases == nil || p.activeRegistry == nil || p.runtimeSnapshot.Load() == nil {
		panic(errNilPoolPartition)
	}
}
