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

import "strconv"

const (
	// errSizeNegativeInt is used when a signed integer is converted to Size with
	// a negative value.
	//
	// Buffer sizes, capacities, and byte budgets cannot be negative. Public
	// option validation may choose to return classified errors, but this low-level
	// conversion helper treats negative input as a programmer error.
	errSizeNegativeInt = "bufferpool.Size: value must not be negative"

	// errSizeTooLargeForInt is used when Size cannot be represented as int on
	// the current platform.
	//
	// This matters before using Size with Go APIs that require int lengths or
	// capacities, such as make([]byte, 0, n). The conversion must fail fast
	// instead of silently truncating.
	errSizeTooLargeForInt = "bufferpool.Size: value exceeds int range"
)

// Size is a generic byte-size value used by bufferpool configuration, requests,
// capacities, limits, and byte budgets.
//
// Size is intentionally not named ClassSize. A Size value may represent many
// different byte-sized concepts:
//
//   - caller-requested buffer length;
//   - minimum configured buffer size;
//   - maximum configured request size;
//   - maximum retained buffer capacity;
//   - pool, partition, class, or group byte budget;
//   - observed returned capacity;
//   - observed allocated capacity;
//   - observed retained bytes.
//
// A size class is a stricter concept: it is a normalized reusable buffer
// capacity selected by a class table. That concept should be represented by a
// separate ClassSize type in class_size.go or class.go.
//
// The zero value is valid as a generic size value. Specific option validators
// decide whether zero means "unset", "disabled", or "invalid" in a given
// context.
//
// Size uses binary memory units. In this package:
//
//	1 KiB = 1024 bytes
//	1 MiB = 1024 KiB
//	1 GiB = 1024 MiB
type Size uint64

const (
	// Byte is one byte.
	Byte Size = 1

	// KiB is one binary kibibyte, equal to 1024 bytes.
	KiB Size = 1 << 10

	// MiB is one binary mebibyte, equal to 1024 KiB.
	MiB Size = 1 << 20

	// GiB is one binary gibibyte, equal to 1024 MiB.
	GiB Size = 1 << 30
)

// SizeFromInt converts a non-negative int to Size.
//
// Use this helper at API or runtime boundaries where Go naturally exposes sizes
// as int, for example len(buffer), cap(buffer), or user-provided integer options.
//
// Negative values panic because a negative byte size has no valid interpretation
// in bufferpool accounting.
func SizeFromInt(value int) Size {
	if value < 0 {
		panic(errSizeNegativeInt)
	}

	return Size(value)
}

// SizeFromBytes returns a Size from a raw byte count.
//
// This helper is intentionally small, but it gives call sites a readable way to
// mark that a uint64 value is being treated as a byte-size quantity rather than a
// counter, generation, shard count, or buffer count.
func SizeFromBytes(bytes uint64) Size {
	return Size(bytes)
}

// Bytes returns the raw byte count represented by s.
func (s Size) Bytes() uint64 {
	return uint64(s)
}

// Int returns s as int.
//
// The method panics if s cannot be represented as int on the current platform.
// This prevents silent truncation before using Size with APIs such as:
//
//	make([]byte, 0, size.Int())
func (s Size) Int() int {
	maxInt := uint64(^uint(0) >> 1)
	if uint64(s) > maxInt {
		panic(errSizeTooLargeForInt)
	}

	return int(s)
}

// IsZero reports whether s is zero.
func (s Size) IsZero() bool {
	return s == 0
}

// IsPositive reports whether s is greater than zero.
func (s Size) IsPositive() bool {
	return s > 0
}

// IsPowerOfTwo reports whether s is a non-zero power of two.
//
// This helper is useful for class-table and profile validation. It does not mean
// every Size must be a power of two. Generic byte-size values may be arbitrary.
func (s Size) IsPowerOfTwo() bool {
	return s != 0 && s&(s-1) == 0
}

// Less reports whether s is smaller than other.
func (s Size) Less(other Size) bool {
	return s < other
}

// LessOrEqual reports whether s is smaller than or equal to other.
func (s Size) LessOrEqual(other Size) bool {
	return s <= other
}

// Greater reports whether s is larger than other.
func (s Size) Greater(other Size) bool {
	return s > other
}

// GreaterOrEqual reports whether s is larger than or equal to other.
func (s Size) GreaterOrEqual(other Size) bool {
	return s >= other
}

// Clamp returns s constrained to the inclusive range [minValue, maxValue].
//
// The caller MUST pass a valid range where minValue <= maxValue. Invalid ranges
// are treated as programmer errors by the underlying clamp helper.
func (s Size) Clamp(minValue, maxValue Size) Size {
	if s < minValue {
		return minValue
	}

	if s > maxValue {
		return maxValue
	}

	return s
}

// String returns a stable human-readable representation of s.
//
// The formatter uses binary units only when the value is an exact multiple of
// GiB, MiB, or KiB. Non-aligned values are rendered as bytes so diagnostics do
// not hide precision.
//
// Examples:
//
//	0        -> "0 B"
//	512      -> "512 B"
//	1024     -> "1 KiB"
//	1536     -> "1536 B"
//	1048576  -> "1 MiB"
func (s Size) String() string {
	switch {
	case s == 0:
		return "0 B"
	case s >= GiB && s%GiB == 0:
		return strconv.FormatUint(uint64(s/GiB), 10) + " GiB"
	case s >= MiB && s%MiB == 0:
		return strconv.FormatUint(uint64(s/MiB), 10) + " MiB"
	case s >= KiB && s%KiB == 0:
		return strconv.FormatUint(uint64(s/KiB), 10) + " KiB"
	default:
		return strconv.FormatUint(uint64(s), 10) + " B"
	}
}
