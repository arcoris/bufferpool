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
	"io"
	"testing"
)

// TestPublicErrorClasses verifies that all exported sentinel errors are
// initialized and expose stable diagnostic messages.
//
// These sentinel errors are part of the public classification contract. Callers
// should use errors.Is rather than string matching, but stable messages still
// matter for logs, debugging, and documentation.
func TestPublicErrorClasses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "closed",
			err:  ErrClosed,
			want: "bufferpool: closed",
		},
		{
			name: "invalid options",
			err:  ErrInvalidOptions,
			want: "bufferpool: invalid options",
		},
		{
			name: "invalid policy",
			err:  ErrInvalidPolicy,
			want: "bufferpool: invalid policy",
		},
		{
			name: "invalid size",
			err:  ErrInvalidSize,
			want: "bufferpool: invalid size",
		},
		{
			name: "request too large",
			err:  ErrRequestTooLarge,
			want: "bufferpool: request too large",
		},
		{
			name: "buffer too large",
			err:  ErrBufferTooLarge,
			want: "bufferpool: buffer too large",
		},
		{
			name: "invalid buffer",
			err:  ErrInvalidBuffer,
			want: "bufferpool: invalid buffer",
		},
		{
			name: "nil buffer",
			err:  ErrNilBuffer,
			want: "bufferpool: nil buffer",
		},
		{
			name: "zero capacity",
			err:  ErrZeroCapacity,
			want: "bufferpool: zero-capacity buffer",
		},
		{
			name: "unsupported class",
			err:  ErrUnsupportedClass,
			want: "bufferpool: unsupported size class",
		},
		{
			name: "ownership violation",
			err:  ErrOwnershipViolation,
			want: "bufferpool: ownership violation",
		},
		{
			name: "invalid lease",
			err:  ErrInvalidLease,
			want: "bufferpool: invalid lease",
		},
		{
			name: "double release",
			err:  ErrDoubleRelease,
			want: "bufferpool: double release",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.err == nil {
				t.Fatal("sentinel error is nil")
			}

			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("error message = %q, want %q", got, tt.want)
			}

			if !errors.Is(tt.err, tt.err) {
				t.Fatalf("errors.Is(%v, itself) = false, want true", tt.err)
			}
		})
	}
}

// TestPublicErrorClassesAreDistinct verifies that public sentinel errors do not
// accidentally alias each other.
//
// Distinct sentinels allow callers to classify errors precisely with errors.Is.
// If two sentinel variables point to the same value, classification becomes
// ambiguous and public API behavior becomes harder to reason about.
func TestPublicErrorClassesAreDistinct(t *testing.T) {
	t.Parallel()

	errs := []struct {
		name string
		err  error
	}{
		{name: "ErrClosed", err: ErrClosed},
		{name: "ErrInvalidOptions", err: ErrInvalidOptions},
		{name: "ErrInvalidPolicy", err: ErrInvalidPolicy},
		{name: "ErrInvalidSize", err: ErrInvalidSize},
		{name: "ErrRequestTooLarge", err: ErrRequestTooLarge},
		{name: "ErrBufferTooLarge", err: ErrBufferTooLarge},
		{name: "ErrInvalidBuffer", err: ErrInvalidBuffer},
		{name: "ErrNilBuffer", err: ErrNilBuffer},
		{name: "ErrZeroCapacity", err: ErrZeroCapacity},
		{name: "ErrUnsupportedClass", err: ErrUnsupportedClass},
		{name: "ErrOwnershipViolation", err: ErrOwnershipViolation},
		{name: "ErrInvalidLease", err: ErrInvalidLease},
		{name: "ErrDoubleRelease", err: ErrDoubleRelease},
	}

	for i := range errs {
		for j := i + 1; j < len(errs); j++ {
			left := errs[i]
			right := errs[j]

			t.Run(left.name+" is distinct from "+right.name, func(t *testing.T) {
				t.Parallel()

				if left.err == right.err {
					t.Fatalf("%s and %s refer to the same sentinel error", left.name, right.name)
				}
			})
		}
	}
}

// TestNewErrorClassifiesByKind verifies that newError preserves public
// classification while exposing an operation-specific diagnostic message.
//
// The message can change per operation, but errors.Is must still match the
// stable sentinel kind.
func TestNewErrorClassifiesByKind(t *testing.T) {
	t.Parallel()

	err := newError(ErrClosed, "bufferpool: pool is closed")

	if err == nil {
		t.Fatal("newError returned nil")
	}

	if got := err.Error(); got != "bufferpool: pool is closed" {
		t.Fatalf("newError Error() = %q, want %q", got, "bufferpool: pool is closed")
	}

	if !errors.Is(err, ErrClosed) {
		t.Fatal("errors.Is(newError, ErrClosed) = false, want true")
	}

	if errors.Is(err, ErrInvalidOptions) {
		t.Fatal("errors.Is(newError, ErrInvalidOptions) = true, want false")
	}
}

