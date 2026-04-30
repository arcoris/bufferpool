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
	"math/bits"
	"sync"
	"sync/atomic"
)

const (
	// errPartitionActiveRegistryPoolMissing reports an activity mark for an
	// unknown partition-local Pool name.
	errPartitionActiveRegistryPoolMissing = "bufferpool.PoolPartition: active registry pool not found"

	// errPartitionActiveRegistryIndexInvalid reports an activity mark for an
	// index outside the immutable partition Pool registry.
	errPartitionActiveRegistryIndexInvalid = "bufferpool.PoolPartition: active registry pool index out of range"
)

const (
	// partitionActiveRegistryIdleWindowLimit is the number of clean sampled
	// controller windows after which an active Pool may be removed from the active
	// set.
	partitionActiveRegistryIdleWindowLimit uint8 = 2
)

// partitionActiveRegistry is the partition-local active/dirty Pool set.
//
// This is control-plane state, not adaptive scoring. Active means "eligible for
// controller attention", not "recently used". The partition controller may mark
// clean sampled Pools inactive after repeated no-delta windows, while partition
// Acquire/Release paths mark their Pool active again without adding hooks to
// Pool.Get or Pool.Put.
//
// Dirty is a state marker, not an activity event log. Marking dirty does not by
// itself imply active; partition operations that observe real activity mark both
// active and dirty. Repeated activity on an already-dirty Pool intentionally
// does not advance generation. generation is the version of active/dirty state,
// not a count of Pool operations.
type partitionActiveRegistry struct {
	// mu protects controller-side idle window state and generation movement.
	//
	// Hot-path active/dirty marking uses atomic bitsets below; it takes mu only
	// when a bit actually changes and generation must advance. Controller-side
	// idle expiry remains locked because it is sampled foreground work rather
	// than per-acquire/per-release activity.
	mu sync.Mutex

	// names preserves deterministic partition registry order.
	names []string

	// byName maps partition-local names to immutable registry indexes.
	byName map[string]int

	// activeBits marks Pools eligible for controller sampling.
	activeBits []atomic.Uint64

	// dirtyBits marks Pools changed by partition-owned cold/control paths.
	dirtyBits []atomic.Uint64

	// idleWindows counts consecutive sampled windows without activity while a Pool
	// is active and clean.
	idleWindows []uint8

	// generation advances when active or dirty marker state changes.
	generation Generation
}

// newPartitionActiveRegistry creates an active registry for immutable Pool names.
func newPartitionActiveRegistry(names []string) *partitionActiveRegistry {
	registry := &partitionActiveRegistry{
		names:       append([]string(nil), names...),
		byName:      make(map[string]int, len(names)),
		activeBits:  make([]atomic.Uint64, partitionActiveRegistryChunkCount(len(names))),
		dirtyBits:   make([]atomic.Uint64, partitionActiveRegistryChunkCount(len(names))),
		idleWindows: make([]uint8, len(names)),
		generation:  InitialGeneration,
	}
	for index, name := range registry.names {
		registry.byName[name] = index
		partitionActiveRegistrySetBit(registry.activeBits, index)
	}
	return registry
}

// markActive marks a Pool active by partition-local name.
func (r *partitionActiveRegistry) markActive(name string) error {
	if r == nil {
		return nil
	}
	index, ok := r.byName[name]
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
	if !r.validIndex(index) {
		return newError(ErrInvalidOptions, errPartitionActiveRegistryIndexInvalid)
	}
	if partitionActiveRegistrySetBit(r.activeBits, index) {
		r.mu.Lock()
		r.idleWindows[index] = 0
		r.advanceLocked()
		r.mu.Unlock()
	}
	return nil
}

