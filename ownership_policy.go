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
	// errOwnershipPolicyUnsupportedMode is used when a lease-aware owner receives
	// an unknown ownership mode.
	errOwnershipPolicyUnsupportedMode = "bufferpool.OwnershipPolicy: unsupported ownership mode"

	// errOwnershipPolicyNoLeaseMode is used when a LeaseRegistry is configured
	// with ownership disabled.
	errOwnershipPolicyNoLeaseMode = "bufferpool.OwnershipPolicy: lease registry requires accounting or strict ownership mode"

	// errOwnershipPolicyMissingGrowthRatio is used when strict ownership has no
	// returned-capacity growth limit.
	errOwnershipPolicyMissingGrowthRatio = "bufferpool.OwnershipPolicy: strict ownership requires max returned capacity growth"
)

// AccountingOwnershipPolicy returns the default ownership policy for a
// lease-aware accounting owner.
//
// Accounting mode tracks checked-out leases and in-use capacity, but it does not
// require strict backing-array identity validation. It still enforces configured
// origin-class capacity growth because LeaseRegistry knows the acquired class.
// It is useful as the first PoolPartition-managed ownership layer when the goal
// is observability and safe lifecycle accounting rather than strict caller
// policing.
func AccountingOwnershipPolicy() OwnershipPolicy {
	return OwnershipPolicy{
		Mode:                      OwnershipModeAccounting,
		TrackInUseBytes:           true,
		TrackInUseBuffers:         true,
		DetectDoubleRelease:       true,
		MaxReturnedCapacityGrowth: DefaultPolicyMaxReturnedCapacityGrowth,
	}
}

// StrictOwnershipPolicy returns the default ownership policy for a strict
// lease-aware owner.
//
// Strict mode validates the lease token, detects double release, checks that the
// returned slice preserves the acquired base pointer, and canonicalizes the
// Pool handoff to the acquired capacity. MaxReturnedCapacityGrowth remains a
// secondary guard: ordinary append beyond capacity normally reallocates and is
// rejected as foreign before a growth-ratio check matters.
func StrictOwnershipPolicy() OwnershipPolicy {
	return OwnershipPolicy{
		Mode:                      OwnershipModeStrict,
		TrackInUseBytes:           true,
		TrackInUseBuffers:         true,
		DetectDoubleRelease:       true,
		MaxReturnedCapacityGrowth: DefaultPolicyMaxReturnedCapacityGrowth,
	}
}

// NormalizeForLeaseRegistry returns the ownership policy used by a LeaseRegistry.
//
// A zero ownership policy becomes StrictOwnershipPolicy. This differs from
// DefaultOwnershipPolicy, where ownership is disabled for the bare []byte Pool
// data path. LeaseRegistry exists specifically to provide ownership-aware
// acquisition/release, so its zero config should produce a useful strict owner.
func (p OwnershipPolicy) NormalizeForLeaseRegistry() OwnershipPolicy {
	if p.IsZero() || p.Mode == OwnershipModeUnset {
		return StrictOwnershipPolicy()
	}

	if p.MaxReturnedCapacityGrowth == 0 {
		p.MaxReturnedCapacityGrowth = DefaultPolicyMaxReturnedCapacityGrowth
	}

	switch p.Mode {
	case OwnershipModeAccounting:
		p.TrackInUseBytes = true
		p.TrackInUseBuffers = true
		p.DetectDoubleRelease = true

	case OwnershipModeStrict:
		p.TrackInUseBytes = true
		p.TrackInUseBuffers = true
		p.DetectDoubleRelease = true
	}

	return p
}

// ValidateForLeaseRegistry validates ownership policy for a lease-aware owner.
//
// This validation is intentionally different from standalone Pool support
// validation. Standalone Pool rejects accounting/strict modes for the bare []byte
// path. LeaseRegistry requires those modes because it owns the lease tokens and
// checked-out registry that make the guarantees real.
func (p OwnershipPolicy) ValidateForLeaseRegistry() error {
	switch p.Mode {
	case OwnershipModeAccounting, OwnershipModeStrict:
	case OwnershipModeNone, OwnershipModeUnset:
		return newError(ErrInvalidPolicy, errOwnershipPolicyNoLeaseMode)
	default:
		return newError(ErrInvalidPolicy, errOwnershipPolicyUnsupportedMode)
	}

	if p.Mode == OwnershipModeStrict && p.MaxReturnedCapacityGrowth == 0 {
		return newError(ErrInvalidPolicy, errOwnershipPolicyMissingGrowthRatio)
	}

	return nil
}

// RequiresLeaseLayer reports whether the policy cannot be implemented by bare
// []byte Put alone.
//
// This predicate is used by Pool construction support checks and by tests that
// document the boundary between capacity-based Pool returns and lease-verified
// ownership. Tracking flags also require the lease layer because Pool does not
// know which buffers are checked out.
func (p OwnershipPolicy) RequiresLeaseLayer() bool {
	return ownershipModeRequiresLease(p.Mode) ||
		p.TrackInUseBytes ||
		p.TrackInUseBuffers ||
		p.DetectDoubleRelease
}

// TracksInUse reports whether the policy requests checked-out in-use gauges.
//
// LeaseRegistry can provide these gauges from active lease records. Pool cannot
// provide them for bare []byte users because it does not own checked-out buffer
// identity after Get returns.
func (p OwnershipPolicy) TracksInUse() bool { return ownershipModeTracksInUse(p) }

// DetectsDoubleRelease reports whether repeated release attempts should be
// classified as double-release violations.
//
// Double-release detection is based on lease record state. Bare Pool.Put cannot
// perform this check because it receives only a []byte without a lease token.
func (p OwnershipPolicy) DetectsDoubleRelease() bool { return ownershipModeDetectsDoubleRelease(p) }

// IsStrict reports whether strict release validation is enabled.
//
// Strict validation requires the returned slice to preserve the acquired base
// pointer and lets the registry canonicalize Pool handoff to acquired capacity.
func (p OwnershipPolicy) IsStrict() bool { return p.Mode == OwnershipModeStrict }

// IsAccounting reports whether accounting ownership mode is enabled.
//
// Accounting mode tracks lease lifecycle and checked-out gauges but does not
// enforce strict backing identity. Replacement or foreign buffers can complete
// ownership in this mode when they satisfy capacity-growth validation.
func (p OwnershipPolicy) IsAccounting() bool { return p.Mode == OwnershipModeAccounting }
