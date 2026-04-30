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

// TestPoolPartitionWindowIdenticalSamplesHaveZeroDelta verifies stable samples.
func TestPoolPartitionWindowIdenticalSamplesHaveZeroDelta(t *testing.T) {
	sample := PoolPartitionSample{
		Generation:       Generation(3),
		PolicyGeneration: Generation(7),
		PoolCounters: PoolCountersSnapshot{
			Gets:   10,
			Hits:   6,
			Misses: 4,
		},
		LeaseCounters: LeaseCountersSnapshot{
			Acquisitions: 3,
			Releases:     2,
		},
	}

	window := NewPoolPartitionWindow(sample, sample)

	if !window.Delta.IsZero() {
		t.Fatalf("delta = %+v, want zero", window.Delta)
	}
	if window.Generation != sample.Generation || window.PolicyGeneration != sample.PolicyGeneration {
		t.Fatalf("window generations = %s/%s, want %s/%s", window.Generation, window.PolicyGeneration, sample.Generation, sample.PolicyGeneration)
	}
}

// TestPoolPartitionWindowComputesCounterDeltas verifies pool and lease deltas.
func TestPoolPartitionWindowComputesCounterDeltas(t *testing.T) {
	previous := PoolPartitionSample{
		PoolCounters: PoolCountersSnapshot{
			Gets:          10,
			Hits:          4,
			Misses:        6,
			Allocations:   5,
			Puts:          8,
			ReturnedBytes: 1024,
			Retains:       3,
			RetainedBytes: 512,
			Drops:         5,
			DroppedBytes:  256,
			DropReasons: PoolDropReasonCounters{
				ClosedPool:              1,
				ReturnedBuffersDisabled: 2,
				Oversized:               3,
				UnsupportedClass:        4,
				InvalidPolicy:           5,
			},
		},
		LeaseCounters: LeaseCountersSnapshot{
			Acquisitions:                7,
			Releases:                    5,
			InvalidReleases:             1,
			DoubleReleases:              2,
			OwnershipViolations:         3,
			PoolReturnAttempts:          5,
			PoolReturnSuccesses:         4,
			PoolReturnFailures:          1,
			PoolReturnClosedFailures:    1,
			PoolReturnAdmissionFailures: 0,
		},
	}
	current := PoolPartitionSample{
		Generation:       Generation(11),
		PolicyGeneration: Generation(13),
		PoolCounters: PoolCountersSnapshot{
			Gets:          15,
			Hits:          9,
			Misses:        6,
			Allocations:   8,
			Puts:          12,
			ReturnedBytes: 2048,
			Retains:       7,
			RetainedBytes: 1536,
			Drops:         5,
			DroppedBytes:  640,
			DropReasons: PoolDropReasonCounters{
				ClosedPool:              3,
				ReturnedBuffersDisabled: 4,
				Oversized:               7,
				UnsupportedClass:        4,
				InvalidPolicy:           6,
			},
		},
		LeaseCounters: LeaseCountersSnapshot{
			Acquisitions:                10,
			Releases:                    9,
			InvalidReleases:             2,
			DoubleReleases:              3,
			OwnershipViolations:         6,
			PoolReturnAttempts:          9,
			PoolReturnSuccesses:         7,
			PoolReturnFailures:          2,
			PoolReturnClosedFailures:    2,
			PoolReturnAdmissionFailures: 1,
		},
	}

	window := NewPoolPartitionWindow(previous, current)
	delta := window.Delta

	if delta.Gets != 5 || delta.Hits != 5 || delta.Misses != 0 || delta.Allocations != 3 {
		t.Fatalf("pool get delta = %+v", delta)
	}
	if delta.Puts != 4 || delta.Retains != 4 || delta.Drops != 0 {
		t.Fatalf("pool put delta = %+v", delta)
	}
	if delta.ReturnedBytes != 1024 || delta.RetainedBytes != 1024 || delta.DroppedBytes != 384 {
		t.Fatalf("pool byte delta = %+v", delta)
	}
	if delta.ClosedPoolDrops != 2 || delta.ReturnedBuffersDisabledDrops != 2 || delta.OversizedDrops != 4 {
		t.Fatalf("pool drop reason delta = %+v", delta)
	}
	if delta.UnsupportedClassDrops != 0 || delta.InvalidPolicyDrops != 1 {
		t.Fatalf("pool drop reason delta = %+v", delta)
	}
	if delta.LeaseAcquisitions != 3 || delta.LeaseReleases != 4 || delta.LeaseInvalidReleases != 1 {
		t.Fatalf("lease ownership delta = %+v", delta)
	}
	if delta.LeaseDoubleReleases != 1 || delta.LeaseOwnershipViolations != 3 {
		t.Fatalf("lease violation delta = %+v", delta)
	}
	if delta.LeasePoolReturnAttempts != 4 || delta.LeasePoolReturnSuccesses != 3 || delta.LeasePoolReturnFailures != 1 {
		t.Fatalf("lease pool-return delta = %+v", delta)
	}
	if delta.LeasePoolReturnClosedFailures != 1 || delta.LeasePoolReturnAdmissionFailures != 1 {
		t.Fatalf("lease pool-return reason delta = %+v", delta)
	}
	if window.Generation != current.Generation || window.PolicyGeneration != current.PolicyGeneration {
		t.Fatalf("window generations = %s/%s, want current generations", window.Generation, window.PolicyGeneration)
	}
}

