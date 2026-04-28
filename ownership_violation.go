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

// OwnershipViolationKind identifies the ownership invariant violated by a lease
// release attempt.
//
// The kind is diagnostic state. Public callers should use errors.Is with
// ErrInvalidLease, ErrDoubleRelease, ErrNilBuffer, ErrZeroCapacity, or
// ErrOwnershipViolation. The fine-grained kind is used by counters, snapshots,
// tests, and future Partition/Group controller diagnostics.
type OwnershipViolationKind uint8

const (
	// OwnershipViolationNone means no ownership invariant was violated.
	//
	// It is the zero value so an untouched LeaseSnapshot can represent "no
	// recorded violation" without extra booleans.
	OwnershipViolationNone OwnershipViolationKind = iota

	// OwnershipViolationInvalidLease means the release handle is zero, malformed,
	// or not associated with the registry that is processing the release.
	OwnershipViolationInvalidLease

	// OwnershipViolationDoubleRelease means the lease was already released.
	OwnershipViolationDoubleRelease

	// OwnershipViolationNilBuffer means Release was called with a nil returned
	// buffer.
	OwnershipViolationNilBuffer

	// OwnershipViolationZeroCapacity means the returned buffer has no reusable
	// backing capacity.
	OwnershipViolationZeroCapacity

	// OwnershipViolationForeignBuffer means strict mode observed a returned
	// slice base different from the one acquired by the lease.
	//
	// Strict mode requires acquired base-pointer identity, not merely overlap
	// with the same backing allocation. A shifted subslice is rejected because it
	// changes the slice base and can change retained-capacity classification.
	OwnershipViolationForeignBuffer

	// OwnershipViolationCapacityGrowth means strict mode observed returned
	// capacity above the allowed origin-class growth ratio.
	//
	// In current strict []byte releases, ordinary append beyond capacity usually
	// reallocates and is detected first as a foreign buffer. This kind remains
	// useful for direct validator coverage and future ownership modes where a
	// non-strict replacement may still need capacity-growth policy checks.
	OwnershipViolationCapacityGrowth
)

// String returns a stable diagnostic label for k.
//
// These labels are stable for tests, snapshots, logs, and future metrics. They
// are deliberately lower_snake_case so they can cross text boundaries without
// formatting conversion.
func (k OwnershipViolationKind) String() string {
	switch k {
	case OwnershipViolationNone:
		return "none"
	case OwnershipViolationInvalidLease:
		return "invalid_lease"
	case OwnershipViolationDoubleRelease:
		return "double_release"
	case OwnershipViolationNilBuffer:
		return "nil_buffer"
	case OwnershipViolationZeroCapacity:
		return "zero_capacity"
	case OwnershipViolationForeignBuffer:
		return "foreign_buffer"
	case OwnershipViolationCapacityGrowth:
		return "capacity_growth"
	default:
		return "unknown"
	}
}

// IsZero reports whether k is OwnershipViolationNone.
//
// Snapshot and counter helpers use this to distinguish "no violation" from a
// meaningful rejected-release reason without hard-coding the enum value.
func (k OwnershipViolationKind) IsZero() bool {
	return k == OwnershipViolationNone
}
