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
	"fmt"
	"runtime"
	"strings"

	"arcoris.dev/bufferpool/internal/multierr"
	"arcoris.dev/bufferpool/internal/randx"
)

const (
	defaultPoolGroupTargetActiveEntries           = 100000
	defaultPoolGroupEstimatedActiveClassesPerPool = 8

	errGroupConfigUnknownPartitioningMode      = "bufferpool.PoolGroupPartitioningPolicy: unknown mode"
	errGroupConfigInvalidMinPartitions         = "bufferpool.PoolGroupPartitioningPolicy: min partitions must be greater than zero"
	errGroupConfigInvalidMaxPartitions         = "bufferpool.PoolGroupPartitioningPolicy: max partitions must be greater than zero"
	errGroupConfigInvalidPartitionBounds       = "bufferpool.PoolGroupPartitioningPolicy: min partitions must not exceed max partitions"
	errGroupConfigInvalidTargetActiveEntries   = "bufferpool.PoolGroupPartitioningPolicy: target active entries must be greater than zero"
	errGroupConfigInvalidEstimatedClassCount   = "bufferpool.PoolGroupPartitioningPolicy: estimated active classes per pool must be greater than zero"
	errGroupConfigInvalidEstimatedShardCount   = "bufferpool.PoolGroupPartitioningPolicy: estimated active shards per class must be greater than zero"
	errGroupConfigUnknownPlacementPolicy       = "bufferpool.PoolGroupPartitioningPolicy: unknown placement policy"
	errGroupConfigUnknownPoolPriority          = "bufferpool.GroupPoolConfig: unknown pool priority"
	errGroupConfigAmbiguousPoolsAndPartitions  = "bufferpool.PoolGroupConfig: pools and partitions require explicit partitioning mode"
	errGroupConfigExplicitPartitionsRequired   = "bufferpool.PoolGroupConfig: explicit pool placement requires partitions"
	errGroupConfigPoolPartitionRequired        = "bufferpool.PoolGroupConfig: explicit pool placement requires a partition name"
	errGroupConfigUnknownPoolPartition         = "bufferpool.PoolGroupConfig: pool references unknown partition"
	errGroupConfigAutoPoolPartitionUnsupported = "bufferpool.PoolGroupConfig: automatic placement does not accept per-pool partition names"
	errGroupConfigInvalidComputedPartitions    = "bufferpool.PoolGroupConfig: computed partition count must be greater than zero"
)

// PoolPriority is group-level metadata reserved for future budget allocation.
type PoolPriority uint8

const (
	// PoolPriorityUnset means the group default priority should be used.
	PoolPriorityUnset PoolPriority = iota

	// PoolPriorityDefault is the neutral priority for group-level Pools.
	PoolPriorityDefault
)

// Normalize returns p with the default priority applied.
func (p PoolPriority) Normalize() PoolPriority {
	if p == PoolPriorityUnset {
		return PoolPriorityDefault
	}
	return p
}

// Validate rejects unknown priority values.
func (p PoolPriority) Validate() error {
	switch p.Normalize() {
	case PoolPriorityDefault:
		return nil
	default:
		return newError(ErrInvalidOptions, errGroupConfigUnknownPoolPriority)
	}
}

// IsZero reports whether p is unset.
func (p PoolPriority) IsZero() bool { return p == PoolPriorityUnset }

// PoolGroupPartitioningMode selects group Pool placement mode.
type PoolGroupPartitioningMode uint8

const (
	// PoolGroupPartitioningModeUnset means automatic partitioning defaults apply.
	PoolGroupPartitioningModeUnset PoolGroupPartitioningMode = iota

	// PoolGroupPartitioningModeAuto computes partitions and hashes Pools into them.
	PoolGroupPartitioningModeAuto

	// PoolGroupPartitioningModeExplicit uses caller-supplied partition names.
	PoolGroupPartitioningModeExplicit
)

// PoolPlacementPolicy selects deterministic group Pool placement.
type PoolPlacementPolicy uint8

const (
	// PoolPlacementPolicyUnset means deterministic hash placement.
	PoolPlacementPolicyUnset PoolPlacementPolicy = iota

	// PoolPlacementPolicyHash places Pools by stable non-security hashing.
	PoolPlacementPolicyHash
)

