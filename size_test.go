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
	"strconv"
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestSizeUnitConstants verifies binary byte-size units.
//
// Bufferpool uses binary memory units. These constants are generic byte-size
// units, not size-class definitions.
func TestSizeUnitConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size Size
		want uint64
	}{
		{
			name: "Byte",
			size: Byte,
			want: 1,
		},
		{
			name: "KiB",
			size: KiB,
			want: 1024,
		},
		{
			name: "MiB",
			size: MiB,
			want: 1024 * 1024,
		},
		{
			name: "GiB",
			size: GiB,
			want: 1024 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.size.Bytes(); got != tt.want {
				t.Fatalf("%s.Bytes() = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

// TestSizeUnitOrdering verifies that binary units are strictly increasing.
//
// This protects accidental changes to unit definitions that would corrupt
// option validation, diagnostics, and future class-table construction.
func TestSizeUnitOrdering(t *testing.T) {
	t.Parallel()

	units := []Size{
		Byte,
		KiB,
		MiB,
		GiB,
	}

	for index := 1; index < len(units); index++ {
		previous := units[index-1]
		current := units[index]

		if current <= previous {
			t.Fatalf("unit at index %d = %d, previous = %d; want strictly increasing", index, current, previous)
		}
	}
}

// TestSizeFromInt verifies conversion from non-negative int.
//
// This helper is used at Go API boundaries where sizes naturally appear as int,
// for example len(buffer), cap(buffer), or user-provided integer options.
func TestSizeFromInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value int
		want  Size
	}{
		{
			name:  "zero",
			value: 0,
			want:  0,
		},
		{
			name:  "one byte",
			value: 1,
			want:  Byte,
		},
		{
			name:  "one kibibyte",
			value: 1024,
			want:  KiB,
		},
		{
			name:  "non aligned value",
			value: 1536,
			want:  Size(1536),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := SizeFromInt(tt.value)
			if got != tt.want {
				t.Fatalf("SizeFromInt(%d) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

// TestSizeFromIntPanicsForNegativeValue verifies that negative sizes are
// rejected.
//
// Negative byte sizes cannot be represented in bufferpool accounting.
func TestSizeFromIntPanicsForNegativeValue(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errSizeNegativeInt, func() {
		_ = SizeFromInt(-1)
	})
}

// TestSizeFromBytes verifies conversion from raw uint64 bytes.
//
// This helper documents that a raw uint64 value is being interpreted as a byte
// size rather than a counter, generation, shard count, or buffer count.
func TestSizeFromBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		bytes uint64
		want  Size
	}{
		{
			name:  "zero",
			bytes: 0,
			want:  0,
		},
		{
			name:  "one byte",
			bytes: 1,
			want:  Byte,
		},
		{
			name:  "one mibibyte",
			bytes: uint64(MiB),
			want:  MiB,
		},
		{
			name:  "large raw value",
			bytes: 123456789,
			want:  Size(123456789),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := SizeFromBytes(tt.bytes)
			if got != tt.want {
				t.Fatalf("SizeFromBytes(%d) = %d, want %d", tt.bytes, got, tt.want)
			}
		})
	}
}

// TestSizeBytes verifies raw byte extraction.
func TestSizeBytes(t *testing.T) {
	t.Parallel()

	size := Size(4096)

	if got := size.Bytes(); got != 4096 {
		t.Fatalf("Size(4096).Bytes() = %d, want 4096", got)
	}
}

// TestSizeInt verifies safe conversion to int.
//
// The conversion is required before passing Size values to APIs such as
// make([]byte, 0, size.Int()).
func TestSizeInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size Size
		want int
	}{
		{
			name: "zero",
			size: 0,
			want: 0,
		},
		{
			name: "one byte",
			size: Byte,
			want: 1,
		},
		{
			name: "one kibibyte",
			size: KiB,
			want: 1024,
		},
		{
			name: "non aligned value",
			size: Size(1536),
			want: 1536,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.size.Int()
			if got != tt.want {
				t.Fatalf("Size(%d).Int() = %d, want %d", tt.size, got, tt.want)
			}
		})
	}
}

// TestSizeIntPanicsWhenValueExceedsIntRange verifies platform-safe int
// conversion.
//
// The test computes the platform int maximum exactly the same way as production
// code, then constructs one value above it.
func TestSizeIntPanicsWhenValueExceedsIntRange(t *testing.T) {
	t.Parallel()

	maxInt := uint64(^uint(0) >> 1)
	tooLarge := Size(maxInt + 1)

	testutil.MustPanicWithMessage(t, errSizeTooLargeForInt, func() {
		_ = tooLarge.Int()
	})
}

