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

import "testing"

// TestOwnershipViolationKindString verifies stable diagnostic names.
func TestOwnershipViolationKindString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind OwnershipViolationKind
		want string
	}{
		{OwnershipViolationNone, "none"},
		{OwnershipViolationInvalidLease, "invalid_lease"},
		{OwnershipViolationDoubleRelease, "double_release"},
		{OwnershipViolationNilBuffer, "nil_buffer"},
		{OwnershipViolationZeroCapacity, "zero_capacity"},
		{OwnershipViolationForeignBuffer, "foreign_buffer"},
		{OwnershipViolationCapacityGrowth, "capacity_growth"},
		{OwnershipViolationKind(255), "unknown"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.kind.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestOwnershipViolationKindIsZero verifies zero-kind semantics used by lease
// snapshots and release validators.
func TestOwnershipViolationKindIsZero(t *testing.T) {
	t.Parallel()

	if !OwnershipViolationNone.IsZero() {
		t.Fatal("OwnershipViolationNone.IsZero() = false, want true")
	}
	if OwnershipViolationDoubleRelease.IsZero() {
		t.Fatal("OwnershipViolationDoubleRelease.IsZero() = true, want false")
	}
}
