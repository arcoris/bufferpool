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
func (p *PoolPartition) SetPressure(level PressureLevel) error {
	p.mustBeInitialized()
	if err := (PressureSignal{Level: level}).validate(); err != nil {
		return err
	}

	generation := p.generation.Advance()
	return p.applyPressure(PressureSignal{Level: level, Source: PressureSourceManual, Generation: generation})
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
	if !p.lifecycle.AllowsWork() {
		return newError(ErrClosed, errPartitionClosed)
	}

	runtime := p.currentRuntimeSnapshot()
	generation := budgetPublicationGeneration(runtime.Generation, signal.Generation)
	p.publishRuntimeSnapshot(newPartitionRuntimeSnapshotWithPressure(generation, runtime.Policy, signal))

	for _, entry := range p.registry.entries {
		partitionSignal := signal
		partitionSignal.Source = PressureSourcePartition
		if err := entry.pool.applyPressure(partitionSignal); err != nil {
			return err
		}
	}
	return nil
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
