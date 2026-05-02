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
	// errPoolTrimInvalidClass reports a trim target outside the Pool class table.
	errPoolTrimInvalidClass = "invalid_class"

	// errPoolTrimInvalidShard reports a trim target outside a class shard set.
	errPoolTrimInvalidShard = "invalid_shard"

	// errPoolTrimInvalidLimit reports a negative trim limit.
	errPoolTrimInvalidLimit = "invalid_limit"

	// errPoolTrimNoLimit reports a trim request with no positive removal bound.
	errPoolTrimNoLimit = "no_limit"

	// errPoolTrimCompleted reports a valid bounded trim attempt.
	errPoolTrimCompleted = "completed"

	// errPoolTrimClosed reports lifecycle rejection before retained storage is
	// inspected or mutated.
	errPoolTrimClosed = "trim_closed"
)

// PoolTrimPlan bounds one Pool-wide physical trim operation.
type PoolTrimPlan struct {
	// MaxBuffers bounds the number of retained buffers removed.
	MaxBuffers int

	// MaxBytes bounds the retained backing capacity removed.
	MaxBytes Size

	// MaxClasses bounds class visits when positive.
	MaxClasses int

	// MaxShardsPerClass bounds shard visits per class when positive.
	MaxShardsPerClass int
}

// PoolTrimResult reports a physical Pool trim operation.
type PoolTrimResult struct {
	// Attempted reports whether the trim request passed basic limit validation.
	Attempted bool

	// Executed reports whether retained buffers were removed.
	Executed bool

	// Reason is a stable diagnostic reason.
	Reason string

	// VisitedClasses is the number of classes inspected.
	VisitedClasses uint64

	// VisitedShards is the number of shards trimmed or inspected.
	VisitedShards uint64

	// TrimmedBuffers is the number of retained buffers removed.
	TrimmedBuffers uint64

	// TrimmedBytes is the retained backing capacity removed.
	TrimmedBytes uint64

	// CandidateClasses records the target-aware, score-ordered class candidates
	// considered by Trim.
	CandidateClasses []PoolTrimCandidate
}

// PoolTrimCandidate describes one class selected for scored trim order.
type PoolTrimCandidate struct {
	// ClassID identifies the candidate class.
	ClassID ClassID

	// Score is the normalized candidate-level victim score.
	Score TrimVictimScore

	// OverTargetBytes is retained bytes above the observed class budget.
	OverTargetBytes uint64

	// RetainedBytes is the current retained byte gauge for the class.
	RetainedBytes uint64

	// CapacityWasteBytes is retained capacity above nominal class capacity.
	CapacityWasteBytes uint64

	// Coldness is the coldness component used for Score.
	Coldness float64

	// PressureSeverity is the pressure component used for Score.
	PressureSeverity float64
}

// Trim removes retained buffers from the Pool in deterministic target-aware
// class and shard order.
//
// Trim is a cold corrective path. It removes only retained bucket storage and
// cannot see or force checked-out buffers owned by LeaseRegistry. Candidate
// order is selected by TrimVictimScore over class-level signals such as
// over-target storage, retained bytes, neutral coldness, capacity waste, pressure
// severity, and class size. It does not score individual buffers. Execution
// remains bounded by MaxBuffers, MaxBytes, MaxClasses, and MaxShardsPerClass;
// zero limits make the call a no-op.
func (p *Pool) Trim(plan PoolTrimPlan) PoolTrimResult {
	p.mustBeInitialized()

	if plan.MaxBuffers < 0 {
		return PoolTrimResult{Reason: errPoolTrimInvalidLimit}
	}
	if plan.MaxBuffers == 0 || plan.MaxBytes.IsZero() {
		return PoolTrimResult{Reason: errPoolTrimNoLimit}
	}

	if err := p.beginPoolControlOperation(); err != nil {
		return poolTrimClosedResult()
	}
	defer p.endOperation()

	p.controlMu.Lock()
	defer p.controlMu.Unlock()

	return p.trimLocked(plan)
}

// trimLocked executes a Pool-wide physical trim while a Pool control operation
// and controlMu are already held.
//
// The helper exists so Pool.PublishPolicy can publish a contraction and execute
// optional bounded cleanup inside one ordered Pool-local control operation
// without re-entering the lifecycle gate or control mutex.
func (p *Pool) trimLocked(plan PoolTrimPlan) PoolTrimResult {
	result := PoolTrimResult{Attempted: true, Reason: errPoolTrimCompleted}
	pressureLevel := p.currentRuntimeSnapshot().Pressure.Level
	candidates := p.poolTrimCandidates(pressureLevel)
	result.CandidateClasses = poolTrimCandidateReports(candidates)

	for _, candidate := range candidates {
		if plan.MaxClasses > 0 && int(result.VisitedClasses) >= plan.MaxClasses {
			break
		}

		result.VisitedClasses++
		classResult := p.trimClassState(&p.classes[candidate.index], plan.MaxBuffers-int(result.TrimmedBuffers), plan.MaxBytes.Bytes()-result.TrimmedBytes, plan.MaxShardsPerClass, pressureLevel)
		result.add(classResult)

		if result.limitsReached(plan.MaxBuffers, plan.MaxBytes.Bytes()) {
			break
		}
	}
	return result
}

