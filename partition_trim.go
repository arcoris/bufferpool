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

import "sort"

const (
	// defaultPartitionTrimMaxPoolsPerCycle bounds planning when trim is enabled.
	defaultPartitionTrimMaxPoolsPerCycle = 1

	// defaultPartitionTrimMaxBuffersPerCycle bounds physical buffer removals in
	// one partition trim cycle.
	defaultPartitionTrimMaxBuffersPerCycle uint64 = 1024

	// defaultPartitionTrimMaxClassesPerPoolPerCycle bounds class fan-out for one
	// Pool during a partition trim cycle.
	defaultPartitionTrimMaxClassesPerPoolPerCycle = 64

	// defaultPartitionTrimMaxShardsPerClassPerCycle bounds shard fan-out for one
	// class during a partition trim cycle.
	defaultPartitionTrimMaxShardsPerClassPerCycle = 32
)

// PartitionTrimPolicy defines bounded partition trim defaults.
type PartitionTrimPolicy struct {
	// Enabled controls whether partition trim can produce physical work.
	Enabled bool

	// MaxPoolsPerCycle bounds the number of candidate Pools in one plan.
	MaxPoolsPerCycle int

	// MaxBuffersPerCycle bounds the number of retained buffers removed by one
	// physical trim cycle.
	MaxBuffersPerCycle uint64

	// MaxBytesPerCycle bounds retained bytes that one physical trim may remove.
	MaxBytesPerCycle Size

	// MaxClassesPerPoolPerCycle bounds how many classes may be visited in each
	// Pool.
	MaxClassesPerPoolPerCycle int

	// MaxShardsPerClassPerCycle bounds how many shards may be visited for each
	// class.
	MaxShardsPerClassPerCycle int

	// TrimOnPressure means trim planning is enabled only while pressure is above
	// normal. Normal pressure produces a disabled "no_pressure" plan.
	TrimOnPressure bool

	// TrimOnPolicyShrink enables bounded retained cleanup after a live
	// partition policy publication contracts retained targets.
	//
	// This flag is intentionally separate from TrimOnPressure: policy shrink is
	// an explicit foreground control operation, not a pressure observation. It
	// never reclaims active leases and it uses the same per-cycle pool, buffer,
	// byte, class, and shard bounds as ordinary partition trim.
	TrimOnPolicyShrink bool
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

	// MaxBuffersPerCycle is the effective retained-buffer removal bound.
	MaxBuffersPerCycle uint64

	// MaxBytesPerCycle is the effective byte bound for the plan.
	MaxBytesPerCycle uint64

	// MaxClassesPerPoolPerCycle is the effective class-visit bound per Pool.
	MaxClassesPerPoolPerCycle int

	// MaxShardsPerClassPerCycle is the effective shard-visit bound per class.
	MaxShardsPerClassPerCycle int
}

// PartitionTrimResult reports a bounded physical trim attempt.
type PartitionTrimResult struct {
	// Attempted reports whether execution was requested while trim was enabled.
	Attempted bool

	// Executed reports whether retained buffers were physically removed.
	Executed bool

	// Reason is a stable diagnostic result reason.
	Reason string

	// VisitedPools is the number of Pools visited by physical execution.
	VisitedPools uint64

	// VisitedClasses is the number of Pool classes visited by physical execution.
	VisitedClasses uint64

	// VisitedShards is the number of Pool shards visited by physical execution.
	VisitedShards uint64

	// TrimmedBuffers is the number of retained buffers physically removed.
	TrimmedBuffers uint64

	// TrimmedBytes is the retained backing capacity physically removed.
	TrimmedBytes uint64

	// CandidatePools records the target-aware Pool order considered by execution.
	CandidatePools []PartitionTrimCandidate
}

// PartitionTrimCandidate describes one Pool selected for scored trim order.
type PartitionTrimCandidate struct {
	// PoolName is the partition-local Pool name.
	PoolName string

	// Score is the normalized candidate-level victim score.
	Score TrimVictimScore

	// OverTargetBytes is retained bytes above observed class budgets.
	OverTargetBytes uint64

	// RetainedBytes is the Pool current retained byte gauge.
	RetainedBytes uint64

	// CapacityWasteBytes is retained capacity above nominal class capacity.
	CapacityWasteBytes uint64

	// Coldness is the coldness component used for Score.
	Coldness float64

	// PressureSeverity is the pressure component used for Score.
	PressureSeverity float64
}

