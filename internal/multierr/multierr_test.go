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

package multierr

import (
	"errors"
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// customError gives errors.As a concrete target type.
type customError struct {
	message string
}

func (e *customError) Error() string {
	return e.message
}

// TestCombineSkipsNilErrors verifies empty aggregate behavior.
func TestCombineSkipsNilErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		errs []error
	}{
		{name: "empty"},
		{name: "one nil", errs: []error{nil}},
		{name: "multiple nils", errs: []error{nil, nil}},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Combine(tt.errs...); got != nil {
				t.Fatalf("Combine() = %v, want nil", got)
			}
		})
	}
}

// TestCombineReturnsSingleErrorUnchanged verifies that Combine does not wrap
// the common single-error case.
func TestCombineReturnsSingleErrorUnchanged(t *testing.T) {
	t.Parallel()

	err := errors.New("first")

	tests := []struct {
		name string
		errs []error
	}{
		{name: "single", errs: []error{err}},
		{name: "single with nils", errs: []error{nil, err, nil}},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Combine(tt.errs...); got != err {
				t.Fatalf("Combine() = %v, want original error", got)
			}
		})
	}
}

// TestCombineAggregatesMultipleErrors verifies ordering, message formatting,
// and extraction for aggregate errors.
func TestCombineAggregatesMultipleErrors(t *testing.T) {
	t.Parallel()

	first := errors.New("first")
	second := errors.New("second")

	err := Combine(first, second)
	if err == nil {
		t.Fatal("Combine(first, second) = nil, want error")
	}

	if got := err.Error(); got != "first; second" {
		t.Fatalf("Error() = %q, want %q", got, "first; second")
	}

	assertErrors(t, Errors(err), []error{first, second})
}

// TestAppend verifies the two-argument accumulation helper.
func TestAppend(t *testing.T) {
	t.Parallel()

	first := errors.New("first")
	second := errors.New("second")

	tests := []struct {
		name  string
		left  error
		right error
		want  []error
	}{
		{name: "both nil"},
		{name: "left only", left: first, want: []error{first}},
		{name: "right only", right: first, want: []error{first}},
		{name: "both", left: first, right: second, want: []error{first, second}},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Append(tt.left, tt.right)
			if len(tt.want) == 0 {
				if err != nil {
					t.Fatalf("Append() = %v, want nil", err)
				}

				return
			}

			if len(tt.want) == 1 && err != tt.want[0] {
				t.Fatalf("Append() = %v, want original error", err)
			}

			assertErrors(t, Errors(err), tt.want)
		})
	}
}

// TestAppendInto verifies in-place accumulation semantics.
func TestAppendInto(t *testing.T) {
	t.Parallel()

	first := errors.New("first")
	second := errors.New("second")

	var err error
	if AppendInto(&err, nil) {
		t.Fatal("AppendInto(nil error) = true, want false")
	}

	if err != nil {
		t.Fatalf("destination changed after nil append: %v", err)
	}

	if !AppendInto(&err, first) {
		t.Fatal("AppendInto(first) = false, want true")
	}

	if err != first {
		t.Fatalf("destination = %v, want first error unchanged", err)
	}

	if !AppendInto(&err, second) {
		t.Fatal("AppendInto(second) = false, want true")
	}

	assertErrors(t, Errors(err), []error{first, second})
}

// TestAppendIntoPanicsForNilDestination keeps programmer-error behavior stable.
func TestAppendIntoPanicsForNilDestination(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errNilAppendDestination, func() {
		AppendInto(nil, errors.New("boom"))
	})
}

// TestCombineFlattensNestedAggregates verifies that nested aggregate shapes do
// not leak into callers that inspect independent failures.
func TestCombineFlattensNestedAggregates(t *testing.T) {
	t.Parallel()

	first := errors.New("first")
	second := errors.New("second")
	third := errors.New("third")

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "left nested",
			err:  Combine(Combine(first, second), third),
		},
		{
			name: "right nested",
			err:  Combine(first, Combine(second, third)),
		},
		{
			name: "nested nils",
			err:  Combine(Combine(nil, first), nil, Combine(second, nil, third)),
		},
		{
			name: "join compatible",
			err:  Combine(errors.Join(first, second), third),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assertErrors(t, Errors(tt.err), []error{first, second, third})
		})
	}
}

// TestErrors verifies extraction and defensive-copy behavior.
func TestErrors(t *testing.T) {
	t.Parallel()

	first := errors.New("first")
	second := errors.New("second")
	third := errors.New("third")

	if got := Errors(nil); got != nil {
		t.Fatalf("Errors(nil) = %#v, want nil", got)
	}

	assertErrors(t, Errors(first), []error{first})

	combined := Combine(first, second)
	errs := Errors(combined)
	assertErrors(t, errs, []error{first, second})

	errs[0] = third

	assertErrors(t, Errors(combined), []error{first, second})
}

// TestErrorsIs verifies standard sentinel matching through aggregate unwraps.
func TestErrorsIs(t *testing.T) {
	t.Parallel()

	first := errors.New("first")
	second := errors.New("second")
	unrelated := errors.New("unrelated")

	err := Combine(first, second)

	if !errors.Is(err, first) {
		t.Fatal("errors.Is(aggregate, first) = false, want true")
	}

	if !errors.Is(err, second) {
		t.Fatal("errors.Is(aggregate, second) = false, want true")
	}

	if errors.Is(err, unrelated) {
		t.Fatal("errors.Is(aggregate, unrelated) = true, want false")
	}
}

// TestErrorsAs verifies standard typed-error matching through aggregate unwraps.
func TestErrorsAs(t *testing.T) {
	t.Parallel()

	custom := &customError{message: "custom"}
	err := Combine(errors.New("ordinary"), custom)

	var target *customError
	if !errors.As(err, &target) {
		t.Fatal("errors.As(aggregate, *customError) = false, want true")
	}

	if target != custom {
		t.Fatal("errors.As did not return original custom error")
	}
}

func assertErrors(t *testing.T, got []error, want []error) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len(errors) = %d, want %d: %#v", len(got), len(want), got)
	}

	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("errors[%d] = %v, want %v", index, got[index], want[index])
		}
	}
}
