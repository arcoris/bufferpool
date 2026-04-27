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

import "errors"

var (
	// ErrClosed reports that an operation was attempted on a closed runtime
	// component.
	//
	// This error class is intended for public operations that cannot proceed
	// after a pool, partition, group, controller, lease, or related runtime object
	// has entered a closed or closing lifecycle state.
	//
	// Callers may use errors.Is(err, ErrClosed) to detect this condition.
	ErrClosed = errors.New("bufferpool: closed")

	// ErrInvalidOptions reports invalid user-supplied construction options.
	//
	// This error class is intended for configuration errors detected before a
	// runtime object is created, for example invalid budgets, invalid size
	// limits, invalid shard counts, invalid partition counts, or contradictory
	// option combinations.
	//
	// Callers may use errors.Is(err, ErrInvalidOptions) to detect this condition.
	ErrInvalidOptions = errors.New("bufferpool: invalid options")

	// ErrInvalidPolicy reports an invalid memory-retention policy.
	//
	// This error class is intended for policy validation failures, for example
	// invalid pressure thresholds, invalid retention targets, invalid trim
	// settings, invalid class limits, or inconsistent adaptive-control settings.
	//
	// Callers may use errors.Is(err, ErrInvalidPolicy) to detect this condition.
	ErrInvalidPolicy = errors.New("bufferpool: invalid policy")

	// ErrInvalidSize reports a size value that is not valid for the requested
	// operation.
	//
	// This error class is intended for invalid request sizes, class sizes,
	// retained capacities, trim sizes, budget sizes, or other size-related
	// values that violate the operation contract.
	//
	// Callers may use errors.Is(err, ErrInvalidSize) to detect this condition.
	ErrInvalidSize = errors.New("bufferpool: invalid size")

	// ErrRequestTooLarge reports that a requested buffer size exceeds the
	// configured request limit.
	//
	// This error class is about acquisition/request-side limits. It is distinct
	// from ErrBufferTooLarge, which is about a returned or retained buffer whose
	// capacity is too large to keep.
	//
	// Callers may use errors.Is(err, ErrRequestTooLarge) to detect this condition.
	ErrRequestTooLarge = errors.New("bufferpool: request too large")

	// ErrBufferTooLarge reports that a buffer capacity exceeds the configured
	// retained-buffer limit.
	//
	// This error class is about return/admission-side limits. A pool may choose
	// to discard such a buffer rather than retain it. If the public API reports
	// the condition as an error, it should use this class.
	//
	// Callers may use errors.Is(err, ErrBufferTooLarge) to detect this condition.
	ErrBufferTooLarge = errors.New("bufferpool: buffer too large")

	// ErrInvalidBuffer reports that a buffer or buffer handle is not valid for
	// the requested operation.
	//
	// This is the general buffer-validation class. More specific errors such as
	// ErrNilBuffer, ErrZeroCapacity, ErrUnsupportedClass, ErrInvalidLease, and
	// ErrDoubleRelease should be preferred when the exact reason is known.
	//
	// Callers may use errors.Is(err, ErrInvalidBuffer) to detect this condition.
	ErrInvalidBuffer = errors.New("bufferpool: invalid buffer")

	// ErrNilBuffer reports that a nil buffer was supplied where a valid buffer
	// was required.
	//
	// A nil buffer may be treated as a drop reason in admission paths or as an
	// invalid input in stricter public API paths.
	//
	// Callers may use errors.Is(err, ErrNilBuffer) to detect this condition.
	ErrNilBuffer = errors.New("bufferpool: nil buffer")

	// ErrZeroCapacity reports that a buffer has zero capacity where reusable
	// capacity was required.
	//
	// A zero-capacity buffer cannot contribute useful retained memory and should
	// not be admitted into retained storage.
	//
	// Callers may use errors.Is(err, ErrZeroCapacity) to detect this condition.
	ErrZeroCapacity = errors.New("bufferpool: zero-capacity buffer")

	// ErrUnsupportedClass reports that a size or capacity cannot be mapped to a
	// supported size class.
	//
	// This may happen when a request or returned buffer falls outside the class
	// table, violates class-alignment rules, or targets a class disabled by the
	// current policy.
	//
	// Callers may use errors.Is(err, ErrUnsupportedClass) to detect this condition.
	ErrUnsupportedClass = errors.New("bufferpool: unsupported size class")

	// ErrRetentionRejected reports that a returned buffer was classified but not
	// accepted into retained storage.
	//
	// This is a retention/admission outcome, not necessarily an invalid caller
	// buffer. More specific retention errors such as ErrRetentionCreditExhausted
	// and ErrRetentionStorageFull should be preferred when the rejecting layer is
	// known.
	//
	// Callers may use errors.Is(err, ErrRetentionRejected) to detect this condition.
	ErrRetentionRejected = errors.New("bufferpool: retention rejected")

	// ErrRetentionCreditExhausted reports that shard-local retained-buffer credit
	// is exhausted.
	//
	// The returned buffer may be valid and class-compatible, but the selected
	// shard has no remaining byte or buffer credit under the current policy
	// snapshot.
	//
	// Callers may use errors.Is(err, ErrRetentionCreditExhausted) to detect this condition.
	ErrRetentionCreditExhausted = errors.New("bufferpool: retention credit exhausted")

	// ErrRetentionStorageFull reports that physical retained-buffer storage is
	// full after credit accepted the returned buffer.
	//
	// This is a storage-capacity outcome, distinct from invalid buffer input or
	// unsupported class routing.
	//
	// Callers may use errors.Is(err, ErrRetentionStorageFull) to detect this condition.
	ErrRetentionStorageFull = errors.New("bufferpool: retention storage full")

	// ErrOwnershipViolation reports that a buffer ownership invariant was
	// violated.
	//
	// This is the general ownership/accounting class. It is intended for strict
	// ownership modes where the runtime tracks origin class, requested size,
	// acquired capacity, returned capacity, hold duration, in-use bytes, or
	// repeated return attempts.
	//
	// Callers may use errors.Is(err, ErrOwnershipViolation) to detect this condition.
	ErrOwnershipViolation = errors.New("bufferpool: ownership violation")

	// ErrInvalidLease reports that a lease, ownership token, or checked-out
	// buffer handle is invalid for the requested operation.
	//
	// This may indicate that the handle does not belong to this runtime object,
	// has already been completed, was created by a different accounting mode, or
	// otherwise cannot be used safely.
	//
	// Callers may use errors.Is(err, ErrInvalidLease) to detect this condition.
	ErrInvalidLease = errors.New("bufferpool: invalid lease")

	// ErrDoubleRelease reports that a checked-out buffer, lease, or ownership
	// handle was returned more than once.
	//
	// In strict ownership modes, double release is a correctness violation. In
	// less strict modes, the same condition may be observable as a drop reason
	// instead of a returned error.
	//
	// Callers may use errors.Is(err, ErrDoubleRelease) to detect this condition.
	ErrDoubleRelease = errors.New("bufferpool: double release")
)

