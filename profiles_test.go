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

// TestPolicyProfileString verifies stable diagnostic labels for named policy
// profiles.
//
// These strings may appear in logs, validation messages, configuration parsing,
// CLI output, examples, and tests. Unknown values must not panic because profile
// values may be constructed from external numeric configuration.
func TestPolicyProfileString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile PolicyProfile
		want    string
	}{
		{
			name:    "unset",
			profile: PolicyProfileUnset,
			want:    "unset",
		},
		{
			name:    "default",
			profile: PolicyProfileDefault,
			want:    "default",
		},
		{
			name:    "throughput",
			profile: PolicyProfileThroughput,
			want:    "throughput",
		},
		{
			name:    "memory constrained",
			profile: PolicyProfileMemoryConstrained,
			want:    "memory_constrained",
		},
		{
			name:    "bursty",
			profile: PolicyProfileBursty,
			want:    "bursty",
		},
		{
			name:    "strict bounded",
			profile: PolicyProfileStrictBounded,
			want:    "strict_bounded",
		},
		{
			name:    "secure",
			profile: PolicyProfileSecure,
			want:    "secure",
		},
		{
			name:    "unknown",
			profile: PolicyProfile(255),
			want:    "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.profile.String(); got != tt.want {
				t.Fatalf("PolicyProfile.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPolicyForProfile verifies profile selection semantics.
//
// PolicyProfileUnset means "no profile selected" and must not silently resolve
// to DefaultPolicy. This keeps absence of configuration distinct from an
// explicit default-profile choice.
func TestPolicyForProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile PolicyProfile
		wantOK  bool
	}{
		{
			name:    "unset",
			profile: PolicyProfileUnset,
			wantOK:  false,
		},
		{
			name:    "default",
			profile: PolicyProfileDefault,
			wantOK:  true,
		},
		{
			name:    "throughput",
			profile: PolicyProfileThroughput,
			wantOK:  true,
		},
		{
			name:    "memory constrained",
			profile: PolicyProfileMemoryConstrained,
			wantOK:  true,
		},
		{
			name:    "bursty",
			profile: PolicyProfileBursty,
			wantOK:  true,
		},
		{
			name:    "strict bounded",
			profile: PolicyProfileStrictBounded,
			wantOK:  true,
		},
		{
			name:    "secure",
			profile: PolicyProfileSecure,
			wantOK:  true,
		},
		{
			name:    "unknown",
			profile: PolicyProfile(255),
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy, ok := PolicyForProfile(tt.profile)
			if ok != tt.wantOK {
				t.Fatalf("PolicyForProfile(%s) ok = %v, want %v", tt.profile, ok, tt.wantOK)
			}

			if !ok {
				if !policy.IsZero() {
					t.Fatalf("PolicyForProfile(%s) returned non-zero policy for non-selectable profile", tt.profile)
				}

				return
			}

			if policy.IsZero() {
				t.Fatalf("PolicyForProfile(%s) returned zero policy", tt.profile)
			}

			if err := policy.Validate(); err != nil {
				t.Fatalf("PolicyForProfile(%s).Validate() returned error: %v", tt.profile, err)
			}
		})
	}
}

// TestPolicyForProfileMatchesProfileConstructors verifies that profile lookup is
// a thin mapping to the documented constructor for each selectable profile.
//
// This protects the named profile registry from silently returning the wrong
// preset while individual constructor tests still verify each policy's shape.
func TestPolicyForProfileMatchesProfileConstructors(t *testing.T) {
	t.Parallel()

	for _, tt := range policyProfileFactories() {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := PolicyForProfile(tt.profile)
			if !ok {
				t.Fatalf("PolicyForProfile(%s) returned ok=false", tt.profile)
			}

			want := tt.policy()
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("PolicyForProfile(%s) returned a policy different from %s constructor", tt.profile, tt.name)
			}
		})
	}
}

