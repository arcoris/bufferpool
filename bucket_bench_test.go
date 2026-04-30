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
	"testing"
)

// BenchmarkBucketPushPopSegmented measures the raw segmented bucket LIFO loop.
//
// The benchmark intentionally pushes into an empty bucket and pops back to
// empty. That exposes the metadata allocation cost of the fully lazy model. Pool
// benchmarks below cover the ordinary steady-state data-plane loop where bucket
// metadata is usually already present through retained traffic.
func BenchmarkBucketPushPopSegmented(b *testing.B) {
	buffer := make([]byte, 0, 1024)
	bucket := newBucket(64, 8)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if !bucket.push(buffer) {
			b.Fatal("bucket push rejected")
		}

		popped, ok := bucket.pop()
		if !ok {
			b.Fatal("bucket pop missed")
		}

		poolBenchmarkBufferSink = popped
	}
}

// BenchmarkBucketPushPopSegmentSizes compares steady push/pop cost across
// supported lazy segment sizes.
func BenchmarkBucketPushPopSegmentSizes(b *testing.B) {
	for _, segmentSlots := range []int{4, 8, 16, 32} {
		segmentSlots := segmentSlots

		b.Run("segment_slots_"+strconv.Itoa(segmentSlots), func(b *testing.B) {
			buffer := make([]byte, 0, 1024)
			bucket := newBucket(64, segmentSlots)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if !bucket.push(buffer) {
					b.Fatal("bucket push rejected")
				}

				popped, ok := bucket.pop()
				if !ok {
					b.Fatal("bucket pop missed")
				}

				poolBenchmarkBufferSink = popped
			}
		})
	}
}

// BenchmarkBucketPushPopFixedEquivalent is a lower-bound reference for the old
// eager fixed-slot shape.
//
// It does not implement full bucket invariants, retained-byte accounting, trim,
// or corruption checks. It exists only to keep segmented metadata overhead
// visible while the production bucket uses lazy segments.
func BenchmarkBucketPushPopFixedEquivalent(b *testing.B) {
	buffer := make([]byte, 0, 1024)
	slots := make([][]byte, 64)
	var count int

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		slots[count] = buffer
		count++

		count--
		popped := slots[count]
		slots[count] = nil

		poolBenchmarkBufferSink = popped
	}
}

// BenchmarkBucketTrimSegmented measures bounded LIFO trim over segmented
// metadata.
//
// Setup is outside the timed region for each trim batch so the benchmark reports
// trim work, slot clearing, and segment release rather than seed allocation.
func BenchmarkBucketTrimSegmented(b *testing.B) {
	const (
		slotLimit    = 64
		segmentSlots = 8
		capacity     = 1024
	)

	b.ReportAllocs()
	b.StopTimer()

	for i := 0; i < b.N; i++ {
		bucket := newBucket(slotLimit, segmentSlots)
		for index := 0; index < slotLimit; index++ {
			if !bucket.push(make([]byte, 0, capacity)) {
				b.Fatal("bucket seed push rejected")
			}
		}

		b.StartTimer()
		result := bucket.trim(slotLimit)
		b.StopTimer()

		if result.RemovedBuffers != slotLimit {
			b.Fatalf("trim removed %d buffers, want %d", result.RemovedBuffers, slotLimit)
		}
		poolBenchmarkIntSink = result.AvailableSlots
	}
}

// BenchmarkPoolGetPutSegmentedBucket measures the full Pool data-plane loop
// with segmented bucket storage enabled.
//
// This benchmark keeps class routing, lifecycle checks, owner admission,
// selector behavior, shard locking, retained accounting, and bucket storage in
// the measured path. It is the semantic companion to raw bucket benchmarks.
func BenchmarkPoolGetPutSegmentedBucket(b *testing.B) {
	tc := poolBenchmarkCase{
		name:         "segmented_bucket",
		size:         900,
		capacity:     1024,
		shards:       4,
		selector:     ShardSelectionModeProcessorInspired,
		bucketSlots:  64,
		retainedSeed: 32,
	}
	pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(tc))
	poolBenchmarkSeedRetainedPublic(b, pool, tc.capacity, tc.retainedSeed)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buffer, err := pool.Get(tc.size)
		if err != nil {
			b.Fatalf("Get() returned error: %v", err)
		}

		if err := pool.Put(buffer); err != nil {
			b.Fatalf("Put() returned error: %v", err)
		}

		poolBenchmarkBufferSink = buffer
	}
}