// PoolGroupPartitioningPolicy controls automatic partition count and placement.
type PoolGroupPartitioningPolicy struct {
	// Mode selects automatic or explicit Pool placement.
	Mode PoolGroupPartitioningMode

	// MinPartitions clamps automatic partition count from below.
	MinPartitions int

	// MaxPartitions clamps automatic partition count from above.
	MaxPartitions int

	// TargetActiveEntries is the estimated active entry count per partition.
	TargetActiveEntries int

	// EstimatedActiveClassesPerPool estimates active size classes per Pool.
	EstimatedActiveClassesPerPool int

	// EstimatedActiveShardsPerClass estimates active shards per class.
	EstimatedActiveShardsPerClass int

	// Placement selects deterministic Pool placement.
	Placement PoolPlacementPolicy
}

// DefaultPoolGroupPartitioningPolicy returns default automatic partitioning.
func DefaultPoolGroupPartitioningPolicy() PoolGroupPartitioningPolicy {
	maxPartitions := runtime.GOMAXPROCS(0) / 2
	if maxPartitions < 1 {
		maxPartitions = 1
	}
	if maxPartitions > 8 {
		maxPartitions = 8
	}
	return PoolGroupPartitioningPolicy{
		Mode:                          PoolGroupPartitioningModeAuto,
		MinPartitions:                 DefaultConfigPartitionCount,
		MaxPartitions:                 maxPartitions,
		TargetActiveEntries:           defaultPoolGroupTargetActiveEntries,
		EstimatedActiveClassesPerPool: defaultPoolGroupEstimatedActiveClassesPerPool,
		EstimatedActiveShardsPerClass: DefaultPolicyShardsPerClass(),
		Placement:                     PoolPlacementPolicyHash,
	}
}

// Normalize returns p with automatic partitioning defaults applied.
func (p PoolGroupPartitioningPolicy) Normalize() PoolGroupPartitioningPolicy {
	defaults := DefaultPoolGroupPartitioningPolicy()
	if p.Mode == PoolGroupPartitioningModeUnset {
		p.Mode = defaults.Mode
	}
	if p.MinPartitions == 0 {
		p.MinPartitions = defaults.MinPartitions
	}
	if p.MaxPartitions == 0 {
		p.MaxPartitions = defaults.MaxPartitions
	}
	if p.TargetActiveEntries == 0 {
		p.TargetActiveEntries = defaults.TargetActiveEntries
	}
	if p.EstimatedActiveClassesPerPool == 0 {
		p.EstimatedActiveClassesPerPool = defaults.EstimatedActiveClassesPerPool
	}
	if p.EstimatedActiveShardsPerClass == 0 {
		p.EstimatedActiveShardsPerClass = defaults.EstimatedActiveShardsPerClass
	}
	if p.Placement == PoolPlacementPolicyUnset {
		p.Placement = defaults.Placement
	}
	return p
}

// Validate validates partitioning policy values.
func (p PoolGroupPartitioningPolicy) Validate() error {
	p = p.Normalize()
	var err error
	if p.Mode != PoolGroupPartitioningModeAuto && p.Mode != PoolGroupPartitioningModeExplicit {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigUnknownPartitioningMode))
	}
	if p.MinPartitions <= 0 {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigInvalidMinPartitions))
	}
	if p.MaxPartitions <= 0 {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigInvalidMaxPartitions))
	}
	if p.MinPartitions > 0 && p.MaxPartitions > 0 && p.MinPartitions > p.MaxPartitions {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigInvalidPartitionBounds))
	}
	if p.TargetActiveEntries <= 0 {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigInvalidTargetActiveEntries))
	}
	if p.EstimatedActiveClassesPerPool <= 0 {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigInvalidEstimatedClassCount))
	}
	if p.EstimatedActiveShardsPerClass <= 0 {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigInvalidEstimatedShardCount))
	}
	if p.Placement != PoolPlacementPolicyHash {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigUnknownPlacementPolicy))
	}
	return err
}

// IsZero reports whether p contains no explicit partitioning settings.
func (p PoolGroupPartitioningPolicy) IsZero() bool {
	return p.Mode == PoolGroupPartitioningModeUnset &&
		p.MinPartitions == 0 &&
		p.MaxPartitions == 0 &&
		p.TargetActiveEntries == 0 &&
		p.EstimatedActiveClassesPerPool == 0 &&
		p.EstimatedActiveShardsPerClass == 0 &&
		p.Placement == PoolPlacementPolicyUnset
}

