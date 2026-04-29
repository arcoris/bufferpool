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
	// errGroupRegistryNoPartitions protects direct registry construction.
	errGroupRegistryNoPartitions = "bufferpool.PoolGroup: at least one partition must be configured"

	// errGroupRegistryEmptyPartitionName protects direct registry construction.
	errGroupRegistryEmptyPartitionName = "bufferpool.PoolGroup: partition name must not be empty"

	// errGroupRegistryDuplicatePartition protects direct registry construction.
	errGroupRegistryDuplicatePartition = "bufferpool.PoolGroup: duplicate partition name"
)

// groupRegistry is the immutable partition registry owned by one PoolGroup.
type groupRegistry struct {
	// entries preserves deterministic iteration order for samples and snapshots.
	entries []groupPartitionEntry

	// byName provides group-local partition lookup.
	byName map[string]*PoolPartition

	// indexByName maps group-local names to deterministic registry indexes.
	indexByName map[string]int

	// indexByPartition maps owned partition pointers back to registry indexes.
	indexByPartition map[*PoolPartition]int

	// names stores the defensive-copy source returned by PartitionNames.
	names []string
}

// groupPartitionEntry ties a group-local name to an owned partition.
type groupPartitionEntry struct {
	// index is the deterministic registry index.
	index int

	// name is the group-local partition name.
	name string

	// partition is the owned PoolPartition.
	partition *PoolPartition
}

// newGroupRegistry constructs the group-owned partition registry.
func newGroupRegistry(configs []GroupPartitionConfig) (groupRegistry, error) {
	if len(configs) == 0 {
		return groupRegistry{}, newError(ErrInvalidOptions, errGroupRegistryNoPartitions)
	}
	registry := groupRegistry{
		entries:          make([]groupPartitionEntry, 0, len(configs)),
		byName:           make(map[string]*PoolPartition, len(configs)),
		indexByName:      make(map[string]int, len(configs)),
		indexByPartition: make(map[*PoolPartition]int, len(configs)),
		names:            make([]string, 0, len(configs)),
	}
	for _, config := range configs {
		normalized := config.Normalize()
		if normalized.Name == "" {
			err := newError(ErrInvalidOptions, errGroupRegistryEmptyPartitionName)
			return groupRegistry{}, multierr.Append(err, registry.closeAll())
		}
		if _, exists := registry.byName[normalized.Name]; exists {
			err := newError(ErrInvalidOptions, errGroupRegistryDuplicatePartition+": "+normalized.Name)
			return groupRegistry{}, multierr.Append(err, registry.closeAll())
		}
		// Registry construction can be called directly by tests or future
		// internal code, so keep the same name invariant enforced by config
		// validation before allocating the child partition.
		if err := validateGroupPartitionNameMatch(normalized); err != nil {
			return groupRegistry{}, multierr.Append(err, registry.closeAll())
		}
		partition, err := NewPoolPartition(normalized.Config)
		if err != nil {
			return groupRegistry{}, multierr.Append(err, registry.closeAll())
		}
		index := len(registry.entries)
		registry.entries = append(registry.entries, groupPartitionEntry{index: index, name: normalized.Name, partition: partition})
		registry.byName[normalized.Name] = partition
		registry.indexByName[normalized.Name] = index
		registry.indexByPartition[partition] = index
		registry.names = append(registry.names, normalized.Name)
	}
	return registry, nil
}

// partition looks up a raw partition for internal group code only.
func (r groupRegistry) partition(name string) (*PoolPartition, bool) {
	partition, ok := r.byName[name]
	return partition, ok
}

// partitionIndex looks up a deterministic partition index by group-local name.
func (r groupRegistry) partitionIndex(name string) (int, bool) {
	index, ok := r.indexByName[name]
	return index, ok
}

// partitionIndexForPartition looks up a deterministic partition index by pointer.
func (r groupRegistry) partitionIndexForPartition(partition *PoolPartition) (int, bool) {
	index, ok := r.indexByPartition[partition]
	return index, ok
}

// namesCopy returns a caller-owned copy of deterministic partition names.
func (r groupRegistry) namesCopy() []string { return append([]string(nil), r.names...) }

// len reports the number of partitions owned by the registry.
func (r groupRegistry) len() int { return len(r.entries) }

// closeAll closes every owned partition and aggregates all close errors.
func (r groupRegistry) closeAll() error {
	var err error
	for _, entry := range r.entries {
		multierr.AppendInto(&err, entry.partition.Close())
	}
	return err
}