// TestNewErrorWithoutMessageFallsBackToKind verifies classifiedError fallback
// behavior.
//
// Internal helpers may create a classified error with only a kind. In that case,
// Error should return the sentinel kind message instead of an empty string.
func TestNewErrorWithoutMessageFallsBackToKind(t *testing.T) {
	t.Parallel()

	err := newError(ErrInvalidPolicy, "")

	if got := err.Error(); got != ErrInvalidPolicy.Error() {
		t.Fatalf("newError without message Error() = %q, want %q", got, ErrInvalidPolicy.Error())
	}

	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatal("errors.Is(newError without message, ErrInvalidPolicy) = false, want true")
	}
}

// TestNewErrorWithNilKindUsesMessageOnly verifies the wrapper's defensive
// behavior when an internal path has a message but no public classification.
func TestNewErrorWithNilKindUsesMessageOnly(t *testing.T) {
	t.Parallel()

	err := newError(nil, "bufferpool: internal diagnostic")

	if got := err.Error(); got != "bufferpool: internal diagnostic" {
		t.Fatalf("newError with nil kind Error() = %q, want %q", got, "bufferpool: internal diagnostic")
	}

	if errors.Is(err, ErrClosed) {
		t.Fatal("errors.Is(newError with nil kind, ErrClosed) = true, want false")
	}

	if got := errors.Unwrap(err); got != nil {
		t.Fatalf("errors.Unwrap(newError with nil kind) = %v, want nil", got)
	}
}

// TestWrapErrorClassifiesByKindAndPreservesCause verifies the two-level error
// contract.
//
// wrapError should expose a stable bufferpool-level classification through
// errors.Is while still preserving the lower-level cause through Unwrap.
func TestWrapErrorClassifiesByKindAndPreservesCause(t *testing.T) {
	t.Parallel()

	cause := io.ErrUnexpectedEOF
	err := wrapError(ErrInvalidBuffer, cause, "bufferpool: failed to inspect returned buffer")

	if err == nil {
		t.Fatal("wrapError returned nil")
	}

	if got := err.Error(); got != "bufferpool: failed to inspect returned buffer" {
		t.Fatalf("wrapError Error() = %q, want %q", got, "bufferpool: failed to inspect returned buffer")
	}

	if !errors.Is(err, ErrInvalidBuffer) {
		t.Fatal("errors.Is(wrapError, ErrInvalidBuffer) = false, want true")
	}

	if !errors.Is(err, cause) {
		t.Fatal("errors.Is(wrapError, cause) = false, want true")
	}

	if got := errors.Unwrap(err); got != cause {
		t.Fatalf("errors.Unwrap(wrapError) = %v, want %v", got, cause)
	}
}

// TestWrapErrorWithoutMessageFallsBackToKind verifies that wrapped classified
// errors still have a useful message when the caller omits an operation-specific
// diagnostic.
//
// The public kind should be preferred over the cause for the top-level message
// because the returned error is primarily classified as a bufferpool error.
func TestWrapErrorWithoutMessageFallsBackToKind(t *testing.T) {
	t.Parallel()

	err := wrapError(ErrOwnershipViolation, io.ErrClosedPipe, "")

	if got := err.Error(); got != ErrOwnershipViolation.Error() {
		t.Fatalf("wrapError without message Error() = %q, want %q", got, ErrOwnershipViolation.Error())
	}

	if !errors.Is(err, ErrOwnershipViolation) {
		t.Fatal("errors.Is(wrapError without message, ErrOwnershipViolation) = false, want true")
	}

	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("errors.Is(wrapError without message, cause) = false, want true")
	}
}

// TestWrapErrorWithoutKindOrMessageFallsBackToCause verifies that a wrapped
// cause still produces a useful diagnostic when no public kind is available.
func TestWrapErrorWithoutKindOrMessageFallsBackToCause(t *testing.T) {
	t.Parallel()

	err := wrapError(nil, io.ErrClosedPipe, "")

	if got := err.Error(); got != io.ErrClosedPipe.Error() {
		t.Fatalf("wrapError without kind or message Error() = %q, want %q", got, io.ErrClosedPipe.Error())
	}

	if errors.Is(err, ErrClosed) {
		t.Fatal("errors.Is(wrapError without kind or message, ErrClosed) = true, want false")
	}

	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("errors.Is(wrapError without kind or message, cause) = false, want true")
	}
}

