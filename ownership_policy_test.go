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

// TestAccountingOwnershipPolicy verifies the accounting preset used by future
// PoolPartition-managed observability paths.
func TestAccountingOwnershipPolicy(t *testing.T) {
	t.Parallel()

	policy := AccountingOwnershipPolicy()
	if policy.Mode != OwnershipModeAccounting {
		t.Fatalf("Mode = %s, want %s", policy.Mode, OwnershipModeAccounting)
	}
	if !policy.TrackInUseBytes || !policy.TrackInUseBuffers || !policy.DetectDoubleRelease {
		t.Fatalf("accounting policy did not enable accounting flags: %#v", policy)
	}
	if policy.MaxReturnedCapacityGrowth == 0 {
		t.Fatal("accounting policy did not set capacity-growth ratio")
	}
	if !policy.RequiresLeaseLayer() || !policy.TracksInUse() || !policy.DetectsDoubleRelease() || !policy.IsAccounting() {
		t.Fatalf("accounting helper predicates are inconsistent: %#v", policy)
	}
	if policy.IsStrict() {
		t.Fatal("accounting policy reported IsStrict")
	}
}

// TestStrictOwnershipPolicy verifies the strict preset used by the default
// LeaseRegistry config.
func TestStrictOwnershipPolicy(t *testing.T) {
	t.Parallel()

	policy := StrictOwnershipPolicy()
	if policy.Mode != OwnershipModeStrict {
		t.Fatalf("Mode = %s, want %s", policy.Mode, OwnershipModeStrict)
	}
	if !policy.TrackInUseBytes || !policy.TrackInUseBuffers || !policy.DetectDoubleRelease {
		t.Fatalf("strict policy did not enable strict accounting flags: %#v", policy)
	}
	if policy.MaxReturnedCapacityGrowth == 0 {
		t.Fatal("strict policy did not set capacity-growth ratio")
	}
	if !policy.RequiresLeaseLayer() || !policy.TracksInUse() || !policy.DetectsDoubleRelease() || !policy.IsStrict() {
		t.Fatalf("strict helper predicates are inconsistent: %#v", policy)
	}
	if policy.IsAccounting() {
		t.Fatal("strict policy reported IsAccounting")
	}
}

// TestOwnershipPolicyNormalizeForLeaseRegistry verifies lease-specific
// normalization without changing the bare Pool default ownership policy.
func TestOwnershipPolicyNormalizeForLeaseRegistry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   OwnershipPolicy
		want OwnershipMode
	}{
		{name: "zero", in: OwnershipPolicy{}, want: OwnershipModeStrict},
		{name: "unset", in: OwnershipPolicy{Mode: OwnershipModeUnset}, want: OwnershipModeStrict},
		{name: "accounting", in: OwnershipPolicy{Mode: OwnershipModeAccounting}, want: OwnershipModeAccounting},
		{name: "strict", in: OwnershipPolicy{Mode: OwnershipModeStrict}, want: OwnershipModeStrict},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.in.NormalizeForLeaseRegistry()
			if got.Mode != tt.want {
				t.Fatalf("Mode = %s, want %s", got.Mode, tt.want)
			}
			if !got.TrackInUseBytes || !got.TrackInUseBuffers || !got.DetectDoubleRelease {
				t.Fatalf("normalization did not enable lease accounting flags: %#v", got)
			}
			if got.MaxReturnedCapacityGrowth == 0 {
				t.Fatal("normalization did not set default capacity-growth ratio")
			}
		})
	}
}

// TestOwnershipPolicyValidateForLeaseRegistry verifies that only lease-capable
// modes are accepted by a LeaseRegistry.
func TestOwnershipPolicyValidateForLeaseRegistry(t *testing.T) {
	t.Parallel()

	valid := []OwnershipPolicy{
		AccountingOwnershipPolicy(),
		StrictOwnershipPolicy(),
		{Mode: OwnershipModeAccounting, MaxReturnedCapacityGrowth: PolicyRatioOne},
		{Mode: OwnershipModeStrict, MaxReturnedCapacityGrowth: PolicyRatioOne},
	}
	for _, policy := range valid {
		policy := policy
		t.Run("valid_"+policy.Mode.String(), func(t *testing.T) {
			t.Parallel()
			if err := policy.ValidateForLeaseRegistry(); err != nil {
				t.Fatalf("ValidateForLeaseRegistry() returned error: %v", err)
			}
		})
	}

	invalid := []struct {
		name string
		in   OwnershipPolicy
	}{
		{name: "none", in: OwnershipPolicy{Mode: OwnershipModeNone}},
		{name: "unset", in: OwnershipPolicy{Mode: OwnershipModeUnset}},
		{name: "unknown", in: OwnershipPolicy{Mode: OwnershipMode(255)}},
		{name: "strict_missing_growth", in: OwnershipPolicy{Mode: OwnershipModeStrict}},
	}
	for _, tt := range invalid {
		tt := tt
		t.Run("invalid_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.in.ValidateForLeaseRegistry()
			if err == nil {
				t.Fatal("ValidateForLeaseRegistry() returned nil")
			}
			if !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("ValidateForLeaseRegistry() error does not match ErrInvalidPolicy: %v", err)
			}
		})
	}
}

// TestOwnershipPolicyRequiresLeaseLayer verifies that explicit ownership flags
// are treated as lease-dependent even if Mode was not set to strict/accounting.
func TestOwnershipPolicyRequiresLeaseLayer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   OwnershipPolicy
		want bool
	}{
		{name: "none", in: OwnershipPolicy{Mode: OwnershipModeNone}, want: false},
		{name: "accounting", in: OwnershipPolicy{Mode: OwnershipModeAccounting}, want: true},
		{name: "strict", in: OwnershipPolicy{Mode: OwnershipModeStrict}, want: true},
		{name: "track_bytes", in: OwnershipPolicy{Mode: OwnershipModeNone, TrackInUseBytes: true}, want: true},
		{name: "track_buffers", in: OwnershipPolicy{Mode: OwnershipModeNone, TrackInUseBuffers: true}, want: true},
		{name: "double_release", in: OwnershipPolicy{Mode: OwnershipModeNone, DetectDoubleRelease: true}, want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.in.RequiresLeaseLayer(); got != tt.want {
				t.Fatalf("RequiresLeaseLayer() = %v, want %v", got, tt.want)
			}
		})
	}
}