// groupPoolDirectory is the immutable routing directory owned by one PoolGroup.
type groupPoolDirectory struct {
	byPoolName map[string]groupPoolLocation
	byPool     map[*Pool]groupPoolLocation
	names      []string
}

// groupPoolLocation identifies where a group-global Pool name is owned.
type groupPoolLocation struct {
	PartitionIndex int
	PartitionName  string
	PoolName       string
}

// location looks up a group-global Pool name.
func (d groupPoolDirectory) location(poolName string) (groupPoolLocation, bool) {
	location, ok := d.byPoolName[poolName]
	return location, ok
}

// locationForLease looks up the owning Pool location from package-internal lease metadata.
func (d groupPoolDirectory) locationForLease(lease Lease) (groupPoolLocation, bool) {
	if lease.record == nil || lease.record.pool == nil {
		return groupPoolLocation{}, false
	}
	location, ok := d.byPool[lease.record.pool]
	return location, ok
}

// namesCopy returns deterministic group-global Pool names.
func (d groupPoolDirectory) namesCopy() []string { return append([]string(nil), d.names...) }

// bindRegistry adds runtime Pool pointer lookup to an assignment directory.
func (d groupPoolDirectory) bindRegistry(registry groupRegistry) (groupPoolDirectory, error) {
	bound := groupPoolDirectory{
		byPoolName: make(map[string]groupPoolLocation, len(d.byPoolName)),
		byPool:     make(map[*Pool]groupPoolLocation, len(d.byPoolName)),
		names:      append([]string(nil), d.names...),
	}
	for name, location := range d.byPoolName {
		bound.byPoolName[name] = location
	}
	for _, partitionEntry := range registry.entries {
		for _, poolEntry := range partitionEntry.partition.registry.entries {
			location, ok := bound.byPoolName[poolEntry.name]
			if !ok {
				return groupPoolDirectory{}, newError(ErrInvalidOptions, errGroupConfigInvalidPool+": "+poolEntry.name)
			}
			bound.byPool[poolEntry.pool] = location
		}
	}
	return bound, nil
}

// newGroupPartitionAssignments validates group topology and builds partition configs.
func newGroupPartitionAssignments(config PoolGroupConfig) ([]GroupPartitionConfig, groupPoolDirectory, error) {
	normalized := config.Normalize()
	if err := normalized.Partitioning.Validate(); err != nil {
		return nil, groupPoolDirectory{}, wrapError(ErrInvalidOptions, err, errGroupConfigInvalidPartitioning)
	}
	switch {
	case len(normalized.Pools) == 0 && len(normalized.Partitions) == 0:
		return nil, groupPoolDirectory{}, newError(ErrInvalidOptions, errGroupConfigNoPoolsOrPartitions)
	case len(normalized.Pools) == 0:
		return partitionsOnlyAssignments(normalized.Partitions)
	case len(normalized.Partitions) == 0:
		return automaticPoolAssignments(normalized.Pools, normalized.Partitioning)
	default:
		return explicitPoolAssignments(normalized.Pools, normalized.Partitions, normalized.Partitioning)
	}
}

// computeDefaultPartitionCount computes the automatic partition count.
func computeDefaultPartitionCount(policy PoolGroupPartitioningPolicy, poolCount int) int {
	policy = policy.Normalize()
	if poolCount <= 0 {
		return 0
	}
	activeEntries := estimateActiveEntries(policy, poolCount)
	target := uint64(policy.TargetActiveEntries)
	count64 := ceilDivUint64(activeEntries, target)
	maxInt := uint64(int(^uint(0) >> 1))
	if count64 > maxInt {
		count64 = maxInt
	}
	count := int(count64)
	if count < policy.MinPartitions {
		count = policy.MinPartitions
	}
	if count > policy.MaxPartitions {
		count = policy.MaxPartitions
	}
	return count
}

// estimateActiveEntries returns the saturated active-entry estimate.
func estimateActiveEntries(policy PoolGroupPartitioningPolicy, poolCount int) uint64 {
	policy = policy.Normalize()
	if poolCount <= 0 {
		return 0
	}
	activeClasses := poolSaturatingProduct(uint64(poolCount), uint64(policy.EstimatedActiveClassesPerPool))
	return poolSaturatingProduct(activeClasses, uint64(policy.EstimatedActiveShardsPerClass))
}

