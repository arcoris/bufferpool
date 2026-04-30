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
	"testing"
)

// TestPoolSupportedPolicyAcceptsStandaloneSubset verifies that direct Pool
// construction accepts the policy subset implemented by the raw data plane.
func TestPoolSupportedPolicyAcceptsStandaloneSubset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Policy)
	}{
		{
			name: "default policy",
		},
		{
			name: "explicit ownership none",
			mutate: func(policy *Policy) {
				policy.Ownership.Mode = OwnershipModeNone
			},
		},
		{
			name: "unset ownership with no active ownership fields",
			mutate: func(policy *Policy) {
				policy.Ownership = OwnershipPolicy{}
			},
		},
		{
			name: "pressure and trim metadata",
			mutate: func(policy *Policy) {
				policy.Pressure = DefaultPressurePolicy()
				policy.Trim = DefaultTrimPolicy()
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy := poolTestSingleShardPolicy()
			if tt.mutate != nil {
				tt.mutate(&policy)
			}

			if err := validatePoolSupportedPolicy(policy, poolConstructionModeStandalone); err != nil {
				t.Fatalf("validatePoolSupportedPolicy() returned error: %v", err)
			}

			pool := MustNew(PoolConfig{Policy: policy})
			closePoolForTest(t, pool)
		})
	}
}

// TestPoolNewRejectsUnsupportedStandalonePolicyFeatures verifies that Pool.New
// rejects policy fields that require lease ownership or unsupported return
// fallback probing.
func TestPoolNewRejectsUnsupportedStandalonePolicyFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Policy)
		message string
	}{
		{
			name: "ownership accounting",
			mutate: func(policy *Policy) {
				policy.Ownership.Mode = OwnershipModeAccounting
				policy.Ownership.MaxReturnedCapacityGrowth = PolicyRatioOne
			},
			message: errPoolUnsupportedOwnershipMode,
		},
		{
			name: "ownership strict",
			mutate: func(policy *Policy) {
				policy.Ownership.Mode = OwnershipModeStrict
				policy.Ownership.MaxReturnedCapacityGrowth = PolicyRatioOne
			},
			message: errPoolUnsupportedOwnershipMode,
		},
		{
			name: "unknown ownership mode",
			mutate: func(policy *Policy) {
				policy.Ownership.Mode = OwnershipMode(255)
			},
			message: errPoolUnsupportedOwnershipMode,
		},
		{
			name: "track in-use bytes",
			mutate: func(policy *Policy) {
				policy.Ownership.TrackInUseBytes = true
			},
			message: errPoolUnsupportedTrackInUseBytes,
		},
		{
			name: "track in-use buffers",
			mutate: func(policy *Policy) {
				policy.Ownership.TrackInUseBuffers = true
			},
			message: errPoolUnsupportedTrackInUseBuffers,
		},
		{
			name: "double-release detection",
			mutate: func(policy *Policy) {
				policy.Ownership.DetectDoubleRelease = true
			},
			message: errPoolUnsupportedDoubleReleaseDetection,
		},
		{
			name: "return fallback shards",
			mutate: func(policy *Policy) {
				policy.Shards.ReturnFallbackShards = 1
			},
			message: errPoolUnsupportedReturnFallbackShards,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy := poolTestSingleShardPolicy()
			tt.mutate(&policy)

			supportErr := validatePoolSupportedPolicy(policy, poolConstructionModeStandalone)
			if supportErr == nil {
				t.Fatal("validatePoolSupportedPolicy() returned nil")
			}
			if !errors.Is(supportErr, ErrInvalidPolicy) {
				t.Fatalf("validatePoolSupportedPolicy() error does not match ErrInvalidPolicy: %v", supportErr)
			}
			if supportErr.Error() != tt.message {
				t.Fatalf("validatePoolSupportedPolicy() error = %q, want %q", supportErr.Error(), tt.message)
			}

			_, err := New(PoolConfig{
				Policy:           policy,
				PolicyValidation: PoolPolicyValidationModeDisabled,
			})
			if err == nil {
				t.Fatal("New() returned nil")
			}
			if !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("New() error does not match ErrInvalidOptions: %v", err)
			}
			if !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("New() error does not unwrap ErrInvalidPolicy: %v", err)
			}
		})
	}
}

// TestPoolSupportedPolicyAcceptsPartitionOwnedOwnership verifies that
// partition-owned Pool construction can carry ownership-aware policy metadata.
func TestPoolSupportedPolicyAcceptsPartitionOwnedOwnership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ownership OwnershipPolicy
	}{
		{name: "accounting", ownership: AccountingOwnershipPolicy()},
		{name: "strict", ownership: StrictOwnershipPolicy()},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy := poolTestSingleShardPolicy()
			policy.Ownership = tt.ownership

			if err := validatePoolSupportedPolicy(policy, poolConstructionModePartitionOwned); err != nil {
				t.Fatalf("partition-owned validatePoolSupportedPolicy() returned error: %v", err)
			}
		})
	}
}

// TestPoolSupportedPolicyRejectsReturnFallbackInManagedMode verifies that
// construction mode does not silently enable unsupported return probing.
func TestPoolSupportedPolicyRejectsReturnFallbackInManagedMode(t *testing.T) {
	t.Parallel()

	policy := poolTestSingleShardPolicy()
	policy.Ownership = StrictOwnershipPolicy()
	policy.Shards.ReturnFallbackShards = 1

	err := validatePoolSupportedPolicy(policy, poolConstructionModePartitionOwned)
	if err == nil {
		t.Fatal("validatePoolSupportedPolicy(partition-owned) returned nil")
	}
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("error = %v, want ErrInvalidPolicy", err)
	}
	if err.Error() != errPoolUnsupportedReturnFallbackShards {
		t.Fatalf("error = %q, want %q", err.Error(), errPoolUnsupportedReturnFallbackShards)
	}
}
