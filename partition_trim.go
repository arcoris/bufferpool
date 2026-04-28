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

const (
	// defaultPartitionTrimMaxPoolsPerCycle bounds planning when trim is enabled.
	defaultPartitionTrimMaxPoolsPerCycle = 1
)

// PartitionTrimPolicy defines bounded trim planning defaults.
type PartitionTrimPolicy struct {
	// Enabled controls whether partition trim planning can produce work.
	Enabled bool

	// MaxPoolsPerCycle bounds the number of candidate Pools in one plan.
	MaxPoolsPerCycle int

	// MaxBytesPerCycle bounds retained bytes that future physical trim may remove.
	MaxBytesPerCycle Size

	// TrimOnPressure means trim planning is enabled only while pressure is above
	// normal. Normal pressure produces a disabled "no_pressure" plan.
	TrimOnPressure bool
}

// PartitionTrimPlan is a non-mutating trim plan derived from sample and policy.
type PartitionTrimPlan struct {
	// Enabled reports whether the plan would allow trim work.
	Enabled bool

	// Reason is a stable diagnostic reason for the plan decision.
	Reason string

	// PressureLevel is the pressure level used to derive the plan.
	PressureLevel PressureLevel

	// CandidatePools is the number of partition-owned Pools eligible for planning.
	CandidatePools int

	// MaxPoolsPerCycle is the effective pool-visit bound for the plan.
	MaxPoolsPerCycle int

	// MaxBytesPerCycle is the effective byte bound for the plan.
	MaxBytesPerCycle uint64
}

// PartitionTrimResult reports a trim attempt.
//
// Current PoolPartition does not execute physical trim. Result fields are kept
// explicit so future Pool trim execution can add physical effects without
// changing the diagnostic shape.
type PartitionTrimResult struct {
	// Attempted reports whether execution was requested while trim was enabled.
	Attempted bool

	// Executed reports whether retained buffers were physically removed.
	Executed bool

	// Reason is a stable diagnostic result reason.
	Reason string

	// VisitedPools is the number of Pools visited by physical execution.
	VisitedPools uint64

	// TrimmedBuffers is the number of retained buffers physically removed.
	TrimmedBuffers uint64

	// TrimmedBytes is the retained backing capacity physically removed.
	TrimmedBytes uint64
}

// Normalize returns p with enabled trim defaults completed.
func (p PartitionTrimPolicy) Normalize() PartitionTrimPolicy {
	if p.Enabled && p.MaxPoolsPerCycle == 0 {
		p.MaxPoolsPerCycle = defaultPartitionTrimMaxPoolsPerCycle
	}
	return p
}

// IsZero reports whether p contains no trim settings.
func (p PartitionTrimPolicy) IsZero() bool {
	return !p.Enabled && p.MaxPoolsPerCycle == 0 && p.MaxBytesPerCycle.IsZero() && !p.TrimOnPressure
}

// Validate validates trim planning settings.
func (p PartitionTrimPolicy) Validate() error {
	p = p.Normalize()
	if !p.Enabled {
		return nil
	}
	if p.MaxPoolsPerCycle <= 0 {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionTrimPolicy: max pools per cycle must be positive")
	}
	if p.MaxBytesPerCycle.IsZero() {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionTrimPolicy: max bytes per cycle must be positive")
	}
	return nil
}

// PlanTrim returns a non-mutating trim plan.
func (p *PoolPartition) PlanTrim() PartitionTrimPlan {
	p.mustBeInitialized()
	runtime := p.currentRuntimeSnapshot()
	var sample PoolPartitionSample
	p.sampleWithRuntimeAndGeneration(&sample, runtime, p.generation.Load(), false)
	pressure := newPartitionPressureSnapshot(runtime.Policy.Pressure, sample)
	return newPartitionTrimPlan(runtime.Policy.Trim, pressure, sample)
}

// ExecuteTrim currently returns a planning-only result.
//
// PoolPartition does not yet have a stable physical Pool trim API to invoke.
// The method is retained for controller wiring tests, but it does not visit
// Pools or remove retained buffers.
func (p *PoolPartition) ExecuteTrim() PartitionTrimResult {
	p.mustBeInitialized()
	if !p.Policy().Trim.Enabled {
		return PartitionTrimResult{Reason: "trim_disabled"}
	}

	return PartitionTrimResult{Attempted: true, Reason: "planning_only"}
}

// newPartitionTrimPlan derives a planning-only trim decision.
func newPartitionTrimPlan(policy PartitionTrimPolicy, pressure PartitionPressureSnapshot, sample PoolPartitionSample) PartitionTrimPlan {
	policy = policy.Normalize()
	plan := PartitionTrimPlan{Enabled: policy.Enabled, PressureLevel: pressure.Level, CandidatePools: sample.PoolCount, MaxPoolsPerCycle: policy.MaxPoolsPerCycle, MaxBytesPerCycle: policy.MaxBytesPerCycle.Bytes()}
	if !policy.Enabled {
		plan.Reason = "trim_disabled"
		return plan
	}
	if policy.TrimOnPressure {
		if pressure.Level == PressureLevelNormal {
			plan.Enabled = false
			plan.Reason = "no_pressure"
			return plan
		}
		plan.Reason = "pressure"
		return plan
	}
	plan.Reason = "policy"
	return plan
}
