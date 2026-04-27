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

import "math/bits"

// PoolMetrics is a compact aggregate metrics projection of a Pool snapshot.
//
// Metrics intentionally do not expose internal shard, bucket, or class-state
// types. Detailed public diagnostics should use Snapshot; ordinary aggregate
// reporting can use Metrics. Metrics uses an internal aggregate sample so future
// controller code does not need to allocate public class/shard snapshots on
// every tick.
type PoolMetrics struct {
	// Name is diagnostic metadata copied from the sampled Pool.
	Name string

	// Lifecycle is the sampled lifecycle state.
	Lifecycle LifecycleState

	// Generation is the sampled runtime policy publication generation.
	Generation Generation

	// ClassCount is the number of configured size classes.
	ClassCount int

	// ShardCount is the total number of shards across all classes.
	ShardCount int

	// CurrentRetainedBuffers is the aggregate retained-buffer gauge.
	CurrentRetainedBuffers uint64

	// CurrentRetainedBytes is the aggregate retained-capacity gauge.
	CurrentRetainedBytes uint64

	// Gets is the aggregate acquisition count.
	Gets uint64

	// RequestedBytes is the aggregate requested logical length.
	RequestedBytes uint64

	// Hits and Misses split Get outcomes into reuse and allocation paths.
	Hits   uint64
	Misses uint64

	// HitBytes is the aggregate retained capacity returned on hits.
	HitBytes uint64

	// Allocations is the number of miss-path allocations.
	Allocations uint64

	// AllocatedBytes is the aggregate capacity allocated on misses.
	AllocatedBytes uint64

	// Puts is the valid public Put attempt count.
	Puts uint64

	// ReturnedBytes is the aggregate returned buffer capacity.
	ReturnedBytes uint64

	// Retains and Drops split Put outcomes into retained and discarded paths.
	Retains uint64
	Drops   uint64

	// RetainedBytes and DroppedBytes are capacity sums for return outcomes.
	RetainedBytes uint64
	DroppedBytes  uint64

	// DropReasons describes owner-side drop reasons included in Drops.
	DropReasons PoolDropReasonCounters

	// TrimOperations and ClearOperations count storage reduction calls.
	TrimOperations  uint64
	ClearOperations uint64

	// Trimmed/Cleared fields describe retained storage removed by reductions.
	TrimmedBuffers uint64
	TrimmedBytes   uint64
	ClearedBuffers uint64
	ClearedBytes   uint64

	// HitRatio and MissRatio use Hits+Misses as denominator.
	HitRatio  PolicyRatio
	MissRatio PolicyRatio

	// RetainRatio and DropRatio use Retains+Drops as denominator.
	RetainRatio PolicyRatio
	DropRatio   PolicyRatio
}

// Metrics returns an aggregate Pool metrics projection.
//
// Metrics is observational and not a globally atomic transaction across all
// classes and shards. Unlike Snapshot, it uses an internal aggregate sample and
// does not allocate public class/shard snapshot slices.
func (p *Pool) Metrics() PoolMetrics {
	p.mustBeInitialized()

	var sample poolCounterSample
	p.sampleCounters(&sample)

	return newPoolMetricsFromCounters(p.name, sample.Lifecycle, sample.Generation, sample.ClassCount, sample.ShardCount, sample.Counters)
}

// NewPoolMetrics converts snapshot into aggregate metrics.
//
// This helper is useful for tests and future exporters that sample snapshots
// separately from metrics projection.
func NewPoolMetrics(snapshot PoolSnapshot) PoolMetrics {
	return newPoolMetricsFromCounters(
		snapshot.Name,
		snapshot.Lifecycle,
		snapshot.Generation,
		snapshot.ClassCount(),
		snapshot.ShardCount(),
		snapshot.Counters,
	)
}