// Normalize returns p with enabled trim defaults completed.
func (p PartitionTrimPolicy) Normalize() PartitionTrimPolicy {
	if p.Enabled && p.MaxPoolsPerCycle == 0 {
		p.MaxPoolsPerCycle = defaultPartitionTrimMaxPoolsPerCycle
	}
	if p.Enabled && p.MaxBuffersPerCycle == 0 {
		p.MaxBuffersPerCycle = defaultPartitionTrimMaxBuffersPerCycle
	}
	if p.Enabled && p.MaxClassesPerPoolPerCycle == 0 {
		p.MaxClassesPerPoolPerCycle = defaultPartitionTrimMaxClassesPerPoolPerCycle
	}
	if p.Enabled && p.MaxShardsPerClassPerCycle == 0 {
		p.MaxShardsPerClassPerCycle = defaultPartitionTrimMaxShardsPerClassPerCycle
	}
	return p
}

// IsZero reports whether p contains no trim settings.
func (p PartitionTrimPolicy) IsZero() bool {
	return !p.Enabled &&
		p.MaxPoolsPerCycle == 0 &&
		p.MaxBuffersPerCycle == 0 &&
		p.MaxBytesPerCycle.IsZero() &&
		p.MaxClassesPerPoolPerCycle == 0 &&
		p.MaxShardsPerClassPerCycle == 0 &&
		!p.TrimOnPressure &&
		!p.TrimOnPolicyShrink
}

// Validate validates partition trim settings.
func (p PartitionTrimPolicy) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.MaxPoolsPerCycle <= 0 {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionTrimPolicy: max pools per cycle must be positive")
	}
	if p.MaxBuffersPerCycle == 0 {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionTrimPolicy: max buffers per cycle must be positive")
	}
	if p.MaxBytesPerCycle.IsZero() {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionTrimPolicy: max bytes per cycle must be positive")
	}
	if p.MaxClassesPerPoolPerCycle <= 0 {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionTrimPolicy: max classes per pool per cycle must be positive")
	}
	if p.MaxShardsPerClassPerCycle <= 0 {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionTrimPolicy: max shards per class per cycle must be positive")
	}
	return nil
}

// PlanTrim returns a non-mutating trim plan.
func (p *PoolPartition) PlanTrim() PartitionTrimPlan {
	p.mustBeInitialized()
	runtime := p.currentRuntimeSnapshot()
	var sample PoolPartitionSample
	p.sampleWithRuntimeAndGeneration(&sample, runtime, p.generation.Load(), false)
	pressure := newEffectivePartitionPressureSnapshot(runtime.Policy.Pressure, runtime.Pressure, sample)
	return newPartitionTrimPlan(runtime.Policy.Trim, pressure, sample)
}

// ExecuteTrim performs one bounded physical retained-storage trim cycle.
//
// ExecuteTrim removes retained buffers only. It does not force active leases,
// does not call Pool.Get or Pool.Put, and does not scan outside partition-owned
// Pools. The cycle follows PlanTrim limits and visits Pools in deterministic
// target-aware order: over-target and retained-heavy Pools first, then
// activity-adjusted coldness and capacity waste when that context is available,
// with stable names and registry indexes as tie-breakers.
func (p *PoolPartition) ExecuteTrim() PartitionTrimResult {
	p.mustBeInitialized()
	if err := p.beginForegroundOperation(); err != nil {
		return PartitionTrimResult{Reason: errPoolTrimClosed}
	}
	defer p.endForegroundOperation()

	plan := p.PlanTrim()
	return p.executeTrimPlan(plan)
}

