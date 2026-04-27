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

// TestNewClassTable verifies construction from ordered class sizes.
//
// The table should assign dense ordinal ClassID values in input order and should
// preserve the exact ClassSize values.
func TestNewClassTable(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
	})

	if table.len() != 3 {
		t.Fatalf("len() = %d, want 3", table.len())
	}

	if table.isEmpty() {
		t.Fatal("isEmpty() = true, want false")
	}

	classes := table.classesCopy()

	for index, class := range classes {
		wantID := ClassID(index)

		if class.ID() != wantID {
			t.Fatalf("class %d ID() = %s, want %s", index, class.ID(), wantID)
		}

		if class.Size() != ClassSizeFromSize(Size(1<<uint(10+index))) {
			t.Fatalf("class %d Size() = %s, want %s", index, class.Size(), ClassSizeFromSize(Size(1<<uint(10+index))))
		}

		if !class.IsValid() {
			t.Fatalf("class %d IsValid() = false, want true", index)
		}
	}
}

// TestNewClassTableCopiesInput verifies that table construction does not retain
// the caller's slice.
//
// classTable is intended to be immutable after construction. Mutating the input
// slice after construction must not affect the table.
func TestNewClassTableCopiesInput(t *testing.T) {
	t.Parallel()

	input := []ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
	}

	table := newClassTable(input)

	input[0] = ClassSizeFromSize(8 * KiB)

	first, ok := table.first()
	if !ok {
		t.Fatal("first() ok = false, want true")
	}

	if first.Size() != ClassSizeFromSize(1*KiB) {
		t.Fatalf("first size after input mutation = %s, want %s", first.Size(), ClassSizeFromSize(1*KiB))
	}
}

// TestNewClassTablePanicsForInvalidInput verifies class-table construction
// invariants.
//
// Class sizes must be non-empty, positive, unique, and strictly increasing.
func TestNewClassTablePanicsForInvalidInput(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errClassTableEmpty, func() {
			_ = newClassTable(nil)
		})
	})

	t.Run("too many classes", func(t *testing.T) {
		t.Parallel()

		sizes := make([]ClassSize, maxClassTableClasses+1)

		testutil.MustPanicWithMessage(t, errClassTableTooManyClasses, func() {
			_ = newClassTable(sizes)
		})
	})

	t.Run("zero class size", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errClassTableZeroClassSize, func() {
			_ = newClassTable([]ClassSize{
				ClassSizeFromSize(1 * KiB),
				ClassSize(0),
			})
		})
	})

	t.Run("duplicate class size", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errClassTableUnorderedClassSizes, func() {
			_ = newClassTable([]ClassSize{
				ClassSizeFromSize(1 * KiB),
				ClassSizeFromSize(1 * KiB),
			})
		})
	})

	t.Run("descending class size", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errClassTableUnorderedClassSizes, func() {
			_ = newClassTable([]ClassSize{
				ClassSizeFromSize(2 * KiB),
				ClassSizeFromSize(1 * KiB),
			})
		})
	})
}

// TestNewClassTableFromSizes verifies construction from generic Size values.
//
// This helper belongs at option/profile boundaries where user-facing
// configuration is expressed as Size rather than ClassSize.
func TestNewClassTableFromSizes(t *testing.T) {
	t.Parallel()

	table := newClassTableFromSizes([]Size{
		1 * KiB,
		2 * KiB,
		4 * KiB,
	})

	classes := table.classesCopy()
	if len(classes) != 3 {
		t.Fatalf("len(classesCopy()) = %d, want 3", len(classes))
	}

	for index, class := range classes {
		if class.ID() != ClassID(index) {
			t.Fatalf("class %d ID() = %s, want class#%d", index, class.ID(), index)
		}
	}

	last, ok := table.last()
	if !ok {
		t.Fatal("last() ok = false, want true")
	}

	if last.Size() != ClassSizeFromSize(4*KiB) {
		t.Fatalf("last size = %s, want %s", last.Size(), ClassSizeFromSize(4*KiB))
	}
}

