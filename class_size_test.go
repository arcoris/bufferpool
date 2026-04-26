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
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestClassSizeFromSize verifies conversion from generic Size to ClassSize.
//
// ClassSizeFromSize is the main boundary where a generic byte-size value becomes
// a normalized size-class capacity. The function should preserve the exact byte
// value for positive sizes.
func TestClassSizeFromSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// size is the generic byte-size value being converted.
		size Size

		// want is the expected class-size value.
		want ClassSize
	}{
		{
			name: "one byte",
			size: Byte,
			want: ClassSize(Byte),
		},
		{
			name: "one kibibyte",
			size: KiB,
			want: ClassSize(KiB),
		},
		{
			name: "non power of two",
			size: Size(1536),
			want: ClassSize(1536),
		},
		{
			name: "one mibibyte",
			size: MiB,
			want: ClassSize(MiB),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ClassSizeFromSize(tt.size)
			if got != tt.want {
				t.Fatalf("ClassSizeFromSize(%d) = %d, want %d", tt.size, got, tt.want)
			}
		})
	}
}

// TestClassSizeFromSizePanicsForZero verifies that enabled class sizes cannot be
// zero.
//
// A zero class size cannot serve positive requests and would make class-to-shard
// credit arithmetic invalid.
func TestClassSizeFromSizePanicsForZero(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errClassSizeZero, func() {
		_ = ClassSizeFromSize(0)
	})
}

// TestClassSizeFromBytes verifies conversion from raw byte counts.
//
// This helper is useful at lower-level boundaries where class capacities arrive
// as uint64 values.
func TestClassSizeFromBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// bytes is the raw class capacity in bytes.
		bytes uint64

		// want is the expected class-size value.
		want ClassSize
	}{
		{
			name:  "one byte",
			bytes: 1,
			want:  ClassSize(Byte),
		},
		{
			name:  "one kibibyte",
			bytes: uint64(KiB),
			want:  ClassSize(KiB),
		},
		{
			name:  "non aligned",
			bytes: 3000,
			want:  ClassSize(3000),
		},
		{
			name:  "one gibibyte",
			bytes: uint64(GiB),
			want:  ClassSize(GiB),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ClassSizeFromBytes(tt.bytes)
			if got != tt.want {
				t.Fatalf("ClassSizeFromBytes(%d) = %d, want %d", tt.bytes, got, tt.want)
			}
		})
	}
}

// TestClassSizeFromBytesPanicsForZero verifies zero validation through raw-byte
// construction.
func TestClassSizeFromBytesPanicsForZero(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errClassSizeZero, func() {
		_ = ClassSizeFromBytes(0)
	})
}

// TestClassSizeFromInt verifies conversion from int.
//
// This helper is useful when class capacities are derived from Go APIs that use
// int lengths or capacities.
func TestClassSizeFromInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is the int class capacity.
		value int

		// want is the expected class-size value.
		want ClassSize
	}{
		{
			name:  "one byte",
			value: 1,
			want:  ClassSize(Byte),
		},
		{
			name:  "one kibibyte",
			value: 1024,
			want:  ClassSize(KiB),
		},
		{
			name:  "non aligned",
			value: 1536,
			want:  ClassSize(1536),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ClassSizeFromInt(tt.value)
			if got != tt.want {
				t.Fatalf("ClassSizeFromInt(%d) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

// TestClassSizeFromIntPanicsForZero verifies that zero int values are rejected as
// class sizes.
func TestClassSizeFromIntPanicsForZero(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errClassSizeZero, func() {
		_ = ClassSizeFromInt(0)
	})
}

// TestClassSizeFromIntPanicsForNegativeValue verifies that negative values are
// rejected by the underlying Size conversion.
//
// The exact panic message belongs to SizeFromInt because negative validation is
// a generic size boundary, not a class-size-specific invariant.
func TestClassSizeFromIntPanicsForNegativeValue(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errSizeNegativeInt, func() {
		_ = ClassSizeFromInt(-1)
	})
}

// TestClassSizeSize verifies conversion back to generic Size.
//
// ClassSize should be usable in generic byte-size calculations without losing
// the original capacity value.
func TestClassSizeSize(t *testing.T) {
	t.Parallel()

	classSize := ClassSizeFromSize(4 * KiB)

	got := classSize.Size()
	if got != 4*KiB {
		t.Fatalf("ClassSize.Size() = %d, want %d", got, 4*KiB)
	}
}

// TestClassSizeBytes verifies raw byte extraction.
func TestClassSizeBytes(t *testing.T) {
	t.Parallel()

	classSize := ClassSizeFromBytes(4096)

	if got := classSize.Bytes(); got != 4096 {
		t.Fatalf("ClassSize.Bytes() = %d, want 4096", got)
	}
}

// TestClassSizeInt verifies safe conversion to int.
//
// This conversion is needed before allocating buffers with class capacity.
func TestClassSizeInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// classSize is the value being converted to int.
		classSize ClassSize

		// want is the expected int value.
		want int
	}{
		{
			name:      "one byte",
			classSize: ClassSizeFromSize(Byte),
			want:      1,
		},
		{
			name:      "one kibibyte",
			classSize: ClassSizeFromSize(KiB),
			want:      1024,
		},
		{
			name:      "non aligned",
			classSize: ClassSizeFromBytes(1536),
			want:      1536,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.classSize.Int()
			if got != tt.want {
				t.Fatalf("ClassSize(%d).Int() = %d, want %d", tt.classSize, got, tt.want)
			}
		})
	}
}

