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

func TestClassCountersZeroSnapshot(t *testing.T) {
	t.Parallel()

	var counters classCounters
	snapshot := counters.snapshot()

	if !snapshot.IsZero() {
		t.Fatalf("zero snapshot IsZero() = false, snapshot=%+v", snapshot)
	}
	if snapshot.ReuseAttempts() != 0 {
		t.Fatalf("ReuseAttempts() = %d, want 0", snapshot.ReuseAttempts())
	}
	if snapshot.PutOutcomes() != 0 {
		t.Fatalf("PutOutcomes() = %d, want 0", snapshot.PutOutcomes())
	}
	if snapshot.RemovalOperations() != 0 {
		t.Fatalf("RemovalOperations() = %d, want 0", snapshot.RemovalOperations())
	}
	if snapshot.RemovedBuffers() != 0 {
		t.Fatalf("RemovedBuffers() = %d, want 0", snapshot.RemovedBuffers())
	}
	if snapshot.RemovedBytes() != 0 {
		t.Fatalf("RemovedBytes() = %d, want 0", snapshot.RemovedBytes())
	}
}

func TestClassCountersRecordGetResult(t *testing.T) {
	t.Parallel()

	var counters classCounters

	counters.recordGet(Size(64).Bytes())
	counters.recordGetResult(Size(128), shardGetResult{
		Hit:      true,
		Capacity: 512,
	})
	counters.recordGetResult(Size(256), shardGetResult{})

	snapshot := counters.snapshot()

	if snapshot.Gets != 3 {
		t.Fatalf("Gets = %d, want 3", snapshot.Gets)
	}
	if snapshot.RequestedBytes != 448 {
		t.Fatalf("RequestedBytes = %d, want 448", snapshot.RequestedBytes)
	}
	if snapshot.Hits != 1 {
		t.Fatalf("Hits = %d, want 1", snapshot.Hits)
	}
	if snapshot.HitBytes != 512 {
		t.Fatalf("HitBytes = %d, want 512", snapshot.HitBytes)
	}
	if snapshot.Misses != 1 {
		t.Fatalf("Misses = %d, want 1", snapshot.Misses)
	}
	if snapshot.ReuseAttempts() != 2 {
		t.Fatalf("ReuseAttempts() = %d, want 2", snapshot.ReuseAttempts())
	}
}

func TestClassCountersRecordAllocation(t *testing.T) {
	t.Parallel()

	var counters classCounters

	counters.recordAllocation(512)
	counters.recordAllocatedBuffer(make([]byte, 7, 1024))

	snapshot := counters.snapshot()

	if snapshot.Allocations != 2 {
		t.Fatalf("Allocations = %d, want 2", snapshot.Allocations)
	}
	if snapshot.AllocatedBytes != 1536 {
		t.Fatalf("AllocatedBytes = %d, want 1536", snapshot.AllocatedBytes)
	}
}

func TestClassCountersRecordRetainOutcomes(t *testing.T) {
	t.Parallel()

	var counters classCounters

	counters.recordPut(128)
	counters.recordRetainResult(shardRetainResult{
		Capacity:       512,
		Retained:       true,
		CreditDecision: shardCreditAccept,
	})
	counters.recordRetainResult(shardRetainResult{
		Capacity:       1024,
		CreditDecision: shardCreditRejectByteCreditExhausted,
	})
	counters.recordRejectedPut(256)

	snapshot := counters.snapshot()

	if snapshot.Puts != 4 {
		t.Fatalf("Puts = %d, want 4", snapshot.Puts)
	}
	if snapshot.ReturnedBytes != 1920 {
		t.Fatalf("ReturnedBytes = %d, want 1920", snapshot.ReturnedBytes)
	}
	if snapshot.Retains != 1 {
		t.Fatalf("Retains = %d, want 1", snapshot.Retains)
	}
	if snapshot.RetainedBytes != 512 {
		t.Fatalf("RetainedBytes = %d, want 512", snapshot.RetainedBytes)
	}
	if snapshot.Drops != 2 {
		t.Fatalf("Drops = %d, want 2", snapshot.Drops)
	}
	if snapshot.DroppedBytes != 1280 {
		t.Fatalf("DroppedBytes = %d, want 1280", snapshot.DroppedBytes)
	}
	if snapshot.PutOutcomes() != 3 {
		t.Fatalf("PutOutcomes() = %d, want 3", snapshot.PutOutcomes())
	}
}

