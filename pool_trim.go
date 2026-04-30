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
}

// Trim removes retained buffers from the Pool in deterministic class/shard
// order.
//
// Trim is a cold corrective path. It removes only retained bucket storage and
// cannot see or force checked-out buffers owned by LeaseRegistry. The operation
// is bounded by MaxBuffers and MaxBytes; zero limits make the call a no-op.
func (p *Pool) Trim(plan PoolTrimPlan) PoolTrimResult {
	p.mustBeInitialized()
	if plan.MaxBuffers < 0 {
		return PoolTrimResult{Reason: errPoolTrimInvalidLimit}
	}
	if plan.MaxBuffers == 0 || plan.MaxBytes.IsZero() {
		return PoolTrimResult{Reason: errPoolTrimNoLimit}
	}

	result := PoolTrimResult{Attempted: true, Reason: errPoolTrimCompleted}
	for classIndex := range p.classes {
		if plan.MaxClasses > 0 && int(result.VisitedClasses) >= plan.MaxClasses {
			break
		}
		result.VisitedClasses++
		classResult := p.trimClassState(&p.classes[classIndex], plan.MaxBuffers-int(result.TrimmedBuffers), plan.MaxBytes.Bytes()-result.TrimmedBytes, plan.MaxShardsPerClass)
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
	if _, ok := p.table.classByID(classID); !ok {
		return PoolTrimResult{Reason: errPoolTrimInvalidClass}
	}
	result := PoolTrimResult{Attempted: true, VisitedClasses: 1, Reason: errPoolTrimCompleted}
	result.add(p.trimClassState(&p.classes[classID.Index()], maxBuffers, maxBytes.Bytes(), 0))
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
	if _, ok := p.table.classByID(classID); !ok {
		return PoolTrimResult{Reason: errPoolTrimInvalidClass}
	}
	state := &p.classes[classID.Index()]
	if shardIndex < 0 || shardIndex >= state.shardCount() {
		return PoolTrimResult{Reason: errPoolTrimInvalidShard}
	}
	bucketResult := state.trimShardBounded(shardIndex, maxBuffers, maxBytes.Bytes())
	result := PoolTrimResult{Attempted: true, VisitedClasses: 1, VisitedShards: 1, Reason: errPoolTrimCompleted}
	result.addBucket(bucketResult)
	return result
}

func (p *Pool) trimClassState(state *classState, maxBuffers int, maxBytes uint64, maxShards int) PoolTrimResult {
	result := PoolTrimResult{Attempted: true, Reason: errPoolTrimCompleted}
	if maxBuffers <= 0 || maxBytes == 0 {
		return result
	}
	for shardIndex := 0; shardIndex < state.shardCount(); shardIndex++ {
		if maxShards > 0 && int(result.VisitedShards) >= maxShards {
			break
		}
		remainingBuffers := maxBuffers - int(result.TrimmedBuffers)
		remainingBytes := maxBytes - result.TrimmedBytes
		if remainingBuffers <= 0 || remainingBytes == 0 {
			break
		}
		result.VisitedShards++
		result.addBucket(state.trimShardBounded(shardIndex, remainingBuffers, remainingBytes))
	}
	return result
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