func TestPoolPartitionWindowComputesClassDeltas(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(0), ClassSizeFromSize(KiB))
	previous := PoolPartitionSample{
		Pools: []PoolPartitionPoolSample{
			{
				Name: "primary",
				Classes: []PoolPartitionClassSample{
					{
						Class:   class,
						ClassID: class.ID(),
						Counters: ClassCountersSnapshot{
							Gets:        10,
							Hits:        7,
							Misses:      3,
							Allocations: 3,
							Puts:        8,
							Retains:     6,
							Drops:       2,
						},
					},
				},
			},
		},
	}
	current := clonePoolPartitionSample(previous)
	current.Pools[0].Counters.Gets = 5
	current.Pools[0].Classes[0].Counters.Gets += 5
	current.Pools[0].Classes[0].Counters.Hits += 4
	current.Pools[0].Classes[0].Counters.Misses += 1
	current.Pools[0].Classes[0].Counters.Allocations += 1
	current.Pools[0].Classes[0].Counters.Puts += 4
	current.Pools[0].Classes[0].Counters.Retains += 3
	current.Pools[0].Classes[0].Counters.Drops += 1

	window := NewPoolPartitionWindow(previous, current)
	if len(window.Pools) != 1 || len(window.Pools[0].Classes) != 1 {
		t.Fatalf("window class shape = %+v", window.Pools)
	}
	delta := window.Pools[0].Classes[0].Delta
	if delta.Gets != 5 || delta.Hits != 4 || delta.Misses != 1 || delta.Allocations != 1 || delta.Puts != 4 || delta.Retains != 3 || delta.Drops != 1 {
		t.Fatalf("class delta = %+v", delta)
	}
}

// TestPoolPartitionWindowTreatsBackwardCountersAsCurrent verifies reset handling.
func TestPoolPartitionWindowTreatsBackwardCountersAsCurrent(t *testing.T) {
	previous := PoolPartitionSample{
		PoolCounters: PoolCountersSnapshot{
			Hits:          10,
			ReturnedBytes: 20,
			DropReasons:   PoolDropReasonCounters{ClosedPool: 5},
		},
		LeaseCounters: LeaseCountersSnapshot{
			Acquisitions:             20,
			PoolReturnClosedFailures: 9,
		},
	}
	current := PoolPartitionSample{
		PoolCounters: PoolCountersSnapshot{
			Hits:          3,
			ReturnedBytes: 7,
			DropReasons:   PoolDropReasonCounters{ClosedPool: 2},
		},
		LeaseCounters: LeaseCountersSnapshot{
			Acquisitions:             4,
			PoolReturnClosedFailures: 1,
		},
	}

	window := NewPoolPartitionWindow(previous, current)

	if window.Delta.Hits != 3 {
		t.Fatalf("Hits delta after reset = %d, want current value 3", window.Delta.Hits)
	}
	if window.Delta.LeaseAcquisitions != 4 {
		t.Fatalf("LeaseAcquisitions delta after reset = %d, want current value 4", window.Delta.LeaseAcquisitions)
	}
	if window.Delta.ReturnedBytes != 7 || window.Delta.ClosedPoolDrops != 2 || window.Delta.LeasePoolReturnClosedFailures != 1 {
		t.Fatalf("expanded reset deltas = %+v, want current values", window.Delta)
	}
}

// TestPoolPartitionWindowDoesNotTreatGaugesAsCounterDeltas verifies gauge boundaries.
func TestPoolPartitionWindowDoesNotTreatGaugesAsCounterDeltas(t *testing.T) {
	previous := PoolPartitionSample{CurrentRetainedBytes: 100, CurrentActiveBytes: 50}
	current := PoolPartitionSample{CurrentRetainedBytes: 20, CurrentActiveBytes: 200}

	window := NewPoolPartitionWindow(previous, current)

	if !window.Delta.IsZero() {
		t.Fatalf("gauge-only window delta = %+v, want zero", window.Delta)
	}
}