// TestPolicyProfilePolicy verifies that the enum method delegates to the package
// selection function without changing semantics.
func TestPolicyProfilePolicy(t *testing.T) {
	t.Parallel()

	tests := []PolicyProfile{
		PolicyProfileUnset,
		PolicyProfileDefault,
		PolicyProfileThroughput,
		PolicyProfileMemoryConstrained,
		PolicyProfileBursty,
		PolicyProfileStrictBounded,
		PolicyProfileSecure,
		PolicyProfile(255),
	}

	for _, profile := range tests {
		profile := profile

		t.Run(profile.String(), func(t *testing.T) {
			t.Parallel()

			fromMethod, methodOK := profile.Policy()
			fromFunction, functionOK := PolicyForProfile(profile)

			if methodOK != functionOK {
				t.Fatalf("PolicyProfile.Policy() ok = %v, want %v", methodOK, functionOK)
			}

			if !reflect.DeepEqual(fromMethod, fromFunction) {
				t.Fatalf("PolicyProfile.Policy() returned different policy than PolicyForProfile")
			}
		})
	}
}

// TestMustPolicyForProfile verifies panic behavior for programmer-error profile
// selection.
//
// MustPolicyForProfile is appropriate for examples, tests, and static internal
// construction paths. User-facing configuration should prefer PolicyForProfile
// so invalid input can be reported as an error.
func TestMustPolicyForProfile(t *testing.T) {
	t.Parallel()

	t.Run("selectable profiles", func(t *testing.T) {
		t.Parallel()

		for _, profile := range selectablePolicyProfiles() {
			profile := profile

			t.Run(profile.String(), func(t *testing.T) {
				t.Parallel()

				policy := MustPolicyForProfile(profile)
				if policy.IsZero() {
					t.Fatalf("MustPolicyForProfile(%s) returned zero policy", profile)
				}
			})
		}
	})

	t.Run("unset panics", func(t *testing.T) {
		t.Parallel()

		assertPolicyProfilePanic(t, func() {
			_ = MustPolicyForProfile(PolicyProfileUnset)
		})
	})

	t.Run("unknown panics", func(t *testing.T) {
		t.Parallel()

		assertPolicyProfilePanic(t, func() {
			_ = MustPolicyForProfile(PolicyProfile(255))
		})
	})
}

// TestPolicyProfilesValidate verifies that every named profile produces a
// complete policy accepted by policy validation.
//
// This test is intentionally broad. Profiles are public presets, so a profile
// that cannot pass Validate is a broken exported construction path.
func TestPolicyProfilesValidate(t *testing.T) {
	t.Parallel()

	for _, tt := range policyProfileFactories() {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy := tt.policy()

			if policy.IsZero() {
				t.Fatalf("%s returned zero policy", tt.name)
			}

			if err := policy.Validate(); err != nil {
				if !errors.Is(err, ErrInvalidPolicy) {
					t.Fatalf("%s returned unexpected validation error class: %v", tt.name, err)
				}

				t.Fatalf("%s returned invalid policy: %v", tt.name, err)
			}
		})
	}
}

// TestPolicyProfilesReturnIndependentPolicies verifies that profile constructors
// return fresh mutable policy values.
//
// Users must be able to take a profile as a starting point and modify it without
// mutating later calls to the same profile constructor.
func TestPolicyProfilesReturnIndependentPolicies(t *testing.T) {
	t.Parallel()

	for _, tt := range policyProfileFactories() {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			first := tt.policy()
			second := tt.policy()

			if len(first.Classes.Sizes) == 0 {
				t.Fatalf("%s returned policy with no class sizes", tt.name)
			}

			originalSecondFirstSize := second.Classes.Sizes[0]

			first.Retention.HardRetainedBytes = 1
			first.Classes.Sizes[0] = ClassSizeFromSize(8 * MiB)
			first.Shards.ShardsPerClass = 1
			first.Admission.ReturnedBuffers = ReturnedBufferPolicyDrop

			third := tt.policy()

			if second.Classes.Sizes[0] != originalSecondFirstSize {
				t.Fatalf("%s second policy changed after mutating first policy", tt.name)
			}

			if third.Classes.Sizes[0] != originalSecondFirstSize {
				t.Fatalf("%s later policy call observed mutation from earlier result", tt.name)
			}

			if reflect.DeepEqual(first, second) {
				t.Fatalf("%s mutation did not change first policy as expected", tt.name)
			}
		})
	}
}

