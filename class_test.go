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

// TestClassIDUint16 verifies raw ClassID conversion.
//
// ClassID is an ordinal class identifier. It is not a byte size and not a class
// capacity. Uint16 is useful for compact snapshots, diagnostics, and tests.
func TestClassIDUint16(t *testing.T) {
	t.Parallel()

	id := ClassID(42)

	if got := id.Uint16(); got != 42 {
		t.Fatalf("ClassID(42).Uint16() = %d, want 42", got)
	}
}

// TestClassIDIndex verifies conversion to an int slice index.
//
// ClassID is backed by uint16, so converting to int is safe and useful for
// indexing class-owned slices.
func TestClassIDIndex(t *testing.T) {
	t.Parallel()

	id := ClassID(42)

	if got := id.Index(); got != 42 {
		t.Fatalf("ClassID(42).Index() = %d, want 42", got)
	}
}

// TestClassIDString verifies stable diagnostic formatting.
//
// ClassID.String should not expose a byte size. It represents an ordinal class
// identifier inside a class table.
func TestClassIDString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// id is the class identifier being formatted.
		id ClassID

		// want is the expected diagnostic representation.
		want string
	}{
		{
			name: "zero id",
			id:   ClassID(0),
			want: "class#0",
		},
		{
			name: "ordinary id",
			id:   ClassID(42),
			want: "class#42",
		},
		{
			name: "max id",
			id:   ClassID(^uint16(0)),
			want: "class#65535",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.id.String()
			if got != tt.want {
				t.Fatalf("ClassID(%d).String() = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// TestNewSizeClass verifies construction of a valid size-class descriptor.
//
// ClassID(0) is valid. The class size must be positive. The descriptor should
// preserve both the ordinal identifier and the normalized class capacity.
func TestNewSizeClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// id is the ordinal class identifier.
		id ClassID

		// size is the normalized reusable class capacity.
		size ClassSize
	}{
		{
			name: "zero id",
			id:   ClassID(0),
			size: ClassSizeFromSize(KiB),
		},
		{
			name: "ordinary id",
			id:   ClassID(7),
			size: ClassSizeFromSize(4 * KiB),
		},
		{
			name: "non power of two class size",
			id:   ClassID(8),
			size: ClassSizeFromBytes(3000),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			class := NewSizeClass(tt.id, tt.size)

			if got := class.ID(); got != tt.id {
				t.Fatalf("ID() = %s, want %s", got, tt.id)
			}

			if got := class.Size(); got != tt.size {
				t.Fatalf("Size() = %s, want %s", got, tt.size)
			}

			if !class.IsValid() {
				t.Fatal("IsValid() = false, want true")
			}

			if class.IsZero() {
				t.Fatal("IsZero() = true, want false")
			}
		})
	}
}

// TestNewSizeClassPanicsForZeroSize verifies construction invariant.
//
// An enabled SizeClass must have positive normalized capacity. Zero-capacity
// classes cannot serve requests and would corrupt budget, shard-credit,
// admission, and owner-side accounting calculations.
func TestNewSizeClassPanicsForZeroSize(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errSizeClassZeroSize, func() {
		_ = NewSizeClass(ClassID(0), ClassSize(0))
	})
}

// TestSizeClassAccessors verifies descriptor accessors.
//
// SizeClass should expose the same capacity through ClassSize, generic Size,
// raw bytes, and int conversion without changing the stored value.
func TestSizeClassAccessors(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(3), ClassSizeFromSize(4*KiB))

	if got := class.ID(); got != ClassID(3) {
		t.Fatalf("ID() = %s, want class#3", got)
	}

	if got := class.Size(); got != ClassSizeFromSize(4*KiB) {
		t.Fatalf("Size() = %s, want %s", got, ClassSizeFromSize(4*KiB))
	}

	if got := class.ByteSize(); got != 4*KiB {
		t.Fatalf("ByteSize() = %s, want %s", got, 4*KiB)
	}

	if got := class.Bytes(); got != uint64(4*KiB) {
		t.Fatalf("Bytes() = %d, want %d", got, uint64(4*KiB))
	}

	if got := class.Int(); got != int(4*KiB) {
		t.Fatalf("Int() = %d, want %d", got, int(4*KiB))
	}
}