func TestClassCountersRecordTrimAndClear(t *testing.T) {
	t.Parallel()

	var counters classCounters

	counters.recordTrim(bucketTrimResult{})
	counters.recordTrim(bucketTrimResult{
		RemovedBuffers: 2,
		RemovedBytes:   768,
	})
	counters.recordClear(bucketTrimResult{})
	counters.recordClear(bucketTrimResult{
		RemovedBuffers: 1,
		RemovedBytes:   512,
	})
	counters.recordClearAmount(0, 0)
	counters.recordClearAmount(3, 1536)

	snapshot := counters.snapshot()

	if snapshot.TrimOperations != 2 {
		t.Fatalf("TrimOperations = %d, want 2", snapshot.TrimOperations)
	}
	if snapshot.TrimmedBuffers != 2 {
		t.Fatalf("TrimmedBuffers = %d, want 2", snapshot.TrimmedBuffers)
	}
	if snapshot.TrimmedBytes != 768 {
		t.Fatalf("TrimmedBytes = %d, want 768", snapshot.TrimmedBytes)
	}
	if snapshot.ClearOperations != 4 {
		t.Fatalf("ClearOperations = %d, want 4", snapshot.ClearOperations)
	}
	if snapshot.ClearedBuffers != 4 {
		t.Fatalf("ClearedBuffers = %d, want 4", snapshot.ClearedBuffers)
	}
	if snapshot.ClearedBytes != 2048 {
		t.Fatalf("ClearedBytes = %d, want 2048", snapshot.ClearedBytes)
	}
	if snapshot.RemovalOperations() != 6 {
		t.Fatalf("RemovalOperations() = %d, want 6", snapshot.RemovalOperations())
	}
	if snapshot.RemovedBuffers() != 6 {
		t.Fatalf("RemovedBuffers() = %d, want 6", snapshot.RemovedBuffers())
	}
	if snapshot.RemovedBytes() != 2816 {
		t.Fatalf("RemovedBytes() = %d, want 2816", snapshot.RemovedBytes())
	}
}

func TestClassCountersDeltaSince(t *testing.T) {
	t.Parallel()

	previous := classCountersSnapshot{
		Gets:            10,
		RequestedBytes:  100,
		Hits:            4,
		HitBytes:        80,
		Misses:          2,
		Allocations:     3,
		AllocatedBytes:  96,
		Puts:            7,
		ReturnedBytes:   224,
		Retains:         5,
		RetainedBytes:   160,
		Drops:           1,
		DroppedBytes:    32,
		TrimOperations:  1,
		TrimmedBuffers:  2,
		TrimmedBytes:    64,
		ClearOperations: 3,
		ClearedBuffers:  4,
		ClearedBytes:    128,
	}
	current := classCountersSnapshot{
		Gets:            15,
		RequestedBytes:  180,
		Hits:            8,
		HitBytes:        176,
		Misses:          5,
		Allocations:     5,
		AllocatedBytes:  224,
		Puts:            12,
		ReturnedBytes:   608,
		Retains:         8,
		RetainedBytes:   512,
		Drops:           4,
		DroppedBytes:    96,
		TrimOperations:  2,
		TrimmedBuffers:  3,
		TrimmedBytes:    128,
		ClearOperations: 5,
		ClearedBuffers:  6,
		ClearedBytes:    256,
	}

	delta := current.deltaSince(previous)

	if delta.Gets != 5 || delta.RequestedBytes != 80 {
		t.Fatalf("get delta = (%d, %d), want (5, 80)", delta.Gets, delta.RequestedBytes)
	}
	if delta.Hits != 4 || delta.HitBytes != 96 || delta.Misses != 3 {
		t.Fatalf("reuse delta = hits %d hitBytes %d misses %d, want 4 96 3", delta.Hits, delta.HitBytes, delta.Misses)
	}
	if delta.Allocations != 2 || delta.AllocatedBytes != 128 {
		t.Fatalf("allocation delta = (%d, %d), want (2, 128)", delta.Allocations, delta.AllocatedBytes)
	}
	if delta.Puts != 5 || delta.ReturnedBytes != 384 {
		t.Fatalf("put delta = (%d, %d), want (5, 384)", delta.Puts, delta.ReturnedBytes)
	}
	if delta.Retains != 3 || delta.RetainedBytes != 352 {
		t.Fatalf("retain delta = (%d, %d), want (3, 352)", delta.Retains, delta.RetainedBytes)
	}
	if delta.Drops != 3 || delta.DroppedBytes != 64 {
		t.Fatalf("drop delta = (%d, %d), want (3, 64)", delta.Drops, delta.DroppedBytes)
	}
	if delta.TrimOperations != 1 || delta.TrimmedBuffers != 1 || delta.TrimmedBytes != 64 {
		t.Fatalf("trim delta = (%d, %d, %d), want (1, 1, 64)", delta.TrimOperations, delta.TrimmedBuffers, delta.TrimmedBytes)
	}
	if delta.ClearOperations != 2 || delta.ClearedBuffers != 2 || delta.ClearedBytes != 128 {
		t.Fatalf("clear delta = (%d, %d, %d), want (2, 2, 128)", delta.ClearOperations, delta.ClearedBuffers, delta.ClearedBytes)
	}
	if delta.ReuseAttempts() != 7 {
		t.Fatalf("delta.ReuseAttempts() = %d, want 7", delta.ReuseAttempts())
	}
	if delta.PutOutcomes() != 6 {
		t.Fatalf("delta.PutOutcomes() = %d, want 6", delta.PutOutcomes())
	}
	if delta.RemovalOperations() != 3 {
		t.Fatalf("delta.RemovalOperations() = %d, want 3", delta.RemovalOperations())
	}
	if delta.RemovedBuffers() != 3 {
		t.Fatalf("delta.RemovedBuffers() = %d, want 3", delta.RemovedBuffers())
	}
	if delta.RemovedBytes() != 192 {
		t.Fatalf("delta.RemovedBytes() = %d, want 192", delta.RemovedBytes())
	}
}

