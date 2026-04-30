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

// PoolGroupPressureSnapshot describes pressure interpreted from aggregate group
// usage and group-level pressure signals.
//
// The fields intentionally mirror partition pressure snapshots because the
// current group policy is expressed in aggregate owned bytes. The distinct type
// keeps group reports from exposing partition vocabulary as the public concept.
type PoolGroupPressureSnapshot PartitionPressureSnapshot

// PoolGroupPressurePublication reports one group pressure publication.
//
// AppliedPartitions and SkippedPartitions make propagation explicit. The group
// snapshot is published only when every child partition accepts the signal.
// When a later child rejects the signal, earlier applied children remain listed
// so the caller can see the partial publication instead of inferring it from a
// generic error.
type PoolGroupPressurePublication struct {
	// Generation identifies the fully applied group pressure publication. It is
	// NoGeneration when group runtime was not published.
	Generation Generation

	// AttemptGeneration identifies the child signal attempted for this call.
	AttemptGeneration Generation

	// Signal is the group-level pressure signal.
	Signal PressureSignal

	// GroupRuntimePublished reports whether the group runtime snapshot was
	// published after every child partition accepted the signal.
	GroupRuntimePublished bool

	// Partial reports whether at least one child accepted the signal while the
	// group runtime snapshot was not published.
	Partial bool

	// FailureReason is empty on full success and diagnostic on skip/failure.
	FailureReason string

	// AppliedPartitions contains partitions that accepted the signal.
	AppliedPartitions []PoolGroupPressurePublicationEntry

	// SkippedPartitions contains partitions that could not accept the signal.
	SkippedPartitions []PoolGroupPressurePublicationEntry
}

// FullyApplied reports whether every group-owned partition accepted the signal.
func (p PoolGroupPressurePublication) FullyApplied() bool {
	return p.GroupRuntimePublished && len(p.SkippedPartitions) == 0
}

// PoolGroupPressurePublicationEntry describes one partition in pressure
// publication.
type PoolGroupPressurePublicationEntry struct {
	// PartitionName is the group-local partition name.
	PartitionName string

	// Reason is empty for applied entries and diagnostic for skipped entries.
	Reason string
}

// SetPressure publishes a group pressure signal and propagates it to partitions.
//
// SetPressure is a foreground control-plane operation. It serializes with group
// hard Close, validates the level, publishes an immutable group pressure snapshot
// only after child partition prevalidation, and asks each partition to publish
// the signal to its owned Pools. It does not scan Pool shards and does not
// execute physical trim. Call PublishPressure when the caller needs the applied
// and skipped partition list.
func (g *PoolGroup) SetPressure(level PressureLevel) error {
	g.mustBeInitialized()
	if err := (PressureSignal{Level: level}).validate(); err != nil {
		return err
	}
	publication, err := g.PublishPressure(level)
	if err != nil {
		return err
	}
	if !publication.FullyApplied() {
		return newError(ErrClosed, errGroupClosed)
	}
	return nil
}

// PublishPressure publishes a group pressure signal and returns exact
// propagation diagnostics.
func (g *PoolGroup) PublishPressure(level PressureLevel) (PoolGroupPressurePublication, error) {
	g.mustBeInitialized()
	if err := (PressureSignal{Level: level}).validate(); err != nil {
		return PoolGroupPressurePublication{}, err
	}

	g.runtimeMu.Lock()
	defer g.runtimeMu.Unlock()

	if !g.lifecycle.AllowsWork() {
		return PoolGroupPressurePublication{}, newError(ErrClosed, errGroupClosed)
	}

	attemptGeneration := g.generation.Load().Next()
	signal := PressureSignal{Level: level, Source: PressureSourceGroup, Generation: attemptGeneration}
	publication := PoolGroupPressurePublication{AttemptGeneration: attemptGeneration, Signal: signal}
	for _, entry := range g.registry.entries {
		if entry.partition.IsClosed() {
			publication.SkippedPartitions = append(publication.SkippedPartitions, PoolGroupPressurePublicationEntry{
				PartitionName: entry.name,
				Reason:        errPartitionClosed,
			})
		}
	}
	if len(publication.SkippedPartitions) > 0 {
		publication.FailureReason = errPartitionClosed
		return publication, nil
	}

	for _, entry := range g.registry.entries {
		partitionPublication, err := entry.partition.publishPressureSignal(signal)
		if err != nil {
			publication.Partial = len(publication.AppliedPartitions) > 0
			publication.FailureReason = err.Error()
			publication.SkippedPartitions = append(publication.SkippedPartitions, PoolGroupPressurePublicationEntry{
				PartitionName: entry.name,
				Reason:        err.Error(),
			})
			return publication, err
		}
		if !partitionPublication.FullyApplied() {
			publication.Partial = len(publication.AppliedPartitions) > 0
			publication.FailureReason = errPartitionClosed
			publication.SkippedPartitions = append(publication.SkippedPartitions, PoolGroupPressurePublicationEntry{
				PartitionName: entry.name,
				Reason:        errPartitionClosed,
			})
			return publication, nil
		}
		publication.AppliedPartitions = append(publication.AppliedPartitions, PoolGroupPressurePublicationEntry{PartitionName: entry.name})
	}

	g.generation.Store(attemptGeneration)
	runtime := g.currentRuntimeSnapshot()
	g.publishRuntimeSnapshot(newGroupRuntimeSnapshotWithPressure(attemptGeneration, runtime.Policy, signal))
	publication.Generation = attemptGeneration
	publication.GroupRuntimePublished = true
	return publication, nil
}
