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

const (
	// errLeaseConfigInvalidOwnership is used when LeaseConfig contains an
	// ownership policy that cannot back a lease-aware registry.
	errLeaseConfigInvalidOwnership = "bufferpool.LeaseConfig: invalid ownership policy"
)

// LeaseConfig configures a LeaseRegistry.
//
// LeaseConfig is intentionally separate from PoolConfig. Pool owns retained
// storage and capacity-based admission. LeaseRegistry owns checked-out identity,
// in-use accounting, and release validation. Future PoolPartition/PoolGroup
// wiring can derive both configs from a broader runtime policy without making
// Pool itself a controller.
type LeaseConfig struct {
	// Ownership defines lease validation and checked-out accounting behavior.
	//
	// A zero OwnershipPolicy normalizes to StrictOwnershipPolicy because a
	// LeaseRegistry exists specifically to provide ownership-aware behavior.
	Ownership OwnershipPolicy
}

// DefaultLeaseConfig returns the default strict lease registry configuration.
//
// The default is intentionally stricter than Pool's bare []byte ownership
// policy. A LeaseRegistry exists only when callers want ownership-aware
// acquisition and release, so the default registry config validates lease token
// identity, detects double release, and tracks checked-out memory.
func DefaultLeaseConfig() LeaseConfig {
	return LeaseConfig{Ownership: StrictOwnershipPolicy()}
}

// LeaseConfigFromOwnershipPolicy creates a LeaseConfig from ownership policy.
//
// This helper keeps call sites readable when a future PoolPartition or tests
// derive lease-registry behavior from a broader Policy value. The returned
// config still goes through Normalize and Validate during registry construction.
func LeaseConfigFromOwnershipPolicy(policy OwnershipPolicy) LeaseConfig {
	return LeaseConfig{Ownership: policy}
}

// Normalize returns a LeaseConfig with ownership defaults completed.
//
// Normalization is value-based and does not mutate the receiver. A zero
// ownership policy becomes strict ownership because a registry without
// ownership behavior would duplicate Pool's bare data-plane API without adding
// safety.
func (c LeaseConfig) Normalize() LeaseConfig {
	c.Ownership = c.Ownership.NormalizeForLeaseRegistry()
	return c
}

// Validate validates a normalized LeaseConfig.
//
// Validation is registry-specific. Standalone Pool rejects strict/accounting
// ownership because bare Put cannot enforce it; LeaseRegistry requires those
// modes because it owns lease records and can make the guarantees real.
func (c LeaseConfig) Validate() error {
	if err := c.Ownership.ValidateForLeaseRegistry(); err != nil {
		return wrapError(ErrInvalidOptions, err, errLeaseConfigInvalidOwnership)
	}
	return nil
}

// IsZero reports whether c contains no explicit values.
//
// A zero config is accepted by NewLeaseRegistry after normalization. The helper
// is useful for tests and future config composition code that needs to
// distinguish caller-provided ownership settings from default strict behavior.
func (c LeaseConfig) IsZero() bool { return c.Ownership.IsZero() }
