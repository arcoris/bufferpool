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
	"sync"
	"testing"
)

// This file contains lower-bound references, not semantic competitors.
//
// Direct make, sync.Pool, and raw mutex bucket benchmarks omit most Pool
// responsibilities: lifecycle admission, class-table routing, bounded retention,
// policy-driven admission, owner-side counters, snapshots, metrics, and close
// behavior. They are useful only for understanding the overhead envelope around
// the full Pool data plane.

// BenchmarkPoolBaselineMake measures direct allocation cost for common request
// and class-capacity pairs.
func BenchmarkPoolBaselineMake(b *testing.B) {
	cases := []struct {
		name     string
		size     int
		capacity int
	}{
		{name: "size_300/cap_512", size: 300, capacity: 512},
		{name: "size_900/cap_1024", size: 900, capacity: 1024},
		{name: "size_4096/cap_4096", size: 4096, capacity: 4096},
	}

	for _, tc := range cases {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				poolBenchmarkBufferSink = make([]byte, tc.size, tc.capacity)
			}
		})
	}
}

// BenchmarkPoolBaselineSyncPool measures a minimal sync.Pool []byte loop.
//
// sync.Pool is intentionally not equivalent to Pool: it is unbounded from this
// package's perspective, GC may discard entries, there is no class table, no
// deterministic retained storage, no lifecycle, and no observability contract.
func BenchmarkPoolBaselineSyncPool(b *testing.B) {
	cases := []struct {
		name     string
		size     int
		capacity int
		putBack  bool
	}{
		{name: "get_put_512", size: 300, capacity: 512, putBack: true},
		{name: "get_put_1024", size: 900, capacity: 1024, putBack: true},
		{name: "miss_new_512", size: 300, capacity: 512, putBack: false},
	}

	for _, tc := range cases {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			sp := sync.Pool{
				New: func() any {
					return make([]byte, 0, tc.capacity)
				},
			}
			if tc.putBack {
				sp.Put(make([]byte, 0, tc.capacity))
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				buffer := sp.Get().([]byte)
				if cap(buffer) < tc.capacity {
					buffer = make([]byte, 0, tc.capacity)
				}

				buffer = buffer[:tc.size]
				poolBenchmarkBufferSink = buffer
				if tc.putBack {
					sp.Put(buffer[:0])
				}
			}
		})
	}
}

// BenchmarkPoolBaselineRawMutexBucket measures the physical LIFO storage cost
// beneath shard without policy, counters, lifecycle, class routing, or public
// admission.
func BenchmarkPoolBaselineRawMutexBucket(b *testing.B) {
	cases := []struct {
		name     string
		capacity int
	}{
		{name: "pop_push_512", capacity: 512},
		{name: "pop_push_1024", capacity: 1024},
		{name: "pop_push_4096", capacity: 4096},
	}

	for _, tc := range cases {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			var mu sync.Mutex
			bucket := newBucket(1)
			if !bucket.push(make([]byte, 0, tc.capacity)) {
				b.Fatal("seed bucket push rejected")
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				mu.Lock()
				buffer, ok := bucket.pop()
				if !ok {
					mu.Unlock()
					b.Fatal("bucket pop missed")
				}
				if !bucket.push(buffer) {
					mu.Unlock()
					b.Fatal("bucket push rejected")
				}
				mu.Unlock()

				poolBenchmarkBufferSink = buffer
			}
		})
	}
}