// TestSizeClassIntPanicsWhenSizeExceedsIntRange verifies that SizeClass.Int
// preserves Size.Int safety.
//
// The test constructs a descriptor directly because NewSizeClass only rejects
// zero size and does not clamp or normalize capacity.
func TestSizeClassIntPanicsWhenSizeExceedsIntRange(t *testing.T) {
	t.Parallel()

	maxInt := uint64(^uint(0) >> 1)

	class := NewSizeClass(ClassID(1), ClassSize(maxInt+1))

	testutil.MustPanicWithMessage(t, errSizeTooLargeForInt, func() {
		_ = class.Int()
	})
}

// TestSizeClassZeroValue verifies zero-value descriptor semantics.
//
// The zero value is an unset descriptor. It is not a valid enabled class because
// its size is zero.
func TestSizeClassZeroValue(t *testing.T) {
	t.Parallel()

	var class SizeClass

	if !class.IsZero() {
		t.Fatal("zero SizeClass IsZero() = false, want true")
	}

	if class.IsValid() {
		t.Fatal("zero SizeClass IsValid() = true, want false")
	}

	if got := class.ID(); got != ClassID(0) {
		t.Fatalf("zero SizeClass ID() = %s, want class#0", got)
	}

	if got := class.Size(); got != ClassSize(0) {
		t.Fatalf("zero SizeClass Size() = %s, want 0 B", got)
	}

	if got := class.ByteSize(); got != Size(0) {
		t.Fatalf("zero SizeClass ByteSize() = %s, want 0 B", got)
	}

	if got := class.Bytes(); got != 0 {
		t.Fatalf("zero SizeClass Bytes() = %d, want 0", got)
	}

	if got := class.String(); got != "class#0:0 B" {
		t.Fatalf("zero SizeClass String() = %q, want %q", got, "class#0:0 B")
	}
}

// TestSizeClassIsZeroWithZeroSizeAndNonZeroID verifies unset/invalid descriptor
// distinction.
//
// A descriptor with non-zero ID and zero size is invalid, but it is not the zero
// value. This can occur only through direct struct construction inside the
// package; NewSizeClass rejects it.
func TestSizeClassIsZeroWithZeroSizeAndNonZeroID(t *testing.T) {
	t.Parallel()

	class := SizeClass{
		id:   ClassID(9),
		size: ClassSize(0),
	}

	if class.IsZero() {
		t.Fatal("SizeClass{id:9, size:0}.IsZero() = true, want false")
	}

	if class.IsValid() {
		t.Fatal("SizeClass{id:9, size:0}.IsValid() = true, want false")
	}
}

// TestSizeClassPowerOfTwo verifies class-size power-of-two classification.
//
// Power-of-two is not a universal invariant for SizeClass, but the helper is
// useful for validating default class profiles.
func TestSizeClassPowerOfTwo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// class is the size-class descriptor being inspected.
		class SizeClass

		// want reports whether the class size is a power of two.
		want bool
	}{
		{
			name:  "power of two",
			class: NewSizeClass(ClassID(1), ClassSizeFromSize(4*KiB)),
			want:  true,
		},
		{
			name:  "non power of two",
			class: NewSizeClass(ClassID(2), ClassSizeFromBytes(3000)),
			want:  false,
		},
		{
			name:  "zero class",
			class: SizeClass{},
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.class.IsPowerOfTwo()
			if got != tt.want {
				t.Fatalf("IsPowerOfTwo() = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestSizeClassCanServe verifies request-to-class capacity relation.
//
// A class can serve a request when its normalized capacity is greater than or
// equal to the requested size.
func TestSizeClassCanServe(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(4), ClassSizeFromSize(4*KiB))

	tests := []struct {
		name string

		// requestedSize is the caller-requested byte size.
		requestedSize Size

		// want reports whether class can serve requestedSize.
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

			got := class.CanServe(tt.requestedSize)
			if got != tt.want {
				t.Fatalf("CanServe(%s) = %t, want %t", tt.requestedSize, got, tt.want)
			}
		})
	}
}

// TestSizeClassWasteFor verifies byte-level internal fragmentation calculation.
//
// WasteFor returns bytes, not a ratio. Ratio calculation belongs outside the
// size-class descriptor.
func TestSizeClassWasteFor(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(4), ClassSizeFromSize(4*KiB))

	tests := []struct {
		name string

		// requestedSize is the request being served by class.
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

			got := class.WasteFor(tt.requestedSize)
			if got != tt.want {
				t.Fatalf("WasteFor(%s) = %s, want %s", tt.requestedSize, got, tt.want)
			}
		})
	}
}

