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

import "testing"

const pressureTrimBenchmarkBatch = 64

var (
	trimVictimScoreSink         TrimVictimScore
	poolTrimCandidatesSink      []poolTrimCandidate
	partitionTrimCandidatesSink []partitionTrimCandidate
)

// BenchmarkPoolTrimClass measures one class-bounded physical trim operation.
func BenchmarkPoolTrimClass(b *testing.B) {
	pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(poolBenchmarkCase{
		capacity:    512,
		shards:      1,
		selector:    ShardSelectionModeSingle,
		bucketSlots: 128,
	}))
	buffer := make([]byte, 0, 512)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		poolBenchmarkClearRetained(pool)
		benchmarkSeedRetainedBuffers(b, pool, buffer, pressureTrimBenchmarkBatch)

		result := pool.TrimClass(ClassID(0), pressureTrimBenchmarkBatch, SizeFromBytes(pressureTrimBenchmarkBatch*512))
		if !result.Executed || result.TrimmedBuffers != pressureTrimBenchmarkBatch {
			b.Fatalf("TrimClass() = %+v, want %d trimmed buffers", result, pressureTrimBenchmarkBatch)
		}
	}
}

// BenchmarkPoolTrimShard measures one shard-bounded physical trim operation.
func BenchmarkPoolTrimShard(b *testing.B) {
	pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(poolBenchmarkCase{
		capacity:    512,
		shards:      1,
		selector:    ShardSelectionModeSingle,
		bucketSlots: 128,
	}))
	buffer := make([]byte, 0, 512)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		poolBenchmarkClearRetained(pool)
		benchmarkSeedRetainedBuffers(b, pool, buffer, pressureTrimBenchmarkBatch)

		result := pool.TrimShard(ClassID(0), 0, pressureTrimBenchmarkBatch, SizeFromBytes(pressureTrimBenchmarkBatch*512))
		if !result.Executed || result.TrimmedBuffers != pressureTrimBenchmarkBatch {
			b.Fatalf("TrimShard() = %+v, want %d trimmed buffers", result, pressureTrimBenchmarkBatch)
		}
	}
}

// BenchmarkPartitionExecuteTrim measures partition-local bounded trim dispatch.
func BenchmarkPartitionExecuteTrim(b *testing.B) {
	config := testPartitionConfig("primary")
	config.Policy.Trim = PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 1, MaxBytesPerCycle: SizeFromBytes(pressureTrimBenchmarkBatch * 512)}
	config.Pools[0].Config.Policy = poolBenchmarkPolicyForCase(poolBenchmarkCase{
		capacity:    512,
		shards:      1,
		selector:    ShardSelectionModeSingle,
		bucketSlots: 128,
	})
	partition := MustNewPoolPartition(config)
	b.Cleanup(func() { _ = partition.Close() })
	pool, ok := partition.registry.pool("primary")
	if !ok {
		b.Fatal("primary pool missing")
	}
	buffer := make([]byte, 0, 512)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		poolBenchmarkClearRetained(pool)
		benchmarkSeedRetainedBuffers(b, pool, buffer, pressureTrimBenchmarkBatch)

		result := partition.ExecuteTrim()
		if !result.Executed || result.TrimmedBuffers != pressureTrimBenchmarkBatch {
			b.Fatalf("ExecuteTrim() = %+v, want %d trimmed buffers", result, pressureTrimBenchmarkBatch)
		}
	}
}

// BenchmarkTrimVictimScore measures pure candidate-level victim scoring.
func BenchmarkTrimVictimScore(b *testing.B) {
	input := TrimVictimScoreInput{
		OverTargetBytes:    4 * KiB.Bytes(),
		RetainedBytes:      8 * KiB.Bytes(),
		RetainedBuffers:    8,
		ClassBytes:         KiB.Bytes(),
		CapacityWasteBytes: 2 * KiB.Bytes(),
		ActivityKnown:      true,
		RecentActivity:     8,
		PressureLevel:      PressureLevelHigh,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trimVictimScoreSink = NewTrimVictimScore(input)
	}
}

