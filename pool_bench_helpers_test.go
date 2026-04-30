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

import (
	"strconv"
	"strings"
	"testing"
)

const (
	poolBenchmarkDefaultBucketSlots = 512
	poolBenchmarkDefaultSeedBatch   = 64
)

var (
	poolBenchmarkBufferSink   []byte
	poolBenchmarkErrorSink    error
	poolBenchmarkIntSink      int
	poolBenchmarkMetricsSink  PoolMetrics
	poolBenchmarkSnapshotSink PoolSnapshot
	poolBenchmarkSampleSink   poolCounterSample
)

// Pool benchmark command guide:
//
//   - full matrix:
//     go test -run '^$' -bench . -benchmem ./...
//   - quick pool data-plane slice:
//     go test -run '^$' -bench 'BenchmarkPool(GetHit|GetMiss|PutRetain|Metrics|SampleCounters)' -benchmem ./...
//   - parallel CPU matrix:
//     go test -run '^$' -bench 'BenchmarkPoolGetPutParallel' -benchmem -cpu 1,2,4,8,16 ./...
//   - broad pool-only matrix:
//     go test -run '^$' -bench 'BenchmarkPool(Baseline|Get|Put|GetPut|Zeroing|Metrics|Snapshot|Sample|ShardSelector)' -benchmem ./...
//
// The full matrix is intentionally diagnostic and can be slow. Use focused
// expressions when iterating on one part of the data plane.

// poolBenchmarkCase describes one benchmark topology or data-plane scenario.
//
// Benchmarks use explicit cases instead of hidden package defaults so output
// names tell the reader which class size, shard count, selector mode, zeroing
// mode, and retained seed shape were measured.
type poolBenchmarkCase struct {
	name         string
	size         int
	capacity     int
	shards       int
	selector     ShardSelectionMode
	classes      []ClassSize
	bucketSlots  int
	retainedSeed int
	zeroRetained bool
	zeroDropped  bool
}

// poolBenchmarkPolicyForCase builds a valid, high-credit static policy for a
// benchmark case.
//
// The policy keeps the Pool semantics intact: class table lookup, lifecycle,
// counters, owner-side admission, shard credit, and bounded buckets all remain
// active. The limits are deliberately generous so benchmarks named as hit or
// retain paths do not silently become credit-drop benchmarks.
func poolBenchmarkPolicyForCase(tc poolBenchmarkCase) Policy {
	classes := tc.classes
	if len(classes) == 0 {
		classes = poolBenchmarkDefaultClasses()
	}

	shards := tc.shards
	if shards <= 0 {
		shards = 1
	}

	bucketSlots := tc.bucketSlots
	if bucketSlots <= 0 {
		bucketSlots = poolBenchmarkDefaultBucketSlots
	}
	bucketSegmentSlots := poolBenchmarkBucketSegmentSlots(bucketSlots)

	selector := tc.selector
	if selector == ShardSelectionModeUnset {
		selector = ShardSelectionModeSingle
	}

	largest := poolBenchmarkLargestClassSize(classes)
	shardBuffers := uint64(bucketSlots)
	classBuffers := poolSaturatingProduct(uint64(shards), shardBuffers)
	totalBuffers := poolSaturatingProduct(uint64(len(classes)), classBuffers)
	shardBytes := poolSaturatingProduct(shardBuffers, largest.Bytes())
	classBytes := poolSaturatingProduct(uint64(shards), shardBytes)
	totalBytes := poolSaturatingProduct(uint64(len(classes)), classBytes)

	return Policy{
		Retention: RetentionPolicy{
			SoftRetainedBytes:         SizeFromBytes(totalBytes),
			HardRetainedBytes:         SizeFromBytes(totalBytes),
			MaxRetainedBuffers:        totalBuffers,
			MaxRequestSize:            largest.Size(),
			MaxRetainedBufferCapacity: largest.Size(),
			MaxClassRetainedBytes:     SizeFromBytes(classBytes),
			MaxClassRetainedBuffers:   classBuffers,
			MaxShardRetainedBytes:     SizeFromBytes(shardBytes),
			MaxShardRetainedBuffers:   shardBuffers,
		},
		Classes: ClassPolicy{
			Sizes: append([]ClassSize(nil), classes...),
		},
		Shards: ShardPolicy{
			Selection:                  selector,
			ShardsPerClass:             shards,
			BucketSlotsPerShard:        bucketSlots,
			BucketSegmentSlotsPerShard: bucketSegmentSlots,
			AcquisitionFallbackShards:  0,
			ReturnFallbackShards:       0,
		},
		Admission: AdmissionPolicy{
			ZeroSizeRequests:    ZeroSizeRequestEmptyBuffer,
			ReturnedBuffers:     ReturnedBufferPolicyAdmit,
			OversizedReturn:     AdmissionActionDrop,
			UnsupportedClass:    AdmissionActionDrop,
			ClassMismatch:       AdmissionActionDrop,
			CreditExhausted:     AdmissionActionDrop,
			BucketFull:          AdmissionActionDrop,
			ZeroRetainedBuffers: tc.zeroRetained,
			ZeroDroppedBuffers:  tc.zeroDropped,
		},
		Pressure: PressurePolicy{},
		Trim:     TrimPolicy{},
		Ownership: OwnershipPolicy{
			Mode:                      OwnershipModeNone,
			TrackInUseBytes:           false,
			TrackInUseBuffers:         false,
			DetectDoubleRelease:       false,
			MaxReturnedCapacityGrowth: 2 * PolicyRatioOne,
		},
	}
}

