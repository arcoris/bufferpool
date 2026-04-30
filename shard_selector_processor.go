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
	"sync/atomic"

	"arcoris.dev/bufferpool/internal/randx"
)

const (
	// processorInspiredShardSelectorStep is the odd golden-ratio increment used
	// to advance per-class selector sequences.
	//
	// The exact value is not a public contract. It gives neighboring sequence
	// values different bit patterns before randx bounded mapping and avoids a
	// cheap "+1 then low-bit mask" pattern when shard counts are powers of two.
	processorInspiredShardSelectorStep = uint64(0x9e3779b97f4a7c15)
)

// processorInspiredShardSelector implements ShardSelectionModeProcessorInspired.
//
// It uses one owner-local sequence per class and mixes it with a class seed.
// This avoids one pool-wide atomic counter, does not call private runtime P
// APIs, and keeps selection allocation-free. The selector provides striped
// local entropy only; it does not represent caller affinity.
type processorInspiredShardSelector struct {
	// next is the class-local monotonic sequence. It is atomic because Pool.Get
	// may call the same class selector concurrently from many goroutines.
	next atomic.Uint64

	// seed differentiates selectors for different class indexes so all classes
	// do not walk the same shard sequence at the same time.
	seed uint64
}

// newProcessorInspiredShardSelector returns one selector for one Pool-owned
// class index.
//
// The class index is construction-time metadata, not public affinity. It only
// perturbs the local sequence so adjacent classes do not share identical shard
// walk order.
func newProcessorInspiredShardSelector(classIndex int) *processorInspiredShardSelector {
	return &processorInspiredShardSelector{
		seed: randx.MixUint64(uint64(classIndex) + processorInspiredShardSelectorStep),
	}
}

// SelectShard returns a mixed bounded shard index in [0, shardCount).
func (s *processorInspiredShardSelector) SelectShard(shardCount int) int {
	if s == nil {
		panic(errShardSelectorNil)
	}

	validateShardSelectorShardCount(shardCount)

	sequence := s.next.Add(processorInspiredShardSelectorStep)
	value := sequence ^ s.seed
	return int(randx.BoundedIndexUint64(value, uint64(shardCount)))
}