// IsZero reports whether metrics contain no observed activity and no retained
// memory.
func (m PoolMetrics) IsZero() bool {
	return m.CurrentRetainedBuffers == 0 &&
		m.CurrentRetainedBytes == 0 &&
		m.Gets == 0 &&
		m.RequestedBytes == 0 &&
		m.Hits == 0 &&
		m.HitBytes == 0 &&
		m.Misses == 0 &&
		m.Allocations == 0 &&
		m.AllocatedBytes == 0 &&
		m.Puts == 0 &&
		m.ReturnedBytes == 0 &&
		m.Retains == 0 &&
		m.RetainedBytes == 0 &&
		m.Drops == 0 &&
		m.DroppedBytes == 0 &&
		m.DropReasons.IsZero() &&
		m.TrimOperations == 0 &&
		m.TrimmedBuffers == 0 &&
		m.TrimmedBytes == 0 &&
		m.ClearOperations == 0 &&
		m.ClearedBuffers == 0 &&
		m.ClearedBytes == 0
}

// ReuseAttempts returns Hits + Misses.
func (m PoolMetrics) ReuseAttempts() uint64 {
	return m.Hits + m.Misses
}

// PutOutcomes returns Retains + Drops.
func (m PoolMetrics) PutOutcomes() uint64 {
	return m.Retains + m.Drops
}

// RemovalOperations returns trim + clear operation attempts.
func (m PoolMetrics) RemovalOperations() uint64 {
	return m.TrimOperations + m.ClearOperations
}

// RemovedBuffers returns buffers removed by trim and clear operations.
func (m PoolMetrics) RemovedBuffers() uint64 {
	return m.TrimmedBuffers + m.ClearedBuffers
}

// RemovedBytes returns retained capacity removed by trim and clear operations.
func (m PoolMetrics) RemovedBytes() uint64 {
	return m.TrimmedBytes + m.ClearedBytes
}

// poolRatio returns numerator/denominator in PolicyRatio fixed-point scale.
//
// Ratios are diagnostic projections. They are clamped to PolicyRatioScale so
// temporarily inconsistent observational samples cannot produce values above
// 100%. The multiplication path uses math/bits for extremely large counters so
// long-running pools do not overflow while computing a ratio.
func poolRatio(numerator, denominator uint64) PolicyRatio {
	if denominator == 0 || numerator == 0 {
		return 0
	}

	if numerator >= denominator {
		return PolicyRatio(PolicyRatioScale)
	}

	scale := uint64(PolicyRatioScale)
	max := ^uint64(0)
	if numerator > max/scale {
		hi, lo := bits.Mul64(numerator, scale)
		quotient, _ := bits.Div64(hi, lo, denominator)
		if quotient >= scale {
			return PolicyRatio(PolicyRatioScale)
		}

		return PolicyRatio(quotient)
	}

	return PolicyRatio((numerator * scale) / denominator)
}

func newPoolMetricsFromCounters(name string, lifecycle LifecycleState, generation Generation, classCount int, shardCount int, counters PoolCountersSnapshot) PoolMetrics {
	reuseAttempts := counters.ReuseAttempts()
	putOutcomes := counters.PutOutcomes()

	return PoolMetrics{
		Name:       name,
		Lifecycle:  lifecycle,
		Generation: generation,

		ClassCount: classCount,
		ShardCount: shardCount,

		CurrentRetainedBuffers: counters.CurrentRetainedBuffers,
		CurrentRetainedBytes:   counters.CurrentRetainedBytes,

		Gets:           counters.Gets,
		RequestedBytes: counters.RequestedBytes,
		Hits:           counters.Hits,
		HitBytes:       counters.HitBytes,
		Misses:         counters.Misses,

		Allocations:    counters.Allocations,
		AllocatedBytes: counters.AllocatedBytes,

		Puts:          counters.Puts,
		ReturnedBytes: counters.ReturnedBytes,
		Retains:       counters.Retains,
		RetainedBytes: counters.RetainedBytes,
		Drops:         counters.Drops,
		DroppedBytes:  counters.DroppedBytes,
		DropReasons:   counters.DropReasons,

		TrimOperations:  counters.TrimOperations,
		TrimmedBuffers:  counters.TrimmedBuffers,
		TrimmedBytes:    counters.TrimmedBytes,
		ClearOperations: counters.ClearOperations,
		ClearedBuffers:  counters.ClearedBuffers,
		ClearedBytes:    counters.ClearedBytes,

		HitRatio:    poolRatio(counters.Hits, reuseAttempts),
		MissRatio:   poolRatio(counters.Misses, reuseAttempts),
		RetainRatio: poolRatio(counters.Retains, putOutcomes),
		DropRatio:   poolRatio(counters.Drops, putOutcomes),
	}
}
