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

const (
	// errClassSizeZero is used when a ClassSize is constructed from a zero value.
	//
	// A size class represents reusable buffer capacity. Zero-capacity classes are
	// invalid because they cannot satisfy any positive request, cannot contribute
	// retained capacity, and would make class-to-shard credit arithmetic
	// ambiguous.
	errClassSizeZero = "bufferpool.ClassSize: value must be greater than zero"
)

// ClassSize is the normalized reusable capacity of one size class.
//
// ClassSize is intentionally distinct from Size. Size is a generic byte-size
// value used for requests, options, capacities, and budgets. ClassSize is a
// stricter domain value: it means "the capacity assigned to a concrete size
// class".
//
// Examples:
//
//	requested size = 3000 B
//	class size     = 4 KiB
//
//	requested size = 900 B
//	class size     = 1 KiB
//
// A ClassSize is normally produced by class-table construction or by class-table
// lookup. Code that has only a user request size should not treat it as a
// ClassSize until it has been normalized through the class table.
//
// ClassSize is used by:
//
//   - class-table lookup;
//   - class-level retained accounting;
//   - class budget allocation;
//   - shard credit calculation;
//   - admission checks;
//   - owner-side capacity checks;
//   - snapshots that need to report normalized class capacity.
//
// The zero value is invalid as an enabled class size. It may appear only as an
// unset field before validation. Constructors in this file reject zero.
type ClassSize Size

// ClassSizeFromSize converts a generic Size into ClassSize.
//
// The input MUST be greater than zero. This function does not require the value
// to be a power of two. Power-of-two class profiles are a policy/table decision,
// not a universal ClassSize invariant.
//
// Use this function when class-table or profile validation has decided that a
// generic Size value is intended to be a concrete class capacity.
func ClassSizeFromSize(size Size) ClassSize {
	if size.IsZero() {
		panic(errClassSizeZero)
	}

	return ClassSize(size)
}

// ClassSizeFromBytes converts a raw byte count into ClassSize.
//
// The input MUST be greater than zero. Prefer ClassSizeFromSize when the caller
// already works with Size values. Use this helper at boundaries where byte counts
// arrive as uint64 values.
func ClassSizeFromBytes(bytes uint64) ClassSize {
	return ClassSizeFromSize(SizeFromBytes(bytes))
}

// ClassSizeFromInt converts a non-negative int into ClassSize.
//
// The value MUST be greater than zero. Negative values are rejected by SizeFromInt.
// Zero is rejected by ClassSizeFromSize.
func ClassSizeFromInt(value int) ClassSize {
	return ClassSizeFromSize(SizeFromInt(value))
}

// Size returns c as a generic Size.
//
// Use this when a ClassSize needs to participate in generic byte-size helpers,
// option comparisons, formatting, or budget calculations that operate on Size.
func (c ClassSize) Size() Size {
	return Size(c)
}

// Bytes returns the raw byte count represented by c.
func (c ClassSize) Bytes() uint64 {
	return Size(c).Bytes()
}

// Int returns c as int.
//
// The method panics if c cannot be represented as int on the current platform.
// This protects call sites before using ClassSize with APIs such as:
//
//	make([]byte, 0, classSize.Int())
func (c ClassSize) Int() int {
	return Size(c).Int()
}

// IsZero reports whether c is zero.
//
// A zero ClassSize is invalid for an enabled class. The predicate exists for
// validation code that needs to inspect partially initialized values.
func (c ClassSize) IsZero() bool {
	return c == 0
}

// IsPositive reports whether c is greater than zero.
func (c ClassSize) IsPositive() bool {
	return c > 0
}

// IsPowerOfTwo reports whether c is a power-of-two class size.
//
// This helper is useful for validating default power-of-two profiles. It is not
// a mandatory invariant for every possible class table.
func (c ClassSize) IsPowerOfTwo() bool {
	return Size(c).IsPowerOfTwo()
}

// Less reports whether c is smaller than other.
func (c ClassSize) Less(other ClassSize) bool {
	return c < other
}

// LessOrEqual reports whether c is smaller than or equal to other.
func (c ClassSize) LessOrEqual(other ClassSize) bool {
	return c <= other
}

// Greater reports whether c is larger than other.
func (c ClassSize) Greater(other ClassSize) bool {
	return c > other
}

// GreaterOrEqual reports whether c is larger than or equal to other.
func (c ClassSize) GreaterOrEqual(other ClassSize) bool {
	return c >= other
}

// CanServe reports whether this class size can serve requestedSize.
//
// A class can serve a request when its normalized capacity is greater than or
// equal to the requested size. Zero requested size is allowed here because
// higher-level API validation decides whether zero-length requests are valid.
// The class-size relation itself is purely capacity-based.
func (c ClassSize) CanServe(requestedSize Size) bool {
	return Size(c).GreaterOrEqual(requestedSize)
}

// WasteFor returns the internal fragmentation introduced when requestedSize is
// served by this class.
//
// If requestedSize is greater than the class size, the result is zero because the
// class cannot serve the request and there is no valid in-class waste to report.
//
// This helper returns bytes, not a ratio. Ratio calculation belongs outside this
// class-size primitive.
func (c ClassSize) WasteFor(requestedSize Size) Size {
	classSize := Size(c)
	if requestedSize >= classSize {
		return 0
	}

	return classSize - requestedSize
}

// String returns a stable human-readable representation of c.
func (c ClassSize) String() string {
	return Size(c).String()
}
