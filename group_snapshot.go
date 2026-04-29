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

// PoolGroupSnapshot is a diagnostic group snapshot.
type PoolGroupSnapshot struct {
	// Name is diagnostic group metadata.
	Name string

	// Lifecycle is the observed group lifecycle state.
	Lifecycle LifecycleState

	// Generation is the observed group state/event generation.
	Generation Generation

	// PolicyGeneration is the observed group runtime-policy generation.
	PolicyGeneration Generation

	// Config is a defensive copy of normalized construction config.
	Config PoolGroupConfig

	// Policy is the observed group runtime policy.
	Policy PoolGroupPolicy

	// Partitions contains diagnostic snapshots for group-owned partitions.
	Partitions []PoolGroupPartitionSnapshot

	// Metrics is an aggregate projection derived from one group sample.
	Metrics PoolGroupMetrics
}

// PoolGroupPartitionSnapshot is one named partition diagnostic snapshot.
type PoolGroupPartitionSnapshot struct {
	// Name is the group-local partition name.
	Name string

	// Partition is the partition diagnostic snapshot.
	Partition PoolPartitionSnapshot
}

// Snapshot returns a diagnostic group snapshot.
//
// The snapshot is observational across partitions, not a global transaction.
// Metrics are derived from one group sample, while partition snapshots may
// reflect nearby different instants under concurrent activity. Snapshot is not
// controller input; coordinator code should use Sample or SampleInto.
func (g *PoolGroup) Snapshot() PoolGroupSnapshot {
	g.mustBeInitialized()
	runtime := g.currentRuntimeSnapshot()
	var sample PoolGroupSample
	g.sampleWithRuntimeAndGeneration(&sample, runtime, g.generation.Load())
	partitions := make([]PoolGroupPartitionSnapshot, 0, g.registry.len())
	for _, entry := range g.registry.entries {
		partitions = append(partitions, PoolGroupPartitionSnapshot{Name: entry.name, Partition: entry.partition.Snapshot()})
	}
	return PoolGroupSnapshot{
		Name:             g.name,
		Lifecycle:        g.lifecycle.Load(),
		Generation:       sample.Generation,
		PolicyGeneration: runtime.Generation,
		Config:           g.Config(),
		Policy:           runtime.Policy,
		Partitions:       partitions,
		Metrics:          newPoolGroupMetrics(g.name, sample),
	}
}

// PartitionCount returns the number of partition snapshots.
func (s PoolGroupSnapshot) PartitionCount() int { return len(s.Partitions) }

// IsEmpty reports whether snapshot contains no owned memory and no observed activity.
func (s PoolGroupSnapshot) IsEmpty() bool { return s.Metrics.IsZero() }