// TestPoolPartitionCounterDeltaIsZeroIncludesExpandedFields verifies zero predicate coverage.
func TestPoolPartitionCounterDeltaIsZeroIncludesExpandedFields(t *testing.T) {
	tests := []struct {
		name  string
		delta PoolPartitionCounterDelta
	}{
		{name: "returned bytes", delta: PoolPartitionCounterDelta{ReturnedBytes: 1}},
		{name: "retained bytes", delta: PoolPartitionCounterDelta{RetainedBytes: 1}},
		{name: "dropped bytes", delta: PoolPartitionCounterDelta{DroppedBytes: 1}},
		{name: "closed pool drops", delta: PoolPartitionCounterDelta{ClosedPoolDrops: 1}},
		{name: "disabled drops", delta: PoolPartitionCounterDelta{ReturnedBuffersDisabledDrops: 1}},
		{name: "oversized drops", delta: PoolPartitionCounterDelta{OversizedDrops: 1}},
		{name: "unsupported drops", delta: PoolPartitionCounterDelta{UnsupportedClassDrops: 1}},
		{name: "invalid policy drops", delta: PoolPartitionCounterDelta{InvalidPolicyDrops: 1}},
		{name: "closed return failures", delta: PoolPartitionCounterDelta{LeasePoolReturnClosedFailures: 1}},
		{name: "admission return failures", delta: PoolPartitionCounterDelta{LeasePoolReturnAdmissionFailures: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.delta.IsZero() {
				t.Fatalf("IsZero() = true for %+v", tt.delta)
			}
		})
	}
}

// TestPoolPartitionWindowCopiesPoolSlices verifies window sample copy boundaries.
func TestPoolPartitionWindowCopiesPoolSlices(t *testing.T) {
	previous := PoolPartitionSample{Pools: []PoolPartitionPoolSample{{Name: "previous"}}}
	current := PoolPartitionSample{Pools: []PoolPartitionPoolSample{{Name: "current"}}}

	window := NewPoolPartitionWindow(previous, current)
	previous.Pools[0].Name = "mutated-previous"
	current.Pools[0].Name = "mutated-current"

	if window.Previous.Pools[0].Name != "previous" {
		t.Fatalf("previous sample slice was not copied")
	}
	if window.Current.Pools[0].Name != "current" {
		t.Fatalf("current sample slice was not copied")
	}
}

// TestPoolPartitionWindowResetMatchesConstructor verifies reusable construction.
func TestPoolPartitionWindowResetMatchesConstructor(t *testing.T) {
	previous := PoolPartitionSample{
		Generation: Generation(1),
		Pools:      []PoolPartitionPoolSample{{Name: "previous"}},
		PoolCounters: PoolCountersSnapshot{
			Gets: 1,
		},
	}
	current := PoolPartitionSample{
		Generation: Generation(2),
		Pools:      []PoolPartitionPoolSample{{Name: "current"}},
		PoolCounters: PoolCountersSnapshot{
			Gets: 3,
		},
	}

	constructed := NewPoolPartitionWindow(previous, current)
	var reset PoolPartitionWindow
	reset.Reset(previous, current)

	if reset.Generation != constructed.Generation || reset.Delta != constructed.Delta {
		t.Fatalf("Reset window = %+v, constructor = %+v", reset, constructed)
	}
	if reset.Previous.Pools[0].Name != constructed.Previous.Pools[0].Name || reset.Current.Pools[0].Name != constructed.Current.Pools[0].Name {
		t.Fatalf("Reset sample copies differ from constructor")
	}
}

// TestPoolPartitionWindowResetReusesPoolStorage verifies allocation-control semantics.
func TestPoolPartitionWindowResetReusesPoolStorage(t *testing.T) {
	window := PoolPartitionWindow{
		Previous: PoolPartitionSample{Pools: make([]PoolPartitionPoolSample, 0, 4)},
		Current:  PoolPartitionSample{Pools: make([]PoolPartitionPoolSample, 0, 4)},
	}
	previous := PoolPartitionSample{Pools: []PoolPartitionPoolSample{{Name: "a"}, {Name: "b"}}}
	current := PoolPartitionSample{Pools: []PoolPartitionPoolSample{{Name: "c"}, {Name: "d"}}}

	window.Reset(previous, current)

	if len(window.Previous.Pools) != 2 || len(window.Current.Pools) != 2 {
		t.Fatalf("window pool lengths = previous %d current %d, want 2/2", len(window.Previous.Pools), len(window.Current.Pools))
	}
	if cap(window.Previous.Pools) != 4 || cap(window.Current.Pools) != 4 {
		t.Fatalf("window capacities = previous %d current %d, want reused 4/4", cap(window.Previous.Pools), cap(window.Current.Pools))
	}

	previous.Pools[0].Name = "mutated"
	current.Pools[0].Name = "mutated"
	if window.Previous.Pools[0].Name != "a" || window.Current.Pools[0].Name != "c" {
		t.Fatalf("Reset retained caller-owned pool slices")
	}

	smallerPrevious := PoolPartitionSample{Pools: []PoolPartitionPoolSample{{Name: "only"}}}
	smallerCurrent := PoolPartitionSample{}
	window.Reset(smallerPrevious, smallerCurrent)
	if len(window.Previous.Pools) != 1 || window.Previous.Pools[0].Name != "only" {
		t.Fatalf("Reset previous pools = %+v, want single non-stale entry", window.Previous.Pools)
	}
	if len(window.Current.Pools) != 0 {
		t.Fatalf("Reset current pools = %+v, want no stale entries", window.Current.Pools)
	}
}
