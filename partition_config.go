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
	// DefaultPartitionName is the diagnostic name used when a partition config
	// does not provide an explicit name.
	DefaultPartitionName = "default-partition"

	// errPartitionConfigInvalidPolicy identifies invalid partition policy input.
	errPartitionConfigInvalidPolicy = "bufferpool.PoolPartitionConfig: invalid partition policy"

	// errPartitionConfigInvalidLease identifies invalid LeaseRegistry config input.
	errPartitionConfigInvalidLease = "bufferpool.PoolPartitionConfig: invalid lease registry config"

	// errPartitionConfigNoPools rejects partitions with no owned Pool set.
	errPartitionConfigNoPools = "bufferpool.PoolPartitionConfig: at least one pool must be configured"

	// errPartitionConfigEmptyPoolName rejects unnamed partition-local Pools.
	errPartitionConfigEmptyPoolName = "bufferpool.PoolPartitionConfig: pool name must not be empty"

	// errPartitionConfigDuplicatePool rejects duplicate partition-local Pool names.
	errPartitionConfigDuplicatePool = "bufferpool.PoolPartitionConfig: duplicate pool name"

	// errPartitionConfigInvalidPool wraps invalid owned Pool construction config.
	errPartitionConfigInvalidPool = "bufferpool.PoolPartitionConfig: invalid pool config"
)

// PoolPartitionConfig configures one PoolPartition.
//
// PoolPartitionConfig is the construction boundary for the first owner above
// Pool. It composes partition policy, a LeaseRegistry config, and one or more
// named Pool configs. It contains no runtime state.
type PoolPartitionConfig struct {
	// Name is diagnostic metadata for this partition.
	Name string

	// Policy defines partition-level control-plane behavior.
	Policy PartitionPolicy

	// Lease configures the partition-owned LeaseRegistry.
	Lease LeaseConfig

	// Pools is the deterministic set of named Pools owned by the partition.
	Pools []PartitionPoolConfig
}

// PartitionPoolConfig configures one Pool owned by a PoolPartition.
type PartitionPoolConfig struct {
	// Name is the partition-local pool name.
	Name string

	// Config is the Pool construction config. If Config.Name is empty,
	// normalization sets it to Name.
	Config PoolConfig
}

// DefaultPoolPartitionConfig returns the package default partition config.
//
// The default has no pools because a partition without an explicit pool set is
// ambiguous. Callers should add at least one named pool before construction.
func DefaultPoolPartitionConfig() PoolPartitionConfig {
	return PoolPartitionConfig{Name: DefaultPartitionName, Policy: DefaultPartitionPolicy(), Lease: DefaultLeaseConfig()}
}

// Normalize returns c with partition defaults applied.
func (c PoolPartitionConfig) Normalize() PoolPartitionConfig {
	normalized := c
	normalized.Name = normalizePartitionName(normalized.Name)
	normalized.Policy = normalized.Policy.Normalize()
	normalized.Lease = normalized.Lease.Normalize()
	if len(normalized.Pools) > 0 {
		pools := make([]PartitionPoolConfig, len(normalized.Pools))
		for index, poolConfig := range normalized.Pools {
			pools[index] = poolConfig.Normalize()
		}
		normalized.Pools = pools
	}
	return normalized
}

// Validate validates normalized partition construction configuration.
func (c PoolPartitionConfig) Validate() error {
	normalized := c.Normalize()
	var err error
	if policyErr := normalized.Policy.Validate(); policyErr != nil {
		multierr.AppendInto(&err, wrapError(ErrInvalidOptions, policyErr, errPartitionConfigInvalidPolicy))
	}
	if leaseErr := normalized.Lease.Validate(); leaseErr != nil {
		multierr.AppendInto(&err, wrapError(ErrInvalidOptions, leaseErr, errPartitionConfigInvalidLease))
	}
	if len(normalized.Pools) == 0 {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errPartitionConfigNoPools))
	}
	seen := make(map[string]struct{}, len(normalized.Pools))
	for _, poolConfig := range normalized.Pools {
		if poolConfig.Name == "" {
			multierr.AppendInto(&err, newError(ErrInvalidOptions, errPartitionConfigEmptyPoolName))
			continue
		}
		if _, exists := seen[poolConfig.Name]; exists {
			multierr.AppendInto(&err, newError(ErrInvalidOptions, errPartitionConfigDuplicatePool+": "+poolConfig.Name))
			continue
		}
		seen[poolConfig.Name] = struct{}{}
		normalizedPool := poolConfig.Config.Normalize()
		if poolErr := normalizedPool.Validate(); poolErr != nil {
			multierr.AppendInto(&err, wrapError(ErrInvalidOptions, poolErr, errPartitionConfigInvalidPool+": "+poolConfig.Name))
			continue
		}
		if supportErr := validatePoolSupportedPolicy(normalizedPool.Policy); supportErr != nil {
			multierr.AppendInto(&err, wrapError(ErrInvalidOptions, supportErr, errPartitionConfigInvalidPool+": "+poolConfig.Name))
		}
	}
	return err
}

// IsZero reports whether c contains no explicit construction settings.
func (c PoolPartitionConfig) IsZero() bool {
	return strings.TrimSpace(c.Name) == "" && c.Policy.IsZero() && c.Lease.IsZero() && len(c.Pools) == 0
}

// Normalize returns p with defaults applied.
func (p PartitionPoolConfig) Normalize() PartitionPoolConfig {
	normalized := p
	normalized.Name = strings.TrimSpace(normalized.Name)

	poolConfig := normalized.Config
	if normalized.Name != "" && isPoolConfigNameUnset(poolConfig.Name) {
		poolConfig.Name = normalized.Name
	}
	normalized.Config = poolConfig.Normalize()

	return normalized
}

// normalizePartitionName applies the partition diagnostic-name default.
func normalizePartitionName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultPartitionName
	}
	return name
}

// clonePartitionConfig returns a caller-owned copy of normalized partition config.
func clonePartitionConfig(config PoolPartitionConfig) PoolPartitionConfig {
	config.Policy = config.Policy.Normalize()
	config.Lease = config.Lease.Normalize()
	if len(config.Pools) > 0 {
		pools := make([]PartitionPoolConfig, len(config.Pools))
		for index, poolConfig := range config.Pools {
			pools[index] = poolConfig
			pools[index].Config.Policy = clonePoolPolicy(poolConfig.Config.Policy)
		}
		config.Pools = pools
	}
	return config
}
