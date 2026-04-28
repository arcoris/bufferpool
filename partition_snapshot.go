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

// PoolPartitionSnapshot is a diagnostic partition snapshot.
type PoolPartitionSnapshot struct {
	// Name is diagnostic partition metadata.
	Name string

	// Lifecycle is the observed partition lifecycle state.
	Lifecycle LifecycleState

	// Generation is the observed partition state/event generation.
	Generation Generation

	// PolicyGeneration is the observed partition runtime-policy generation.
	PolicyGeneration Generation

	// Config is a defensive copy of normalized construction config.
	Config PoolPartitionConfig

	// Policy is the observed partition runtime policy.
	Policy PartitionPolicy

	// Pools contains diagnostic snapshots for partition-owned Pools.
	Pools []PoolPartitionPoolSnapshot

	// Leases is the diagnostic snapshot of the partition LeaseRegistry.
	Leases LeaseRegistrySnapshot

	// Metrics is an aggregate projection derived from one partition sample.
	Metrics PoolPartitionMetrics
}

// PoolPartitionPoolSnapshot is one named Pool diagnostic snapshot.
type PoolPartitionPoolSnapshot struct {
	// Name is the partition-local Pool name.
	Name string

	// Pool is the Pool diagnostic snapshot.
	Pool PoolSnapshot
}

// Snapshot returns a diagnostic partition snapshot.
//
// The snapshot is observational across Pools and the LeaseRegistry. It is meant
// for diagnostics, not controller ticks. Controller-facing code should use
// Sample, which avoids copying active lease snapshots.
func (p *PoolPartition) Snapshot() PoolPartitionSnapshot {
	p.mustBeInitialized()
	runtime := p.currentRuntimeSnapshot()
	var sample PoolPartitionSample
	p.sampleWithRuntimeAndGeneration(&sample, runtime, p.generation.Load(), false)
	pools := make([]PoolPartitionPoolSnapshot, 0, p.registry.len())
	for _, entry := range p.registry.entries {
		pools = append(pools, PoolPartitionPoolSnapshot{Name: entry.name, Pool: entry.pool.Snapshot()})
	}
	leases := p.leases.Snapshot()
	return PoolPartitionSnapshot{
		Name:             p.name,
		Lifecycle:        p.lifecycle.Load(),
		Generation:       sample.Generation,
		PolicyGeneration: runtime.Generation,
		Config:           p.Config(),
		Policy:           runtime.Policy,
		Pools:            pools,
		Leases:           leases,
		Metrics:          newPoolPartitionMetrics(p.name, sample),
	}
}

// PoolCount returns the number of Pool snapshots.
func (s PoolPartitionSnapshot) PoolCount() int { return len(s.Pools) }

// IsEmpty reports whether snapshot contains no retained storage, no active leases, and no observed activity.
func (s PoolPartitionSnapshot) IsEmpty() bool { return s.Metrics.IsZero() && s.Leases.IsEmpty() }
