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
	// errNilPoolGroup reports nil or zero-value PoolGroup receivers.
	errNilPoolGroup = "bufferpool.PoolGroup: receiver must not be nil"

	// errGroupPartitionMissing reports lookup failure for a group-owned partition.
	errGroupPartitionMissing = "bufferpool.PoolGroup: partition not found"

	// errGroupPoolMissing reports lookup failure for a group-owned Pool.
	errGroupPoolMissing = "bufferpool.PoolGroup: pool not found"

	// errGroupLeasePoolMissing reports a lease that does not belong to the group.
	errGroupLeasePoolMissing = "bufferpool.PoolGroup: lease pool not found"
)

// PoolGroup owns a deterministic set of PoolPartitions, a group-global Pool
// directory, and a manual group-level coordinator boundary.
//
// PoolGroup is the first owner above PoolPartition. It routes managed
// acquisition and release by Pool name through the owning partition while
// aggregating partition samples, bounded windows, rates, metrics, snapshots,
// and foreground coordinator reports. TickInto may publish partition retained
// budgets into owned PoolPartitions. It deliberately does not execute physical
// trim, scan shards directly, compute class EWMA, propagate pressure, or start
// background coordinator goroutines.
//
// Responsibility boundary:
//
//   - group.go owns the PoolGroup type, construction, metadata accessors, and
//     receiver validation;
//   - group_config.go owns construction config normalization and validation;
//   - group_policy.go owns manual group coordinator policy;
//   - group_lifecycle.go owns hard close behavior;
//   - group_registry.go owns the immutable partition registry;
//   - group_runtime.go owns group runtime policy snapshots;
//   - group_sample.go owns group sampling and aggregation;
//   - group_window.go owns bounded group windows;
//   - group_rate.go owns aggregate rate projection;
//   - group_score*.go owns score value projection;
//   - group_snapshot.go and group_metrics.go own diagnostics;
//   - group_coordinator*.go and group_controller_report.go own foreground
//     coordinator state, budget publication, and reports.
//
// Copying:
//
// PoolGroup MUST NOT be copied after first use. It embeds atomic lifecycle and
// generation state and owns partition pointers through an immutable registry.
type PoolGroup struct {
	// lifecycle gates group-level foreground work and hard close.
	lifecycle AtomicLifecycle

	// runtimeMu serializes group-routed foreground operations with hard close:
	// Acquire, AcquireSize, Release, and TickInto take the read side; Close takes
	// the write side. It is not a Pool hot-path lock, is not taken by direct
	// PoolPartition or Pool users, and does not make diagnostics transactional.
	// The lock protects only the group ownership boundary so Close cannot close
	// child partitions while group-routed work is inside that boundary.
	runtimeMu sync.RWMutex

	// generation tracks group-visible state and explicit coordinator events.
	generation AtomicGeneration

	// name is diagnostic group metadata.
	name string

	// config is the normalized construction config kept for diagnostics.
	config PoolGroupConfig

	// runtimeSnapshot publishes immutable group policy views.
	runtimeSnapshot atomic.Pointer[groupRuntimeSnapshot]

	// registry owns the deterministic set of group partitions.
	registry groupRegistry

	// poolDirectory maps group-global Pool names and owned Pool pointers to
	// immutable partition locations.
	poolDirectory groupPoolDirectory

	// scoreEvaluator owns prepared score adapters for group evaluation.
	scoreEvaluator PoolGroupScoreEvaluator

	// coordinator owns applied group-local state mutated only by TickInto.
	coordinator groupCoordinator
}