// poolBenchmarkBucketSegmentSlots resolves a benchmark segment size that keeps
// bucket metadata lazy without making benchmark setup invalid for small bucket
// slot counts.
func poolBenchmarkBucketSegmentSlots(bucketSlots int) int {
	if bucketSlots < DefaultPolicyBucketSegmentSlotsPerShard {
		return bucketSlots
	}

	return DefaultPolicyBucketSegmentSlotsPerShard
}

// poolBenchmarkNewPool constructs a Pool and registers cleanup for a benchmark.
func poolBenchmarkNewPool(b *testing.B, policy Policy) *Pool {
	b.Helper()

	pool := MustNew(PoolConfig{Policy: policy})
	b.Cleanup(func() {
		_ = pool.Close()
	})

	return pool
}

// poolBenchmarkNewPoolWithConfig constructs a Pool from a full config.
func poolBenchmarkNewPoolWithConfig(b *testing.B, config PoolConfig) *Pool {
	b.Helper()

	pool := MustNew(config)
	b.Cleanup(func() {
		_ = pool.Close()
	})

	return pool
}

// poolBenchmarkSeedRetainedInternal preloads retained storage without measuring
// setup and without passing through public Pool.Put.
//
// The helper uses classState directly so seed distribution can be exact and does
// not depend on selector state. This keeps hit-path benchmarks from accidentally
// measuring miss allocation because setup and measured selector sequences drift.
// Use this only for controlled storage setup. Observability and realistic flow
// benchmarks should use poolBenchmarkSeedRetainedPublic so owner counters and
// lifecycle admission describe a public Pool state.
func poolBenchmarkSeedRetainedInternal(b *testing.B, pool *Pool, capacity int, count int) {
	b.Helper()

	if count <= 0 {
		return
	}

	class, ok := pool.table.classForCapacity(SizeFromInt(capacity))
	if !ok {
		b.Fatalf("capacity %d is not supported by benchmark policy", capacity)
	}

	state := pool.mustClassStateFor(class)
	shardCount := len(state.shards)
	for index := 0; index < count; index++ {
		result := state.tryRetain(index%shardCount, make([]byte, 0, capacity))
		if !result.Retained() {
			b.Fatalf("seed retain %d failed: %#v", index, result)
		}
	}
}

// poolBenchmarkSeedRetainedShardInternal preloads one exact shard for fallback
// probing benchmarks.
func poolBenchmarkSeedRetainedShardInternal(b *testing.B, pool *Pool, capacity int, shardIndex int, count int) {
	b.Helper()

	if count <= 0 {
		return
	}

	class, ok := pool.table.classForCapacity(SizeFromInt(capacity))
	if !ok {
		b.Fatalf("capacity %d is not supported by benchmark policy", capacity)
	}

	state := pool.mustClassStateFor(class)
	if shardIndex < 0 || shardIndex >= len(state.shards) {
		b.Fatalf("shard index %d outside [0,%d)", shardIndex, len(state.shards))
	}
	for index := 0; index < count; index++ {
		result := state.tryRetain(shardIndex, make([]byte, 0, capacity))
		if !result.Retained() {
			b.Fatalf("seed retain %d on shard %d failed: %#v", index, shardIndex, result)
		}
	}
}