// TrimClass removes retained buffers from one Pool class.
func (p *Pool) TrimClass(classID ClassID, maxBuffers int, maxBytes Size) PoolTrimResult {
	p.mustBeInitialized()

	if maxBuffers < 0 {
		return PoolTrimResult{Reason: errPoolTrimInvalidLimit}
	}
	if maxBuffers == 0 || maxBytes.IsZero() {
		return PoolTrimResult{Reason: errPoolTrimNoLimit}
	}

	if err := p.beginPoolControlOperation(); err != nil {
		return poolTrimClosedResult()
	}
	defer p.endOperation()

	if _, ok := p.table.classByID(classID); !ok {
		return PoolTrimResult{Reason: errPoolTrimInvalidClass}
	}

	p.controlMu.Lock()
	defer p.controlMu.Unlock()

	result := PoolTrimResult{Attempted: true, VisitedClasses: 1, Reason: errPoolTrimCompleted}
	result.add(p.trimClassState(&p.classes[classID.Index()], maxBuffers, maxBytes.Bytes(), 0, p.currentRuntimeSnapshot().Pressure.Level))
	return result
}

// TrimShard removes retained buffers from one class shard.
func (p *Pool) TrimShard(classID ClassID, shardIndex int, maxBuffers int, maxBytes Size) PoolTrimResult {
	p.mustBeInitialized()

	if maxBuffers < 0 {
		return PoolTrimResult{Reason: errPoolTrimInvalidLimit}
	}
	if maxBuffers == 0 || maxBytes.IsZero() {
		return PoolTrimResult{Reason: errPoolTrimNoLimit}
	}

	if err := p.beginPoolControlOperation(); err != nil {
		return poolTrimClosedResult()
	}
	defer p.endOperation()

	if _, ok := p.table.classByID(classID); !ok {
		return PoolTrimResult{Reason: errPoolTrimInvalidClass}
	}

	state := &p.classes[classID.Index()]
	if shardIndex < 0 || shardIndex >= state.shardCount() {
		return PoolTrimResult{Reason: errPoolTrimInvalidShard}
	}

	p.controlMu.Lock()
	defer p.controlMu.Unlock()

	bucketResult := state.trimShardBounded(shardIndex, maxBuffers, maxBytes.Bytes())
	result := PoolTrimResult{Attempted: true, VisitedClasses: 1, VisitedShards: 1, Reason: errPoolTrimCompleted}
	result.addBucket(bucketResult)
	return result
}

func (p *Pool) trimClassState(state *classState, maxBuffers int, maxBytes uint64, maxShards int, pressureLevel PressureLevel) PoolTrimResult {
	result := PoolTrimResult{Attempted: true, Reason: errPoolTrimCompleted}
	if maxBuffers <= 0 || maxBytes == 0 {
		return result
	}

	for _, candidate := range classTrimShardCandidates(state, pressureLevel) {
		if maxShards > 0 && int(result.VisitedShards) >= maxShards {
			break
		}

		remainingBuffers := maxBuffers - int(result.TrimmedBuffers)
		remainingBytes := maxBytes - result.TrimmedBytes
		if remainingBuffers <= 0 || remainingBytes == 0 {
			break
		}

		result.VisitedShards++
		result.addBucket(state.trimShardBounded(candidate.index, remainingBuffers, remainingBytes))
	}

	return result
}

type poolTrimCandidate struct {
	index           int
	classID         ClassID
	score           TrimVictimScore
	overTargetBytes uint64
	retainedBytes   uint64
	retainedBuffers uint64
	capacityWaste   uint64
	classBytes      uint64
	coldness        float64
	pressure        float64
}

type classTrimShardCandidate struct {
	index           int
	score           TrimVictimScore
	overTargetBytes uint64
	retainedBytes   uint64
	retainedBuffers uint64
	capacityWaste   uint64
}

