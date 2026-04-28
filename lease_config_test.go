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

// TestDefaultLeaseConfigUsesStrictOwnership verifies the default lease registry
// posture.
//
// A LeaseRegistry exists specifically to provide ownership-aware acquisition and
// release. Its zero/default config must therefore normalize to strict ownership,
// not to the lightweight OwnershipModeNone used by the bare Pool data path.
func TestDefaultLeaseConfigUsesStrictOwnership(t *testing.T) {
	t.Parallel()

	config := DefaultLeaseConfig()
	if config.Ownership.Mode != OwnershipModeStrict {
		t.Fatalf("DefaultLeaseConfig().Ownership.Mode = %s, want %s", config.Ownership.Mode, OwnershipModeStrict)
	}

	if !config.Ownership.TrackInUseBytes {
		t.Fatal("DefaultLeaseConfig() does not track in-use bytes")
	}
	if !config.Ownership.TrackInUseBuffers {
		t.Fatal("DefaultLeaseConfig() does not track in-use buffers")
	}
	if !config.Ownership.DetectDoubleRelease {
		t.Fatal("DefaultLeaseConfig() does not detect double release")
	}
	if config.Ownership.MaxReturnedCapacityGrowth == 0 {
		t.Fatal("DefaultLeaseConfig() did not set capacity-growth limit")
	}
}

// TestLeaseConfigNormalizeCompletesZeroConfig verifies that a zero LeaseConfig
// becomes a usable strict ownership registry config.
func TestLeaseConfigNormalizeCompletesZeroConfig(t *testing.T) {
	t.Parallel()

	normalized := (LeaseConfig{}).Normalize()
	if normalized.Ownership.Mode != OwnershipModeStrict {
		t.Fatalf("Normalize().Ownership.Mode = %s, want %s", normalized.Ownership.Mode, OwnershipModeStrict)
	}
	if err := normalized.Validate(); err != nil {
		t.Fatalf("normalized zero LeaseConfig failed validation: %v", err)
	}
}

// TestLeaseConfigFromOwnershipPolicy verifies that explicit ownership policy is
// preserved by config construction and then completed by normalization.
func TestLeaseConfigFromOwnershipPolicy(t *testing.T) {
	t.Parallel()

	policy := OwnershipPolicy{Mode: OwnershipModeAccounting}
	config := LeaseConfigFromOwnershipPolicy(policy)
	if config.Ownership.Mode != OwnershipModeAccounting {
		t.Fatalf("LeaseConfigFromOwnershipPolicy().Ownership.Mode = %s, want %s", config.Ownership.Mode, OwnershipModeAccounting)
	}

	normalized := config.Normalize()
	if normalized.Ownership.Mode != OwnershipModeAccounting {
		t.Fatalf("normalized mode = %s, want %s", normalized.Ownership.Mode, OwnershipModeAccounting)
	}
	if !normalized.Ownership.TrackInUseBytes || !normalized.Ownership.TrackInUseBuffers || !normalized.Ownership.DetectDoubleRelease {
		t.Fatalf("accounting ownership was not completed by normalization: %#v", normalized.Ownership)
	}
	if normalized.Ownership.MaxReturnedCapacityGrowth == 0 {
		t.Fatal("accounting ownership did not receive default capacity-growth ratio")
	}
}

// TestLeaseConfigValidateRejectsDisabledOwnership verifies that LeaseRegistry
// construction cannot silently operate with ownership disabled.
func TestLeaseConfigValidateRejectsDisabledOwnership(t *testing.T) {
	t.Parallel()

	config := LeaseConfig{Ownership: OwnershipPolicy{Mode: OwnershipModeNone}}
	err := config.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil for disabled ownership")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate() error does not match ErrInvalidOptions: %v", err)
	}
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("Validate() error does not unwrap ErrInvalidPolicy: %v", err)
	}
}

// TestLeaseConfigIsZero verifies zero-value diagnostics.
func TestLeaseConfigIsZero(t *testing.T) {
	t.Parallel()

	if !(LeaseConfig{}).IsZero() {
		t.Fatal("zero LeaseConfig did not report IsZero")
	}
	if DefaultLeaseConfig().IsZero() {
		t.Fatal("DefaultLeaseConfig reported IsZero")
	}
}
