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

// ownershipModeRequiresLease reports whether mode requires ownership-aware
// handles instead of the bare []byte Put path.
//
// Bare []byte Put is intentionally capacity-admitted only. It cannot prove
// origin, checked-out identity, repeated release, in-use bytes, or origin-class
// growth. Modes that require those invariants must be implemented through Lease
// and LeaseRegistry or through a future Partition/Group ownership layer.
func ownershipModeRequiresLease(mode OwnershipMode) bool {
	return mode == OwnershipModeAccounting || mode == OwnershipModeStrict
}

// ownershipModeTracksInUse reports whether the policy wants checked-out resource
// gauges to be visible.
func ownershipModeTracksInUse(policy OwnershipPolicy) bool {
	return policy.TrackInUseBytes || policy.TrackInUseBuffers || ownershipModeRequiresLease(policy.Mode)
}

// ownershipModeDetectsDoubleRelease reports whether repeated release attempts
// should be classified as ErrDoubleRelease instead of a generic invalid lease.
func ownershipModeDetectsDoubleRelease(policy OwnershipPolicy) bool {
	return policy.DetectDoubleRelease || ownershipModeRequiresLease(policy.Mode)
}

// ownershipModeEnforcesStrictBufferIdentity reports whether release validation
// must prove that the returned slice preserves the acquired base pointer.
func ownershipModeEnforcesStrictBufferIdentity(policy OwnershipPolicy) bool {
	return policy.Mode == OwnershipModeStrict
}

// ownershipModeEnforcesCapacityGrowth reports whether release validation must
// enforce MaxReturnedCapacityGrowth against the origin class.
//
// Both accounting and strict managed modes can enforce growth because both have
// a lease record with an origin class. Strict mode often rejects ordinary
// append-past-capacity cases earlier as foreign-buffer releases, but the growth
// guard still protects unusual slice-header shapes and accounting-mode
// replacement buffers.
func ownershipModeEnforcesCapacityGrowth(policy OwnershipPolicy) bool {
	return ownershipModeRequiresLease(policy.Mode) && policy.MaxReturnedCapacityGrowth != 0
}
