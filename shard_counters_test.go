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

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestShardCountersZeroValueSnapshot verifies zero-value counter behavior.
//
// shardCounters should be embeddable directly into shard runtime state without
// explicit initialization. The zero value represents no observed workload and no
// current retained storage.
func TestShardCountersZeroValueSnapshot(t *testing.T) {
	t.Parallel()

	var counters shardCounters

	snapshot := counters.snapshot()

	if !snapshot.IsZero() {
		t.Fatalf("zero-value shardCounters snapshot = %+v, want zero snapshot", snapshot)
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

// TestShardCountersRecordGetMissAllocationPutDrop verifies the non-retention
// workload path.
//
// This covers a get routed to the shard, a reuse miss, a new allocation, a put
// routed back to the shard, and a drop outcome.
func TestShardCountersRecordGetMissAllocationPutDrop(t *testing.T) {
	t.Parallel()

	var counters shardCounters

	counters.recordGet(128)
	counters.recordMiss()
	counters.recordAllocation(256)
	counters.recordPut(256)
	counters.recordDrop(256)

	snapshot := counters.snapshot()

	if snapshot.Gets != 1 {
		t.Fatalf("Gets = %d, want 1", snapshot.Gets)
	}

	if snapshot.RequestedBytes != 128 {
		t.Fatalf("RequestedBytes = %d, want 128", snapshot.RequestedBytes)
	}

	if snapshot.Misses != 1 {
		t.Fatalf("Misses = %d, want 1", snapshot.Misses)
	}

	if snapshot.Allocations != 1 {
		t.Fatalf("Allocations = %d, want 1", snapshot.Allocations)
	}

	if snapshot.AllocatedBytes != 256 {
		t.Fatalf("AllocatedBytes = %d, want 256", snapshot.AllocatedBytes)
	}

	if snapshot.Puts != 1 {
		t.Fatalf("Puts = %d, want 1", snapshot.Puts)
	}

	if snapshot.ReturnedBytes != 256 {
		t.Fatalf("ReturnedBytes = %d, want 256", snapshot.ReturnedBytes)
	}

	if snapshot.Drops != 1 {
		t.Fatalf("Drops = %d, want 1", snapshot.Drops)
	}

	if snapshot.DroppedBytes != 256 {
		t.Fatalf("DroppedBytes = %d, want 256", snapshot.DroppedBytes)
	}

	if snapshot.CurrentRetainedBuffers != 0 {
		t.Fatalf("CurrentRetainedBuffers = %d, want 0", snapshot.CurrentRetainedBuffers)
	}

	if snapshot.CurrentRetainedBytes != 0 {
		t.Fatalf("CurrentRetainedBytes = %d, want 0", snapshot.CurrentRetainedBytes)
	}

	if snapshot.ReuseAttempts() != 1 {
		t.Fatalf("ReuseAttempts() = %d, want 1", snapshot.ReuseAttempts())
	}

	if snapshot.PutOutcomes() != 1 {
		t.Fatalf("PutOutcomes() = %d, want 1", snapshot.PutOutcomes())
	}
}

// TestShardCountersRecordRetainThenHit verifies retained-storage accounting.
//
// Retaining a returned buffer should increase current retained gauges. Reusing
// that retained buffer as a hit should decrease the same gauges back to zero.
func TestShardCountersRecordRetainThenHit(t *testing.T) {
	t.Parallel()

	var counters shardCounters

	counters.recordPut(512)
	counters.recordRetain(512)

	afterRetain := counters.snapshot()

	if afterRetain.Retains != 1 {
		t.Fatalf("Retains after retain = %d, want 1", afterRetain.Retains)
	}

	if afterRetain.RetainedBytes != 512 {
		t.Fatalf("RetainedBytes after retain = %d, want 512", afterRetain.RetainedBytes)
	}

	if afterRetain.CurrentRetainedBuffers != 1 {
		t.Fatalf("CurrentRetainedBuffers after retain = %d, want 1", afterRetain.CurrentRetainedBuffers)
	}

	if afterRetain.CurrentRetainedBytes != 512 {
		t.Fatalf("CurrentRetainedBytes after retain = %d, want 512", afterRetain.CurrentRetainedBytes)
	}

	counters.recordGet(128)
	counters.recordHit(512)

	afterHit := counters.snapshot()

	if afterHit.Gets != 1 {
		t.Fatalf("Gets after hit = %d, want 1", afterHit.Gets)
	}

	if afterHit.RequestedBytes != 128 {
		t.Fatalf("RequestedBytes after hit = %d, want 128", afterHit.RequestedBytes)
	}

	if afterHit.Hits != 1 {
		t.Fatalf("Hits after hit = %d, want 1", afterHit.Hits)
	}

	if afterHit.HitBytes != 512 {
		t.Fatalf("HitBytes after hit = %d, want 512", afterHit.HitBytes)
	}

	if afterHit.CurrentRetainedBuffers != 0 {
		t.Fatalf("CurrentRetainedBuffers after hit = %d, want 0", afterHit.CurrentRetainedBuffers)
	}

	if afterHit.CurrentRetainedBytes != 0 {
		t.Fatalf("CurrentRetainedBytes after hit = %d, want 0", afterHit.CurrentRetainedBytes)
	}

	if afterHit.ReuseAttempts() != 1 {
		t.Fatalf("ReuseAttempts() after hit = %d, want 1", afterHit.ReuseAttempts())
	}

	if afterHit.PutOutcomes() != 1 {
		t.Fatalf("PutOutcomes() after hit = %d, want 1", afterHit.PutOutcomes())
	}
}

// TestShardCountersRecordTrim verifies trim accounting.
//
// Trim operations are counted even when they remove nothing. Removed buffers and
// bytes should update only when the bucket-level trim result reports removal.
func TestShardCountersRecordTrim(t *testing.T) {
	t.Parallel()

	var counters shardCounters

	counters.recordRetain(64)
	counters.recordRetain(128)
	counters.recordRetain(256)

	counters.recordTrim(bucketTrimResult{})

	afterZeroTrim := counters.snapshot()

	if afterZeroTrim.TrimOperations != 1 {
		t.Fatalf("TrimOperations after zero trim = %d, want 1", afterZeroTrim.TrimOperations)
	}

	if afterZeroTrim.TrimmedBuffers != 0 {
		t.Fatalf("TrimmedBuffers after zero trim = %d, want 0", afterZeroTrim.TrimmedBuffers)
	}

	if afterZeroTrim.TrimmedBytes != 0 {
		t.Fatalf("TrimmedBytes after zero trim = %d, want 0", afterZeroTrim.TrimmedBytes)
	}

	if afterZeroTrim.CurrentRetainedBuffers != 3 {
		t.Fatalf("CurrentRetainedBuffers after zero trim = %d, want 3", afterZeroTrim.CurrentRetainedBuffers)
	}

	if afterZeroTrim.CurrentRetainedBytes != 448 {
		t.Fatalf("CurrentRetainedBytes after zero trim = %d, want 448", afterZeroTrim.CurrentRetainedBytes)
	}

	counters.recordTrim(bucketTrimResult{
		RemovedBuffers: 2,
		RemovedBytes:   384,
	})

	afterTrim := counters.snapshot()

	if afterTrim.TrimOperations != 2 {
		t.Fatalf("TrimOperations after non-zero trim = %d, want 2", afterTrim.TrimOperations)
	}

	if afterTrim.TrimmedBuffers != 2 {
		t.Fatalf("TrimmedBuffers after non-zero trim = %d, want 2", afterTrim.TrimmedBuffers)
	}

	if afterTrim.TrimmedBytes != 384 {
		t.Fatalf("TrimmedBytes after non-zero trim = %d, want 384", afterTrim.TrimmedBytes)
	}

	if afterTrim.CurrentRetainedBuffers != 1 {
		t.Fatalf("CurrentRetainedBuffers after non-zero trim = %d, want 1", afterTrim.CurrentRetainedBuffers)
	}

	if afterTrim.CurrentRetainedBytes != 64 {
		t.Fatalf("CurrentRetainedBytes after non-zero trim = %d, want 64", afterTrim.CurrentRetainedBytes)
	}

	if afterTrim.RemovalOperations() != 2 {
		t.Fatalf("RemovalOperations() = %d, want 2", afterTrim.RemovalOperations())
	}

	if afterTrim.RemovedBuffers() != 2 {
		t.Fatalf("RemovedBuffers() = %d, want 2", afterTrim.RemovedBuffers())
	}

	if afterTrim.RemovedBytes() != 384 {
		t.Fatalf("RemovedBytes() = %d, want 384", afterTrim.RemovedBytes())
	}
}

// TestShardCountersRecordClear verifies clear accounting.
//
// Clear operations are tracked separately from trim operations so hard cleanup
// does not get mixed with adaptive trimming.
func TestShardCountersRecordClear(t *testing.T) {
	t.Parallel()

	var counters shardCounters

	counters.recordRetain(32)
	counters.recordRetain(64)

	counters.recordClear(bucketTrimResult{})

	afterZeroClear := counters.snapshot()

	if afterZeroClear.ClearOperations != 1 {
		t.Fatalf("ClearOperations after zero clear = %d, want 1", afterZeroClear.ClearOperations)
	}

	if afterZeroClear.ClearedBuffers != 0 {
		t.Fatalf("ClearedBuffers after zero clear = %d, want 0", afterZeroClear.ClearedBuffers)
	}

	if afterZeroClear.ClearedBytes != 0 {
		t.Fatalf("ClearedBytes after zero clear = %d, want 0", afterZeroClear.ClearedBytes)
	}

	if afterZeroClear.CurrentRetainedBuffers != 2 {
		t.Fatalf("CurrentRetainedBuffers after zero clear = %d, want 2", afterZeroClear.CurrentRetainedBuffers)
	}

	if afterZeroClear.CurrentRetainedBytes != 96 {
		t.Fatalf("CurrentRetainedBytes after zero clear = %d, want 96", afterZeroClear.CurrentRetainedBytes)
	}

	counters.recordClear(bucketTrimResult{
		RemovedBuffers: 2,
		RemovedBytes:   96,
	})

	afterClear := counters.snapshot()

	if afterClear.ClearOperations != 2 {
		t.Fatalf("ClearOperations after non-zero clear = %d, want 2", afterClear.ClearOperations)
	}

	if afterClear.ClearedBuffers != 2 {
		t.Fatalf("ClearedBuffers after non-zero clear = %d, want 2", afterClear.ClearedBuffers)
	}

	if afterClear.ClearedBytes != 96 {
		t.Fatalf("ClearedBytes after non-zero clear = %d, want 96", afterClear.ClearedBytes)
	}

	if afterClear.CurrentRetainedBuffers != 0 {
		t.Fatalf("CurrentRetainedBuffers after non-zero clear = %d, want 0", afterClear.CurrentRetainedBuffers)
	}

	if afterClear.CurrentRetainedBytes != 0 {
		t.Fatalf("CurrentRetainedBytes after non-zero clear = %d, want 0", afterClear.CurrentRetainedBytes)
	}

	if afterClear.RemovalOperations() != 2 {
		t.Fatalf("RemovalOperations() = %d, want 2", afterClear.RemovalOperations())
	}

	if afterClear.RemovedBuffers() != 2 {
		t.Fatalf("RemovedBuffers() = %d, want 2", afterClear.RemovedBuffers())
	}

	if afterClear.RemovedBytes() != 96 {
		t.Fatalf("RemovedBytes() = %d, want 96", afterClear.RemovedBytes())
	}
}

// TestShardCountersUnderflowPanics verifies retained gauge protection.
//
// recordHit, recordTrim, and recordClear remove retained storage from current
// gauges. If the shard records removal without prior retained accounting, the
// underlying gauge must panic instead of silently wrapping.
func TestShardCountersUnderflowPanics(t *testing.T) {
	t.Parallel()

	t.Run("hit without retained buffer", func(t *testing.T) {
		t.Parallel()

		var counters shardCounters

		testutil.MustPanic(t, func() {
			counters.recordHit(64)
		})
	})

	t.Run("trim without retained buffers", func(t *testing.T) {
		t.Parallel()

		var counters shardCounters

		testutil.MustPanic(t, func() {
			counters.recordTrim(bucketTrimResult{
				RemovedBuffers: 1,
				RemovedBytes:   64,
			})
		})
	})

	t.Run("clear without retained buffers", func(t *testing.T) {
		t.Parallel()

		var counters shardCounters

		testutil.MustPanic(t, func() {
			counters.recordClear(bucketTrimResult{
				RemovedBuffers: 1,
				RemovedBytes:   64,
			})
		})
	})
}

// TestShardCountersSnapshotPredicates verifies snapshot helper methods.
//
// These helpers are used by future workload-window, scoring, pressure, and
// metrics aggregation code to avoid duplicating derived counter expressions.
func TestShardCountersSnapshotPredicates(t *testing.T) {
	t.Parallel()

	snapshot := shardCountersSnapshot{
		Gets:           10,
		RequestedBytes: 1024,

		Hits:     6,
		HitBytes: 768,
		Misses:   4,

		Allocations:    4,
		AllocatedBytes: 512,

		Puts:          8,
		ReturnedBytes: 896,

		Retains:       5,
		RetainedBytes: 640,

		Drops:        3,
		DroppedBytes: 256,

		TrimOperations: 2,
		TrimmedBuffers: 3,
		TrimmedBytes:   384,

		ClearOperations: 1,
		ClearedBuffers:  2,
		ClearedBytes:    128,

		CurrentRetainedBuffers: 7,
		CurrentRetainedBytes:   512,
	}

	if snapshot.IsZero() {
		t.Fatal("IsZero() = true, want false")
	}

	if got := snapshot.ReuseAttempts(); got != 10 {
		t.Fatalf("ReuseAttempts() = %d, want 10", got)
	}

	if got := snapshot.PutOutcomes(); got != 8 {
		t.Fatalf("PutOutcomes() = %d, want 8", got)
	}

	if got := snapshot.RemovalOperations(); got != 3 {
		t.Fatalf("RemovalOperations() = %d, want 3", got)
	}

	if got := snapshot.RemovedBuffers(); got != 5 {
		t.Fatalf("RemovedBuffers() = %d, want 5", got)
	}

	if got := snapshot.RemovedBytes(); got != 512 {
		t.Fatalf("RemovedBytes() = %d, want 512", got)
	}
}

// TestShardCountersSnapshotDeltaSince verifies ordinary delta calculation.
//
// Lifetime counters should become deltas. Current retained gauges should be
// copied from the newer snapshot because gauges are current state, not lifetime
// activity.
func TestShardCountersSnapshotDeltaSince(t *testing.T) {
	t.Parallel()

	previous := shardCountersSnapshot{
		Gets:           10,
		RequestedBytes: 1000,
		Hits:           6,
		HitBytes:       600,
		Misses:         4,

		Allocations:    4,
		AllocatedBytes: 800,

		Puts:          9,
		ReturnedBytes: 900,
		Retains:       5,
		RetainedBytes: 500,
		Drops:         4,
		DroppedBytes:  400,

		TrimOperations: 1,
		TrimmedBuffers: 2,
		TrimmedBytes:   200,

		ClearOperations: 1,
		ClearedBuffers:  1,
		ClearedBytes:    100,

		CurrentRetainedBuffers: 4,
		CurrentRetainedBytes:   256,
	}

	current := shardCountersSnapshot{
		Gets:           18,
		RequestedBytes: 1800,
		Hits:           11,
		HitBytes:       1200,
		Misses:         7,

		Allocations:    7,
		AllocatedBytes: 1400,

		Puts:          15,
		ReturnedBytes: 1600,
		Retains:       9,
		RetainedBytes: 1100,
		Drops:         6,
		DroppedBytes:  500,

		TrimOperations: 3,
		TrimmedBuffers: 5,
		TrimmedBytes:   700,

		ClearOperations: 2,
		ClearedBuffers:  4,
		ClearedBytes:    300,

		CurrentRetainedBuffers: 6,
		CurrentRetainedBytes:   384,
	}

	delta := current.deltaSince(previous)

	if delta.Gets != 8 {
		t.Fatalf("delta.Gets = %d, want 8", delta.Gets)
	}

	if delta.RequestedBytes != 800 {
		t.Fatalf("delta.RequestedBytes = %d, want 800", delta.RequestedBytes)
	}

	if delta.Hits != 5 {
		t.Fatalf("delta.Hits = %d, want 5", delta.Hits)
	}

	if delta.HitBytes != 600 {
		t.Fatalf("delta.HitBytes = %d, want 600", delta.HitBytes)
	}

	if delta.Misses != 3 {
		t.Fatalf("delta.Misses = %d, want 3", delta.Misses)
	}

	if delta.Allocations != 3 {
		t.Fatalf("delta.Allocations = %d, want 3", delta.Allocations)
	}

	if delta.AllocatedBytes != 600 {
		t.Fatalf("delta.AllocatedBytes = %d, want 600", delta.AllocatedBytes)
	}

	if delta.Puts != 6 {
		t.Fatalf("delta.Puts = %d, want 6", delta.Puts)
	}

	if delta.ReturnedBytes != 700 {
		t.Fatalf("delta.ReturnedBytes = %d, want 700", delta.ReturnedBytes)
	}

	if delta.Retains != 4 {
		t.Fatalf("delta.Retains = %d, want 4", delta.Retains)
	}

	if delta.RetainedBytes != 600 {
		t.Fatalf("delta.RetainedBytes = %d, want 600", delta.RetainedBytes)
	}

	if delta.Drops != 2 {
		t.Fatalf("delta.Drops = %d, want 2", delta.Drops)
	}

	if delta.DroppedBytes != 100 {
		t.Fatalf("delta.DroppedBytes = %d, want 100", delta.DroppedBytes)
	}

	if delta.TrimOperations != 2 {
		t.Fatalf("delta.TrimOperations = %d, want 2", delta.TrimOperations)
	}

	if delta.TrimmedBuffers != 3 {
		t.Fatalf("delta.TrimmedBuffers = %d, want 3", delta.TrimmedBuffers)
	}

	if delta.TrimmedBytes != 500 {
		t.Fatalf("delta.TrimmedBytes = %d, want 500", delta.TrimmedBytes)
	}

	if delta.ClearOperations != 1 {
		t.Fatalf("delta.ClearOperations = %d, want 1", delta.ClearOperations)
	}

	if delta.ClearedBuffers != 3 {
		t.Fatalf("delta.ClearedBuffers = %d, want 3", delta.ClearedBuffers)
	}

	if delta.ClearedBytes != 200 {
		t.Fatalf("delta.ClearedBytes = %d, want 200", delta.ClearedBytes)
	}

	if delta.CurrentRetainedBuffers != current.CurrentRetainedBuffers {
		t.Fatalf("delta.CurrentRetainedBuffers = %d, want %d", delta.CurrentRetainedBuffers, current.CurrentRetainedBuffers)
	}

	if delta.CurrentRetainedBytes != current.CurrentRetainedBytes {
		t.Fatalf("delta.CurrentRetainedBytes = %d, want %d", delta.CurrentRetainedBytes, current.CurrentRetainedBytes)
	}

	if got := delta.ReuseAttempts(); got != 8 {
		t.Fatalf("delta.ReuseAttempts() = %d, want 8", got)
	}

	if got := delta.PutOutcomes(); got != 6 {
		t.Fatalf("delta.PutOutcomes() = %d, want 6", got)
	}

	if got := delta.RemovalOperations(); got != 3 {
		t.Fatalf("delta.RemovalOperations() = %d, want 3", got)
	}

	if got := delta.RemovedBuffers(); got != 6 {
		t.Fatalf("delta.RemovedBuffers() = %d, want 6", got)
	}

	if got := delta.RemovedBytes(); got != 700 {
		t.Fatalf("delta.RemovedBytes() = %d, want 700", got)
	}
}

// TestShardCountersSnapshotDeltaSinceWrapAware verifies wrap-aware monotonic
// counter arithmetic.
//
// Lifetime counters use uint64 modulo arithmetic through shardCounterDelta.
// Gauges are still copied from the newer snapshot.
func TestShardCountersSnapshotDeltaSinceWrapAware(t *testing.T) {
	t.Parallel()

	previous := shardCountersSnapshot{
		Gets:                   ^uint64(0) - 2,
		CurrentRetainedBuffers: 9,
		CurrentRetainedBytes:   900,
	}

	current := shardCountersSnapshot{
		Gets:                   4,
		CurrentRetainedBuffers: 1,
		CurrentRetainedBytes:   64,
	}

	delta := current.deltaSince(previous)

	if delta.Gets != 7 {
		t.Fatalf("wrap-aware delta.Gets = %d, want 7", delta.Gets)
	}

	if delta.CurrentRetainedBuffers != 1 {
		t.Fatalf("delta.CurrentRetainedBuffers = %d, want 1", delta.CurrentRetainedBuffers)
	}

	if delta.CurrentRetainedBytes != 64 {
		t.Fatalf("delta.CurrentRetainedBytes = %d, want 64", delta.CurrentRetainedBytes)
	}
}

// TestShardCountersDeltaPredicates verifies delta helper methods.
func TestShardCountersDeltaPredicates(t *testing.T) {
	t.Parallel()

	delta := shardCountersDelta{
		Hits:                 3,
		Misses:               2,
		Retains:              4,
		Drops:                1,
		TrimOperations:       2,
		ClearOperations:      1,
		TrimmedBuffers:       5,
		ClearedBuffers:       6,
		TrimmedBytes:         700,
		ClearedBytes:         300,
		CurrentRetainedBytes: 64,
	}

	if delta.IsZero() {
		t.Fatal("IsZero() = true, want false")
	}

	if got := delta.ReuseAttempts(); got != 5 {
		t.Fatalf("ReuseAttempts() = %d, want 5", got)
	}

	if got := delta.PutOutcomes(); got != 5 {
		t.Fatalf("PutOutcomes() = %d, want 5", got)
	}

	if got := delta.RemovalOperations(); got != 3 {
		t.Fatalf("RemovalOperations() = %d, want 3", got)
	}

	if got := delta.RemovedBuffers(); got != 11 {
		t.Fatalf("RemovedBuffers() = %d, want 11", got)
	}

	if got := delta.RemovedBytes(); got != 1000 {
		t.Fatalf("RemovedBytes() = %d, want 1000", got)
	}
}

// TestShardCountersDeltaZero verifies zero delta classification.
func TestShardCountersDeltaZero(t *testing.T) {
	t.Parallel()

	var delta shardCountersDelta

	if !delta.IsZero() {
		t.Fatalf("zero shardCountersDelta = %+v, want IsZero true", delta)
	}
}

// TestShardCounterDelta verifies direct wrap-aware helper behavior.
func TestShardCounterDelta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		previous uint64
		current  uint64
		want     uint64
	}{
		{
			name:     "ordinary increase",
			previous: 10,
			current:  15,
			want:     5,
		},
		{
			name:     "same value",
			previous: 42,
			current:  42,
			want:     0,
		},
		{
			name:     "single wrap",
			previous: ^uint64(0) - 2,
			current:  4,
			want:     7,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := shardCounterDelta(tt.previous, tt.current)
			if got != tt.want {
				t.Fatalf("shardCounterDelta(%d, %d) = %d, want %d", tt.previous, tt.current, got, tt.want)
			}
		})
	}
}

