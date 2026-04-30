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

import "arcoris.dev/bufferpool/internal/multierr"

const (
	// errPartitionRegistryEmptyPoolName protects direct registry construction.
	errPartitionRegistryEmptyPoolName = "bufferpool.PoolPartition: pool name must not be empty"

	// errPartitionRegistryDuplicatePool protects direct registry construction.
	errPartitionRegistryDuplicatePool = "bufferpool.PoolPartition: duplicate pool name"
)

// partitionRegistry is the immutable pool registry owned by one PoolPartition.
type partitionRegistry struct {
	// entries preserves deterministic iteration order for samples and snapshots.
	entries []partitionPoolEntry

	// byName provides partition-local Pool lookup for Acquire and diagnostics.
	byName map[string]*Pool

	// indexByName maps partition-local names to deterministic registry indexes.
	indexByName map[string]int

	// indexByPool maps owned Pool pointers back to deterministic registry indexes.
	indexByPool map[*Pool]int

	// names stores the defensive-copy source returned by PoolNames.
	names []string
}

// partitionPoolEntry ties a partition-local name to an owned Pool.
type partitionPoolEntry struct {
	// index is the deterministic registry index.
	index int

	// name is the partition-local Pool name.
	name string

	// pool is the retained-storage data-plane owner.
	pool *Pool
}

// newPartitionRegistry constructs the partition-owned Pool registry.
func newPartitionRegistry(configs []PartitionPoolConfig) (partitionRegistry, error) {
	registry := partitionRegistry{
		entries:     make([]partitionPoolEntry, 0, len(configs)),
		byName:      make(map[string]*Pool, len(configs)),
		indexByName: make(map[string]int, len(configs)),
		indexByPool: make(map[*Pool]int, len(configs)),
		names:       make([]string, 0, len(configs)),
	}
	for _, config := range configs {
		normalized := config.Normalize()
		if normalized.Name == "" {
			err := newError(ErrInvalidOptions, errPartitionRegistryEmptyPoolName)
			return partitionRegistry{}, multierr.Append(err, registry.closeAll())
		}
		if _, exists := registry.byName[normalized.Name]; exists {
			err := newError(ErrInvalidOptions, errPartitionRegistryDuplicatePool+": "+normalized.Name)
			return partitionRegistry{}, multierr.Append(err, registry.closeAll())
		}
		pool, err := New(normalized.Config)
		if err != nil {
			return partitionRegistry{}, multierr.Append(err, registry.closeAll())
		}
		index := len(registry.entries)
		registry.entries = append(registry.entries, partitionPoolEntry{index: index, name: normalized.Name, pool: pool})
		registry.byName[normalized.Name] = pool
		registry.indexByName[normalized.Name] = index
		registry.indexByPool[pool] = index
		registry.names = append(registry.names, normalized.Name)
	}
	return registry, nil
}

// pool looks up a raw Pool for internal partition code only.
func (r partitionRegistry) pool(name string) (*Pool, bool) {
	pool, ok := r.byName[name]
	return pool, ok
}

// poolIndex looks up a deterministic Pool index by partition-local name.
func (r partitionRegistry) poolIndex(name string) (int, bool) {
	index, ok := r.indexByName[name]
	return index, ok
}

// poolIndexForPool looks up a deterministic Pool index by owned Pool pointer.
func (r partitionRegistry) poolIndexForPool(pool *Pool) (int, bool) {
	index, ok := r.indexByPool[pool]
	return index, ok
}

// namesCopy returns a caller-owned copy of deterministic Pool names.
func (r partitionRegistry) namesCopy() []string { return append([]string(nil), r.names...) }

// len reports the number of Pools owned by the registry.
func (r partitionRegistry) len() int { return len(r.entries) }

// closeAll closes every owned Pool and aggregates all close errors.
func (r partitionRegistry) closeAll() error {
	var err error
	for _, entry := range r.entries {
		multierr.AppendInto(&err, entry.pool.Close())
	}
	return err
}