// TestSizeClassIdentityHelpers verifies ID, size, and full descriptor equality.
//
// SameID and SameSize intentionally check different dimensions. Equal requires
// both dimensions to match.
func TestSizeClassIdentityHelpers(t *testing.T) {
	t.Parallel()

	base := NewSizeClass(ClassID(1), ClassSizeFromSize(4*KiB))
	same := NewSizeClass(ClassID(1), ClassSizeFromSize(4*KiB))
	sameIDDifferentSize := NewSizeClass(ClassID(1), ClassSizeFromSize(8*KiB))
	differentIDDifferentSize := NewSizeClass(ClassID(2), ClassSizeFromSize(8*KiB))
	differentIDSameSize := NewSizeClass(ClassID(2), ClassSizeFromSize(4*KiB))

	if !base.SameID(same) {
		t.Fatal("base.SameID(same) = false, want true")
	}

	if !base.SameSize(same) {
		t.Fatal("base.SameSize(same) = false, want true")
	}

	if !base.Equal(same) {
		t.Fatal("base.Equal(same) = false, want true")
	}

	if !base.SameID(sameIDDifferentSize) {
		t.Fatal("base.SameID(sameIDDifferentSize) = false, want true")
	}

	if base.SameSize(sameIDDifferentSize) {
		t.Fatal("base.SameSize(sameIDDifferentSize) = true, want false")
	}

	if base.Equal(sameIDDifferentSize) {
		t.Fatal("base.Equal(sameIDDifferentSize) = true, want false")
	}

	if base.SameID(differentIDDifferentSize) {
		t.Fatal("base.SameID(differentIDDifferentSize) = true, want false")
	}

	if base.SameSize(differentIDDifferentSize) {
		t.Fatal("base.SameSize(differentIDDifferentSize) = true, want false")
	}

	if base.Equal(differentIDDifferentSize) {
		t.Fatal("base.Equal(differentIDDifferentSize) = true, want false")
	}

	if base.SameID(differentIDSameSize) {
		t.Fatal("base.SameID(differentIDSameSize) = true, want false")
	}

	if !base.SameSize(differentIDSameSize) {
		t.Fatal("base.SameSize(differentIDSameSize) = false, want true")
	}

	if base.Equal(differentIDSameSize) {
		t.Fatal("base.Equal(differentIDSameSize) = true, want false")
	}
}

// TestSizeClassString verifies stable diagnostic formatting.
//
// SizeClass.String should include both class ID and normalized capacity.
func TestSizeClassString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// class is the size-class descriptor being formatted.
		class SizeClass

		// want is the expected diagnostic representation.
		want string
	}{
		{
			name:  "zero descriptor",
			class: SizeClass{},
			want:  "class#0:0 B",
		},
		{
			name:  "ordinary kibibyte class",
			class: NewSizeClass(ClassID(1), ClassSizeFromSize(KiB)),
			want:  "class#1:1 KiB",
		},
		{
			name:  "ordinary mibibyte class",
			class: NewSizeClass(ClassID(14), ClassSizeFromSize(MiB)),
			want:  "class#14:1 MiB",
		},
		{
			name:  "non aligned class",
			class: NewSizeClass(ClassID(9), ClassSizeFromBytes(3000)),
			want:  "class#9:3000 B",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.class.String()
			if got != tt.want {
				t.Fatalf("SizeClass.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