func TestClassCountersDeltaSinceHandlesWrap(t *testing.T) {
	t.Parallel()

	previous := classCountersSnapshot{
		Gets:           ^uint64(0) - 1,
		RequestedBytes: ^uint64(0),
	}
	current := classCountersSnapshot{
		Gets:           1,
		RequestedBytes: 2,
	}

	delta := current.deltaSince(previous)

	if delta.Gets != 3 {
		t.Fatalf("wrapped Gets delta = %d, want 3", delta.Gets)
	}
	if delta.RequestedBytes != 3 {
		t.Fatalf("wrapped RequestedBytes delta = %d, want 3", delta.RequestedBytes)
	}
}

func TestClassCountersDeltaIsZero(t *testing.T) {
	t.Parallel()

	var delta classCountersDelta

	if !delta.IsZero() {
		t.Fatalf("zero delta IsZero() = false, delta=%+v", delta)
	}

	delta.Retains = 1
	if delta.IsZero() {
		t.Fatal("non-zero delta IsZero() = true, want false")
	}
}

func TestClassCountersConcurrentRecording(t *testing.T) {
	t.Parallel()

	const goroutines = 8
	const iterations = 256

	var counters classCounters
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				counters.recordGetResult(Size(64), shardGetResult{
					Hit:      true,
					Capacity: 128,
				})
				counters.recordAllocation(256)
				counters.recordRetainResult(shardRetainResult{
					Capacity:       512,
					Retained:       true,
					CreditDecision: shardCreditAccept,
				})
				counters.recordTrim(bucketTrimResult{
					RemovedBuffers: 1,
					RemovedBytes:   512,
				})
			}
		}()
	}

	wg.Wait()

	wantEvents := uint64(goroutines * iterations)
	snapshot := counters.snapshot()

	if snapshot.Gets != wantEvents {
		t.Fatalf("Gets = %d, want %d", snapshot.Gets, wantEvents)
	}
	if snapshot.Hits != wantEvents {
		t.Fatalf("Hits = %d, want %d", snapshot.Hits, wantEvents)
	}
	if snapshot.Allocations != wantEvents {
		t.Fatalf("Allocations = %d, want %d", snapshot.Allocations, wantEvents)
	}
	if snapshot.Retains != wantEvents {
		t.Fatalf("Retains = %d, want %d", snapshot.Retains, wantEvents)
	}
	if snapshot.TrimOperations != wantEvents {
		t.Fatalf("TrimOperations = %d, want %d", snapshot.TrimOperations, wantEvents)
	}
	if snapshot.TrimmedBuffers != wantEvents {
		t.Fatalf("TrimmedBuffers = %d, want %d", snapshot.TrimmedBuffers, wantEvents)
	}
	if snapshot.TrimmedBytes != wantEvents*512 {
		t.Fatalf("TrimmedBytes = %d, want %d", snapshot.TrimmedBytes, wantEvents*512)
	}
}
