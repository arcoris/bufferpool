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

// PartitionPressurePolicy defines partition-local owned-memory pressure
// thresholds.
//
// A zero threshold is disabled for that level. Configured thresholds are
// evaluated critical, then high, then medium; omitted levels simply do not
// participate. For example, omitting MediumOwnedBytes lets pressure jump from
// normal to high when HighOwnedBytes is configured.
type PartitionPressurePolicy struct {
	// Enabled controls whether partition pressure is interpreted.
	Enabled bool

	// MediumOwnedBytes is the optional owned-memory threshold for medium pressure.
	MediumOwnedBytes Size

	// HighOwnedBytes is the optional owned-memory threshold for high pressure.
	HighOwnedBytes Size

	// CriticalOwnedBytes is the optional owned-memory threshold for critical pressure.
	CriticalOwnedBytes Size
}

// PartitionPressureSnapshot describes pressure derived from a sample.
type PartitionPressureSnapshot struct {
	// Enabled reports whether pressure interpretation was enabled.
	Enabled bool

	// Level is the interpreted pressure level for CurrentOwnedBytes.
	Level PressureLevel

	// CurrentOwnedBytes is retained plus active bytes from the sample.
	CurrentOwnedBytes uint64

	// MediumOwnedBytes is the configured medium threshold.
	MediumOwnedBytes uint64

	// HighOwnedBytes is the configured high threshold.
	HighOwnedBytes uint64

	// CriticalOwnedBytes is the configured critical threshold.
	CriticalOwnedBytes uint64
}

// PoolPartitionPressurePublication reports one partition pressure publication.
//
// AppliedPools and SkippedPools make partial propagation explicit. A closed Pool
// is skipped instead of being silently ignored or mutated after close.
type PoolPartitionPressurePublication struct {
	// Generation identifies the fully applied partition pressure publication. It
	// is NoGeneration when partition runtime was not published.
	Generation Generation

	// AttemptGeneration identifies the signal attempted for owned Pools.
	AttemptGeneration Generation

	// Signal is the partition-level pressure signal.
	Signal PressureSignal

	// PartitionRuntimePublished reports whether the partition runtime snapshot
	// was published after every owned Pool accepted the signal.
	PartitionRuntimePublished bool

	// Partial reports whether at least one Pool accepted the signal while the
	// partition runtime snapshot was not published.
	Partial bool

	// FailureReason is empty on full success and diagnostic on skip/failure.
	FailureReason string

	// AppliedPools contains partition-local Pool names that accepted the signal.
	AppliedPools []PoolPressurePublicationEntry

	// SkippedPools contains partition-local Pool names that did not accept the
	// signal.
	SkippedPools []PoolPressurePublicationEntry
}

// FullyApplied reports whether every owned Pool accepted the pressure signal.
func (p PoolPartitionPressurePublication) FullyApplied() bool {
	return p.PartitionRuntimePublished && len(p.SkippedPools) == 0
}

// PoolPressurePublicationEntry describes one Pool in a partition pressure
// publication.
type PoolPressurePublicationEntry struct {
	// PoolName is the partition-local Pool name.
	PoolName string

	// Reason is empty for applied entries and diagnostic for skipped entries.
	Reason string
}

// IsZero reports whether p contains no pressure settings.
func (p PartitionPressurePolicy) IsZero() bool {
	return !p.Enabled && p.MediumOwnedBytes.IsZero() && p.HighOwnedBytes.IsZero() && p.CriticalOwnedBytes.IsZero()
}