// NewPoolGroup constructs and activates a PoolGroup.
//
// NewPoolGroup constructs owned PoolPartitions from explicit partitions or from
// group-level Pool assignments. It does not start background work. Runtime
// partition budget publication happens only when callers explicitly invoke
// TickInto/Tick.
func NewPoolGroup(config PoolGroupConfig) (*PoolGroup, error) {
	normalized := config.Normalize()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	partitions, directory, err := newGroupPartitionAssignments(normalized)
	if err != nil {
		return nil, err
	}
	registry, err := newGroupRegistry(partitions)
	if err != nil {
		return nil, err
	}
	directory, err = directory.bindRegistry(registry)
	if err != nil {
		_ = registry.closeAll()
		return nil, err
	}
	normalized.Partitions = partitions
	normalized.Pools = nil
	group := &PoolGroup{
		name:           normalized.Name,
		config:         cloneGroupConfig(normalized),
		registry:       registry,
		poolDirectory:  directory,
		scoreEvaluator: NewPoolGroupScoreEvaluator(normalized.Policy.Score),
		coordinator:    newGroupCoordinator(normalized.Policy),
	}
	group.generation.Store(InitialGeneration)
	group.publishRuntimeSnapshot(newGroupRuntimeSnapshot(InitialGeneration, normalized.Policy))
	group.lifecycle.Activate()
	return group, nil
}

// MustNewPoolGroup constructs a PoolGroup and panics on failure.
func MustNewPoolGroup(config PoolGroupConfig) *PoolGroup {
	group, err := NewPoolGroup(config)
	if err != nil {
		panic(err)
	}
	return group
}

// Name returns the diagnostic group name.
func (g *PoolGroup) Name() string { g.mustBeInitialized(); return g.name }

// Config returns a defensive copy of the normalized construction config.
func (g *PoolGroup) Config() PoolGroupConfig {
	g.mustBeInitialized()
	return cloneGroupConfig(g.config)
}

// Policy returns the currently published group policy.
func (g *PoolGroup) Policy() PoolGroupPolicy {
	g.mustBeInitialized()
	return g.currentRuntimeSnapshot().Policy
}

// PartitionNames returns the deterministic group-local partition names.
func (g *PoolGroup) PartitionNames() []string {
	g.mustBeInitialized()
	return g.registry.namesCopy()
}

// PoolNames returns the deterministic group-global Pool names.
func (g *PoolGroup) PoolNames() []string {
	g.mustBeInitialized()
	return g.poolDirectory.namesCopy()
}

// PartitionSnapshot returns a diagnostic snapshot for one group-owned partition.
//
// PartitionSnapshot is diagnostic and remains available after group close. It
// does not expose the raw partition pointer and does not authorize data-plane
// work after shutdown begins.
func (g *PoolGroup) PartitionSnapshot(name string) (PoolPartitionSnapshot, bool) {
	g.mustBeInitialized()
	partition, ok := g.registry.partition(name)
	if !ok {
		return PoolPartitionSnapshot{}, false
	}
	return partition.Snapshot(), true
}

// PartitionMetrics returns diagnostic metrics for one group-owned partition.
//
// PartitionMetrics is diagnostic and remains available after group close. It
// reads through the owned PoolPartition without exposing it.
func (g *PoolGroup) PartitionMetrics(name string) (PoolPartitionMetrics, bool) {
	g.mustBeInitialized()
	partition, ok := g.registry.partition(name)
	if !ok {
		return PoolPartitionMetrics{}, false
	}
	return partition.Metrics(), true
}

// Acquire obtains an ownership lease through the group pool directory.
//
// Acquire validates the group lifecycle, resolves the group-global Pool name to
// its owning PoolPartition, and delegates to PoolPartition.Acquire so
// checked-out ownership remains in the partition-owned LeaseRegistry. It does
// not expose raw *PoolPartition or *Pool access, and it returns ErrClosed once
// group close has started.
func (g *PoolGroup) Acquire(poolName string, size int) (Lease, error) {
	g.mustBeInitialized()
	g.runtimeMu.RLock()
	defer g.runtimeMu.RUnlock()

	if !g.lifecycle.AllowsWork() {
		return Lease{}, newError(ErrClosed, errGroupClosed)
	}
	location, ok := g.poolDirectory.location(poolName)
	if !ok {
		return Lease{}, newError(ErrInvalidOptions, errGroupPoolMissing+": "+poolName)
	}
	partition, _ := g.registry.partition(location.PartitionName)
	return partition.Acquire(location.PoolName, size)
}

