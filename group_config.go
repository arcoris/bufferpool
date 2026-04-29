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

	// errGroupConfigNoPartitions rejects groups with no owned partition set.
	errGroupConfigNoPartitions = "bufferpool.PoolGroupConfig: at least one partition must be configured"

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
// group policy and one or more named PoolPartition configs. It contains no
// runtime state and does not describe background coordination.
type PoolGroupConfig struct {
	// Name is diagnostic metadata for this group.
	Name string

	// Policy defines group-level observational control behavior.
	Policy PoolGroupPolicy

	// Partitions is the deterministic set of named partitions owned by the group.
	Partitions []GroupPartitionConfig
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
// The default has no partitions because a group without an explicit partition
// set is ambiguous.
func DefaultPoolGroupConfig() PoolGroupConfig {
	return PoolGroupConfig{Name: DefaultGroupName, Policy: DefaultPoolGroupPolicy()}
}

// Normalize returns c with group defaults applied.
func (c PoolGroupConfig) Normalize() PoolGroupConfig {
	normalized := c
	normalized.Name = normalizeGroupName(normalized.Name)
	normalized.Policy = normalized.Policy.Normalize()
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
	if len(normalized.Partitions) == 0 {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errGroupConfigNoPartitions))
	}
	seen := make(map[string]struct{}, len(normalized.Partitions))
	for _, partitionConfig := range normalized.Partitions {
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
	return strings.TrimSpace(c.Name) == "" && c.Policy.IsZero() && len(c.Partitions) == 0
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
