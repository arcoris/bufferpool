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

// poolCounterSample is the allocation-conscious aggregate sample intended for
// future PoolPartition controller ticks.
//
// It avoids policy cloning and public class/shard slice allocation. The sample
// is still observational: it reads owner counters, class counters, and shard
// retained usage without a global Pool lock.
type poolCounterSample struct {
	// Generation is the runtime snapshot generation observed at sample start.
	Generation Generation

	// Lifecycle is an atomic lifecycle sample. It is diagnostic context, not an
	// operation-admission decision.
	Lifecycle LifecycleState

	// ClassCount and ShardCount describe the sampled topology without requiring
	// caller allocation for class or shard detail.
	ClassCount int
	ShardCount int

	// Counters contains aggregate lifetime counters plus derived retained usage.
	Counters PoolCountersSnapshot
}

// sampleCounters fills dst with an aggregate Pool counter sample.
//
// The method is intended for internal controller-facing code that needs cheap
// repeated observations. It deliberately avoids Snapshot because Snapshot must
// allocate public class/shard slices and clone Policy for caller safety.
//
// The result is not transactional across all classes and shards. Each shard
// counter sample is atomic and race-safe, but concurrent Get/Put/trim/clear work
// may move counters while the aggregate is being built. The method does not read
// bucket state and does not take shard.mu; it samples retained gauges from shard
// counters instead.
func (p *Pool) sampleCounters(dst *poolCounterSample) {
	if dst == nil {
		return
	}

	runtime := p.currentRuntimeSnapshot()
	*dst = poolCounterSample{
		Generation: runtime.Generation,
		Lifecycle:  p.lifecycle.Load(),
		ClassCount: len(p.classes),
	}

	var counters PoolCountersSnapshot
	for classIndex := range p.classes {
		class := &p.classes[classIndex]
		classCounters := newPoolCountersFromClass(class.countersSnapshot(), 0, 0)
		poolCountersAdd(&counters, classCounters)

		for shardIndex := range class.shards {
			shardCounters := class.shards[shardIndex].countersSnapshot()
			counters.CurrentRetainedBuffers += shardCounters.CurrentRetainedBuffers
			counters.CurrentRetainedBytes += shardCounters.CurrentRetainedBytes
			dst.ShardCount++
		}
	}

	poolCountersApplyOwner(&counters, p.ownerCounters.snapshot())
	dst.Counters = counters
}