// Validate validates configured pressure thresholds without requiring a full
// medium/high/critical chain.
func (p PartitionPressurePolicy) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.MediumOwnedBytes.IsZero() && p.HighOwnedBytes.IsZero() && p.CriticalOwnedBytes.IsZero() {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionPressurePolicy: enabled pressure requires a threshold")
	}
	if !p.MediumOwnedBytes.IsZero() && !p.HighOwnedBytes.IsZero() && p.MediumOwnedBytes > p.HighOwnedBytes {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionPressurePolicy: medium threshold must not exceed high threshold")
	}
	if !p.MediumOwnedBytes.IsZero() && !p.CriticalOwnedBytes.IsZero() && p.MediumOwnedBytes > p.CriticalOwnedBytes {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionPressurePolicy: medium threshold must not exceed critical threshold")
	}
	if !p.HighOwnedBytes.IsZero() && !p.CriticalOwnedBytes.IsZero() && p.HighOwnedBytes > p.CriticalOwnedBytes {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionPressurePolicy: high threshold must not exceed critical threshold")
	}
	return nil
}

// Pressure returns the current partition pressure interpretation.
//
// The returned snapshot combines threshold-derived pressure from the current
// sample with the foreground pressure signal most recently published into the
// partition runtime snapshot.
func (p *PoolPartition) Pressure() PartitionPressureSnapshot {
	p.mustBeInitialized()
	runtime := p.currentRuntimeSnapshot()
	var sample PoolPartitionSample
	p.sampleWithRuntimeAndGeneration(&sample, runtime, p.generation.Load(), false)
	return newEffectivePartitionPressureSnapshot(runtime.Policy.Pressure, runtime.Pressure, sample)
}

// SetPressure publishes a manual pressure signal to this partition and its
// owned Pools.
//
// The compatibility API returns ErrClosed when publication is only partially
// applied. Call PublishPressure when the caller needs the structured applied and
// skipped Pool list.
func (p *PoolPartition) SetPressure(level PressureLevel) error {
	publication, err := p.PublishPressure(level)
	if err != nil {
		return err
	}
	if !publication.FullyApplied() {
		return newError(ErrClosed, errPartitionClosed)
	}
	return nil
}

// PublishPressure publishes a manual pressure signal and returns exact
// propagation diagnostics.
func (p *PoolPartition) PublishPressure(level PressureLevel) (PoolPartitionPressurePublication, error) {
	p.mustBeInitialized()
	if err := (PressureSignal{Level: level}).validate(); err != nil {
		return PoolPartitionPressurePublication{}, err
	}

	return p.publishPressureSignal(PressureSignal{Level: level, Source: PressureSourceManual})
}

// applyPressure publishes pressure to partition runtime state and owned Pools.
//
// The partition does not scan shards here. Pool return admission reads the
// propagated immutable pressure signal, while physical correction remains a
// bounded ExecuteTrim call.
func (p *PoolPartition) applyPressure(signal PressureSignal) error {
	p.mustBeInitialized()
	if err := signal.validate(); err != nil {
		return err
	}
	publication, err := p.publishPressureSignal(signal)
	if err != nil {
		return err
	}
	if !publication.FullyApplied() {
		return newError(ErrClosed, errPartitionClosed)
	}
	return nil
}

// publishPressureSignal publishes an already validated source signal through the
// partition foreground gate.
func (p *PoolPartition) publishPressureSignal(signal PressureSignal) (PoolPartitionPressurePublication, error) {
	if err := p.beginForegroundOperation(); err != nil {
		return PoolPartitionPressurePublication{}, err
	}
	defer p.endForegroundOperation()

	return p.applyPressureLocked(signal)
}

