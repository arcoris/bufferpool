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
	"sync/atomic"
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestSingleShardSelectorAlwaysReturnsZero verifies deterministic selection.
func TestSingleShardSelectorAlwaysReturnsZero(t *testing.T) {
	t.Parallel()

	var selector singleShardSelector

	for _, shardCount := range []int{1, 2, 16} {
		shardCount := shardCount

		t.Run("shards", func(t *testing.T) {
			t.Parallel()

			if got := selector.SelectShard(shardCount); got != 0 {
				t.Fatalf("SelectShard(%d) = %d, want 0", shardCount, got)
			}
		})
	}
}

// TestShardSelectorsRejectInvalidShardCount verifies shard-count validation.
func TestShardSelectorsRejectInvalidShardCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "single zero",
			fn: func() {
				var selector singleShardSelector
				_ = selector.SelectShard(0)
			},
		},
		{
			name: "single negative",
			fn: func() {
				var selector singleShardSelector
				_ = selector.SelectShard(-1)
			},
		},
		{
			name: "round robin zero",
			fn: func() {
				selector := &roundRobinShardSelector{}
				_ = selector.SelectShard(0)
			},
		},
		{
			name: "round robin negative",
			fn: func() {
				selector := &roundRobinShardSelector{}
				_ = selector.SelectShard(-1)
			},
		},
		{
			name: "processor inspired zero",
			fn: func() {
				selector := newProcessorInspiredShardSelector(0)
				_ = selector.SelectShard(0)
			},
		},
		{
			name: "affinity zero",
			fn: func() {
				selector := newAffinityShardSelector(0)
				_ = selector.SelectShard(0)
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, errShardSelectorInvalidShardCount, tt.fn)
		})
	}
}

// TestProcessorInspiredShardSelectorProducesValidIndexes verifies the default
// selector stays bounded without private runtime APIs.
func TestProcessorInspiredShardSelectorProducesValidIndexes(t *testing.T) {
	t.Parallel()

	selector := newProcessorInspiredShardSelector(3)
	seen := make(map[int]bool)

	for iteration := 0; iteration < 1024; iteration++ {
		index := selector.SelectShard(16)
		if index < 0 || index >= 16 {
			t.Fatalf("SelectShard(16) = %d, want in [0, 16)", index)
		}

		seen[index] = true
	}

	if len(seen) < 8 {
		t.Fatalf("processor-inspired selector visited %d shards, want broad spread", len(seen))
	}
}

// TestProcessorInspiredShardSelectorDoesNotAllocate verifies the selection hot
// path remains allocation-free.
func TestProcessorInspiredShardSelectorDoesNotAllocate(t *testing.T) {
	selector := newProcessorInspiredShardSelector(0)
	allocs := testing.AllocsPerRun(1000, func() {
		_ = selector.SelectShard(32)
	})
	if allocs != 0 {
		t.Fatalf("SelectShard allocations = %v, want zero", allocs)
	}
}

// TestRoundRobinShardSelectorReturnsValidIndexes verifies striped selection.
func TestRoundRobinShardSelectorReturnsValidIndexes(t *testing.T) {
	t.Parallel()

	selector := &roundRobinShardSelector{}
	seen := make(map[int]bool)

	for i := 0; i < 32; i++ {
		index := selector.SelectShard(4)
		if index < 0 || index >= 4 {
			t.Fatalf("SelectShard(4) = %d, want index in [0, 4)", index)
		}

		seen[index] = true
	}

	for index := 0; index < 4; index++ {
		if !seen[index] {
			t.Fatalf("selector never returned shard %d", index)
		}
	}
}

func TestRoundRobinShardSelectorSequence(t *testing.T) {
	t.Parallel()

	selector := &roundRobinShardSelector{}
	want := []int{0, 1, 2, 0, 1, 2}

	for index, wantIndex := range want {
		if got := selector.SelectShard(3); got != wantIndex {
			t.Fatalf("selection %d = %d, want %d", index, got, wantIndex)
		}
	}
}

func TestRoundRobinShardSelectorShardCountOne(t *testing.T) {
	t.Parallel()

	selector := &roundRobinShardSelector{}

	for i := 0; i < 16; i++ {
		if got := selector.SelectShard(1); got != 0 {
			t.Fatalf("SelectShard(1) = %d, want 0", got)
		}
	}
}

func TestRoundRobinShardSelectorHandlesCounterWrap(t *testing.T) {
	t.Parallel()

	selector := &roundRobinShardSelector{}
	selector.next.Store(^uint64(0) - 1)

	for i := 0; i < 4; i++ {
		index := selector.SelectShard(5)
		if index < 0 || index >= 5 {
			t.Fatalf("SelectShard(5) after wrap = %d, want index in [0, 5)", index)
		}
	}
}

func TestRoundRobinShardSelectorConcurrentSelection(t *testing.T) {
	t.Parallel()

	const workers = 16
	const iterations = 128

	selector := &roundRobinShardSelector{}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var invalid atomic.Bool

	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()

			<-start

			for iteration := 0; iteration < iterations; iteration++ {
				index := selector.SelectShard(8)
				if index < 0 || index >= 8 {
					invalid.Store(true)
				}
			}
		}()
	}

	close(start)
	wg.Wait()

	if invalid.Load() {
		t.Fatal("concurrent SelectShard returned index outside [0, 8)")
	}
}

type invalidShardSelector struct{}

func (invalidShardSelector) SelectShard(int) int {
	return -1
}

// TestSelectShardIndexValidatesSelector verifies wrapper validation.
func TestSelectShardIndexValidatesSelector(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errShardSelectorNil, func() {
		_ = selectShardIndex(nil, 1)
	})

	testutil.MustPanicWithMessage(t, errShardSelectorNil, func() {
		var selector *roundRobinShardSelector
		_ = selector.SelectShard(1)
	})

	testutil.MustPanicWithMessage(t, errShardSelectorInvalidIndex, func() {
		_ = selectShardIndex(invalidShardSelector{}, 1)
	})
}