// TestClassSizeIntPanicsWhenValueExceedsIntRange verifies that ClassSize
// preserves Size.Int safety.
func TestClassSizeIntPanicsWhenValueExceedsIntRange(t *testing.T) {
	t.Parallel()

	maxInt := uint64(^uint(0) >> 1)
	tooLarge := ClassSize(maxInt + 1)

	testutil.MustPanicWithMessage(t, errSizeTooLargeForInt, func() {
		_ = tooLarge.Int()
	})
}

// TestClassSizePredicates verifies zero, positive, and power-of-two
// classification.
//
// Power-of-two is not a universal ClassSize invariant, but it is useful for
// validating default power-of-two class profiles.
func TestClassSizePredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// classSize is the value being classified.
		classSize ClassSize

		wantZero       bool
		wantPositive   bool
		wantPowerOfTwo bool
	}{
		{
			name:           "zero",
			classSize:      0,
			wantZero:       true,
			wantPositive:   false,
			wantPowerOfTwo: false,
		},
		{
			name:           "one byte",
			classSize:      ClassSize(Byte),
			wantZero:       false,
			wantPositive:   true,
			wantPowerOfTwo: true,
		},
		{
			name:           "one kibibyte",
			classSize:      ClassSize(KiB),
			wantZero:       false,
			wantPositive:   true,
			wantPowerOfTwo: true,
		},
		{
			name:           "non power of two",
			classSize:      ClassSize(1536),
			wantZero:       false,
			wantPositive:   true,
			wantPowerOfTwo: false,
		},
		{
			name:           "one mibibyte",
			classSize:      ClassSize(MiB),
			wantZero:       false,
			wantPositive:   true,
			wantPowerOfTwo: true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.classSize.IsZero(); got != tt.wantZero {
				t.Fatalf("IsZero() = %t, want %t", got, tt.wantZero)
			}

			if got := tt.classSize.IsPositive(); got != tt.wantPositive {
				t.Fatalf("IsPositive() = %t, want %t", got, tt.wantPositive)
			}

			if got := tt.classSize.IsPowerOfTwo(); got != tt.wantPowerOfTwo {
				t.Fatalf("IsPowerOfTwo() = %t, want %t", got, tt.wantPowerOfTwo)
			}
		})
	}
}

// TestClassSizeComparisonHelpers verifies semantic comparison methods.
func TestClassSizeComparisonHelpers(t *testing.T) {
	t.Parallel()

	smaller := ClassSizeFromBytes(1024)
	equal := ClassSizeFromBytes(1024)
	larger := ClassSizeFromBytes(2048)

	if !smaller.Less(larger) {
		t.Fatal("smaller.Less(larger) = false, want true")
	}

	if smaller.Less(equal) {
		t.Fatal("smaller.Less(equal) = true, want false")
	}

	if !smaller.LessOrEqual(equal) {
		t.Fatal("smaller.LessOrEqual(equal) = false, want true")
	}

	if !smaller.LessOrEqual(larger) {
		t.Fatal("smaller.LessOrEqual(larger) = false, want true")
	}

	if !larger.Greater(smaller) {
		t.Fatal("larger.Greater(smaller) = false, want true")
	}

	if equal.Greater(smaller) {
		t.Fatal("equal.Greater(smaller) = true, want false")
	}

	if !larger.GreaterOrEqual(equal) {
		t.Fatal("larger.GreaterOrEqual(equal) = false, want true")
	}

	if !equal.GreaterOrEqual(smaller) {
		t.Fatal("equal.GreaterOrEqual(smaller) = false, want true")
	}
}