// applyPressureLocked publishes pressure while the partition foreground gate is
// already held.
func (p *PoolPartition) applyPressureLocked(signal PressureSignal) (PoolPartitionPressurePublication, error) {
	if err := signal.validate(); err != nil {
		return PoolPartitionPressurePublication{}, err
	}
	if !p.lifecycle.AllowsWork() {
		return PoolPartitionPressurePublication{}, newError(ErrClosed, errPartitionClosed)
	}

	generation := budgetPublicationGeneration(p.generation.Load(), signal.Generation)
	signal.Generation = generation
	publication := PoolPartitionPressurePublication{
		AttemptGeneration: generation,
		Signal:            signal,
	}

	// Closed Pools are reported before any owned Pool receives the signal. That
	// keeps skipped-before-apply publication distinct from partial publication.
	for _, entry := range p.registry.entries {
		if entry.pool.IsClosed() {
			publication.SkippedPools = append(publication.SkippedPools, PoolPressurePublicationEntry{
				PoolName: entry.name,
				Reason:   policyUpdateFailureClosed,
			})
			continue
		}
	}
	if len(publication.SkippedPools) > 0 {
		publication.FailureReason = policyUpdateFailureClosed
		return publication, nil
	}

	// Propagate the signal to owned Pools through their local control gates. A
	// later failure is reported as partial and the partition runtime snapshot is
	// left unpublished.
	for _, entry := range p.registry.entries {
		partitionSignal := signal
		partitionSignal.Source = PressureSourcePartition
		if err := entry.pool.applyPressure(partitionSignal); err != nil {
			reason := policyUpdateFailureReasonForError(err, policyUpdateFailureInvalid)
			publication.Partial = len(publication.AppliedPools) > 0
			publication.FailureReason = reason
			publication.SkippedPools = append(publication.SkippedPools, PoolPressurePublicationEntry{
				PoolName: entry.name,
				Reason:   reason,
			})
			return publication, err
		}
		publication.AppliedPools = append(publication.AppliedPools, PoolPressurePublicationEntry{PoolName: entry.name})
	}

	// Partition pressure becomes visible only after every Pool accepted the same
	// attempt generation.
	runtime := p.currentRuntimeSnapshot()
	p.generation.Store(generation)
	p.publishRuntimeSnapshot(newPartitionRuntimeSnapshotWithPressure(generation, runtime.Policy, signal))
	publication.Generation = generation
	publication.Signal.Generation = generation
	publication.PartitionRuntimePublished = true
	return publication, nil
}

// newPartitionPressureSnapshot maps sampled owned bytes to a pressure level.
func newPartitionPressureSnapshot(policy PartitionPressurePolicy, sample PoolPartitionSample) PartitionPressureSnapshot {
	s := PartitionPressureSnapshot{
		Enabled:            policy.Enabled,
		Level:              PressureLevelNormal,
		CurrentOwnedBytes:  partitionOwnedBytesFromSample(sample),
		MediumOwnedBytes:   policy.MediumOwnedBytes.Bytes(),
		HighOwnedBytes:     policy.HighOwnedBytes.Bytes(),
		CriticalOwnedBytes: policy.CriticalOwnedBytes.Bytes(),
	}
	if !policy.Enabled {
		return s
	}
	if s.CriticalOwnedBytes > 0 && s.CurrentOwnedBytes >= s.CriticalOwnedBytes {
		s.Level = PressureLevelCritical
		return s
	}
	if s.HighOwnedBytes > 0 && s.CurrentOwnedBytes >= s.HighOwnedBytes {
		s.Level = PressureLevelHigh
		return s
	}
	if s.MediumOwnedBytes > 0 && s.CurrentOwnedBytes >= s.MediumOwnedBytes {
		s.Level = PressureLevelMedium
	}
	return s
}

// newEffectivePartitionPressureSnapshot combines sampled pressure thresholds
// with an explicitly published runtime pressure signal.
//
// Threshold policy remains useful for autonomous partition-local detection.
// Runtime signals are owner-published state from SetPressure or PoolGroup and
// may request stronger contraction before sampled owned bytes cross a static
// threshold. Normal or unknown signals do not raise the sampled level.
func newEffectivePartitionPressureSnapshot(policy PartitionPressurePolicy, signal PressureSignal, sample PoolPartitionSample) PartitionPressureSnapshot {
	snapshot := newPartitionPressureSnapshot(policy, sample)
	if !isKnownPressureLevel(signal.Level) || signal.Level <= snapshot.Level {
		return snapshot
	}
	snapshot.Enabled = true
	snapshot.Level = signal.Level
	return snapshot
}