// markInactiveIndex marks a Pool inactive by immutable registry index.
//
// The partition controller normally expires idle Pools through
// observeControllerActivity. This helper is retained for direct tests and rare
// internal callers that need to move one valid index out of the active set
// without simulating a controller window.
func (r *partitionActiveRegistry) markInactiveIndex(index int) error {
	if r == nil {
		return nil
	}
	if !r.validIndex(index) {
		return newError(ErrInvalidOptions, errPartitionActiveRegistryIndexInvalid)
	}
	if partitionActiveRegistryClearBit(r.activeBits, index) {
		r.mu.Lock()
		r.idleWindows[index] = 0
		r.advanceLocked()
		r.mu.Unlock()
	}
	return nil
}

// deactivateCleanIndexes marks clean candidate Pools inactive.
//
// This is a direct active-set primitive. There is no wall-clock timer or EWMA
// work here; the partition controller decides which clean indexes are safe to
// deactivate after inspecting bounded windows. Dirty candidates are skipped so
// pending state changes are not hidden from controller attention.
func (r *partitionActiveRegistry) deactivateCleanIndexes(indexes []int) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	changed := false
	for _, index := range indexes {
		if !r.validIndex(index) {
			return newError(ErrInvalidOptions, errPartitionActiveRegistryIndexInvalid)
		}
		if partitionActiveRegistryBitSet(r.activeBits, index) && !partitionActiveRegistryBitSet(r.dirtyBits, index) {
			changed = partitionActiveRegistryClearBit(r.activeBits, index) || changed
			if partitionActiveRegistryBitSet(r.dirtyBits, index) {
				changed = partitionActiveRegistrySetBit(r.activeBits, index) || changed
				continue
			}
			r.idleWindows[index] = 0
		}
	}
	if changed {
		r.advanceLocked()
	}
	return nil
}

