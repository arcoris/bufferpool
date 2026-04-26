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
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, errShardSelectorInvalidShardCount, tt.fn)
		})
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

	testutil.MustPanicWithMessage(t, errShardSelectorInvalidIndex, func() {
		_ = selectShardIndex(invalidShardSelector{}, 1)
	})
}