// TestSizePredicates verifies zero, positive, and power-of-two classification.
func TestSizePredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size Size

		wantZero       bool
		wantPositive   bool
		wantPowerOfTwo bool
	}{
		{
			name:           "zero",
			size:           0,
			wantZero:       true,
			wantPositive:   false,
			wantPowerOfTwo: false,
		},
		{
			name:           "one",
			size:           1,
			wantZero:       false,
			wantPositive:   true,
			wantPowerOfTwo: true,
		},
		{
			name:           "two",
			size:           2,
			wantZero:       false,
			wantPositive:   true,
			wantPowerOfTwo: true,
		},
		{
			name:           "three",
			size:           3,
			wantZero:       false,
			wantPositive:   true,
			wantPowerOfTwo: false,
		},
		{
			name:           "kibibyte",
			size:           KiB,
			wantZero:       false,
			wantPositive:   true,
			wantPowerOfTwo: true,
		},
		{
			name:           "non aligned",
			size:           Size(1536),
			wantZero:       false,
			wantPositive:   true,
			wantPowerOfTwo: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.size.IsZero(); got != tt.wantZero {
				t.Fatalf("IsZero() = %t, want %t", got, tt.wantZero)
			}

			if got := tt.size.IsPositive(); got != tt.wantPositive {
				t.Fatalf("IsPositive() = %t, want %t", got, tt.wantPositive)
			}

			if got := tt.size.IsPowerOfTwo(); got != tt.wantPowerOfTwo {
				t.Fatalf("IsPowerOfTwo() = %t, want %t", got, tt.wantPowerOfTwo)
			}
		})
	}
}

// TestSizeComparisonHelpers verifies semantic comparison methods.
//
// These helpers keep future option, policy, and class-table validation readable
// without repeating raw operators at call sites.
func TestSizeComparisonHelpers(t *testing.T) {
	t.Parallel()

	smaller := Size(128)
	equal := Size(128)
	larger := Size(256)

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

// TestSizeClamp verifies inclusive range clamping.
//
// Clamp is intentionally tested only with valid ranges. Invalid range validation
// belongs in the production implementation if the method is later changed to
// fail fast on minValue > maxValue.
func TestSizeClamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		value Size
		min   Size
		max   Size
		want  Size
	}{
		{
			name:  "below range",
			value: Size(32),
			min:   Size(64),
			max:   KiB,
			want:  Size(64),
		},
		{
			name:  "inside range",
			value: Size(512),
			min:   Size(64),
			max:   KiB,
			want:  Size(512),
		},
		{
			name:  "above range",
			value: Size(2048),
			min:   Size(64),
			max:   KiB,
			want:  KiB,
		},
		{
			name:  "equal min",
			value: Size(64),
			min:   Size(64),
			max:   KiB,
			want:  Size(64),
		},
		{
			name:  "equal max",
			value: KiB,
			min:   Size(64),
			max:   KiB,
			want:  KiB,
		},
		{
			name:  "single point range",
			value: Size(512),
			min:   Size(128),
			max:   Size(128),
			want:  Size(128),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.value.Clamp(tt.min, tt.max)
			if got != tt.want {
				t.Fatalf("Size(%d).Clamp(%d, %d) = %d, want %d", tt.value, tt.min, tt.max, got, tt.want)
			}
		})
	}
}

// TestSizeString verifies stable human-readable formatting.
//
// Exact binary-unit multiples are formatted with binary units. Non-aligned
// values are formatted in bytes so diagnostics preserve precision.
func TestSizeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size Size
		want string
	}{
		{
			name: "zero",
			size: 0,
			want: "0 B",
		},
		{
			name: "bytes below KiB",
			size: Size(512),
			want: "512 B",
		},
		{
			name: "one KiB",
			size: KiB,
			want: "1 KiB",
		},
		{
			name: "two KiB",
			size: 2 * KiB,
			want: "2 KiB",
		},
		{
			name: "non aligned above KiB",
			size: Size(1536),
			want: "1536 B",
		},
		{
			name: "one MiB",
			size: MiB,
			want: "1 MiB",
		},
		{
			name: "two MiB",
			size: 2 * MiB,
			want: "2 MiB",
		},
		{
			name: "non aligned above MiB",
			size: MiB + Byte,
			want: strconv.FormatUint(uint64(MiB+Byte), 10) + " B",
		},
		{
			name: "one GiB",
			size: GiB,
			want: "1 GiB",
		},
		{
			name: "two GiB",
			size: 2 * GiB,
			want: "2 GiB",
		},
		{
			name: "non aligned above GiB",
			size: GiB + Byte,
			want: strconv.FormatUint(uint64(GiB+Byte), 10) + " B",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.size.String()
			if got != tt.want {
				t.Fatalf("Size(%d).String() = %q, want %q", tt.size, got, tt.want)
			}
		})
	}
}

// TestSizeDoesNotExposeDefaultClassProfile documents the intended boundary.
//
// size.go should not define default size-class profiles. Default class profiles
// belong in class_table.go, profiles.go, or policy_defaults.go. This test is a
// compile-time boundary test by omission: it validates only generic size units
// and helpers, not Size64B, Size1KiB, or DefaultClassSizes.
func TestSizeDoesNotExposeDefaultClassProfile(t *testing.T) {
	t.Parallel()

	// The assertion is intentionally minimal. It keeps this test file focused on
	// generic Size behavior and documents that class-profile tests must live with
	// the future class-table/profile implementation.
	if Byte == 0 || KiB == 0 || MiB == 0 || GiB == 0 {
		t.Fatal("generic size units must be non-zero")
	}
}