// TestShardCountersConcurrentRecording verifies atomic correctness under
// concurrent shard-local accounting.
//
// This test does not prove snapshot-wide linearizability. It verifies that
// concurrent record methods do not lose individual atomic updates and that
// retained gauges remain consistent when every retain is later matched by a hit.
func TestShardCountersConcurrentRecording(t *testing.T) {
	t.Parallel()

	const goroutines = 16
	const iterations = 1024

	var counters shardCounters
	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				counters.recordGet(1)
				counters.recordMiss()
				counters.recordAllocation(8)
				counters.recordPut(8)
				counters.recordRetain(8)
			}
		}()
	}

	wg.Wait()

	afterRetains := counters.snapshot()

	wantEvents := uint64(goroutines * iterations)
	wantBytes := wantEvents * 8

	if afterRetains.Gets != wantEvents {
		t.Fatalf("Gets after concurrent recording = %d, want %d", afterRetains.Gets, wantEvents)
	}

	if afterRetains.RequestedBytes != wantEvents {
		t.Fatalf("RequestedBytes after concurrent recording = %d, want %d", afterRetains.RequestedBytes, wantEvents)
	}

	if afterRetains.Misses != wantEvents {
		t.Fatalf("Misses after concurrent recording = %d, want %d", afterRetains.Misses, wantEvents)
	}

	if afterRetains.Allocations != wantEvents {
		t.Fatalf("Allocations after concurrent recording = %d, want %d", afterRetains.Allocations, wantEvents)
	}

	if afterRetains.AllocatedBytes != wantBytes {
		t.Fatalf("AllocatedBytes after concurrent recording = %d, want %d", afterRetains.AllocatedBytes, wantBytes)
	}

	if afterRetains.Puts != wantEvents {
		t.Fatalf("Puts after concurrent recording = %d, want %d", afterRetains.Puts, wantEvents)
	}

	if afterRetains.Retains != wantEvents {
		t.Fatalf("Retains after concurrent recording = %d, want %d", afterRetains.Retains, wantEvents)
	}

	if afterRetains.CurrentRetainedBuffers != wantEvents {
		t.Fatalf("CurrentRetainedBuffers after concurrent recording = %d, want %d", afterRetains.CurrentRetainedBuffers, wantEvents)
	}

	if afterRetains.CurrentRetainedBytes != wantBytes {
		t.Fatalf("CurrentRetainedBytes after concurrent recording = %d, want %d", afterRetains.CurrentRetainedBytes, wantBytes)
	}

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				counters.recordHit(8)
			}
		}()
	}

	wg.Wait()

	afterHits := counters.snapshot()

	if afterHits.Hits != wantEvents {
		t.Fatalf("Hits after concurrent hits = %d, want %d", afterHits.Hits, wantEvents)
	}

	if afterHits.HitBytes != wantBytes {
		t.Fatalf("HitBytes after concurrent hits = %d, want %d", afterHits.HitBytes, wantBytes)
	}

	if afterHits.CurrentRetainedBuffers != 0 {
		t.Fatalf("CurrentRetainedBuffers after concurrent hits = %d, want 0", afterHits.CurrentRetainedBuffers)
	}

	if afterHits.CurrentRetainedBytes != 0 {
		t.Fatalf("CurrentRetainedBytes after concurrent hits = %d, want 0", afterHits.CurrentRetainedBytes)
	}
}