// BenchmarkPoolTrimCandidateScoring measures Pool-local class candidate
// ranking without physical trim.
func BenchmarkPoolTrimCandidateScoring(b *testing.B) {
	pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(poolBenchmarkCase{
		shards:      4,
		selector:    ShardSelectionModeSingle,
		bucketSlots: 128,
	}))
	for index, capacity := range []int{1024, 2048, 4096, 65536} {
		benchmarkSeedClassRetainedBuffers(b, pool, ClassID(index), capacity, pressureTrimBenchmarkBatch/4)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		poolTrimCandidatesSink = pool.poolTrimCandidates(PressureLevelHigh)
	}
}

// BenchmarkPartitionTrimCandidateScoring measures partition-local Pool
// candidate ranking without physical trim.
func BenchmarkPartitionTrimCandidateScoring(b *testing.B) {
	config := testPartitionConfig("api", "worker", "events", "batch")
	config.Policy.Trim = PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 4, MaxBytesPerCycle: SizeFromBytes(pressureTrimBenchmarkBatch * 1024)}
	for index := range config.Pools {
		config.Pools[index].Config.Policy = poolBenchmarkPolicyForCase(poolBenchmarkCase{
			shards:      2,
			selector:    ShardSelectionModeSingle,
			bucketSlots: 128,
		})
	}
	partition := MustNewPoolPartition(config)
	b.Cleanup(func() { _ = partition.Close() })
	for index, entry := range partition.registry.entries {
		benchmarkSeedClassRetainedBuffers(b, entry.pool, ClassID(index%2), 1024, pressureTrimBenchmarkBatch/4)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionTrimCandidatesSink = partition.partitionTrimCandidates(PressureLevelHigh, partitionTrimScoringContext{})
	}
}

// BenchmarkPoolTrimWithScoring measures candidate scoring plus bounded
// Pool-local physical trim. Retained setup is outside the timed region.
func BenchmarkPoolTrimWithScoring(b *testing.B) {
	pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(poolBenchmarkCase{
		capacity:    512,
		shards:      1,
		selector:    ShardSelectionModeSingle,
		bucketSlots: 128,
	}))
	buffer := make([]byte, 0, 512)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		poolBenchmarkClearRetained(pool)
		benchmarkSeedRetainedBuffers(b, pool, buffer, pressureTrimBenchmarkBatch)
		b.StartTimer()

		result := pool.Trim(PoolTrimPlan{MaxBuffers: pressureTrimBenchmarkBatch, MaxBytes: SizeFromBytes(pressureTrimBenchmarkBatch * 512)})
		if !result.Executed || result.TrimmedBuffers != pressureTrimBenchmarkBatch {
			b.Fatalf("Trim() = %+v, want %d trimmed buffers", result, pressureTrimBenchmarkBatch)
		}
	}
}

// BenchmarkPartitionTrimWithScoring measures partition candidate scoring plus
// bounded Pool trim dispatch. Retained setup is outside the timed region.
func BenchmarkPartitionTrimWithScoring(b *testing.B) {
	config := testPartitionConfig("primary")
	config.Policy.Trim = PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 1, MaxBytesPerCycle: SizeFromBytes(pressureTrimBenchmarkBatch * 512)}
	config.Pools[0].Config.Policy = poolBenchmarkPolicyForCase(poolBenchmarkCase{
		capacity:    512,
		shards:      1,
		selector:    ShardSelectionModeSingle,
		bucketSlots: 128,
	})
	partition := MustNewPoolPartition(config)
	b.Cleanup(func() { _ = partition.Close() })
	pool, ok := partition.registry.pool("primary")
	if !ok {
		b.Fatal("primary pool missing")
	}
	buffer := make([]byte, 0, 512)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		poolBenchmarkClearRetained(pool)
		benchmarkSeedRetainedBuffers(b, pool, buffer, pressureTrimBenchmarkBatch)
		b.StartTimer()

		result := partition.ExecuteTrim()
		if !result.Executed || result.TrimmedBuffers != pressureTrimBenchmarkBatch {
			b.Fatalf("ExecuteTrim() = %+v, want %d trimmed buffers", result, pressureTrimBenchmarkBatch)
		}
	}
}