// TestNewClassTableFromSizesPanicsForInvalidInput verifies validation through
// generic Size construction.
func TestNewClassTableFromSizesPanicsForInvalidInput(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errClassTableEmpty, func() {
			_ = newClassTableFromSizes(nil)
		})
	})

	t.Run("zero size", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errClassSizeZero, func() {
			_ = newClassTableFromSizes([]Size{
				1 * KiB,
				0,
			})
		})
	})

	t.Run("duplicate size", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errClassTableUnorderedClassSizes, func() {
			_ = newClassTableFromSizes([]Size{
				1 * KiB,
				1 * KiB,
			})
		})
	})
}

// TestZeroValueClassTable verifies disabled zero-value behavior.
//
// The zero-value table contains no classes. All lookup methods should return
// false rather than panic.
func TestZeroValueClassTable(t *testing.T) {
	t.Parallel()

	var table classTable

	if table.len() != 0 {
		t.Fatalf("zero table len() = %d, want 0", table.len())
	}

	if !table.isEmpty() {
		t.Fatal("zero table isEmpty() = false, want true")
	}

	if _, ok := table.first(); ok {
		t.Fatal("zero table first() ok = true, want false")
	}

	if _, ok := table.last(); ok {
		t.Fatal("zero table last() ok = true, want false")
	}

	if _, ok := table.classByID(0); ok {
		t.Fatal("zero table classByID(0) ok = true, want false")
	}

	if _, ok := table.classForExactSize(ClassSizeFromSize(KiB)); ok {
		t.Fatal("zero table classForExactSize(1 KiB) ok = true, want false")
	}

	if _, ok := table.classForRequest(KiB); ok {
		t.Fatal("zero table classForRequest(1 KiB) ok = true, want false")
	}

	if _, ok := table.classForCapacity(KiB); ok {
		t.Fatal("zero table classForCapacity(1 KiB) ok = true, want false")
	}

	if table.supportsRequest(KiB) {
		t.Fatal("zero table supportsRequest(1 KiB) = true, want false")
	}

	if table.supportsCapacity(KiB) {
		t.Fatal("zero table supportsCapacity(1 KiB) = true, want false")
	}
}

// TestClassTableClassesCopy verifies immutable descriptor copy behavior.
func TestClassTableClassesCopy(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
	})

	classes := table.classesCopy()
	if len(classes) != 2 {
		t.Fatalf("len(classesCopy()) = %d, want 2", len(classes))
	}

	classes[0] = NewSizeClass(ClassID(99), ClassSizeFromSize(64*KiB))

	first, ok := table.first()
	if !ok {
		t.Fatal("first() ok = false, want true")
	}

	if first.ID() != ClassID(0) {
		t.Fatalf("first ID after classesCopy mutation = %s, want class#0", first.ID())
	}

	if first.Size() != ClassSizeFromSize(1*KiB) {
		t.Fatalf("first Size after classesCopy mutation = %s, want 1 KiB", first.Size())
	}
}

// TestClassTableClassSizes verifies class-size copy behavior.
func TestClassTableClassSizes(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
	})

	sizes := table.classSizes()

	want := []ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
	}

	if len(sizes) != len(want) {
		t.Fatalf("len(classSizes()) = %d, want %d", len(sizes), len(want))
	}

	for index := range want {
		if sizes[index] != want[index] {
			t.Fatalf("classSizes()[%d] = %s, want %s", index, sizes[index], want[index])
		}
	}

	sizes[0] = ClassSizeFromSize(64 * KiB)

	again := table.classSizes()
	if again[0] != ClassSizeFromSize(1*KiB) {
		t.Fatalf("table class size mutated through returned slice: got %s, want 1 KiB", again[0])
	}
}

