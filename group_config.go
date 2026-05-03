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
	"strings"

	"arcoris.dev/bufferpool/internal/multierr"
)

const (
	// DefaultGroupName is the diagnostic name used when a group config does not
	// provide an explicit name.
	DefaultGroupName = "default-group"

	// errGroupConfigInvalidPolicy identifies invalid group policy input.
	errGroupConfigInvalidPolicy = "bufferpool.PoolGroupConfig: invalid group policy"

	// errGroupConfigInvalidPartitioning identifies invalid partitioning input.
	errGroupConfigInvalidPartitioning = "bufferpool.PoolGroupConfig: invalid partitioning policy"

	// errGroupConfigNoPoolsOrPartitions rejects groups with no runtime topology.
	errGroupConfigNoPoolsOrPartitions = "bufferpool.PoolGroupConfig: at least one pool or partition must be configured"

	// errGroupConfigEmptyPoolName rejects unnamed group-level Pools.
	errGroupConfigEmptyPoolName = "bufferpool.PoolGroupConfig: pool name must not be empty"

	// errGroupConfigDuplicatePool rejects duplicate group-level Pool names.
	errGroupConfigDuplicatePool = "bufferpool.PoolGroupConfig: duplicate pool name"

	// errGroupConfigInvalidPool wraps invalid group-level Pool construction config.
	errGroupConfigInvalidPool = "bufferpool.PoolGroupConfig: invalid pool config"

	// errGroupConfigEmptyPartitionName rejects unnamed group-local partitions.
	errGroupConfigEmptyPartitionName = "bufferpool.PoolGroupConfig: partition name must not be empty"

	// errGroupConfigDuplicatePartition rejects duplicate group-local partition names.
	errGroupConfigDuplicatePartition = "bufferpool.PoolGroupConfig: duplicate partition name"

	// errGroupConfigInvalidPartition wraps invalid owned partition construction config.
	errGroupConfigInvalidPartition = "bufferpool.PoolGroupConfig: invalid partition config"

	// errGroupConfigPartitionNameMismatch rejects divergent group and partition names.
	errGroupConfigPartitionNameMismatch = "bufferpool.PoolGroupConfig: partition config name must match group partition name"
)

// PoolGroupConfig configures one PoolGroup.
//
// PoolGroupConfig is the construction boundary above PoolPartition. It composes
// group policy, partitioning policy, group-level Pool configs, and optional
// explicit PoolPartition configs. It contains no runtime state; scheduler
// fields are construction policy that NewPoolGroup translates into owner-local
// runtime only after validation and full initialization.
type PoolGroupConfig struct {
	// Name is diagnostic metadata for this group.
	Name string

	// Policy defines group-level control behavior.
	Policy PoolGroupPolicy

	// Partitioning controls automatic partition count and Pool placement when
	// Pools is used as the managed construction input.
	Partitioning PoolGroupPartitioningPolicy

	// Pools is the normal managed-mode input. PoolGroup assigns these Pools to
	// owned PoolPartitions and builds a pool-name routing directory.
	Pools []GroupPoolConfig

	// Partitions is the deterministic set of explicitly configured partitions.
	// It remains available for advanced layouts and for explicit per-Pool
	// placement when Partitioning.Mode is PoolGroupPartitioningModeExplicit.
	Partitions []GroupPartitionConfig
}

// GroupPoolConfig configures one Pool owned by a PoolGroup.
type GroupPoolConfig struct {
	// Name is the group-global Pool name used for managed routing.
	Name string

	// Config is the Pool construction config. If Config.Name is empty,
	// normalization sets it to Name.
	Config PoolConfig

	// Partition is the group-local partition name for explicit placement.
	Partition string

	// Priority is metadata reserved for later budget allocation.
	Priority PoolPriority
}

// GroupPartitionConfig configures one PoolPartition owned by a PoolGroup.
type GroupPartitionConfig struct {
	// Name is the group-local partition name.
	Name string

	// Config is the PoolPartition construction config. If Config.Name is empty,
	// normalization sets it to Name.
	Config PoolPartitionConfig
}

// DefaultPoolGroupConfig returns the package default group config.
//
// The default has no pools or partitions because a group without runtime
// topology is invalid. Supplying Pools uses automatic partitioning by default.
func DefaultPoolGroupConfig() PoolGroupConfig {
	return PoolGroupConfig{Name: DefaultGroupName, Policy: DefaultPoolGroupPolicy(), Partitioning: DefaultPoolGroupPartitioningPolicy()}
}