// TestPolicyProfileLookupReturnsIndependentPolicies verifies that every profile
// lookup path returns fresh mutable policy values.
//
// Constructor tests protect the direct functions. This test protects the enum
// registry helpers, which callers are expected to use for option processing.
func TestPolicyProfileLookupReturnsIndependentPolicies(t *testing.T) {
	t.Parallel()

	resolvers := []struct {
		name    string
		resolve func(PolicyProfile) (Policy, bool)
	}{
		{
			name:    "PolicyForProfile",
			resolve: PolicyForProfile,
		},
		{
			name: "PolicyProfile.Policy",
			resolve: func(profile PolicyProfile) (Policy, bool) {
				return profile.Policy()
			},
		},
		{
			name: "MustPolicyForProfile",
			resolve: func(profile PolicyProfile) (Policy, bool) {
				return MustPolicyForProfile(profile), true
			},
		},
	}

	for _, resolver := range resolvers {
		resolver := resolver

		t.Run(resolver.name, func(t *testing.T) {
			t.Parallel()

			for _, tt := range policyProfileFactories() {
				tt := tt

				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					first, ok := resolver.resolve(tt.profile)
					if !ok {
						t.Fatalf("%s(%s) returned ok=false", resolver.name, tt.profile)
					}

					second, ok := resolver.resolve(tt.profile)
					if !ok {
						t.Fatalf("%s(%s) returned ok=false on second call", resolver.name, tt.profile)
					}

					if len(first.Classes.Sizes) == 0 || len(second.Classes.Sizes) == 0 {
						t.Fatalf("%s(%s) returned empty class-size profile", resolver.name, tt.profile)
					}

					originalSecondFirstSize := second.Classes.Sizes[0]
					first.Classes.Sizes[0] = ClassSizeFromSize(8 * MiB)

					if second.Classes.Sizes[0] != originalSecondFirstSize {
						t.Fatalf("%s(%s) returned policies sharing class-size backing storage", resolver.name, tt.profile)
					}
				})
			}
		})
	}
}

// TestPolicyProfilesHaveExpectedPosture verifies the key semantic differences
// between named profiles.
//
// This test avoids checking every field because policy defaults may be tuned over
// time. It instead protects the architectural posture each profile promises.
func TestPolicyProfilesHaveExpectedPosture(t *testing.T) {
	t.Parallel()

	defaultPolicy := DefaultPolicy()
	throughput := ThroughputPolicy()
	memoryConstrained := MemoryConstrainedPolicy()
	bursty := BurstyPolicy()
	strictBounded := StrictBoundedPolicy()
	secure := SecurePolicy()

	if throughput.Retention.HardRetainedBytes <= defaultPolicy.Retention.HardRetainedBytes {
		t.Fatalf("ThroughputPolicy hard retained bytes = %s, want greater than default %s",
			throughput.Retention.HardRetainedBytes,
			defaultPolicy.Retention.HardRetainedBytes,
		)
	}

	if throughput.Shards.ShardsPerClass <= defaultPolicy.Shards.ShardsPerClass {
		t.Fatalf("ThroughputPolicy shards per class = %d, want greater than default %d",
			throughput.Shards.ShardsPerClass,
			defaultPolicy.Shards.ShardsPerClass,
		)
	}

	if throughput.Shards.BucketSlotsPerShard <= defaultPolicy.Shards.BucketSlotsPerShard {
		t.Fatalf("ThroughputPolicy bucket slots per shard = %d, want greater than default %d",
			throughput.Shards.BucketSlotsPerShard,
			defaultPolicy.Shards.BucketSlotsPerShard,
		)
	}

	if memoryConstrained.Retention.HardRetainedBytes >= defaultPolicy.Retention.HardRetainedBytes {
		t.Fatalf("MemoryConstrainedPolicy hard retained bytes = %s, want less than default %s",
			memoryConstrained.Retention.HardRetainedBytes,
			defaultPolicy.Retention.HardRetainedBytes,
		)
	}

	if memoryConstrained.Shards.BucketSlotsPerShard >= defaultPolicy.Shards.BucketSlotsPerShard {
		t.Fatalf("MemoryConstrainedPolicy bucket slots per shard = %d, want less than default %d",
			memoryConstrained.Shards.BucketSlotsPerShard,
			defaultPolicy.Shards.BucketSlotsPerShard,
		)
	}

	if bursty.Retention.HardRetainedBytes <= throughput.Retention.HardRetainedBytes {
		t.Fatalf("BurstyPolicy hard retained bytes = %s, want greater than throughput %s",
			bursty.Retention.HardRetainedBytes,
			throughput.Retention.HardRetainedBytes,
		)
	}

	if bursty.Retention.SoftRetainedBytes >= bursty.Retention.HardRetainedBytes {
		t.Fatalf("BurstyPolicy soft retained bytes = %s, want less than hard retained bytes %s",
			bursty.Retention.SoftRetainedBytes,
			bursty.Retention.HardRetainedBytes,
		)
	}

	if strictBounded.Admission.ZeroSizeRequests != ZeroSizeRequestReject {
		t.Fatalf("StrictBoundedPolicy zero-size requests = %s, want reject", strictBounded.Admission.ZeroSizeRequests)
	}

	if strictBounded.Ownership.Mode != OwnershipModeAccounting {
		t.Fatalf("StrictBoundedPolicy ownership mode = %s, want accounting", strictBounded.Ownership.Mode)
	}

	if strictBounded.Ownership.MaxReturnedCapacityGrowth != PolicyRatioOne {
		t.Fatalf("StrictBoundedPolicy max returned capacity growth = %d, want %d",
			strictBounded.Ownership.MaxReturnedCapacityGrowth,
			PolicyRatioOne,
		)
	}

	if secure.Admission.ZeroSizeRequests != ZeroSizeRequestReject {
		t.Fatalf("SecurePolicy zero-size requests = %s, want reject", secure.Admission.ZeroSizeRequests)
	}

	if !secure.Admission.ZeroRetainedBuffers {
		t.Fatalf("SecurePolicy should zero retained buffers")
	}

	if !secure.Admission.ZeroDroppedBuffers {
		t.Fatalf("SecurePolicy should zero dropped buffers")
	}

	if secure.Ownership.Mode != OwnershipModeStrict {
		t.Fatalf("SecurePolicy ownership mode = %s, want strict", secure.Ownership.Mode)
	}

	if !secure.Ownership.DetectDoubleRelease {
		t.Fatalf("SecurePolicy should detect double release")
	}
}

