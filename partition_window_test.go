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
			Gets:        10,
			Hits:        4,
			Misses:      6,
			Allocations: 5,
			Puts:        8,
			Retains:     3,
			Drops:       5,
		},
		LeaseCounters: LeaseCountersSnapshot{
			Acquisitions:        7,
			Releases:            5,
			InvalidReleases:     1,
			DoubleReleases:      2,
			OwnershipViolations: 3,
			PoolReturnAttempts:  5,
			PoolReturnSuccesses: 4,
			PoolReturnFailures:  1,
		},
	}
	current := PoolPartitionSample{
		Generation:       Generation(11),
		PolicyGeneration: Generation(13),
		PoolCounters: PoolCountersSnapshot{
			Gets:        15,
			Hits:        9,
			Misses:      6,
			Allocations: 8,
			Puts:        12,
			Retains:     7,
			Drops:       5,
		},
		LeaseCounters: LeaseCountersSnapshot{
			Acquisitions:        10,
			Releases:            9,
			InvalidReleases:     2,
			DoubleReleases:      3,
			OwnershipViolations: 6,
			PoolReturnAttempts:  9,
			PoolReturnSuccesses: 7,
			PoolReturnFailures:  2,
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
	if delta.LeaseAcquisitions != 3 || delta.LeaseReleases != 4 || delta.LeaseInvalidReleases != 1 {
		t.Fatalf("lease ownership delta = %+v", delta)
	}
	if delta.LeaseDoubleReleases != 1 || delta.LeaseOwnershipViolations != 3 {
		t.Fatalf("lease violation delta = %+v", delta)
	}
	if delta.LeasePoolReturnAttempts != 4 || delta.LeasePoolReturnSuccesses != 3 || delta.LeasePoolReturnFailures != 1 {
		t.Fatalf("lease pool-return delta = %+v", delta)
	}
	if window.Generation != current.Generation || window.PolicyGeneration != current.PolicyGeneration {
		t.Fatalf("window generations = %s/%s, want current generations", window.Generation, window.PolicyGeneration)
	}
}

// TestPoolPartitionWindowTreatsBackwardCountersAsCurrent verifies reset handling.
func TestPoolPartitionWindowTreatsBackwardCountersAsCurrent(t *testing.T) {
	previous := PoolPartitionSample{
		PoolCounters:  PoolCountersSnapshot{Hits: 10},
		LeaseCounters: LeaseCountersSnapshot{Acquisitions: 20},
	}
	current := PoolPartitionSample{
		PoolCounters:  PoolCountersSnapshot{Hits: 3},
		LeaseCounters: LeaseCountersSnapshot{Acquisitions: 4},
	}

	window := NewPoolPartitionWindow(previous, current)

	if window.Delta.Hits != 3 {
		t.Fatalf("Hits delta after reset = %d, want current value 3", window.Delta.Hits)
	}
	if window.Delta.LeaseAcquisitions != 4 {
		t.Fatalf("LeaseAcquisitions delta after reset = %d, want current value 4", window.Delta.LeaseAcquisitions)
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