// Normalize returns c with group defaults applied.
func (c PoolGroupConfig) Normalize() PoolGroupConfig {
	normalized := c
	normalized.Name = normalizeGroupName(normalized.Name)
	normalized.Policy = normalized.Policy.Normalize()
	normalized.Partitioning = normalized.Partitioning.Normalize()

	if len(normalized.Pools) > 0 {
		pools := make([]GroupPoolConfig, len(normalized.Pools))
		for index, poolConfig := range normalized.Pools {
			pools[index] = poolConfig.Normalize()
		}
		normalized.Pools = pools
	}

	if len(normalized.Partitions) > 0 {
		partitions := make([]GroupPartitionConfig, len(normalized.Partitions))
		for index, partitionConfig := range normalized.Partitions {
			partitions[index] = partitionConfig.Normalize()
		}
		normalized.Partitions = partitions
	}

	return normalized
}

// Validate validates normalized group construction configuration.
func (c PoolGroupConfig) Validate() error {
	normalized := c.Normalize()
	var err error

	if policyErr := normalized.Policy.Validate(); policyErr != nil {
		multierr.AppendInto(&err, wrapError(ErrInvalidOptions, policyErr, errGroupConfigInvalidPolicy))
	}
	if _, _, assignmentErr := newGroupPartitionAssignments(normalized); assignmentErr != nil {
		multierr.AppendInto(&err, assignmentErr)
	}

	return err
}

// validateGroupPartitions validates explicit partition config structure.
func validateGroupPartitions(partitions []GroupPartitionConfig) error {
	var err error
	seen := make(map[string]struct{}, len(partitions))

	for _, partitionConfig := range partitions {
		if partitionConfig.Name == "" {
			multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigEmptyPartitionName))
			continue
		}
		if _, exists := seen[partitionConfig.Name]; exists {
			multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigDuplicatePartition+": "+partitionConfig.Name))
			continue
		}
		seen[partitionConfig.Name] = struct{}{}

		if nameErr := validateGroupPartitionNameMatch(partitionConfig); nameErr != nil {
			multierr.AppendInto(&err, nameErr)
			continue
		}
		if partitionErr := partitionConfig.Config.Validate(); partitionErr != nil {
			multierr.AppendInto(&err, wrapError(ErrInvalidOptions, partitionErr, errGroupConfigInvalidPartition+": "+partitionConfig.Name))
		}
	}

	return err
}

// IsZero reports whether c contains no explicit construction settings.
func (c PoolGroupConfig) IsZero() bool {
	return strings.TrimSpace(c.Name) == "" &&
		c.Policy.IsZero() &&
		c.Partitioning.IsZero() &&
		len(c.Pools) == 0 &&
		len(c.Partitions) == 0
}

// Normalize returns p with defaults applied.
func (p GroupPoolConfig) Normalize() GroupPoolConfig {
	normalized := p
	normalized.Name = strings.TrimSpace(normalized.Name)
	normalized.Partition = strings.TrimSpace(normalized.Partition)
	normalized.Priority = normalized.Priority.Normalize()

	poolConfig := normalized.Config
	if normalized.Name != "" && isPoolConfigNameUnset(poolConfig.Name) {
		poolConfig.Name = normalized.Name
	}
	normalized.Config = poolConfig.Normalize()

	return normalized
}

// Normalize returns p with defaults applied.
func (p GroupPartitionConfig) Normalize() GroupPartitionConfig {
	normalized := p
	normalized.Name = strings.TrimSpace(normalized.Name)

	partitionConfig := normalized.Config
	partitionName := strings.TrimSpace(partitionConfig.Name)
	if normalized.Name != "" && (partitionName == "" || partitionName == DefaultPartitionName) {
		partitionConfig.Name = normalized.Name
	}
	normalized.Config = partitionConfig.Normalize()

	return normalized
}

// validateGroupPartitionNameMatch rejects divergent group-local and diagnostic names.
func validateGroupPartitionNameMatch(config GroupPartitionConfig) error {
	if config.Name == "" || config.Config.Name == "" || config.Config.Name == config.Name {
		return nil
	}
	return newError(ErrInvalidOptions, errGroupConfigPartitionNameMismatch+": "+config.Name+" != "+config.Config.Name)
}

// normalizeGroupName applies the group diagnostic-name default.
func normalizeGroupName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultGroupName
	}
	return name
}

// cloneGroupConfig returns a caller-owned copy of normalized group config.
func cloneGroupConfig(config PoolGroupConfig) PoolGroupConfig {
	config.Policy = config.Policy.Normalize()
	config.Partitioning = config.Partitioning.Normalize()

	if len(config.Pools) > 0 {
		pools := make([]GroupPoolConfig, len(config.Pools))
		for index, poolConfig := range config.Pools {
			pools[index] = poolConfig
			pools[index].Config.Policy = clonePoolPolicy(poolConfig.Config.Policy)
		}
		config.Pools = pools
	}

	if len(config.Partitions) > 0 {
		partitions := make([]GroupPartitionConfig, len(config.Partitions))
		for index, partitionConfig := range config.Partitions {
			partitions[index] = partitionConfig
			partitions[index].Config = clonePartitionConfig(partitionConfig.Config)
		}
		config.Partitions = partitions
	}

	return config
}