// TestClassTableFirstAndLast verifies boundary lookup helpers.
func TestClassTableFirstAndLast(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
	})

	first, ok := table.first()
	if !ok {
		t.Fatal("first() ok = false, want true")
	}

	if first.ID() != ClassID(0) {
		t.Fatalf("first ID() = %s, want class#0", first.ID())
	}

	if first.Size() != ClassSizeFromSize(1*KiB) {
		t.Fatalf("first Size() = %s, want 1 KiB", first.Size())
	}

	last, ok := table.last()
	if !ok {
		t.Fatal("last() ok = false, want true")
	}

	if last.ID() != ClassID(2) {
		t.Fatalf("last ID() = %s, want class#2", last.ID())
	}

	if last.Size() != ClassSizeFromSize(4*KiB) {
		t.Fatalf("last Size() = %s, want 4 KiB", last.Size())
	}
}

// TestClassTableClassByID verifies ordinal class lookup.
func TestClassTableClassByID(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
	})

	tests := []struct {
		name string

		id ClassID

		wantSize ClassSize
		wantOK   bool
	}{
		{
			name:     "first class",
			id:       ClassID(0),
			wantSize: ClassSizeFromSize(1 * KiB),
			wantOK:   true,
		},
		{
			name:     "middle class",
			id:       ClassID(1),
			wantSize: ClassSizeFromSize(2 * KiB),
			wantOK:   true,
		},
		{
			name:     "last class",
			id:       ClassID(2),
			wantSize: ClassSizeFromSize(4 * KiB),
			wantOK:   true,
		},
		{
			name:   "out of range",
			id:     ClassID(3),
			wantOK: false,
		},
		{
			name:   "large out of range",
			id:     ClassID(^uint16(0)),
			wantOK: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			class, ok := table.classByID(tt.id)
			if ok != tt.wantOK {
				t.Fatalf("classByID(%s) ok = %t, want %t", tt.id, ok, tt.wantOK)
			}

			if !tt.wantOK {
				return
			}

			if class.ID() != tt.id {
				t.Fatalf("classByID(%s).ID() = %s, want %s", tt.id, class.ID(), tt.id)
			}

			if class.Size() != tt.wantSize {
				t.Fatalf("classByID(%s).Size() = %s, want %s", tt.id, class.Size(), tt.wantSize)
			}
		})
	}
}

// TestClassTableClassForExactSize verifies exact class-size lookup.
func TestClassTableClassForExactSize(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
	})

	tests := []struct {
		name string

		size ClassSize

		wantID ClassID
		wantOK bool
	}{
		{
			name:   "zero size",
			size:   ClassSize(0),
			wantOK: false,
		},
		{
			name:   "below smallest",
			size:   ClassSizeFromBytes(512),
			wantOK: false,
		},
		{
			name:   "exact first",
			size:   ClassSizeFromSize(1 * KiB),
			wantID: ClassID(0),
			wantOK: true,
		},
		{
			name:   "between classes",
			size:   ClassSizeFromBytes(1536),
			wantOK: false,
		},
		{
			name:   "exact middle",
			size:   ClassSizeFromSize(2 * KiB),
			wantID: ClassID(1),
			wantOK: true,
		},
		{
			name:   "exact last",
			size:   ClassSizeFromSize(4 * KiB),
			wantID: ClassID(2),
			wantOK: true,
		},
		{
			name:   "above largest",
			size:   ClassSizeFromSize(8 * KiB),
			wantOK: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			class, ok := table.classForExactSize(tt.size)
			if ok != tt.wantOK {
				t.Fatalf("classForExactSize(%s) ok = %t, want %t", tt.size, ok, tt.wantOK)
			}

			if !tt.wantOK {
				return
			}

			if class.ID() != tt.wantID {
				t.Fatalf("classForExactSize(%s).ID() = %s, want %s", tt.size, class.ID(), tt.wantID)
			}

			if class.Size() != tt.size {
				t.Fatalf("classForExactSize(%s).Size() = %s, want %s", tt.size, class.Size(), tt.size)
			}
		})
	}
}

