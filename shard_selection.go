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

import "sync/atomic"

const (
	// errShardSelectorNil is used when class code is asked to select a shard
	// without a selector.
	errShardSelectorNil = "bufferpool.shardSelector: selector must not be nil"

	// errShardSelectorInvalidShardCount is used when selection is requested for
	// an empty shard set.
	errShardSelectorInvalidShardCount = "bufferpool.shardSelector: shard count must be greater than zero"

	// errShardSelectorInvalidIndex is used when a selector returns an index
	// outside [0, shardCount).
	errShardSelectorInvalidIndex = "bufferpool.shardSelector: selected shard index out of range"
)

// shardSelector chooses a shard index from a non-empty class-owned shard set.
//
// The interface is internal so the runtime can use deterministic selectors in
// tests and simple striped selectors in ordinary paths without exposing a public
// API before the pool layer exists.
type shardSelector interface {
	SelectShard(shardCount int) int
}

// singleShardSelector always selects shard zero.
//
// It is useful for deterministic tests and for runtime owners that intentionally
// operate on a single shard.
type singleShardSelector struct{}

// SelectShard returns shard zero for any positive shard count.
func (singleShardSelector) SelectShard(shardCount int) int {
	validateShardSelectorShardCount(shardCount)
	return 0
}

// roundRobinShardSelector stripes selections across the available shard indexes.
//
// The zero value is ready to use. It MUST NOT be copied after first use because
// it contains atomic state.
type roundRobinShardSelector struct {
	next atomic.Uint64
}

// SelectShard returns the next striped shard index in [0, shardCount).
//
// The uint64 sequence may wrap; modulo selection still returns a valid index.
func (s *roundRobinShardSelector) SelectShard(shardCount int) int {
	if s == nil {
		panic(errShardSelectorNil)
	}

	validateShardSelectorShardCount(shardCount)

	index := s.next.Add(1) - 1
	return int(index % uint64(shardCount))
}

// selectShardIndex asks selector for a shard index and validates the result.
func selectShardIndex(selector shardSelector, shardCount int) int {
	if selector == nil {
		panic(errShardSelectorNil)
	}

	validateShardSelectorShardCount(shardCount)

	index := selector.SelectShard(shardCount)
	if index < 0 || index >= shardCount {
		panic(errShardSelectorInvalidIndex)
	}

	return index
}

func validateShardSelectorShardCount(shardCount int) {
	if shardCount <= 0 {
		panic(errShardSelectorInvalidShardCount)
	}
}