// markDirty marks a Pool dirty by partition-local name.
func (r *partitionActiveRegistry) markDirty(name string) error {
	if r == nil {
		return nil
	}
	index, ok := r.byName[name]
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
	if !r.validIndex(index) {
		return newError(ErrInvalidOptions, errPartitionActiveRegistryIndexInvalid)
	}
	if partitionActiveRegistrySetBit(r.dirtyBits, index) {
		r.mu.Lock()
		r.advanceLocked()
		r.mu.Unlock()
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
	for index := range r.names {
		changed = partitionActiveRegistrySetBit(r.dirtyBits, index) || changed
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
	for chunk := range r.dirtyBits {
		if r.dirtyBits[chunk].Swap(0) != 0 {
			changed = true
		}
	}
	if changed {
		r.advanceLocked()
	}
}

// observeControllerActivity updates active markers from sampled controller
// window activity.
//
// indexes and activeDeltas are parallel slices produced by one TickInto cycle.
// A true activity bit keeps the Pool active and resets its idle count. A false
// bit increments idle count only when the Pool is already active and clean; dirty
// Pools are kept active until the controller consumes the dirty marker.
func (r *partitionActiveRegistry) observeControllerActivity(indexes []int, activeDeltas []bool) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	changed := false
	for offset, index := range indexes {
		if !r.validIndex(index) {
			return newError(ErrInvalidOptions, errPartitionActiveRegistryIndexInvalid)
		}
		activeDelta := offset < len(activeDeltas) && activeDeltas[offset]
		if activeDelta {
			changed = partitionActiveRegistrySetBit(r.activeBits, index) || changed
			if r.idleWindows[index] != 0 {
				r.idleWindows[index] = 0
				changed = true
			}
			continue
		}
		if !partitionActiveRegistryBitSet(r.activeBits, index) || partitionActiveRegistryBitSet(r.dirtyBits, index) {
			continue
		}
		if r.idleWindows[index] < partitionActiveRegistryIdleWindowLimit {
			r.idleWindows[index]++
			changed = true
		}
		if r.idleWindows[index] >= partitionActiveRegistryIdleWindowLimit {
			changed = partitionActiveRegistryClearBit(r.activeBits, index) || changed
		}
	}
	if changed {
		r.advanceLocked()
	}
	return nil
}

// activeIndexes appends active Pool indexes to dst in deterministic order.
func (r *partitionActiveRegistry) activeIndexes(dst []int) []int {
	if r == nil {
		return dst[:0]
	}
	return partitionActiveRegistryIndexes(dst, r.activeBits, len(r.names))
}

// dirtyIndexes appends dirty Pool indexes to dst in deterministic order.
func (r *partitionActiveRegistry) dirtyIndexes(dst []int) []int {
	if r == nil {
		return dst[:0]
	}
	return partitionActiveRegistryIndexes(dst, r.dirtyBits, len(r.names))
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

// controllerDirtyIndexes appends current dirty indexes for controller code.
func (p *PoolPartition) controllerDirtyIndexes(dst []int) []int {
	p.mustBeInitialized()
	return p.activeRegistry.dirtyIndexes(dst)
}

// markDirtyProcessed explicitly consumes dirty markers after controller work.
//
// TickInto calls this only after it has sampled the selected Pools, applied
// budget targets, and recorded controller state. Dirty consumption stays at this
// boundary so diagnostic Sample, Metrics, and Snapshot calls cannot hide pending
// state changes from the next controller cycle.
func (p *PoolPartition) markDirtyProcessed() {
	p.mustBeInitialized()
	p.activeRegistry.resetDirty()
}

// validIndex reports whether index names a configured Pool.
func (r *partitionActiveRegistry) validIndex(index int) bool {
	return index >= 0 && index < len(r.names)
}

// advanceLocked advances the active-registry generation under mu.
func (r *partitionActiveRegistry) advanceLocked() {
	r.generation = r.generation.Next()
}

// partitionActiveRegistryChunkCount returns the number of uint64 words needed
// to hold size marker bits.
func partitionActiveRegistryChunkCount(size int) int {
	if size <= 0 {
		return 0
	}
	return (size + 63) / 64
}

// partitionActiveRegistryBit returns the word index and bit mask for one
// registry index.
func partitionActiveRegistryBit(index int) (int, uint64) {
	return index / 64, uint64(1) << uint(index%64)
}

// partitionActiveRegistryBitSet reports whether index is currently marked.
func partitionActiveRegistryBitSet(words []atomic.Uint64, index int) bool {
	chunk, mask := partitionActiveRegistryBit(index)
	return chunk >= 0 && chunk < len(words) && words[chunk].Load()&mask != 0
}

// partitionActiveRegistrySetBit atomically sets one marker bit.
//
// It returns true only when the call changed the bit from clear to set, allowing
// hot-path callers to avoid taking the registry mutex for repeated activity on
// an already-marked Pool.
func partitionActiveRegistrySetBit(words []atomic.Uint64, index int) bool {
	chunk, mask := partitionActiveRegistryBit(index)
	if chunk < 0 || chunk >= len(words) {
		return false
	}
	for {
		current := words[chunk].Load()
		if current&mask != 0 {
			return false
		}
		if words[chunk].CompareAndSwap(current, current|mask) {
			return true
		}
	}
}

// partitionActiveRegistryClearBit atomically clears one marker bit.
//
// It returns true only when the bit changed from set to clear. Controller-side
// code uses that result to advance the active-registry generation once for real
// state movement.
func partitionActiveRegistryClearBit(words []atomic.Uint64, index int) bool {
	chunk, mask := partitionActiveRegistryBit(index)
	if chunk < 0 || chunk >= len(words) {
		return false
	}
	for {
		current := words[chunk].Load()
		if current&mask == 0 {
			return false
		}
		if words[chunk].CompareAndSwap(current, current&^mask) {
			return true
		}
	}
}

// partitionActiveRegistryIndexes appends marked indexes in deterministic
// registry order.
//
// The function reads atomic words without locking. The result is a diagnostic or
// controller observation of nearby marker state, not a transactional snapshot.
func partitionActiveRegistryIndexes(dst []int, words []atomic.Uint64, total int) []int {
	dst = dst[:0]
	for chunk := range words {
		word := words[chunk].Load()
		for word != 0 {
			offset := bits.TrailingZeros64(word)
			index := chunk*64 + offset
			if index >= total {
				return dst
			}
			dst = append(dst, index)
			word &^= uint64(1) << uint(offset)
		}
	}
	return dst
}