func (p *Pool) poolTrimCandidates(pressureLevel PressureLevel) []poolTrimCandidate {
	candidates := make([]poolTrimCandidate, 0, len(p.classes))
	for index := range p.classes {
		state := p.classes[index].state()

		retainedBytes := state.CurrentRetainedBytes
		if retainedBytes == 0 {
			continue
		}

		overTargetBytes := poolTrimClassOverTargetBytes(state)
		retainedBuffers := state.CurrentRetainedBuffers
		classBytes := state.Class.Size().Bytes()
		capacityWaste := trimVictimCapacityWasteBytes(retainedBytes, retainedBuffers, classBytes)

		scoreInput := TrimVictimScoreInput{
			OverTargetBytes:    overTargetBytes,
			RetainedBytes:      retainedBytes,
			RetainedBuffers:    retainedBuffers,
			ClassBytes:         classBytes,
			CapacityWasteBytes: capacityWaste,
			PressureLevel:      pressureLevel,
		}

		score := NewTrimVictimScore(scoreInput)
		candidates = append(candidates, poolTrimCandidate{
			index:           index,
			classID:         state.Class.ID(),
			score:           score,
			overTargetBytes: overTargetBytes,
			retainedBytes:   retainedBytes,
			retainedBuffers: retainedBuffers,
			capacityWaste:   capacityWaste,
			classBytes:      classBytes,
			coldness:        trimVictimComponentValue(score, trimVictimScoreComponentColdness),
			pressure:        trimVictimComponentValue(score, trimVictimScoreComponentPressure),
		})
	}

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
		if left.classBytes != right.classBytes {
			return left.classBytes > right.classBytes
		}
		return left.index < right.index
	})

	return candidates
}

func poolTrimCandidateReports(candidates []poolTrimCandidate) []PoolTrimCandidate {
	if len(candidates) == 0 {
		return nil
	}

	reports := make([]PoolTrimCandidate, len(candidates))
	for index, candidate := range candidates {
		reports[index] = PoolTrimCandidate{
			ClassID:            candidate.classID,
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

// poolTrimClosedResult returns the stable closed-path trim report.
func poolTrimClosedResult() PoolTrimResult {
	return PoolTrimResult{Reason: errPoolTrimClosed}
}

func poolTrimClassOverTargetBytes(state classStateSnapshot) uint64 {
	if !state.Budget.IsEffective() {
		return state.CurrentRetainedBytes
	}

	if state.CurrentRetainedBytes <= state.Budget.TargetBytes {
		return 0
	}

	return state.CurrentRetainedBytes - state.Budget.TargetBytes
}

func classTrimShardCandidates(state *classState, pressureLevel PressureLevel) []classTrimShardCandidate {
	snapshot := state.state()
	candidates := make([]classTrimShardCandidate, 0, len(snapshot.Shards))
	for index, shard := range snapshot.Shards {
		retainedBytes := shard.Counters.CurrentRetainedBytes
		if retainedBytes == 0 {
			continue
		}

		overTargetBytes := poolTrimShardOverTargetBytes(shard)
		retainedBuffers := shard.Counters.CurrentRetainedBuffers
		classBytes := snapshot.Class.Size().Bytes()
		capacityWaste := trimVictimCapacityWasteBytes(retainedBytes, retainedBuffers, classBytes)

		scoreInput := TrimVictimScoreInput{
			OverTargetBytes:    overTargetBytes,
			RetainedBytes:      retainedBytes,
			RetainedBuffers:    retainedBuffers,
			ClassBytes:         classBytes,
			CapacityWasteBytes: capacityWaste,
			PressureLevel:      pressureLevel,
		}

		candidates = append(candidates, classTrimShardCandidate{
			index:           index,
			score:           NewTrimVictimScore(scoreInput),
			overTargetBytes: overTargetBytes,
			retainedBytes:   retainedBytes,
			retainedBuffers: retainedBuffers,
			capacityWaste:   capacityWaste,
		})
	}
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
		return left.index < right.index
	})
	return candidates
}

func poolTrimShardOverTargetBytes(state shardState) uint64 {
	if !state.Credit.IsEnabled() {
		return state.Counters.CurrentRetainedBytes
	}
	if state.Counters.CurrentRetainedBytes <= state.Credit.TargetBytes {
		return 0
	}
	return state.Counters.CurrentRetainedBytes - state.Credit.TargetBytes
}

func (r *PoolTrimResult) add(other PoolTrimResult) {
	r.VisitedShards += other.VisitedShards
	r.TrimmedBuffers += other.TrimmedBuffers
	r.TrimmedBytes += other.TrimmedBytes
	if other.Executed {
		r.Executed = true
	}
}

func (r *PoolTrimResult) addBucket(result bucketTrimResult) {
	if result.RemovedBuffers == 0 && result.RemovedBytes == 0 {
		return
	}
	r.Executed = true
	r.TrimmedBuffers += uint64(result.RemovedBuffers)
	r.TrimmedBytes += result.RemovedBytes
}

func (r PoolTrimResult) limitsReached(maxBuffers int, maxBytes uint64) bool {
	return int(r.TrimmedBuffers) >= maxBuffers || r.TrimmedBytes >= maxBytes
}