// TestClassifiedErrorMatchesWrappedKind verifies that public classification is
// stable even if an internal helper passes an already-classified kind.
func TestClassifiedErrorMatchesWrappedKind(t *testing.T) {
	t.Parallel()

	kind := wrapError(ErrInvalidSize, io.ErrUnexpectedEOF, "bufferpool: invalid size from lower layer")
	err := newError(kind, "bufferpool: request size is outside configured limits")

	if !errors.Is(err, ErrInvalidSize) {
		t.Fatal("errors.Is(err, ErrInvalidSize) = false, want true")
	}

	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("errors.Is(err, wrapped kind cause) = false, want true")
	}

	if errors.Is(err, ErrRequestTooLarge) {
		t.Fatal("errors.Is(err, ErrRequestTooLarge) = true, want false")
	}
}

// TestClassifiedErrorFallsBackToCause verifies the final Error fallback.
//
// This case is not expected for normal newError or wrapError usage with a public
// kind, but classifiedError should still produce a meaningful message when only
// a cause is present.
func TestClassifiedErrorFallsBackToCause(t *testing.T) {
	t.Parallel()

	err := &classifiedError{
		cause: io.ErrUnexpectedEOF,
	}

	if got := err.Error(); got != io.ErrUnexpectedEOF.Error() {
		t.Fatalf("classifiedError cause fallback Error() = %q, want %q", got, io.ErrUnexpectedEOF.Error())
	}

	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("errors.Is(classifiedError cause fallback, cause) = false, want true")
	}
}

// TestClassifiedErrorDefaultFallback verifies the defensive fallback for an
// empty classifiedError.
//
// This protects diagnostics from returning an empty string if an internal helper
// accidentally constructs a classifiedError without kind, message, or cause.
func TestClassifiedErrorDefaultFallback(t *testing.T) {
	t.Parallel()

	err := &classifiedError{}

	if got := err.Error(); got != "bufferpool: error" {
		t.Fatalf("empty classifiedError Error() = %q, want %q", got, "bufferpool: error")
	}

	if errors.Is(err, ErrClosed) {
		t.Fatal("errors.Is(empty classifiedError, ErrClosed) = true, want false")
	}

	if got := errors.Unwrap(err); got != nil {
		t.Fatalf("errors.Unwrap(empty classifiedError) = %v, want nil", got)
	}
}

// TestNilClassifiedErrorMethods verifies nil receiver behavior.
//
// These methods are defensive because classifiedError is unexported, but keeping
// nil receiver behavior explicit makes later refactors safer.
func TestNilClassifiedErrorMethods(t *testing.T) {
	t.Parallel()

	var err *classifiedError

	if got := err.Error(); got != "<nil>" {
		t.Fatalf("nil classifiedError Error() = %q, want %q", got, "<nil>")
	}

	if got := err.Unwrap(); got != nil {
		t.Fatalf("nil classifiedError Unwrap() = %v, want nil", got)
	}

	if err.Is(ErrClosed) {
		t.Fatal("nil classifiedError Is(ErrClosed) = true, want false")
	}
}

// TestClassifiedErrorIsRequiresMatchingKind verifies exact kind matching.
//
// classifiedError.Is should match only the stable public kind. It should not
// match unrelated sentinels just because the diagnostic message is similar or a
// cause exists.
func TestClassifiedErrorIsRequiresMatchingKind(t *testing.T) {
	t.Parallel()

	err := wrapError(ErrInvalidLease, ErrOwnershipViolation, "bufferpool: invalid lease ownership")

	if !errors.Is(err, ErrInvalidLease) {
		t.Fatal("errors.Is(err, ErrInvalidLease) = false, want true")
	}

	if !errors.Is(err, ErrOwnershipViolation) {
		t.Fatal("errors.Is(err, ErrOwnershipViolation cause) = false, want true")
	}

	if errors.Is(err, ErrDoubleRelease) {
		t.Fatal("errors.Is(err, ErrDoubleRelease) = true, want false")
	}

	if errors.Is(err, nil) {
		t.Fatal("errors.Is(err, nil) = true, want false")
	}
}

// TestClassifiedErrorWithNilKindStillUnwrapsCause verifies that the wrapper can
// preserve a cause even without a public kind.
//
// Normal bufferpool usage should provide a kind, but this behavior keeps the
// wrapper robust for internal transitional paths.
func TestClassifiedErrorWithNilKindStillUnwrapsCause(t *testing.T) {
	t.Parallel()

	err := wrapError(nil, io.ErrClosedPipe, "bufferpool: lower-level close failure")

	if got := err.Error(); got != "bufferpool: lower-level close failure" {
		t.Fatalf("classified error with nil kind Error() = %q, want %q", got, "bufferpool: lower-level close failure")
	}

	if errors.Is(err, ErrClosed) {
		t.Fatal("errors.Is(err, ErrClosed) = true, want false")
	}

	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("errors.Is(err, io.ErrClosedPipe) = false, want true")
	}
}
