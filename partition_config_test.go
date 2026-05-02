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
	"errors"
	"reflect"
	"testing"
)

// TestDefaultPoolPartitionConfigAllowsEmptyPools verifies default config shape.
func TestDefaultPoolPartitionConfigAllowsEmptyPools(t *testing.T) {
	config := DefaultPoolPartitionConfig()

	if config.Name != DefaultPartitionName {
		t.Fatalf("default name = %q, want %q", config.Name, DefaultPartitionName)
	}
	if !config.Policy.IsZero() {
		t.Fatalf("default partition policy should be zero/default explicit-control policy")
	}
	if config.Lease.IsZero() {
		t.Fatalf("default lease config must be explicit and lease-aware")
	}

	requirePartitionNoError(t, config.Validate())
}

// TestPoolPartitionConfigNormalizeCompletesDefaultsAndCopiesPools verifies normalization copies caller slices.
func TestPoolPartitionConfigNormalizeCompletesDefaultsAndCopiesPools(t *testing.T) {
	config := PoolPartitionConfig{
		Name: "  partition-a  ",
		Policy: PartitionPolicy{
			Trim: PartitionTrimPolicy{Enabled: true, MaxBytesPerCycle: 4 * KiB},
		},
		Pools: []PartitionPoolConfig{{Name: " primary ", Config: PoolConfig{Name: " explicit-pool "}}},
	}

	normalized := config.Normalize()

	if normalized.Name != "partition-a" {
		t.Fatalf("normalized partition name = %q, want partition-a", normalized.Name)
	}
	if normalized.Policy.Controller.Enabled || normalized.Policy.Controller.TickInterval != 0 {
		t.Fatalf("controller scheduler policy = %+v, want unsupported fields left disabled", normalized.Policy.Controller)
	}
	if normalized.Policy.Trim.MaxPoolsPerCycle != defaultPartitionTrimMaxPoolsPerCycle {
		t.Fatalf("trim max pools per cycle = %d, want %d", normalized.Policy.Trim.MaxPoolsPerCycle, defaultPartitionTrimMaxPoolsPerCycle)
	}
	if normalized.Pools[0].Name != "primary" {
		t.Fatalf("normalized pool name = %q, want primary", normalized.Pools[0].Name)
	}
	if normalized.Pools[0].Config.Name != "explicit-pool" {
		t.Fatalf("explicit pool config name = %q, want explicit-pool", normalized.Pools[0].Config.Name)
	}

	config.Pools[0].Name = "mutated"
	if normalized.Pools[0].Name != "primary" {
		t.Fatalf("Normalize must not share Pool slice with caller")
	}
}

// TestPartitionPoolConfigNormalizeUsesPartitionPoolNameWhenPoolConfigNameUnset verifies partition-local name precedence.
func TestPartitionPoolConfigNormalizeUsesPartitionPoolNameWhenPoolConfigNameUnset(t *testing.T) {
	config := PartitionPoolConfig{Name: " primary "}

	normalized := config.Normalize()

	if normalized.Name != "primary" {
		t.Fatalf("normalized partition-local name = %q, want primary", normalized.Name)
	}
	if normalized.Config.Name != "primary" {
		t.Fatalf("normalized PoolConfig.Name = %q, want partition-local name primary", normalized.Config.Name)
	}
}

// TestPoolPartitionConfigValidateAcceptsMinimalPartition verifies minimal valid config.
func TestPoolPartitionConfigValidateAcceptsMinimalPartition(t *testing.T) {
	err := testPartitionConfig("primary", "secondary").Validate()
	requirePartitionNoError(t, err)
}

// TestPoolPartitionConfigValidateAcceptsManagedPoolOwnership verifies that
// partition-owned Pools may carry ownership-aware policy metadata.
func TestPoolPartitionConfigValidateAcceptsManagedPoolOwnership(t *testing.T) {
	strictPoolPolicy := DefaultConfigPolicy()
	strictPoolPolicy.Ownership = StrictOwnershipPolicy()

	config := PoolPartitionConfig{
		Pools: []PartitionPoolConfig{{Name: "primary", Config: PoolConfig{Name: "primary", Policy: strictPoolPolicy}}},
	}

	requirePartitionNoError(t, config.Validate())
}

// TestPoolPartitionConfigValidateRejectsInvalidInputs verifies construction validation errors.
func TestPoolPartitionConfigValidateRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		config PoolPartitionConfig
		want   error
	}{
		{
			name: "empty pool name",
			config: PoolPartitionConfig{
				Pools: []PartitionPoolConfig{{Name: "   ", Config: PoolConfig{Name: "unused"}}},
			},
			want: ErrInvalidOptions,
		},
		{
			name: "duplicate pool name after trim",
			config: PoolPartitionConfig{
				Pools: []PartitionPoolConfig{testPartitionPoolConfig("primary"), testPartitionPoolConfig(" primary ")},
			},
			want: ErrInvalidOptions,
		},
		{
			name: "invalid partition budget",
			config: PoolPartitionConfig{
				Policy: PartitionPolicy{Budget: PartitionBudgetPolicy{MaxRetainedBytes: 2 * MiB, MaxOwnedBytes: MiB}},
				Pools:  []PartitionPoolConfig{testPartitionPoolConfig("primary")},
			},
			want: ErrInvalidPolicy,
		},
		{
			name: "invalid lease config",
			config: PoolPartitionConfig{
				Lease: LeaseConfigFromOwnershipPolicy(OwnershipPolicy{Mode: OwnershipModeNone}),
				Pools: []PartitionPoolConfig{testPartitionPoolConfig("primary")},
			},
			want: ErrInvalidPolicy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil, want %v", tt.want)
			}
			if !errors.Is(err, ErrInvalidOptions) && tt.want == ErrInvalidOptions {
				t.Fatalf("Validate() error = %v, want ErrInvalidOptions", err)
			}
			if tt.want != ErrInvalidOptions && !errors.Is(err, tt.want) {
				t.Fatalf("Validate() error = %v, want errors.Is(..., %v)", err, tt.want)
			}
		})
	}
}

// TestPoolPartitionConfigAccessorsReturnDefensiveCopies verifies config copy boundaries.
func TestPoolPartitionConfigAccessorsReturnDefensiveCopies(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	config := partition.Config()
	if len(config.Pools) != 1 {
		t.Fatalf("Config().Pools length = %d, want 1", len(config.Pools))
	}

	config.Pools[0].Name = "mutated"
	if got := partition.Config().Pools[0].Name; got != "primary" {
		t.Fatalf("partition config shared mutable pool slice, got name %q", got)
	}

	if len(config.Pools[0].Config.Policy.Classes.Sizes) == 0 {
		t.Fatalf("test requires configured class sizes")
	}
	config.Pools[0].Config.Policy.Classes.Sizes[0] = ClassSize(12345)

	fresh := partition.Config()
	if reflect.DeepEqual(config.Pools[0].Config.Policy.Classes.Sizes, fresh.Pools[0].Config.Policy.Classes.Sizes) {
		t.Fatalf("partition config shared mutable policy class sizes")
	}
}