// newPartitionTrimPlan derives a non-mutating trim execution plan.
func newPartitionTrimPlan(policy PartitionTrimPolicy, pressure PartitionPressureSnapshot, sample PoolPartitionSample) PartitionTrimPlan {
	policy = policy.Normalize()
	plan := PartitionTrimPlan{
		Enabled:                   policy.Enabled,
		PressureLevel:             pressure.Level,
		CandidatePools:            sample.PoolCount,
		MaxPoolsPerCycle:          policy.MaxPoolsPerCycle,
		MaxBuffersPerCycle:        policy.MaxBuffersPerCycle,
		MaxBytesPerCycle:          policy.MaxBytesPerCycle.Bytes(),
		MaxClassesPerPoolPerCycle: policy.MaxClassesPerPoolPerCycle,
		MaxShardsPerClassPerCycle: policy.MaxShardsPerClassPerCycle,
	}
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

// newPartitionPolicyShrinkTrimPlan derives a bounded trim plan for live policy
// contraction.
//
// Policy-shrink trim is foreground and explicit. It uses configured trim
// bounds, does not require pressure to be above normal, and only removes
// retained Pool storage through Pool.Trim. Active leases remain owned by
// LeaseRegistry until callers release them normally.
func newPartitionPolicyShrinkTrimPlan(policy PartitionTrimPolicy, pressureLevel PressureLevel, candidatePools int) PartitionTrimPlan {
	policy = policy.Normalize()
	plan := PartitionTrimPlan{
		Enabled:                   policy.Enabled && policy.TrimOnPolicyShrink,
		Reason:                    "policy_shrink",
		PressureLevel:             pressureLevel,
		CandidatePools:            candidatePools,
		MaxPoolsPerCycle:          policy.MaxPoolsPerCycle,
		MaxBuffersPerCycle:        policy.MaxBuffersPerCycle,
		MaxBytesPerCycle:          policy.MaxBytesPerCycle.Bytes(),
		MaxClassesPerPoolPerCycle: policy.MaxClassesPerPoolPerCycle,
		MaxShardsPerClassPerCycle: policy.MaxShardsPerClassPerCycle,
	}
	if !policy.Enabled {
		plan.Enabled = false
		plan.Reason = "trim_disabled"
		return plan
	}
	if !policy.TrimOnPolicyShrink {
		plan.Enabled = false
		plan.Reason = "policy_shrink_trim_disabled"
		return plan
	}
	return plan
}

func (p *PoolPartition) executeTrimPlan(plan PartitionTrimPlan) PartitionTrimResult {
	return p.executeTrimPlanWithScoring(plan, partitionTrimScoringContext{})
}

func (p *PoolPartition) executeTrimPlanWithScoring(plan PartitionTrimPlan, scoring partitionTrimScoringContext) PartitionTrimResult {
	if !plan.Enabled {
		return PartitionTrimResult{Reason: plan.Reason}
	}

	// Candidate scoring only chooses which Pools to visit first. The execution
	// loop below still enforces every pool, buffer, byte, class, and shard bound
	// from the plan.
	result := PartitionTrimResult{Attempted: true, Reason: plan.Reason}
	remainingBytes := plan.MaxBytesPerCycle
	remainingBuffers := plan.MaxBuffersPerCycle

	candidates := p.partitionTrimCandidates(plan.PressureLevel, scoring)
	result.CandidatePools = partitionTrimCandidateReports(candidates)

	for _, candidate := range candidates {
		if plan.MaxPoolsPerCycle > 0 && int(result.VisitedPools) >= plan.MaxPoolsPerCycle {
			break
		}
		if remainingBytes == 0 || remainingBuffers == 0 {
			break
		}

		result.VisitedPools++
		poolResult := candidate.pool.Trim(PoolTrimPlan{
			MaxBuffers:        partitionTrimUint64ToInt(remainingBuffers),
			MaxBytes:          SizeFromBytes(remainingBytes),
			MaxClasses:        plan.MaxClassesPerPoolPerCycle,
			MaxShardsPerClass: plan.MaxShardsPerClassPerCycle,
		})
		result.VisitedClasses += poolResult.VisitedClasses
		result.VisitedShards += poolResult.VisitedShards
		result.TrimmedBuffers += poolResult.TrimmedBuffers
		result.TrimmedBytes += poolResult.TrimmedBytes
		if poolResult.Executed {
			result.Executed = true
		}
		if poolResult.TrimmedBytes >= remainingBytes {
			remainingBytes = 0
			break
		}
		remainingBytes -= poolResult.TrimmedBytes
		if poolResult.TrimmedBuffers >= remainingBuffers {
			remainingBuffers = 0
			break
		}
		remainingBuffers -= poolResult.TrimmedBuffers
	}

	if result.Executed {
		p.activeRegistry.markAllDirty()
	}
	return result
}

type partitionTrimScoringContext struct {
	activityByPool map[string]float64
}

func newPartitionTrimScoringContext(window PoolPartitionWindow) partitionTrimScoringContext {
	if len(window.Pools) == 0 {
		return partitionTrimScoringContext{}
	}
	activityByPool := make(map[string]float64, len(window.Pools))
	for _, pool := range window.Pools {
		activityByPool[pool.Name] = partitionTrimPoolWindowActivity(pool)
	}
	return partitionTrimScoringContext{activityByPool: activityByPool}
}

func (c partitionTrimScoringContext) activity(poolName string) (float64, bool) {
	if len(c.activityByPool) == 0 {
		return 0, false
	}
	activity, ok := c.activityByPool[poolName]
	return activity, ok
}

func partitionTrimPoolWindowActivity(window PoolPartitionPoolWindow) float64 {
	delta := window.Delta
	activity := poolSaturatingAdd(delta.Gets, delta.Hits)
	activity = poolSaturatingAdd(activity, delta.Misses)
	activity = poolSaturatingAdd(activity, delta.Allocations)
	activity = poolSaturatingAdd(activity, delta.Puts)
	activity = poolSaturatingAdd(activity, delta.Retains)
	activity = poolSaturatingAdd(activity, delta.Drops)
	activity = poolSaturatingAdd(activity, delta.LeaseAcquisitions)
	activity = poolSaturatingAdd(activity, delta.LeaseReleases)
	return float64(activity)
}

type partitionTrimCandidate struct {
	index           int
	name            string
	pool            *Pool
	score           TrimVictimScore
	overTargetBytes uint64
	retainedBytes   uint64
	retainedBuffers uint64
	capacityWaste   uint64
	coldness        float64
	pressure        float64
}

func (p *PoolPartition) partitionTrimCandidates(pressureLevel PressureLevel, scoring partitionTrimScoringContext) []partitionTrimCandidate {
	candidates := make([]partitionTrimCandidate, 0, len(p.registry.entries))
	for _, entry := range p.registry.entries {
		retainedBytes, retainedBuffers, overTargetBytes, capacityWaste := partitionTrimPoolUsage(entry.pool)
		if retainedBytes == 0 {
			continue
		}

		activity, activityKnown := scoring.activity(entry.name)
		scoreInput := TrimVictimScoreInput{
			OverTargetBytes:    overTargetBytes,
			RetainedBytes:      retainedBytes,
			RetainedBuffers:    retainedBuffers,
			CapacityWasteBytes: capacityWaste,
			RecentActivity:     activity,
			ActivityKnown:      activityKnown,
			PressureLevel:      pressureLevel,
		}
		score := NewTrimVictimScore(scoreInput)

		candidates = append(candidates, partitionTrimCandidate{
			index:           entry.index,
			name:            entry.name,
			pool:            entry.pool,
			score:           score,
			overTargetBytes: overTargetBytes,
			retainedBytes:   retainedBytes,
			retainedBuffers: retainedBuffers,
			capacityWaste:   capacityWaste,
			coldness:        trimVictimComponentValue(score, trimVictimScoreComponentColdness),
			pressure:        trimVictimComponentValue(score, trimVictimScoreComponentPressure),
		})
	}

	// Ordering is deterministic: strongest victim score first, then concrete
	// retained/over-target signals, then stable registry identity.
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.score.Value != right.score.Value {
			return left.score.Value > right.score.Value
		}
		if left.overTargetBytes != right.overTargetBytes {
			return left.overTargetBytes > right.overTargetBytes
		}
		if left.retainedBytes != right.retainedBytes {
			return left.retainedBytes > right.retainedBytes
		}
		if left.capacityWaste != right.capacityWaste {
			return left.capacityWaste > right.capacityWaste
		}
		if left.name != right.name {
			return left.name < right.name
		}
		return left.index < right.index
	})
	return candidates
}