// TestPolicyProfilesHaveConsistentPressureOrdering verifies pressure policy
// posture for every named profile.
//
// Medium/high/critical pressure should contract retention monotonically and
// increase trim work monotonically. This mirrors policy validation but keeps the
// profile intent obvious at the preset level.
func TestPolicyProfilesHaveConsistentPressureOrdering(t *testing.T) {
	t.Parallel()

	for _, tt := range policyProfileFactories() {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pressure := tt.policy().Pressure
			if !pressure.Enabled {
				t.Fatalf("%s should enable pressure handling", tt.name)
			}

			if pressure.Medium.RetentionScale < pressure.High.RetentionScale {
				t.Fatalf("%s medium retention scale = %d, want >= high retention scale %d",
					tt.name,
					pressure.Medium.RetentionScale,
					pressure.High.RetentionScale,
				)
			}

			if pressure.High.RetentionScale < pressure.Critical.RetentionScale {
				t.Fatalf("%s high retention scale = %d, want >= critical retention scale %d",
					tt.name,
					pressure.High.RetentionScale,
					pressure.Critical.RetentionScale,
				)
			}

			if pressure.Medium.TrimScale > pressure.High.TrimScale {
				t.Fatalf("%s medium trim scale = %d, want <= high trim scale %d",
					tt.name,
					pressure.Medium.TrimScale,
					pressure.High.TrimScale,
				)
			}

			if pressure.High.TrimScale > pressure.Critical.TrimScale {
				t.Fatalf("%s high trim scale = %d, want <= critical trim scale %d",
					tt.name,
					pressure.High.TrimScale,
					pressure.Critical.TrimScale,
				)
			}
		})
	}
}

