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

package testutil

import (
	"fmt"
	"testing"
)

// MustPanic verifies that fn panics.
//
// Use this helper when a test only needs to prove that an invariant guard
// rejects invalid input and the exact panic payload is not part of the contract
// being exercised.
func MustPanic(t *testing.T, fn func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Fatal("function did not panic")
		}
	}()

	fn()
}

// MustPanicWithMessage verifies that fn panics with the expected message.
//
// Use this helper when the panic payload identifies the specific invariant that
// failed. The comparison is string-based so tests can assert ordinary panic
// values, errors, or domain values without duplicating recover boilerplate.
func MustPanicWithMessage(t *testing.T, want string, fn func()) {
	t.Helper()

	defer func() {
		got := recover()
		if got == nil {
			t.Fatalf("function did not panic, want panic %q", want)
		}

		gotMessage := fmt.Sprint(got)
		if gotMessage != want {
			t.Fatalf("panic message = %q, want %q", gotMessage, want)
		}
	}()

	fn()
}