// classifiedError is an internal error wrapper that preserves a stable public
// error class while carrying a more specific diagnostic message and optional
// cause.
//
// The public sentinel errors above are intentionally broad classifications, not
// complete operation diagnostics. Runtime code can use classifiedError to return
// precise messages without forcing callers to match strings.
//
// Example:
//
//	err := newError(ErrClosed, "bufferpool: pool is closed")
//
// errors.Is(err, ErrClosed) will return true.
type classifiedError struct {
	// kind is the stable public error class.
	kind error

	// message is the operation-specific diagnostic message.
	message string

	// cause is an optional underlying error.
	cause error
}

// Error returns the diagnostic message.
//
// If message is empty, Error falls back to the public kind message. This makes
// classifiedError robust for internal helpers that only need classification.
func (e *classifiedError) Error() string {
	if e == nil {
		return "<nil>"
	}

	if e.message != "" {
		return e.message
	}

	if e.kind != nil {
		return e.kind.Error()
	}

	if e.cause != nil {
		return e.cause.Error()
	}

	return "bufferpool: error"
}

// Unwrap returns the optional underlying cause.
//
// The cause is separate from the public error class. Classification is handled
// by Is so that errors.Is can match the stable sentinel even when the wrapped
// cause is unrelated.
func (e *classifiedError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.cause
}

// Is reports whether target matches this error's public class.
//
// This method is what makes:
//
//	errors.Is(err, ErrClosed)
//
// work for errors created through newError or wrapError.
func (e *classifiedError) Is(target error) bool {
	if e == nil || e.kind == nil || target == nil {
		return false
	}

	return errors.Is(e.kind, target)
}

// newError creates a classified error with a stable public kind and a detailed
// message.
//
// The helper is intentionally unexported. Public API should expose stable
// sentinel errors through errors.Is rather than committing to a large exported
// error-construction surface.
func newError(kind error, message string) error {
	return &classifiedError{
		kind:    kind,
		message: message,
	}
}

// wrapError creates a classified error with a stable public kind, a detailed
// message, and an underlying cause.
//
// Use this helper when the operation has both a bufferpool-level classification
// and a lower-level because that should remain available through errors.Unwrap.
func wrapError(kind error, cause error, message string) error {
	return &classifiedError{
		kind:    kind,
		message: message,
		cause:   cause,
	}
}