// TestProfileClassSizes verifies class-size profiles independently from Policy
// constructors.
//
// The class-size helpers are intentionally exported profile building blocks.
// They must return strictly increasing, independent slices because callers may
// use them to build custom policies.
func TestProfileClassSizes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		sizes func() []ClassSize
		first ClassSize
		last  ClassSize
	}{
		{
			name:  "throughput",
			sizes: ThroughputClassSizes,
			first: ClassSizeFromBytes(256),
			last:  ClassSizeFromSize(4 * MiB),
		},
		{
			name:  "memory constrained",
			sizes: MemoryConstrainedClassSizes,
			first: ClassSizeFromBytes(512),
			last:  ClassSizeFromSize(512 * KiB),
		},
		{
			name:  "bursty",
			sizes: BurstyClassSizes,
			first: ClassSizeFromBytes(512),
			last:  ClassSizeFromSize(8 * MiB),
		},
		{
			name:  "strict bounded",
			sizes: StrictBoundedClassSizes,
			first: ClassSizeFromSize(KiB),
			last:  ClassSizeFromSize(512 * KiB),
		},
		{
			name:  "secure",
			sizes: SecureClassSizes,
			first: ClassSizeFromBytes(512),
			last:  ClassSizeFromSize(MiB),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sizes := tt.sizes()

			assertClassSizeProfile(t, sizes, tt.first, tt.last)

			second := tt.sizes()
			if len(sizes) == 0 || len(second) == 0 {
				t.Fatalf("%s returned empty class-size profile", tt.name)
			}

			sizes[0] = ClassSizeFromSize(8 * MiB)

			if second[0] != tt.first {
				t.Fatalf("%s class-size helper returned shared mutable backing storage", tt.name)
			}
		})
	}
}

// TestProfileClassSizesFitRetentionLimits verifies every profile's largest class
// can be retained under its own retention limits.
//
// A profile whose largest class exceeds max retained capacity, class bytes, or
// shard bytes would be unusable even before runtime construction.
func TestProfileClassSizesFitRetentionLimits(t *testing.T) {
	t.Parallel()

	for _, tt := range policyProfileFactories() {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy := tt.policy()
			sizes := policy.Classes.Sizes
			if len(sizes) == 0 {
				t.Fatalf("%s returned no class sizes", tt.name)
			}

			largest := sizes[len(sizes)-1].Size()

			if largest > policy.Retention.MaxRetainedBufferCapacity {
				t.Fatalf("%s largest class = %s, want <= max retained buffer capacity %s",
					tt.name,
					largest,
					policy.Retention.MaxRetainedBufferCapacity,
				)
			}

			if largest > policy.Retention.MaxClassRetainedBytes {
				t.Fatalf("%s largest class = %s, want <= max class retained bytes %s",
					tt.name,
					largest,
					policy.Retention.MaxClassRetainedBytes,
				)
			}

			if largest > policy.Retention.MaxShardRetainedBytes {
				t.Fatalf("%s largest class = %s, want <= max shard retained bytes %s",
					tt.name,
					largest,
					policy.Retention.MaxShardRetainedBytes,
				)
			}
		})
	}
}

// TestPolicyProfileClassTablesRouteConfiguredBoundaries verifies that profile
// class-size lists form usable class tables with the expected request/capacity
// routing semantics.
//
// Profile constructors own the class-size profile, while classTable owns lookup
// mechanics. This integration test catches profile lists that are valid in
// isolation but broken for runtime classification.
func TestPolicyProfileClassTablesRouteConfiguredBoundaries(t *testing.T) {
	t.Parallel()

	for _, tt := range policyProfileFactories() {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy := tt.policy()
			table := newClassTable(policy.Classes.Sizes)

			first, ok := table.first()
			if !ok {
				t.Fatalf("%s class table has no first class", tt.name)
			}

			last, ok := table.last()
			if !ok {
				t.Fatalf("%s class table has no last class", tt.name)
			}

			zeroRequestClass, ok := table.classForRequest(0)
			if !ok {
				t.Fatalf("%s class table does not route zero-size request", tt.name)
			}

			if zeroRequestClass != first {
				t.Fatalf("%s zero-size request routed to %s, want %s", tt.name, zeroRequestClass, first)
			}

			if _, ok := table.classForCapacity(0); ok {
				t.Fatalf("%s class table routed zero capacity", tt.name)
			}

			if _, ok := table.classForCapacity(first.ByteSize() - Byte); ok {
				t.Fatalf("%s class table routed capacity below first class", tt.name)
			}

			aboveLast := last.ByteSize() + Byte
			if _, ok := table.classForRequest(aboveLast); ok {
				t.Fatalf("%s class table routed request above largest class", tt.name)
			}

			aboveLastCapacityClass, ok := table.classForCapacity(aboveLast)
			if !ok {
				t.Fatalf("%s class table did not route capacity above largest class", tt.name)
			}

			if aboveLastCapacityClass != last {
				t.Fatalf("%s capacity above largest class routed to %s, want %s", tt.name, aboveLastCapacityClass, last)
			}

			for index, size := range policy.Classes.Sizes {
				want := NewSizeClass(ClassID(index), size)

				exactClass, ok := table.classForExactSize(size)
				if !ok {
					t.Fatalf("%s class table missing exact class %s", tt.name, size)
				}

				if exactClass != want {
					t.Fatalf("%s exact class for %s = %s, want %s", tt.name, size, exactClass, want)
				}

				requestClass, ok := table.classForRequest(size.Size())
				if !ok {
					t.Fatalf("%s class table did not route request %s", tt.name, size)
				}

				if requestClass != want {
					t.Fatalf("%s request %s routed to %s, want %s", tt.name, size, requestClass, want)
				}

				capacityClass, ok := table.classForCapacity(size.Size())
				if !ok {
					t.Fatalf("%s class table did not route capacity %s", tt.name, size)
				}

				if capacityClass != want {
					t.Fatalf("%s capacity %s routed to %s, want %s", tt.name, size, capacityClass, want)
				}
			}
		})
	}
}