// TestClassSizeCanServe verifies request-to-class capacity relation.
//
// A class can serve a request when class capacity is greater than or equal to
// the requested size.
func TestClassSizeCanServe(t *testing.T) {
	t.Parallel()

	classSize := ClassSizeFromSize(4 * KiB)

	tests := []struct {
		name string

		// requestedSize is the caller-requested byte size.
		requestedSize Size

		// want reports whether classSize can serve the request.
		want bool
	}{
		{
			name:          "zero request",
			requestedSize: 0,
			want:          true,
		},
		{
			name:          "smaller request",
			requestedSize: 3 * KiB,
			want:          true,
		},
		{
			name:          "equal request",
			requestedSize: 4 * KiB,
			want:          true,
		},
		{
			name:          "larger request",
			requestedSize: 4*KiB + Byte,
			want:          false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classSize.CanServe(tt.requestedSize)
			if got != tt.want {
				t.Fatalf("ClassSize(%d).CanServe(%d) = %t, want %t", classSize, tt.requestedSize, got, tt.want)
			}
		})
	}
}

// TestClassSizeWasteFor verifies byte-level internal-fragmentation calculation.
//
// WasteFor returns bytes, not a ratio. Ratio calculation belongs outside the
// class-size primitive.
func TestClassSizeWasteFor(t *testing.T) {
	t.Parallel()

	classSize := ClassSizeFromSize(4 * KiB)

	tests := []struct {
		name string

		// requestedSize is the request being served by classSize.
		requestedSize Size

		// want is the expected wasted capacity.
		want Size
	}{
		{
			name:          "zero request wastes full class capacity",
			requestedSize: 0,
			want:          4 * KiB,
		},
		{
			name:          "smaller request",
			requestedSize: 3 * KiB,
			want:          1 * KiB,
		},
		{
			name:          "equal request",
			requestedSize: 4 * KiB,
			want:          0,
		},
		{
			name:          "larger request reports zero",
			requestedSize: 4*KiB + Byte,
			want:          0,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classSize.WasteFor(tt.requestedSize)
			if got != tt.want {
				t.Fatalf("ClassSize(%d).WasteFor(%d) = %d, want %d", classSize, tt.requestedSize, got, tt.want)
			}
		})
	}
}

// TestClassSizeString verifies that ClassSize uses Size formatting.
func TestClassSizeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// classSize is the value being formatted.
		classSize ClassSize

		// want is the expected stable string.
		want string
	}{
		{
			name:      "bytes",
			classSize: ClassSizeFromBytes(512),
			want:      "512 B",
		},
		{
			name:      "kibibyte",
			classSize: ClassSizeFromSize(KiB),
			want:      "1 KiB",
		},
		{
			name:      "non aligned",
			classSize: ClassSizeFromBytes(1536),
			want:      "1536 B",
		},
		{
			name:      "mibibyte",
			classSize: ClassSizeFromSize(MiB),
			want:      "1 MiB",
		},
		{
			name:      "gibibyte",
			classSize: ClassSizeFromSize(GiB),
			want:      "1 GiB",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.classSize.String()
			if got != tt.want {
				t.Fatalf("ClassSize(%d).String() = %q, want %q", tt.classSize, got, tt.want)
			}
		})
	}
}

// TestClassSizeAllowsNonPowerOfTwo documents that ClassSize itself does not
// require power-of-two values.
//
// Power-of-two behavior belongs to the default class profile or class-table
// validation, not to the ClassSize primitive.
func TestClassSizeAllowsNonPowerOfTwo(t *testing.T) {
	t.Parallel()

	classSize := ClassSizeFromBytes(3000)

	if classSize.Bytes() != 3000 {
		t.Fatalf("ClassSizeFromBytes(3000).Bytes() = %d, want 3000", classSize.Bytes())
	}

	if classSize.IsPowerOfTwo() {
		t.Fatal("ClassSizeFromBytes(3000).IsPowerOfTwo() = true, want false")
	}
}

// TestZeroClassSizeCanBeInspectedButNotConstructed documents zero-value
// semantics.
//
// A zero ClassSize can exist as an unset field before validation, but enabled
// construction helpers reject it.
func TestZeroClassSizeCanBeInspectedButNotConstructed(t *testing.T) {
	t.Parallel()

	var classSize ClassSize

	if !classSize.IsZero() {
		t.Fatal("zero ClassSize IsZero() = false, want true")
	}

	if classSize.IsPositive() {
		t.Fatal("zero ClassSize IsPositive() = true, want false")
	}

	if classSize.Bytes() != 0 {
		t.Fatalf("zero ClassSize Bytes() = %d, want 0", classSize.Bytes())
	}

	if classSize.String() != "0 B" {
		t.Fatalf("zero ClassSize String() = %q, want %q", classSize.String(), "0 B")
	}
}
