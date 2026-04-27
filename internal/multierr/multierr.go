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

// Package multierr combines independent internal errors.
//
// The package is intentionally small: it provides nil-skipping combination,
// flattening for errors that unwrap to multiple children, and compatibility with
// the standard errors.Is/errors.As traversal. It is not a public API and does
// not try to mirror any third-party package completely.
package multierr

import "strings"

const errNilAppendDestination = "bufferpool.multierr: append destination must not be nil"

// unwrapper is the standard multi-error unwrapping shape used by errors.Join
// and by this package's aggregate type.
type unwrapper interface {
	Unwrap() []error
}

// multiError stores a flattened, ordered list of non-nil errors.
//
// Callers only observe this value through the error interface. The concrete type
// stays private so this package can keep the representation small and stable for
// internal use.
type multiError struct {
	errors []error
}

// Combine returns one error representing all non-nil inputs.
//
// Nested aggregates are flattened, nils are skipped, and input order is
// preserved. If exactly one error remains, that original error is returned
// unchanged.
func Combine(errs ...error) error {
	combined := make([]error, 0, len(errs))
	for _, err := range errs {
		combined = appendFlattened(combined, err)
	}

	switch len(combined) {
	case 0:
		return nil
	case 1:
		return combined[0]
	default:
		return &multiError{
			errors: combined,
		}
	}
}

// Append combines left and right.
//
// It is equivalent to Combine(left, right) and is convenient for incremental
// accumulation sites.
func Append(left, right error) error {
	return Combine(left, right)
}

// AppendInto appends err into dst and reports whether dst changed.
//
// A nil destination is a programming error. A nil err is ignored and leaves dst
// unchanged.
func AppendInto(dst *error, err error) bool {
	if dst == nil {
		panic(errNilAppendDestination)
	}

	if err == nil {
		return false
	}

	*dst = Append(*dst, err)

	return true
}

// Errors returns the independent errors represented by err.
//
// The returned slice is caller-owned. Ordinary single errors are returned as a
// one-element slice, and nil returns nil.
func Errors(err error) []error {
	return appendFlattened(nil, err)
}

// Error joins child messages in a compact deterministic form.
func (e *multiError) Error() string {
	if e == nil {
		return "<nil>"
	}

	var builder strings.Builder
	for index, err := range e.errors {
		if index > 0 {
			builder.WriteString("; ")
		}

		builder.WriteString(err.Error())
	}

	return builder.String()
}

// Unwrap exposes children to errors.Is and errors.As.
//
// The slice is copied so callers cannot mutate aggregate storage.
func (e *multiError) Unwrap() []error {
	if e == nil {
		return nil
	}

	return append([]error(nil), e.errors...)
}

// appendFlattened appends err or its multi-error children to dst.
//
// This helper keeps Combine and Errors consistent: both observe the same
// flattening and nil-skipping rules.
func appendFlattened(dst []error, err error) []error {
	if err == nil {
		return dst
	}

	if wrapped, ok := err.(unwrapper); ok {
		children := wrapped.Unwrap()
		if len(children) > 0 {
			for _, child := range children {
				dst = appendFlattened(dst, child)
			}

			return dst
		}
	}

	return append(dst, err)
}
