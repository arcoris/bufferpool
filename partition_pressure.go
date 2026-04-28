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
type PartitionPressurePolicy struct {
	// Enabled controls whether partition pressure is interpreted.
	Enabled bool

	// MediumOwnedBytes is the owned-memory threshold for medium pressure.
	MediumOwnedBytes Size

	// HighOwnedBytes is the owned-memory threshold for high pressure.
	HighOwnedBytes Size

	// CriticalOwnedBytes is the owned-memory threshold for critical pressure.
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

// Validate validates pressure thresholds.
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
func (p *PoolPartition) Pressure() PartitionPressureSnapshot {
	p.mustBeInitialized()
	runtime := p.currentRuntimeSnapshot()
	var sample PoolPartitionSample
	p.sampleWithRuntimeAndGeneration(&sample, runtime, p.generation.Load(), false)
	return newPartitionPressureSnapshot(runtime.Policy.Pressure, sample)
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
