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

// poolRandomShardSelector implements ShardSelectionModeRandom for Pool-owned
// classState operations.
//
// The selector is deterministic and non-cryptographic. It uses class-local
// atomic sequence state plus randx bounded mixing, which avoids global random
// state, locks, allocations, and controller dependencies on the Pool hot path.
type poolRandomShardSelector struct {
	// next is the class-local sequence. The value is atomic because many Get/Put
	// calls may route through the same class concurrently.
	next atomic.Uint64
}

// SelectShard returns a mixed bounded shard index in [0, shardCount).
func (s *poolRandomShardSelector) SelectShard(shardCount int) int {
	if s == nil {
		panic(errShardSelectorNil)
	}

	validateShardSelectorShardCount(shardCount)

	value := s.next.Add(1)
	return int(randx.BoundedIndexUint64(value, uint64(shardCount)))
}