// poolBenchmarkSeedRetainedPublic preloads retained storage through Pool.Put.
//
// Public seeding is less exact than internal seeding, but it produces realistic
// Pool snapshots: owner counters, lifecycle admission, runtime snapshot reads,
// class counters, shard counters, and retained storage all observe the seed.
func poolBenchmarkSeedRetainedPublic(b *testing.B, pool *Pool, capacity int, count int) {
	b.Helper()

	for index := 0; index < count; index++ {
		if err := pool.Put(make([]byte, 0, capacity)); err != nil {
			b.Fatalf("public seed Put(%d) returned error: %v", index, err)
		}
	}
}

// poolBenchmarkClearRetained removes retained storage between measured batches.
func poolBenchmarkClearRetained(pool *Pool) {
	pool.clearRetainedStorage()
}

// poolBenchmarkDefaultClasses returns the common benchmark class profile.
func poolBenchmarkDefaultClasses() []ClassSize {
	return []ClassSize{
		ClassSizeFromBytes(512),
		ClassSizeFromSize(KiB),
		ClassSizeFromSize(4 * KiB),
		ClassSizeFromSize(64 * KiB),
	}
}

// poolBenchmarkClasses returns count power-of-two classes starting at 512 B.
func poolBenchmarkClasses(count int) []ClassSize {
	classes := make([]ClassSize, count)
	size := uint64(512)
	for index := range classes {
		classes[index] = ClassSizeFromBytes(size)
		size *= 2
	}

	return classes
}

// poolBenchmarkLargestClassSize returns the largest class in an ordered profile.
func poolBenchmarkLargestClassSize(classes []ClassSize) ClassSize {
	if len(classes) == 0 {
		panic("bufferpool.poolBenchmark: class profile must not be empty")
	}

	return classes[len(classes)-1]
}

// poolBenchmarkRoutingCases returns representative class/selector topologies
// for Get and Put path benchmarks.
func poolBenchmarkRoutingCases() []poolBenchmarkCase {
	return []poolBenchmarkCase{
		{name: "size_300/cap_512/shards_1/selector_single", size: 300, capacity: 512, shards: 1, selector: ShardSelectionModeSingle},
		{name: "size_300/cap_512/shards_8/selector_round_robin", size: 300, capacity: 512, shards: 8, selector: ShardSelectionModeRoundRobin},
		{name: "size_300/cap_512/shards_32/selector_random", size: 300, capacity: 512, shards: 32, selector: ShardSelectionModeRandom},
		{name: "size_900/cap_1024/shards_8/selector_round_robin", size: 900, capacity: 1024, shards: 8, selector: ShardSelectionModeRoundRobin},
		{name: "size_4096/cap_4096/shards_1/selector_single", size: 4096, capacity: 4096, shards: 1, selector: ShardSelectionModeSingle},
	}
}

// poolBenchmarkHitCases returns only cases where Get can be guaranteed to hit
// without relying on selector distribution.
func poolBenchmarkHitCases() []poolBenchmarkCase {
	return []poolBenchmarkCase{
		{name: "size_300/cap_512/shards_1/selector_single", size: 300, capacity: 512, shards: 1, selector: ShardSelectionModeSingle},
		{name: "size_900/cap_1024/shards_1/selector_single", size: 900, capacity: 1024, shards: 1, selector: ShardSelectionModeSingle},
		{name: "size_4096/cap_4096/shards_1/selector_single", size: 4096, capacity: 4096, shards: 1, selector: ShardSelectionModeSingle},
	}
}

