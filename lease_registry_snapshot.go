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

// Snapshot returns an observational registry snapshot.
//
// Snapshot copies active lease metadata while holding the registry lock and each
// record lock in the same order used by release. It does not expose mutable
// lease records or buffers. The result is useful for diagnostics and future
// controller sampling, but it is not a global transaction across Pool storage.
//
// Active records, active gauges, and generation are sampled while the registry
// lock is held. Pool-return handoff counters may still move after ownership
// release because Pool retained-storage admission deliberately runs outside
// registry locks.
func (r *LeaseRegistry) Snapshot() LeaseRegistrySnapshot {
	r.mustBeInitialized()
	r.mu.Lock()
	active := make([]LeaseSnapshot, 0, len(r.active))
	for _, record := range r.active {
		active = append(active, record.snapshot())
	}
	lifecycle := r.lifecycle.Load()
	generation := r.generation.Load()
	counters := r.counters.snapshot()
	r.mu.Unlock()

	return LeaseRegistrySnapshot{
		Lifecycle:  lifecycle,
		Generation: generation,
		Config:     r.config,
		Counters:   counters,
		Active:     active,
	}
}
