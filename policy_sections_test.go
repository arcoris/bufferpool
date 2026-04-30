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

// TestPolicySectionProjection verifies that Policy can be viewed as separate
// construction-shape and runtime-retention sections without sharing mutable
// class-size storage with the original policy value.
func TestPolicySectionProjection(t *testing.T) {
	t.Parallel()

	policy := DefaultPolicy()
	shape := policy.ShapePolicy()
	retention := policy.RetentionPolicy()

	if err := shape.Validate(); err != nil {
		t.Fatalf("PoolShapePolicy.Validate() returned error: %v", err)
	}
	if err := retention.Validate(); err != nil {
		t.Fatalf("PoolRetentionPolicy.Validate() returned error: %v", err)
	}

	rebuilt := NewPolicyFromSections(shape, retention, policy.Ownership)
	if !reflect.DeepEqual(rebuilt, policy) {
		t.Fatal("NewPolicyFromSections() did not rebuild DefaultPolicy()")
	}

	shape.Classes.Sizes[0] = ClassSizeFromSize(8 * MiB)
	if policy.Classes.Sizes[0] == shape.Classes.Sizes[0] {
		t.Fatal("Policy.ShapePolicy() shared class-size storage with original Policy")
	}
	if rebuilt.Classes.Sizes[0] == shape.Classes.Sizes[0] {
		t.Fatal("NewPolicyFromSections() shared class-size storage with input shape")
	}
}

// TestPolicySectionValidationRejectsZeroSections verifies that split policy
// sections still reject empty construction and retention input.
func TestPolicySectionValidationRejectsZeroSections(t *testing.T) {
	t.Parallel()

	if err := (PoolShapePolicy{}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("PoolShapePolicy{}.Validate() = %v, want ErrInvalidPolicy", err)
	}
	if err := (PoolRetentionPolicy{}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("PoolRetentionPolicy{}.Validate() = %v, want ErrInvalidPolicy", err)
	}
}

// TestValidatePolicyForContext verifies that context-aware validation preserves
// the raw Pool versus managed ownership boundary.
func TestValidatePolicyForContext(t *testing.T) {
	t.Parallel()

	if err := ValidatePolicyForContext(DefaultPolicy(), PolicyValidationContextStandalonePool); err != nil {
		t.Fatalf("ValidatePolicyForContext(default, standalone) returned error: %v", err)
	}

	managedPolicy := DefaultPolicy()
	managedPolicy.Ownership = StrictOwnershipPolicy()

	if err := ValidatePolicyForContext(managedPolicy, PolicyValidationContextStandalonePool); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("ValidatePolicyForContext(strict, standalone) = %v, want ErrInvalidPolicy", err)
	}

	managedContexts := []PolicyValidationContext{
		PolicyValidationContextPartitionOwnedPool,
		PolicyValidationContextPoolPartition,
		PolicyValidationContextPoolGroup,
	}
	for _, context := range managedContexts {
		context := context

		t.Run(context.String(), func(t *testing.T) {
			t.Parallel()

			if err := ValidatePolicyForContext(managedPolicy, context); err != nil {
				t.Fatalf("ValidatePolicyForContext(strict, %s) returned error: %v", context, err)
			}
		})
	}
}

// TestValidatePolicyForContextRejectsUnknownContext keeps context validation
// fail-closed when callers forget to choose an owner.
func TestValidatePolicyForContextRejectsUnknownContext(t *testing.T) {
	t.Parallel()

	err := ValidatePolicyForContext(DefaultPolicy(), PolicyValidationContextUnset)
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("ValidatePolicyForContext(unset) = %v, want ErrInvalidPolicy", err)
	}
	if err == nil || err.Error() != errPolicyUnsupportedValidationContext {
		t.Fatalf("ValidatePolicyForContext(unset) = %v, want %q", err, errPolicyUnsupportedValidationContext)
	}
}

// TestValidatePolicyForContextRejectsUnsupportedReturnFallback verifies that
// context validation does not silently enable unsupported put-side probing.
func TestValidatePolicyForContextRejectsUnsupportedReturnFallback(t *testing.T) {
	t.Parallel()

	policy := DefaultPolicy()
	policy.Shards.ReturnFallbackShards = 1

	err := ValidatePolicyForContext(policy, PolicyValidationContextPoolGroup)
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("ValidatePolicyForContext(return fallback, pool group) = %v, want ErrInvalidPolicy", err)
	}
}

// TestValidatePolicyForContextRejectsAffinityWithoutKey verifies that affinity
// selection remains fail-closed until managed operations can pass an explicit
// affinity key to Pool shard routing.
func TestValidatePolicyForContextRejectsAffinityWithoutKey(t *testing.T) {
	t.Parallel()

	policy := DefaultPolicy()
	policy.Shards.Selection = ShardSelectionModeAffinity

	for _, context := range []PolicyValidationContext{
		PolicyValidationContextStandalonePool,
		PolicyValidationContextPartitionOwnedPool,
		PolicyValidationContextPoolPartition,
		PolicyValidationContextPoolGroup,
	} {
		context := context
		t.Run(context.String(), func(t *testing.T) {
			t.Parallel()

			err := ValidatePolicyForContext(policy, context)
			if !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("ValidatePolicyForContext(affinity, %s) = %v, want ErrInvalidPolicy", context, err)
			}
		})
	}
}
