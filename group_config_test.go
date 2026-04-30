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

import "testing"

func TestPoolGroupConfigNormalize(t *testing.T) {
	config := PoolGroupConfig{
		Partitions: []GroupPartitionConfig{{
			Name: "alpha",
			Config: PoolPartitionConfig{
				Pools: []PartitionPoolConfig{testGroupPoolConfig("pool")},
			},
		}},
	}
	normalized := config.Normalize()
	if normalized.Name != DefaultGroupName {
		t.Fatalf("Name = %q, want %q", normalized.Name, DefaultGroupName)
	}
	if normalized.Partitions[0].Config.Name != "alpha" {
		t.Fatalf("partition Config.Name = %q, want alpha", normalized.Partitions[0].Config.Name)
	}
	if config.Partitions[0].Config.Name != "" {
		t.Fatalf("Normalize mutated caller config")
	}
}

// TestPoolGroupPartitionConfigNormalizeReplacesDefaultPartitionName verifies naming.
func TestPoolGroupPartitionConfigNormalizeReplacesDefaultPartitionName(t *testing.T) {
	config := GroupPartitionConfig{
		Name: "alpha",
		Config: PoolPartitionConfig{
			Name:  DefaultPartitionName,
			Pools: []PartitionPoolConfig{testGroupPoolConfig("pool")},
		},
	}
	normalized := config.Normalize()
	if normalized.Config.Name != "alpha" {
		t.Fatalf("Config.Name = %q, want alpha", normalized.Config.Name)
	}
}

func TestPoolGroupConfigValidateRejectsEmptyTopology(t *testing.T) {
	err := DefaultPoolGroupConfig().Validate()
	requireGroupErrorIs(t, err, ErrInvalidOptions)
}

func TestPoolGroupConfigValidateAcceptsGroupLevelPools(t *testing.T) {
	config := testManagedGroupConfig("api", "worker")
	requireGroupNoError(t, config.Validate())
}

func TestPoolGroupConfigValidateRejectsDuplicatePools(t *testing.T) {
	config := testManagedGroupConfig("api", "api")
	err := config.Validate()
	requireGroupErrorIs(t, err, ErrInvalidOptions)
}

func TestPoolGroupConfigValidateRejectsEmptyPoolName(t *testing.T) {
	config := testManagedGroupConfig("api")
	config.Pools[0].Name = "   "
	err := config.Validate()
	requireGroupErrorIs(t, err, ErrInvalidOptions)
}

func TestPoolGroupConfigValidateRejectsDuplicatePartitions(t *testing.T) {
	config := testGroupConfig("alpha", "alpha")
	err := config.Validate()
	requireGroupErrorIs(t, err, ErrInvalidOptions)
}

func TestPoolGroupConfigValidateRejectsInvalidPartition(t *testing.T) {
	config := DefaultPoolGroupConfig()
	config.Partitions = []GroupPartitionConfig{{
		Name: "broken",
		Config: PoolPartitionConfig{
			Name:  "broken",
			Pools: []PartitionPoolConfig{{Name: "   ", Config: PoolConfig{Name: "unused"}}},
		},
	}}
	err := config.Validate()
	requireGroupErrorIs(t, err, ErrInvalidOptions)
}

// TestPoolGroupConfigValidateRejectsPartitionNameMismatch verifies strict names.
func TestPoolGroupConfigValidateRejectsPartitionNameMismatch(t *testing.T) {
	config := testGroupConfig("alpha")
	config.Partitions[0].Config.Name = "backend"

	err := config.Validate()
	requireGroupErrorIs(t, err, ErrInvalidOptions)
}

// TestPoolGroupConfigValidateAcceptsMatchingPartitionName verifies explicit match.
func TestPoolGroupConfigValidateAcceptsMatchingPartitionName(t *testing.T) {
	config := testGroupConfig("alpha")
	config.Partitions[0].Config.Name = "alpha"

	requireGroupNoError(t, config.Validate())
}

func TestCloneGroupConfigDefensiveCopy(t *testing.T) {
	config := testGroupConfig("alpha", "beta").Normalize()
	config.Pools = []GroupPoolConfig{testManagedGroupPoolConfig("managed")}
	cloned := cloneGroupConfig(config)
	cloned.Pools[0].Name = "changed-managed"
	cloned.Partitions[0].Name = "changed"
	cloned.Partitions[0].Config.Pools[0].Name = "changed-pool"

	if config.Pools[0].Name != "managed" {
		t.Fatalf("clone mutated source managed pool name")
	}
	if config.Partitions[0].Name != "alpha" {
		t.Fatalf("clone mutated source partition name")
	}
	if config.Partitions[0].Config.Pools[0].Name != "alpha-pool" {
		t.Fatalf("clone mutated source pool name")
	}
}
