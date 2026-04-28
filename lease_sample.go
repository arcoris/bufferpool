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

// leaseCounterSample is the allocation-conscious LeaseRegistry sample consumed
// by PoolPartition controller-facing sampling.
//
// Unlike LeaseRegistrySnapshot, this type does not copy active lease metadata.
// It reports lifecycle, registry generation, active count, and aggregate
// counters under the registry lock. Future PoolPartition controller ticks should
// use this path for cheap sampling and reserve snapshots for diagnostics.
type leaseCounterSample struct {
	// Lifecycle is the observed registry lifecycle state.
	Lifecycle LifecycleState

	// Generation is the observed registry generation.
	Generation Generation

	// ActiveCount is len(active) sampled under the registry lock.
	ActiveCount int

	// Counters is the aggregate ownership counter sample.
	Counters LeaseCountersSnapshot
}

// sampleCounters writes a low-allocation aggregate sample into dst.
//
// The method samples the active map length, counters, and generation while the
// registry lock is held. It does not allocate active lease slices and does not
// expose mutable lease records.
func (r *LeaseRegistry) sampleCounters(dst *leaseCounterSample) {
	if dst == nil {
		return
	}

	r.mustBeInitialized()
	r.mu.Lock()
	*dst = leaseCounterSample{
		Lifecycle:   r.lifecycle.Load(),
		Generation:  r.generation.Load(),
		ActiveCount: len(r.active),
		Counters:    r.counters.snapshot(),
	}
	r.mu.Unlock()
}
