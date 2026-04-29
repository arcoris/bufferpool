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

import "sync/atomic"

const (
	// errNilPoolGroup reports nil or zero-value PoolGroup receivers.
	errNilPoolGroup = "bufferpool.PoolGroup: receiver must not be nil"

	// errGroupPartitionMissing reports lookup failure for a group-owned partition.
	errGroupPartitionMissing = "bufferpool.PoolGroup: partition not found"
)

// PoolGroup owns a deterministic set of PoolPartitions and provides an
// observational group-level control boundary.
//
// PoolGroup is the first owner above PoolPartition. It aggregates partition
// samples, bounded windows, rates, metrics, snapshots, and foreground
// coordinator reports. It deliberately does not apply partition policies,
// redistribute budgets, execute physical trim, or start background coordinator
// goroutines.
//
// Responsibility boundary:
//
//   - group.go owns the PoolGroup type, construction, metadata accessors, and
//     receiver validation;
//   - group_config.go owns construction config normalization and validation;
//   - group_policy.go owns observational group policy;
//   - group_lifecycle.go owns hard close behavior;
//   - group_registry.go owns the immutable partition registry;
//   - group_runtime.go owns group runtime policy snapshots;
//   - group_sample.go owns group sampling and aggregation;
//   - group_window.go owns bounded group windows;
//   - group_rate.go owns aggregate rate projection;
//   - group_score*.go owns score value projection;
//   - group_snapshot.go and group_metrics.go own diagnostics;
//   - group_coordinator.go and group_controller_report.go own foreground
//     observation reports.
//
// Copying:
//
// PoolGroup MUST NOT be copied after first use. It embeds atomic lifecycle and
// generation state and owns partition pointers through an immutable registry.
type PoolGroup struct {
	// lifecycle gates group-level foreground work and hard close.
	lifecycle AtomicLifecycle

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

	// scoreEvaluator owns prepared score adapters for group evaluation.
	scoreEvaluator PoolGroupScoreEvaluator
}

// NewPoolGroup constructs and activates an observational PoolGroup.
//
// NewPoolGroup constructs owned PoolPartitions from config. It does not start
// background work and does not publish any policy into partitions beyond their
// own construction configs.
func NewPoolGroup(config PoolGroupConfig) (*PoolGroup, error) {
	normalized := config.Normalize()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	registry, err := newGroupRegistry(normalized.Partitions)
	if err != nil {
		return nil, err
	}
	group := &PoolGroup{
		name:           normalized.Name,
		config:         cloneGroupConfig(normalized),
		registry:       registry,
		scoreEvaluator: NewPoolGroupScoreEvaluator(normalized.Policy.Score),
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

// PartitionSnapshot returns a diagnostic snapshot for one group-owned partition.
func (g *PoolGroup) PartitionSnapshot(name string) (PoolPartitionSnapshot, bool) {
	g.mustBeInitialized()
	partition, ok := g.registry.partition(name)
	if !ok {
		return PoolPartitionSnapshot{}, false
	}
	return partition.Snapshot(), true
}

// PartitionMetrics returns diagnostic metrics for one group-owned partition.
func (g *PoolGroup) PartitionMetrics(name string) (PoolPartitionMetrics, bool) {
	g.mustBeInitialized()
	partition, ok := g.registry.partition(name)
	if !ok {
		return PoolPartitionMetrics{}, false
	}
	return partition.Metrics(), true
}

// partition returns a raw group-owned partition for internal group code only.
func (g *PoolGroup) partition(name string) (*PoolPartition, bool) {
	return g.registry.partition(name)
}

// mustBeInitialized verifies that g was constructed by NewPoolGroup.
func (g *PoolGroup) mustBeInitialized() {
	if g == nil || g.runtimeSnapshot.Load() == nil || g.scoreEvaluator.isZero() {
		panic(errNilPoolGroup)
	}
}
