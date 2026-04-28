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

// TestLeaseIDMethods verifies the small value helpers used by snapshots and
// diagnostics.
func TestLeaseIDMethods(t *testing.T) {
	t.Parallel()

	var zero LeaseID
	if !zero.IsZero() {
		t.Fatal("zero LeaseID did not report IsZero")
	}
	if zero.Uint64() != 0 {
		t.Fatalf("zero.Uint64() = %d, want 0", zero.Uint64())
	}
	if zero.String() != "0" {
		t.Fatalf("zero.String() = %q, want %q", zero.String(), "0")
	}

	id := LeaseID(42)
	if id.IsZero() {
		t.Fatal("non-zero LeaseID reported IsZero")
	}
	if id.Uint64() != 42 {
		t.Fatalf("id.Uint64() = %d, want 42", id.Uint64())
	}
	if id.String() != "42" {
		t.Fatalf("id.String() = %q, want %q", id.String(), "42")
	}
}