func benchmarkSeedRetainedBuffers(b *testing.B, pool *Pool, buffer []byte, count int) {
	b.Helper()

	class, ok := pool.table.classForCapacity(SizeFromBytes(uint64(cap(buffer))))
	if !ok {
		b.Fatalf("capacity %d is not supported by benchmark policy", cap(buffer))
	}
	state := pool.mustClassStateFor(class)
	for index := 0; index < count; index++ {
		result := state.tryRetain(0, buffer[:0])
		if !result.Retained() {
			b.Fatalf("seed retain %d failed: %#v", index, result)
		}
	}
}

func benchmarkSeedClassRetainedBuffers(b *testing.B, pool *Pool, classID ClassID, capacity int, count int) {
	b.Helper()

	class, ok := pool.table.classByID(classID)
	if !ok {
		b.Fatalf("class %s is not supported by benchmark policy", classID)
	}
	state := pool.mustClassStateFor(class)
	for index := 0; index < count; index++ {
		result := state.tryRetain(0, make([]byte, 0, capacity))
		if !result.Retained() {
			b.Fatalf("seed class retain %d failed: %#v", index, result)
		}
	}
}

// BenchmarkPressureAdmission measures critical-pressure return admission.
func BenchmarkPressureAdmission(b *testing.B) {
	policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{
		capacity:    512,
		shards:      1,
		selector:    ShardSelectionModeSingle,
		bucketSlots: 128,
	})
	policy.Pressure = poolTestPressurePolicy()
	pool := poolBenchmarkNewPool(b, policy)
	if err := pool.applyPressure(PressureSignal{Level: PressureLevelCritical, Source: PressureSourceManual, Generation: Generation(10)}); err != nil {
		b.Fatalf("applyPressure() error = %v", err)
	}
	buffer := make([]byte, 0, 512)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := pool.Put(buffer); err != nil {
			b.Fatalf("Put() error = %v", err)
		}
	}
}

// BenchmarkPressureAdmissionLevels measures the pressure-only admission check
// for normal, high, and critical runtime signals.
func BenchmarkPressureAdmissionLevels(b *testing.B) {
	policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{
		capacity:    512,
		shards:      1,
		selector:    ShardSelectionModeSingle,
		bucketSlots: 128,
	})
	policy.Pressure = poolTestPressurePolicy()
	input := poolReturnInput{Buffer: make([]byte, 0, 512), Capacity: 512}

	for _, signal := range []PressureSignal{
		{Level: PressureLevelNormal, Source: PressureSourceManual, Generation: Generation(1)},
		{Level: PressureLevelHigh, Source: PressureSourceManual, Generation: Generation(2)},
		{Level: PressureLevelCritical, Source: PressureSourceManual, Generation: Generation(3)},
	} {
		signal := signal

		b.Run(signal.Level.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				outcome, handled := poolPressureReturnOutcome(input, policy, signal)
				if outcome.Retained {
					b.Fatal("pressure admission retained a buffer")
				}
				if handled {
					poolBenchmarkIntSink++
				}
			}
		})
	}
}

// BenchmarkPolicyContraction measures foreground group policy shrink publication.
func BenchmarkPolicyContraction(b *testing.B) {
	group, err := NewPoolGroup(testManagedGroupConfig("api", "worker"))
	if err != nil {
		b.Fatalf("NewPoolGroup() error = %v", err)
	}
	b.Cleanup(func() { _ = group.Close() })
	policies := []PoolGroupPolicy{
		DefaultPoolGroupPolicy(),
		DefaultPoolGroupPolicy(),
	}
	policies[0].Budget.MaxRetainedBytes = SizeFromBytes(512)
	policies[1].Budget.MaxRetainedBytes = SizeFromBytes(1024)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := group.UpdatePolicy(policies[i&1]); err != nil {
			b.Fatalf("UpdatePolicy() error = %v", err)
		}
	}
}