// TestClassTableClassForRequest verifies ceil request normalization.
//
// A request must map to the smallest class whose capacity can serve it.
func TestClassTableClassForRequest(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
	})

	tests := []struct {
		name string

		requested Size

		wantID   ClassID
		wantSize ClassSize
		wantOK   bool
	}{
		{
			name:      "zero request maps to smallest class",
			requested: 0,
			wantID:    ClassID(0),
			wantSize:  ClassSizeFromSize(1 * KiB),
			wantOK:    true,
		},
		{
			name:      "below smallest maps to smallest",
			requested: Size(512),
			wantID:    ClassID(0),
			wantSize:  ClassSizeFromSize(1 * KiB),
			wantOK:    true,
		},
		{
			name:      "exact first",
			requested: 1 * KiB,
			wantID:    ClassID(0),
			wantSize:  ClassSizeFromSize(1 * KiB),
			wantOK:    true,
		},
		{
			name:      "between first and second",
			requested: Size(1536),
			wantID:    ClassID(1),
			wantSize:  ClassSizeFromSize(2 * KiB),
			wantOK:    true,
		},
		{
			name:      "exact second",
			requested: 2 * KiB,
			wantID:    ClassID(1),
			wantSize:  ClassSizeFromSize(2 * KiB),
			wantOK:    true,
		},
		{
			name:      "between second and third",
			requested: 3 * KiB,
			wantID:    ClassID(2),
			wantSize:  ClassSizeFromSize(4 * KiB),
			wantOK:    true,
		},
		{
			name:      "exact last",
			requested: 4 * KiB,
			wantID:    ClassID(2),
			wantSize:  ClassSizeFromSize(4 * KiB),
			wantOK:    true,
		},
		{
			name:      "above largest",
			requested: 4*KiB + Byte,
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			class, ok := table.classForRequest(tt.requested)
			if ok != tt.wantOK {
				t.Fatalf("classForRequest(%s) ok = %t, want %t", tt.requested, ok, tt.wantOK)
			}

			if !tt.wantOK {
				return
			}

			if class.ID() != tt.wantID {
				t.Fatalf("classForRequest(%s).ID() = %s, want %s", tt.requested, class.ID(), tt.wantID)
			}

			if class.Size() != tt.wantSize {
				t.Fatalf("classForRequest(%s).Size() = %s, want %s", tt.requested, class.Size(), tt.wantSize)
			}

			if !class.CanServe(tt.requested) {
				t.Fatalf("classForRequest(%s) returned %s that cannot serve request", tt.requested, class)
			}
		})
	}
}

// TestClassTableClassForCapacity verifies floor capacity classification.
//
// A returned capacity maps to the largest class that can be safely served by that
// capacity.
func TestClassTableClassForCapacity(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
	})

	tests := []struct {
		name string

		capacity Size

		wantID   ClassID
		wantSize ClassSize
		wantOK   bool
	}{
		{
			name:     "zero capacity",
			capacity: 0,
			wantOK:   false,
		},
		{
			name:     "below smallest",
			capacity: Size(512),
			wantOK:   false,
		},
		{
			name:     "exact first",
			capacity: 1 * KiB,
			wantID:   ClassID(0),
			wantSize: ClassSizeFromSize(1 * KiB),
			wantOK:   true,
		},
		{
			name:     "between first and second",
			capacity: Size(1536),
			wantID:   ClassID(0),
			wantSize: ClassSizeFromSize(1 * KiB),
			wantOK:   true,
		},
		{
			name:     "exact second",
			capacity: 2 * KiB,
			wantID:   ClassID(1),
			wantSize: ClassSizeFromSize(2 * KiB),
			wantOK:   true,
		},
		{
			name:     "between second and third",
			capacity: 3 * KiB,
			wantID:   ClassID(1),
			wantSize: ClassSizeFromSize(2 * KiB),
			wantOK:   true,
		},
		{
			name:     "exact last",
			capacity: 4 * KiB,
			wantID:   ClassID(2),
			wantSize: ClassSizeFromSize(4 * KiB),
			wantOK:   true,
		},
		{
			name:     "above largest maps to largest",
			capacity: 8 * KiB,
			wantID:   ClassID(2),
			wantSize: ClassSizeFromSize(4 * KiB),
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			class, ok := table.classForCapacity(tt.capacity)
			if ok != tt.wantOK {
				t.Fatalf("classForCapacity(%s) ok = %t, want %t", tt.capacity, ok, tt.wantOK)
			}

			if !tt.wantOK {
				return
			}

			if class.ID() != tt.wantID {
				t.Fatalf("classForCapacity(%s).ID() = %s, want %s", tt.capacity, class.ID(), tt.wantID)
			}

			if class.Size() != tt.wantSize {
				t.Fatalf("classForCapacity(%s).Size() = %s, want %s", tt.capacity, class.Size(), tt.wantSize)
			}

			if class.ByteSize() > tt.capacity {
				t.Fatalf("classForCapacity(%s) returned %s larger than capacity", tt.capacity, class)
			}
		})
	}
}