// TestPolicyProfileDefaultMatchesDefaultPolicy verifies that the named default
// profile is exactly the canonical default policy.
func TestPolicyProfileDefaultMatchesDefaultPolicy(t *testing.T) {
	t.Parallel()

	policy, ok := PolicyForProfile(PolicyProfileDefault)
	if !ok {
		t.Fatal("PolicyForProfile(PolicyProfileDefault) returned ok=false")
	}

	if !reflect.DeepEqual(policy, DefaultPolicy()) {
		t.Fatal("PolicyProfileDefault does not match DefaultPolicy()")
	}
}

type policyProfileFactory struct {
	name    string
	profile PolicyProfile
	policy  func() Policy
}

func policyProfileFactories() []policyProfileFactory {
	return []policyProfileFactory{
		{
			name:    "default",
			profile: PolicyProfileDefault,
			policy:  DefaultPolicy,
		},
		{
			name:    "throughput",
			profile: PolicyProfileThroughput,
			policy:  ThroughputPolicy,
		},
		{
			name:    "memory constrained",
			profile: PolicyProfileMemoryConstrained,
			policy:  MemoryConstrainedPolicy,
		},
		{
			name:    "bursty",
			profile: PolicyProfileBursty,
			policy:  BurstyPolicy,
		},
		{
			name:    "strict bounded",
			profile: PolicyProfileStrictBounded,
			policy:  StrictBoundedPolicy,
		},
		{
			name:    "secure",
			profile: PolicyProfileSecure,
			policy:  SecurePolicy,
		},
	}
}

func selectablePolicyProfiles() []PolicyProfile {
	return []PolicyProfile{
		PolicyProfileDefault,
		PolicyProfileThroughput,
		PolicyProfileMemoryConstrained,
		PolicyProfileBursty,
		PolicyProfileStrictBounded,
		PolicyProfileSecure,
	}
}

func assertPolicyProfilePanic(t *testing.T, fn func()) {
	t.Helper()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}

		if recovered != errUnknownPolicyProfile {
			t.Fatalf("panic = %#v, want %q", recovered, errUnknownPolicyProfile)
		}
	}()

	fn()
}

func assertClassSizeProfile(t *testing.T, sizes []ClassSize, first ClassSize, last ClassSize) {
	t.Helper()

	if len(sizes) == 0 {
		t.Fatal("class-size profile is empty")
	}

	if sizes[0] != first {
		t.Fatalf("first class size = %s, want %s", sizes[0], first)
	}

	if sizes[len(sizes)-1] != last {
		t.Fatalf("last class size = %s, want %s", sizes[len(sizes)-1], last)
	}

	for index, size := range sizes {
		if size.IsZero() {
			t.Fatalf("class size at index %d is zero", index)
		}

		if index == 0 {
			continue
		}

		previous := sizes[index-1]
		if !previous.Less(size) {
			t.Fatalf("class sizes must be strictly increasing at index %d: %s must be greater than %s",
				index,
				size,
				previous,
			)
		}
	}
}