// AcquireSize is the Size-typed form of Acquire.
//
// The lifecycle, pool-directory lookup, and LeaseRegistry ownership semantics
// are identical to Acquire.
func (g *PoolGroup) AcquireSize(poolName string, size Size) (Lease, error) {
	g.mustBeInitialized()
	g.runtimeMu.RLock()
	defer g.runtimeMu.RUnlock()

	if !g.lifecycle.AllowsWork() {
		return Lease{}, newError(ErrClosed, errGroupClosed)
	}
	location, ok := g.poolDirectory.location(poolName)
	if !ok {
		return Lease{}, newError(ErrInvalidOptions, errGroupPoolMissing+": "+poolName)
	}
	partition, _ := g.registry.partition(location.PartitionName)
	return partition.AcquireSize(location.PoolName, size)
}

// Release releases a group-acquired lease through its owning partition.
//
// Release intentionally routes to PoolPartition.Release instead of calling
// Lease.Release directly. That preserves partition dirty marking and keeps the
// LeaseRegistry ownership boundary partition-local. Release remains available
// after group close starts so checked-out ownership can complete diagnostically,
// matching PoolPartition release semantics.
func (g *PoolGroup) Release(lease Lease, buffer []byte) error {
	g.mustBeInitialized()
	g.runtimeMu.RLock()
	defer g.runtimeMu.RUnlock()

	location, ok := g.poolDirectory.locationForLease(lease)
	if !ok {
		return newError(ErrInvalidLease, errGroupLeasePoolMissing)
	}
	partition, _ := g.registry.partition(location.PartitionName)
	return partition.Release(lease, buffer)
}

// AcquireFromPartition obtains a lease from a specific group-local partition.
//
// This is an advanced routing method for explicit partition-aware callers.
// Ordinary managed callers should use Acquire with a group-global Pool name.
func (g *PoolGroup) AcquireFromPartition(partitionName string, poolName string, size int) (Lease, error) {
	g.mustBeInitialized()
	g.runtimeMu.RLock()
	defer g.runtimeMu.RUnlock()

	if !g.lifecycle.AllowsWork() {
		return Lease{}, newError(ErrClosed, errGroupClosed)
	}
	partition, ok := g.registry.partition(partitionName)
	if !ok {
		return Lease{}, newError(ErrInvalidOptions, errGroupPartitionMissing+": "+partitionName)
	}
	return partition.Acquire(poolName, size)
}

// AcquireSizeFromPartition is the Size-typed form of AcquireFromPartition.
func (g *PoolGroup) AcquireSizeFromPartition(partitionName string, poolName string, size Size) (Lease, error) {
	g.mustBeInitialized()
	g.runtimeMu.RLock()
	defer g.runtimeMu.RUnlock()

	if !g.lifecycle.AllowsWork() {
		return Lease{}, newError(ErrClosed, errGroupClosed)
	}
	partition, ok := g.registry.partition(partitionName)
	if !ok {
		return Lease{}, newError(ErrInvalidOptions, errGroupPartitionMissing+": "+partitionName)
	}
	return partition.AcquireSize(poolName, size)
}

// ReleaseToPartition releases a lease through a specific group-local partition.
//
// This advanced method is useful for tests and partition-aware integrations.
// It still delegates to PoolPartition.Release and does not duplicate lease
// ownership validation.
func (g *PoolGroup) ReleaseToPartition(partitionName string, lease Lease, buffer []byte) error {
	g.mustBeInitialized()
	g.runtimeMu.RLock()
	defer g.runtimeMu.RUnlock()

	partition, ok := g.registry.partition(partitionName)
	if !ok {
		return newError(ErrInvalidOptions, errGroupPartitionMissing+": "+partitionName)
	}
	return partition.Release(lease, buffer)
}

// partition returns a raw group-owned partition for internal group code only.
//
// This helper must stay unexported. Public callers route data-plane work
// through PoolGroup Acquire/Release and diagnostics through snapshot/metrics
// methods so partition ownership and lease accounting remain intact.
func (g *PoolGroup) partition(name string) (*PoolPartition, bool) {
	return g.registry.partition(name)
}

// mustBeInitialized verifies that g was constructed by NewPoolGroup.
func (g *PoolGroup) mustBeInitialized() {
	if g == nil || g.runtimeSnapshot.Load() == nil || g.scoreEvaluator.isZero() {
		panic(errNilPoolGroup)
	}
}
