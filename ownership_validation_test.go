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

// TestBufferBackingData verifies the low-level backing-array identity helper
// used by strict ownership validation.
func TestBufferBackingData(t *testing.T) {
	t.Parallel()

	if data := bufferBackingData(nil); data != nil {
		t.Fatalf("bufferBackingData(nil) = %p, want nil", data)
	}

	zeroCap := make([]byte, 0)
	if data := bufferBackingData(zeroCap); data != nil {
		t.Fatalf("bufferBackingData(zeroCap) = %p, want nil", data)
	}

	buffer := make([]byte, 0, 16)
	if data := bufferBackingData(buffer); data == nil {
		t.Fatal("bufferBackingData(non-zero-cap buffer) = nil")
	}

	if bufferBackingData(buffer[:0]) != bufferBackingData(buffer[:8]) {
		t.Fatal("same backing array produced different backing data pointers")
	}
}

// TestOwnershipMaxReturnedCapacity verifies fixed-point capacity-growth
// arithmetic.
func TestOwnershipMaxReturnedCapacity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		origin ClassSize
		ratio  PolicyRatio
		want   uint64
	}{
		{name: "zero ratio means origin", origin: ClassSizeFromBytes(512), ratio: 0, want: 512},
		{name: "one", origin: ClassSizeFromBytes(512), ratio: PolicyRatioOne, want: 512},
		{name: "two", origin: ClassSizeFromBytes(512), ratio: 2 * PolicyRatioOne, want: 1024},
		{name: "half", origin: ClassSizeFromBytes(512), ratio: PolicyRatioOne / 2, want: 256},
		{name: "rounds up to one", origin: ClassSizeFromBytes(1), ratio: PolicyRatio(1), want: 1},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ownershipMaxReturnedCapacity(tt.origin, tt.ratio); got != tt.want {
				t.Fatalf("ownershipMaxReturnedCapacity() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestValidateLeaseReleaseBasicFailures verifies validation failures that do not
// depend on strict backing identity.
func TestValidateLeaseReleaseBasicFailures(t *testing.T) {
	t.Parallel()

	record := &leaseRecord{originClass: ClassSizeFromBytes(512), data: bufferBackingData(make([]byte, 0, 512))}
	policy := StrictOwnershipPolicy()

	tests := []struct {
		name     string
		buffer   []byte
		wantKind OwnershipViolationKind
		wantErr  error
	}{
		{name: "nil", buffer: nil, wantKind: OwnershipViolationNilBuffer, wantErr: ErrNilBuffer},
		{name: "zero capacity", buffer: make([]byte, 0), wantKind: OwnershipViolationZeroCapacity, wantErr: ErrZeroCapacity},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kind, err := validateLeaseRelease(record, tt.buffer, policy)
			if kind != tt.wantKind {
				t.Fatalf("violation kind = %s, want %s", kind, tt.wantKind)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want errors.Is(..., %v)", err, tt.wantErr)
			}
		})
	}
}

// TestValidateLeaseReleaseStrictForeignBuffer verifies that strict mode rejects
// a returned slice backed by a different array.
func TestValidateLeaseReleaseStrictForeignBuffer(t *testing.T) {
	t.Parallel()

	acquired := make([]byte, 0, 512)
	foreign := make([]byte, 0, 512)
	record := &leaseRecord{originClass: ClassSizeFromBytes(512), data: bufferBackingData(acquired)}

	kind, err := validateLeaseRelease(record, foreign, StrictOwnershipPolicy())
	if kind != OwnershipViolationForeignBuffer {
		t.Fatalf("violation kind = %s, want %s", kind, OwnershipViolationForeignBuffer)
	}
	if !errors.Is(err, ErrOwnershipViolation) {
		t.Fatalf("error does not match ErrOwnershipViolation: %v", err)
	}
}

// TestValidateLeaseReleaseAccountingAllowsForeignBuffer verifies that accounting
// mode records lease lifecycle without policing backing-array identity.
func TestValidateLeaseReleaseAccountingAllowsForeignBuffer(t *testing.T) {
	t.Parallel()

	acquired := make([]byte, 0, 512)
	foreign := make([]byte, 0, 512)
	record := &leaseRecord{originClass: ClassSizeFromBytes(512), data: bufferBackingData(acquired)}

	kind, err := validateLeaseRelease(record, foreign, AccountingOwnershipPolicy())
	if err != nil {
		t.Fatalf("accounting validation returned error for foreign backing array: %v", err)
	}
	if kind != OwnershipViolationNone {
		t.Fatalf("violation kind = %s, want %s", kind, OwnershipViolationNone)
	}
}

// TestValidateLeaseReleaseStrictCapacityGrowth verifies the growth-ratio check
// directly.
//
// Public Lease.Release normally observes foreign backing-array replacement
// before capacity-growth violation because Go slices cannot grow past cap without
// reallocation. This focused validator test constructs a record where the same
// backing array can be returned with a larger capacity so the growth branch is
// still protected.
func TestValidateLeaseReleaseStrictCapacityGrowth(t *testing.T) {
	t.Parallel()

	backing := make([]byte, 0, 128)
	record := &leaseRecord{
		originClass: ClassSizeFromBytes(32),
		data:        bufferBackingData(backing[:0:32]),
	}
	policy := StrictOwnershipPolicy()
	policy.MaxReturnedCapacityGrowth = PolicyRatioOne

	kind, err := validateLeaseRelease(record, backing[:0:128], policy)
	if kind != OwnershipViolationCapacityGrowth {
		t.Fatalf("violation kind = %s, want %s", kind, OwnershipViolationCapacityGrowth)
	}
	if !errors.Is(err, ErrOwnershipViolation) {
		t.Fatalf("error does not match ErrOwnershipViolation: %v", err)
	}
}

// TestValidateLeaseReleaseAccountingCapacityGrowth verifies that accounting
// mode enforces capacity growth even though it accepts foreign backing arrays.
func TestValidateLeaseReleaseAccountingCapacityGrowth(t *testing.T) {
	t.Parallel()

	record := &leaseRecord{
		originClass: ClassSizeFromBytes(64),
		data:        bufferBackingData(make([]byte, 0, 64)),
	}
	policy := AccountingOwnershipPolicy()
	policy.MaxReturnedCapacityGrowth = PolicyRatioOne

	kind, err := validateLeaseRelease(record, make([]byte, 0, 128), policy)
	if kind != OwnershipViolationCapacityGrowth {
		t.Fatalf("violation kind = %s, want %s", kind, OwnershipViolationCapacityGrowth)
	}
	if !errors.Is(err, ErrOwnershipViolation) {
		t.Fatalf("error does not match ErrOwnershipViolation: %v", err)
	}
}

// TestValidateLeaseReleaseStrictSuccess verifies a valid strict release.
func TestValidateLeaseReleaseStrictSuccess(t *testing.T) {
	t.Parallel()

	buffer := make([]byte, 0, 512)
	record := &leaseRecord{originClass: ClassSizeFromBytes(512), data: bufferBackingData(buffer)}

	kind, err := validateLeaseRelease(record, buffer, StrictOwnershipPolicy())
	if err != nil {
		t.Fatalf("validateLeaseRelease returned error: %v", err)
	}
	if kind != OwnershipViolationNone {
		t.Fatalf("violation kind = %s, want %s", kind, OwnershipViolationNone)
	}
}