// poolBenchmarkSeededMixedCases returns seeded storage scenarios where selector
// behavior can still produce misses.
func poolBenchmarkSeededMixedCases() []poolBenchmarkCase {
	return []poolBenchmarkCase{
		{name: "size_300/cap_512/shards_8/selector_round_robin", size: 300, capacity: 512, shards: 8, selector: ShardSelectionModeRoundRobin},
		{name: "size_300/cap_512/shards_32/selector_random", size: 300, capacity: 512, shards: 32, selector: ShardSelectionModeRandom},
		{name: "size_900/cap_1024/shards_8/selector_round_robin", size: 900, capacity: 1024, shards: 8, selector: ShardSelectionModeRoundRobin},
	}
}

// poolBenchmarkFlowCases returns the Get+Put matrix used for stable flow
// benchmarks.
func poolBenchmarkFlowCases() []poolBenchmarkCase {
	var cases []poolBenchmarkCase
	for _, size := range []struct {
		name     string
		request  int
		capacity int
	}{
		{name: "size_300/cap_512", request: 300, capacity: 512},
		{name: "size_900/cap_1024", request: 900, capacity: 1024},
	} {
		for _, shards := range []int{1, 8, 32} {
			for _, selector := range []ShardSelectionMode{
				ShardSelectionModeSingle,
				ShardSelectionModeRoundRobin,
				ShardSelectionModeRandom,
			} {
				cases = append(cases, poolBenchmarkCase{
					name:     size.name + "/shards_" + strconv.Itoa(shards) + "/selector_" + poolBenchmarkSelectorName(selector),
					size:     size.request,
					capacity: size.capacity,
					shards:   shards,
					selector: selector,
				})
			}
		}
	}

	return cases
}

// poolBenchmarkName joins benchmark name fragments without hiding dimensions in
// helper state.
func poolBenchmarkName(parts ...string) string {
	return strings.Join(parts, "/")
}

// poolBenchmarkSelectorName returns a stable selector label for benchstat.
func poolBenchmarkSelectorName(mode ShardSelectionMode) string {
	switch mode {
	case ShardSelectionModeSingle:
		return "single"
	case ShardSelectionModeRoundRobin:
		return "round_robin"
	case ShardSelectionModeRandom:
		return "random"
	case ShardSelectionModeProcessorInspired:
		return "processor_inspired"
	case ShardSelectionModeAffinity:
		return "affinity"
	default:
		return "unknown"
	}
}

// poolBenchmarkBatchSize keeps setup outside measured loops without creating
// huge out-of-timer allocation bursts for large buffer classes.
func poolBenchmarkBatchSize(capacity int) int {
	if capacity >= 64*KiB.Int() {
		return 8
	}

	if capacity >= 4*KiB.Int() {
		return 32
	}

	return poolBenchmarkDefaultSeedBatch
}

// poolBenchmarkReportGetRatios reports acquisition ratios for Get-only
// benchmarks.
//
// It intentionally omits retain/drop ratios because those may describe seed
// setup rather than the measured Get loop.
func poolBenchmarkReportGetRatios(b *testing.B, pool *Pool) {
	b.Helper()
	b.StopTimer()

	metrics := pool.Metrics()
	b.ReportMetric(metrics.HitRatio.Float64(), "actual_hit_ratio")
	b.ReportMetric(metrics.MissRatio.Float64(), "actual_miss_ratio")
}

// poolBenchmarkReportReturnRatios reports return outcome ratios for Put or
// return-rate benchmarks.
func poolBenchmarkReportReturnRatios(b *testing.B, pool *Pool) {
	b.Helper()
	b.StopTimer()

	metrics := pool.Metrics()
	b.ReportMetric(metrics.RetainRatio.Float64(), "actual_retain_ratio")
	b.ReportMetric(metrics.DropRatio.Float64(), "actual_drop_ratio")
}

// poolBenchmarkReportOutcomeRatios reports acquisition and return ratios for
// benchmarks that actually measure both sides of the data-plane flow.
func poolBenchmarkReportOutcomeRatios(b *testing.B, pool *Pool) {
	b.Helper()
	b.StopTimer()

	metrics := pool.Metrics()
	b.ReportMetric(metrics.HitRatio.Float64(), "actual_hit_ratio")
	b.ReportMetric(metrics.MissRatio.Float64(), "actual_miss_ratio")
	b.ReportMetric(metrics.RetainRatio.Float64(), "actual_retain_ratio")
	b.ReportMetric(metrics.DropRatio.Float64(), "actual_drop_ratio")
}
