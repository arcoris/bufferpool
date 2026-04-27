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

// Pool is the pool-level data-plane owner for reusable byte-buffer capacity.
//
// Pool can be constructed directly for the current static bounded mode, and it
// is also the component that future PoolPartition wiring will manage. It owns
// local class/shard storage and hot-path admission, but it does not own group
// coordination, partition control loops, adaptive scoring, or global memory
// governance.
//
// Pool is the first concrete data-plane owner above the internal static runtime
// core:
//
//	Pool
//	-> classTable
//	-> classState
//	-> shard
//	-> bucket
//
// Responsibility boundary:
//
//   - pool.go owns the Pool type, construction, normalized construction config,
//     effective policy, and stable metadata accessors;
//   - pool_lifecycle.go owns lifecycle state, Close, operation admission gates,
//     operation drain waiting, and close-time retained-storage cleanup;
//   - pool_classes.go owns class-table/class-state wiring, class lookup helpers,
//     initial class-budget publication, and shard-selector construction;
//   - pool_admission.go owns owner-side admission action mapping;
//   - pool_get.go owns public and internal acquisition paths;
//   - pool_put.go owns public and internal return/retention paths;
//   - pool_snapshot.go owns public or internal Pool snapshots;
//   - pool_metrics.go owns metrics-oriented projection.
//
// Pool deliberately does not implement PoolGroup, PoolPartition,
// GroupCoordinator, PartitionController, EWMA scoring, workload windows,
// adaptive redistribution, public metric exporters, or ownership leases. Those
// are higher-level layers and must not be hidden inside Pool. Future control
// planes should publish immutable runtime snapshots into Pool instead of being
// called by Get, Put, or other hot-path operations.
//
// Concurrency:
//
// Pool is safe for concurrent data-plane operations after construction.
// Lifecycle admission is handled by pool_lifecycle.go. Retained-storage mutation
// is delegated to classState/shard/bucket, where shard is the hot-path
// synchronization boundary.
//
// Copying:
//
// Pool MUST NOT be copied after first use. It contains atomics, mutexes,
// condition variables, class states, shards, and buckets.
type Pool struct {
	// lifecycle is the owner lifecycle state.
	//
	// It starts in LifecycleCreated as the zero value and is moved to
	// LifecycleActive only after New has finished wiring the Pool.
	lifecycle AtomicLifecycle

	// activeOperations counts data-plane operations admitted by the lifecycle
	// gate.
	//
	// The counter is owned by pool_lifecycle.go. It lets Close wait for
	// operations that passed the Active lifecycle check before Closing became
	// visible.
	activeOperations atomic.Int64

	// closeMu and closeCond coordinate Close with activeOperations and
	// concurrent Close callers.
	//
	// The lifecycle state and active operation count are atomic. The condition
	// variable prevents busy waiting while Close waits for operation drain or
	// final Closed state.
	closeMu   sync.Mutex
	closeCond *sync.Cond

	// name is diagnostic metadata only.
	//
	// It MUST NOT participate in class lookup, retention admission, shard
	// selection, ownership validation, or budget distribution.
	name string

	// config is the normalized construction config used to build this Pool.
	//
	// It is stored for diagnostics and for lifecycle behavior that belongs to
	// construction config, such as close-time cleanup and post-close operation
	// handling.
	config PoolConfig

	// constructionPolicy is the normalized policy used when this Pool was built.
	//
	// Runtime admission reads runtimeSnapshot. This construction copy remains
	// useful for diagnostics and for future code that needs to distinguish the
	// initial policy from controller-published runtime views.
	constructionPolicy Policy

	// runtimeSnapshot is the immutable policy view used by hot-path admission.
	runtimeSnapshot atomic.Pointer[poolRuntimeSnapshot]

	// ownerCounters records valid Put attempts and owner-side drops that do not
	// reach classState/shard counters.
	ownerCounters poolOwnerCounters

	// table maps requested sizes and returned capacities to configured classes.
	//
	// The table is immutable after construction.
	table classTable

	// classes are class runtime states indexed by ClassID assigned by table
	// construction.
	//
	// Each classState owns class-local counters, class budget, shard list, and
	// shard-local retained storage.
	classes []classState

	// selectors choose shard indexes for classState operations.
	//
	// The slice is indexed by ClassID. Selector state is class-local to avoid a
	// single pool-wide atomic counter under round-robin or random selection.
	selectors []shardSelector
}

// New constructs and activates a standalone Pool.
//
// Construction flow:
//
//   - normalize PoolConfig;
//   - validate the normalized config;
//   - defensively copy the effective Policy;
//   - build classTable from Policy.Classes.Sizes;
//   - build classState values from the class table;
//   - build the shard selector from Policy.Shards.Selection;
//   - initialize close coordination;
//   - publish initial static class budgets;
//   - activate lifecycle.
//
// New does not start background controllers. Standalone Pool is a local
// data-plane owner. Controller lifecycle belongs to future PoolPartition or
// PoolGroup components.
func New(config PoolConfig) (*Pool, error) {
	normalized := config.Normalize()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}

	normalized.Policy = clonePoolPolicy(normalized.Policy)

	table := newClassTable(normalized.Policy.Classes.Sizes)
	classes := newPoolClassStates(table, normalized.Policy)

	selectors, err := newPoolShardSelectors(len(classes), normalized.Policy.Shards.Selection)
	if err != nil {
		return nil, err
	}

	pool := &Pool{
		name:               normalized.Name,
		config:             normalized,
		constructionPolicy: normalized.Policy,
		table:              table,
		classes:            classes,
		selectors:          selectors,
	}

	pool.closeCond = sync.NewCond(&pool.closeMu)
	pool.publishRuntimeSnapshot(newPoolRuntimeSnapshot(InitialGeneration, normalized.Policy))
	pool.publishInitialBudgets()
	pool.lifecycle.Activate()

	return pool, nil
}

// MustNew constructs a Pool and panics if construction fails.
//
// MustNew is intended for tests, examples, and static initialization paths where
// invalid configuration is a programmer error. User-facing construction should
// prefer New and handle the returned error.
func MustNew(config PoolConfig) *Pool {
	pool, err := New(config)
	if err != nil {
		panic(err)
	}

	return pool
}

// Name returns the diagnostic Pool name.
func (p *Pool) Name() string {
	p.mustBeInitialized()

	return p.name
}

// Config returns a copy of the normalized construction config used by this Pool.
//
// The returned config owns its Policy class-size slice. Mutating the returned
// value cannot affect the live Pool.
func (p *Pool) Config() PoolConfig {
	p.mustBeInitialized()

	config := p.config
	config.Policy = clonePoolPolicy(config.Policy)

	return config
}

// Policy returns a copy of the effective runtime Policy used by this Pool.
//
// The returned Policy owns its class-size slice. Mutating the returned value
// cannot affect the live Pool.
func (p *Pool) Policy() Policy {
	p.mustBeInitialized()

	return p.currentRuntimeSnapshot().clonePolicy()
}

// clonePoolPolicy returns a Policy copy that owns mutable slice fields.
//
// Policy is mostly value-shaped, but ClassPolicy.Sizes is a slice. Pool must not
// expose the slice used by construction-time classTable creation or by stored
// Pool config/policy snapshots.
func clonePoolPolicy(policy Policy) Policy {
	policy.Classes.Sizes = policy.Classes.SizesCopy()
	return policy
}