// selectPartitionIndexForPool returns the deterministic automatic partition index.
func selectPartitionIndexForPool(name string, partitionCount int) int {
	if partitionCount <= 0 {
		return 0
	}
	hash := randx.HashString64(name)
	return int(randx.BoundedIndexUint64(hash, uint64(partitionCount)))
}

// assignPoolsToPartitions assigns group-level Pools into auto-created partitions.
func assignPoolsToPartitions(pools []GroupPoolConfig, partitionCount int, placement PoolPlacementPolicy) ([]GroupPartitionConfig, groupPoolDirectory, error) {
	if partitionCount <= 0 {
		return nil, groupPoolDirectory{}, newError(ErrInvalidOptions, errGroupConfigInvalidComputedPartitions)
	}
	if placement != PoolPlacementPolicyHash {
		return nil, groupPoolDirectory{}, newError(ErrInvalidOptions, errGroupConfigUnknownPlacementPolicy)
	}
	partitions := make([]GroupPartitionConfig, partitionCount)
	for index := range partitions {
		name := automaticPartitionName(index)
		config := DefaultPoolPartitionConfig()
		config.Name = name
		partitions[index] = GroupPartitionConfig{Name: name, Config: config}
	}
	directory := newEmptyGroupPoolDirectory(len(pools))
	for _, pool := range pools {
		normalized := pool.Normalize()
		if normalized.Partition != "" {
			return nil, groupPoolDirectory{}, newError(ErrInvalidOptions, errGroupConfigAutoPoolPartitionUnsupported+": "+normalized.Name)
		}
		index := selectPartitionIndexForPool(normalized.Name, partitionCount)
		if err := appendGroupPoolToPartition(&partitions[index], normalized); err != nil {
			return nil, groupPoolDirectory{}, err
		}
		if err := directory.add(normalized.Name, groupPoolLocation{PartitionIndex: index, PartitionName: partitions[index].Name, PoolName: normalized.Name}); err != nil {
			return nil, groupPoolDirectory{}, err
		}
	}
	return partitions, directory, nil
}

func partitionsOnlyAssignments(partitions []GroupPartitionConfig) ([]GroupPartitionConfig, groupPoolDirectory, error) {
	if err := validateGroupPartitions(partitions); err != nil {
		return nil, groupPoolDirectory{}, err
	}
	assignments := cloneGroupPartitions(partitions)
	directory := newEmptyGroupPoolDirectory(countPartitionPools(assignments))
	for partitionIndex, partition := range assignments {
		for _, pool := range partition.Config.Pools {
			if err := directory.add(pool.Name, groupPoolLocation{PartitionIndex: partitionIndex, PartitionName: partition.Name, PoolName: pool.Name}); err != nil {
				return nil, groupPoolDirectory{}, err
			}
		}
	}
	return assignments, directory, nil
}

func automaticPoolAssignments(pools []GroupPoolConfig, policy PoolGroupPartitioningPolicy) ([]GroupPartitionConfig, groupPoolDirectory, error) {
	if err := validateGroupPools(pools); err != nil {
		return nil, groupPoolDirectory{}, err
	}
	if policy.Mode != PoolGroupPartitioningModeAuto {
		return nil, groupPoolDirectory{}, newError(ErrInvalidOptions, errGroupConfigExplicitPartitionsRequired)
	}
	count := computeDefaultPartitionCount(policy, len(pools))
	return assignPoolsToPartitions(pools, count, policy.Placement)
}