func partitionTrimCandidateReports(candidates []partitionTrimCandidate) []PartitionTrimCandidate {
	if len(candidates) == 0 {
		return nil
	}
	reports := make([]PartitionTrimCandidate, len(candidates))
	for index, candidate := range candidates {
		reports[index] = PartitionTrimCandidate{
			PoolName:           candidate.name,
			Score:              candidate.score.Clamp(),
			OverTargetBytes:    candidate.overTargetBytes,
			RetainedBytes:      candidate.retainedBytes,
			CapacityWasteBytes: candidate.capacityWaste,
			Coldness:           candidate.coldness,
			PressureSeverity:   candidate.pressure,
		}
	}
	return reports
}

func partitionTrimPoolUsage(pool *Pool) (uint64, uint64, uint64, uint64) {
	var retainedBytes uint64
	var retainedBuffers uint64
	var overTarget uint64
	var capacityWaste uint64
	for index := range pool.classes {
		state := pool.classes[index].state()
		if state.CurrentRetainedBytes == 0 {
			continue
		}
		retainedBytes = poolSaturatingAdd(retainedBytes, state.CurrentRetainedBytes)
		retainedBuffers = poolSaturatingAdd(retainedBuffers, state.CurrentRetainedBuffers)
		overTarget = poolSaturatingAdd(overTarget, poolTrimClassOverTargetBytes(state))
		capacityWaste = poolSaturatingAdd(
			capacityWaste,
			trimVictimCapacityWasteBytes(
				state.CurrentRetainedBytes,
				state.CurrentRetainedBuffers,
				state.Class.Size().Bytes(),
			),
		)
	}
	return retainedBytes, retainedBuffers, overTarget, capacityWaste
}

func partitionTrimUint64ToInt(value uint64) int {
	if value == 0 {
		return 0
	}
	maxInt := uint64(^uint(0) >> 1)
	if value > maxInt {
		return int(maxInt)
	}
	return int(value)
}