// TestClassTableSupports verifies support helpers.
func TestClassTableSupports(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
	})

	tests := []struct {
		name string

		size Size

		wantRequest  bool
		wantCapacity bool
	}{
		{
			name:         "zero",
			size:         0,
			wantRequest:  true,
			wantCapacity: false,
		},
		{
			name:         "below smallest",
			size:         Size(512),
			wantRequest:  true,
			wantCapacity: false,
		},
		{
			name:         "exact smallest",
			size:         1 * KiB,
			wantRequest:  true,
			wantCapacity: true,
		},
		{
			name:         "between",
			size:         3 * KiB,
			wantRequest:  true,
			wantCapacity: true,
		},
		{
			name:         "exact largest",
			size:         4 * KiB,
			wantRequest:  true,
			wantCapacity: true,
		},
		{
			name:         "above largest",
			size:         8 * KiB,
			wantRequest:  false,
			wantCapacity: true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := table.supportsRequest(tt.size); got != tt.wantRequest {
				t.Fatalf("supportsRequest(%s) = %t, want %t", tt.size, got, tt.wantRequest)
			}

			if got := table.supportsCapacity(tt.size); got != tt.wantCapacity {
				t.Fatalf("supportsCapacity(%s) = %t, want %t", tt.size, got, tt.wantCapacity)
			}
		})
	}
}

// TestClassTableAllowsNonPowerOfTwoProfile verifies that classTable does not
// require power-of-two class sizes.
//
// Power-of-two sizing is a profile property, not a universal class-table
// invariant.
func TestClassTableAllowsNonPowerOfTwoProfile(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromBytes(1000),
		ClassSizeFromBytes(3000),
		ClassSizeFromBytes(9000),
	})

	requestClass, ok := table.classForRequest(Size(2500))
	if !ok {
		t.Fatal("classForRequest(2500 B) ok = false, want true")
	}

	if requestClass.Size() != ClassSizeFromBytes(3000) {
		t.Fatalf("classForRequest(2500 B).Size() = %s, want 3000 B", requestClass.Size())
	}

	capacityClass, ok := table.classForCapacity(Size(2500))
	if !ok {
		t.Fatal("classForCapacity(2500 B) ok = false, want true")
	}

	if capacityClass.Size() != ClassSizeFromBytes(1000) {
		t.Fatalf("classForCapacity(2500 B).Size() = %s, want 1000 B", capacityClass.Size())
	}
}

// TestValidateClassSizesDirect verifies direct validation helper behavior.
//
// Most validation is covered through constructors. This test keeps the internal
// helper guarded in case construction paths change later.
func TestValidateClassSizesDirect(t *testing.T) {
	t.Parallel()

	validateClassSizes([]ClassSize{
		ClassSizeFromSize(1 * KiB),
		ClassSizeFromSize(2 * KiB),
	})

	testutil.MustPanicWithMessage(t, errClassTableEmpty, func() {
		validateClassSizes(nil)
	})
}