func explicitPoolAssignments(pools []GroupPoolConfig, partitions []GroupPartitionConfig, policy PoolGroupPartitioningPolicy) ([]GroupPartitionConfig, groupPoolDirectory, error) {
	if policy.Mode != PoolGroupPartitioningModeExplicit {
		return nil, groupPoolDirectory{}, newError(ErrInvalidOptions, errGroupConfigAmbiguousPoolsAndPartitions)
	}
	if err := validateGroupPools(pools); err != nil {
		return nil, groupPoolDirectory{}, err
	}
	if err := validateGroupPartitions(partitions); err != nil {
		return nil, groupPoolDirectory{}, err
	}
	assignments := cloneGroupPartitions(partitions)
	indexByPartition := make(map[string]int, len(assignments))
	for index, partition := range assignments {
		indexByPartition[partition.Name] = index
	}
	directory := newEmptyGroupPoolDirectory(len(pools) + countPartitionPools(assignments))
	for partitionIndex, partition := range assignments {
		for _, pool := range partition.Config.Pools {
			if err := directory.add(pool.Name, groupPoolLocation{PartitionIndex: partitionIndex, PartitionName: partition.Name, PoolName: pool.Name}); err != nil {
				return nil, groupPoolDirectory{}, err
			}
		}
	}
	for _, pool := range pools {
		normalized := pool.Normalize()
		if normalized.Partition == "" {
			return nil, groupPoolDirectory{}, newError(ErrInvalidOptions, errGroupConfigPoolPartitionRequired+": "+normalized.Name)
		}
		index, ok := indexByPartition[normalized.Partition]
		if !ok {
			return nil, groupPoolDirectory{}, newError(ErrInvalidOptions, errGroupConfigUnknownPoolPartition+": "+normalized.Partition)
		}
		if err := appendGroupPoolToPartition(&assignments[index], normalized); err != nil {
			return nil, groupPoolDirectory{}, err
		}
		if err := directory.add(normalized.Name, groupPoolLocation{PartitionIndex: index, PartitionName: assignments[index].Name, PoolName: normalized.Name}); err != nil {
			return nil, groupPoolDirectory{}, err
		}
	}
	if err := validateGroupPartitions(assignments); err != nil {
		return nil, groupPoolDirectory{}, err
	}
	return assignments, directory, nil
}

func validateGroupPools(pools []GroupPoolConfig) error {
	var err error
	seen := make(map[string]struct{}, len(pools))
	for _, pool := range pools {
		normalized := pool.Normalize()
		if normalized.Name == "" {
			multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigEmptyPoolName))
			continue
		}
		if _, exists := seen[normalized.Name]; exists {
			multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigDuplicatePool+": "+normalized.Name))
			continue
		}
		seen[normalized.Name] = struct{}{}
		if priorityErr := normalized.Priority.Validate(); priorityErr != nil {
			multierr.AppendInto(&err, priorityErr)
		}
		if poolErr := normalized.Config.Validate(); poolErr != nil {
			multierr.AppendInto(&err, wrapError(ErrInvalidOptions, poolErr, errGroupConfigInvalidPool+": "+normalized.Name))
			continue
		}
		if supportErr := validatePoolSupportedPolicy(normalized.Config.Policy); supportErr != nil {
			multierr.AppendInto(&err, wrapError(ErrInvalidOptions, supportErr, errGroupConfigInvalidPool+": "+normalized.Name))
		}
	}
	return err
}

func appendGroupPoolToPartition(partition *GroupPartitionConfig, pool GroupPoolConfig) error {
	partition.Config.Pools = append(partition.Config.Pools, PartitionPoolConfig{Name: pool.Name, Config: pool.Config})
	return nil
}

func newEmptyGroupPoolDirectory(capacity int) groupPoolDirectory {
	return groupPoolDirectory{
		byPoolName: make(map[string]groupPoolLocation, capacity),
		byPool:     make(map[*Pool]groupPoolLocation, capacity),
		names:      make([]string, 0, capacity),
	}
}

func (d *groupPoolDirectory) add(name string, location groupPoolLocation) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return newError(ErrInvalidOptions, errGroupConfigEmptyPoolName)
	}
	if _, exists := d.byPoolName[name]; exists {
		return newError(ErrInvalidOptions, errGroupConfigDuplicatePool+": "+name)
	}
	d.byPoolName[name] = location
	d.names = append(d.names, name)
	return nil
}

func cloneGroupPartitions(partitions []GroupPartitionConfig) []GroupPartitionConfig {
	assignments := make([]GroupPartitionConfig, len(partitions))
	for index, partition := range partitions {
		assignments[index] = partition.Normalize()
		assignments[index].Config = clonePartitionConfig(assignments[index].Config)
	}
	return assignments
}

func countPartitionPools(partitions []GroupPartitionConfig) int {
	count := 0
	for _, partition := range partitions {
		count += len(partition.Config.Pools)
	}
	return count
}

func automaticPartitionName(index int) string {
	return fmt.Sprintf("%s-%d", DefaultConfigPartitionNamePrefix, index)
}

func ceilDivUint64(value, divisor uint64) uint64 {
	if divisor == 0 {
		return 0
	}
	quotient := value / divisor
	if value%divisor != 0 {
		quotient++
	}
	return quotient
}
