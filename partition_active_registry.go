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

import "sync"

const (
	// errPartitionActiveRegistryPoolMissing reports an activity mark for an
	// unknown partition-local Pool name.
	errPartitionActiveRegistryPoolMissing = "bufferpool.PoolPartition: active registry pool not found"

	// errPartitionActiveRegistryIndexInvalid reports an activity mark for an
	// index outside the immutable partition Pool registry.
	errPartitionActiveRegistryIndexInvalid = "bufferpool.PoolPartition: active registry pool index out of range"
)

// partitionActiveRegistry is the partition-local active/dirty Pool set.
//
// This is control-plane scaffolding, not adaptive scoring. It tracks which
// partition-owned Pools are currently eligible for controller attention and
// which Pools have become dirty through partition-owned cold/control paths. It
// does not own Pool storage, buffer ownership, or Pool hot-path hooks. The
// initial implementation keeps all Pools active; future idle expiry and window
// delta logic can reduce that set without changing Pool or LeaseRegistry
// ownership boundaries.
type partitionActiveRegistry struct {
	// mu protects active, dirty, and generation.
	mu sync.Mutex

	// names preserves deterministic partition registry order.
	names []string

	// byName maps partition-local names to immutable registry indexes.
	byName map[string]int

	// active marks Pools eligible for controller sampling.
	active []bool

	// dirty marks Pools changed by partition-owned cold/control paths.
	dirty []bool

	// generation advances when active or dirty state changes.
	generation Generation
}

// newPartitionActiveRegistry creates an active registry for immutable Pool names.
func newPartitionActiveRegistry(names []string) *partitionActiveRegistry {
	registry := &partitionActiveRegistry{
		names:      append([]string(nil), names...),
		byName:     make(map[string]int, len(names)),
		active:     make([]bool, len(names)),
		dirty:      make([]bool, len(names)),
		generation: InitialGeneration,
	}
	for index, name := range registry.names {
		registry.byName[name] = index
		registry.active[index] = true
	}
	return registry
}

// markActive marks a Pool active by partition-local name.
func (r *partitionActiveRegistry) markActive(name string) error {
	index, ok := r.index(name)
	if !ok {
		return newError(ErrInvalidOptions, errPartitionActiveRegistryPoolMissing+": "+name)
	}
	return r.markActiveIndex(index)
}

// markActiveIndex marks a Pool active by immutable registry index.
func (r *partitionActiveRegistry) markActiveIndex(index int) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.validIndexLocked(index) {
		return newError(ErrInvalidOptions, errPartitionActiveRegistryIndexInvalid)
	}
	if !r.active[index] {
		r.active[index] = true
		r.advanceLocked()
	}
	return nil
}

// markDirty marks a Pool dirty by partition-local name.
func (r *partitionActiveRegistry) markDirty(name string) error {
	index, ok := r.index(name)
	if !ok {
		return newError(ErrInvalidOptions, errPartitionActiveRegistryPoolMissing+": "+name)
	}
	return r.markDirtyIndex(index)
}

// markDirtyIndex marks a Pool dirty by immutable registry index.
func (r *partitionActiveRegistry) markDirtyIndex(index int) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.validIndexLocked(index) {
		return newError(ErrInvalidOptions, errPartitionActiveRegistryIndexInvalid)
	}
	if !r.dirty[index] {
		r.dirty[index] = true
		r.advanceLocked()
	}
	return nil
}

// markAllDirty marks every partition-owned Pool dirty.
func (r *partitionActiveRegistry) markAllDirty() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	changed := false
	for index := range r.dirty {
		if !r.dirty[index] {
			r.dirty[index] = true
			changed = true
		}
	}
	if changed {
		r.advanceLocked()
	}
}

// resetDirty clears dirty markers after a controller has consumed them.
func (r *partitionActiveRegistry) resetDirty() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	changed := false
	for index := range r.dirty {
		if r.dirty[index] {
			r.dirty[index] = false
			changed = true
		}
	}
	if changed {
		r.advanceLocked()
	}
}

// activeIndexes appends active Pool indexes to dst in deterministic order.
func (r *partitionActiveRegistry) activeIndexes(dst []int) []int {
	if r == nil {
		return dst[:0]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	dst = dst[:0]
	for index, active := range r.active {
		if active {
			dst = append(dst, index)
		}
	}
	return dst
}

// dirtyIndexes appends dirty Pool indexes to dst in deterministic order.
func (r *partitionActiveRegistry) dirtyIndexes(dst []int) []int {
	if r == nil {
		return dst[:0]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	dst = dst[:0]
	for index, dirty := range r.dirty {
		if dirty {
			dst = append(dst, index)
		}
	}
	return dst
}

// generationSnapshot returns the current active-registry generation.
func (r *partitionActiveRegistry) generationSnapshot() Generation {
	if r == nil {
		return NoGeneration
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.generation
}

// index returns the immutable registry index for name.
func (r *partitionActiveRegistry) index(name string) (int, bool) {
	if r == nil {
		return 0, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	index, ok := r.byName[name]
	return index, ok
}

// validIndexLocked reports whether index names a configured Pool.
func (r *partitionActiveRegistry) validIndexLocked(index int) bool {
	return index >= 0 && index < len(r.names)
}

// advanceLocked advances the active-registry generation under mu.
func (r *partitionActiveRegistry) advanceLocked() {
	r.generation = r.generation.Next()
}
