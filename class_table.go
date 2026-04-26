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

import "sort"

const (
	// errClassTableEmpty is used when class-table construction receives no class
	// sizes.
	//
	// A class table without classes cannot normalize requests, classify returned
	// capacities, or create class-owned runtime state.
	errClassTableEmpty = "bufferpool.classTable: at least one class size is required"

	// errClassTableTooManyClasses is used when class-table construction receives
	// more entries than ClassID can represent.
	//
	// ClassID is backed by uint16. This keeps class identifiers compact for
	// arrays, snapshots, ownership records, and counters. A table must therefore
	// fit into the ClassID ordinal space.
	errClassTableTooManyClasses = "bufferpool.classTable: class count exceeds ClassID range"

	// errClassTableZeroClassSize is used when a class-table entry has zero size.
	//
	// ClassSize constructors already reject zero, but validation is repeated here
	// because a ClassSize value can still be manually constructed as zero.
	errClassTableZeroClassSize = "bufferpool.classTable: class size must be greater than zero"

	// errClassTableUnorderedClassSizes is used when class sizes are not strictly
	// increasing.
	//
	// The table relies on sorted unique sizes for binary-search lookup. Duplicate
	// or descending sizes would make request normalization and capacity
	// classification ambiguous.
	errClassTableUnorderedClassSizes = "bufferpool.classTable: class sizes must be strictly increasing"
)

const (
	// maxClassTableClasses is the maximum number of classes representable by
	// ClassID.
	maxClassTableClasses = int(^uint16(0)) + 1

	// defaultMinClassShift is the exponent for the smallest class in the default
	// power-of-two class profile.
	defaultMinClassShift = 6

	// defaultMaxClassShift is the exponent for the largest class in the default
	// power-of-two class profile.
	defaultMaxClassShift = 25

	// defaultClassCount is the number of classes in the default profile.
	defaultClassCount = defaultMaxClassShift - defaultMinClassShift + 1
)

// classTable is an immutable-ordered table of size classes.
//
// The table maps arbitrary requested sizes to normalized class sizes. It also
// supports classifying returned capacities to the largest class that the capacity
// can safely serve.
//
// Responsibility boundary:
//
//   - class_table.go owns ordered class descriptors and lookup;
//   - class.go owns ClassID and SizeClass descriptor semantics;
//   - class_size.go owns ClassSize semantics;
//   - class_budget.go owns class-level target bytes;
//   - shard_credit.go owns per-shard local credit;
//   - admission.go decides whether a returned buffer is retained;
//   - ownership.go verifies origin class and returned-capacity growth;
//   - bucket.go stores already admitted buffers.
//
// The class table does not perform memory admission. A successful lookup means
// only that a size or capacity can be represented by a configured class. It does
// not mean the buffer should be retained.
//
// Lookup semantics:
//
//   - classForRequest performs ceil lookup:
//     requested size 3000 B with classes [2 KiB, 4 KiB] maps to 4 KiB.
//
//   - classForCapacity performs floor lookup:
//     returned capacity 3000 B with classes [2 KiB, 4 KiB] maps to 2 KiB.
//
// Ceil lookup is correct for requests because the chosen class must be large
// enough to serve the request. Floor lookup is correct for returned capacities
// because a retained buffer with capacity 3000 B can safely serve a 2 KiB class
// but cannot safely serve a 4 KiB class.
//
// The zero value is an empty disabled table. Lookup methods return false.
type classTable struct {
	classes []SizeClass
}

// newClassTable returns an immutable class table from ordered class sizes.
//
// sizes MUST be non-empty, positive, unique, and strictly increasing. The input
// slice is copied, so later caller-side mutations do not affect the table.
func newClassTable(sizes []ClassSize) classTable {
	validateClassSizes(sizes)

	classes := make([]SizeClass, len(sizes))
	for index, size := range sizes {
		classes[index] = NewSizeClass(ClassID(index), size)
	}

	return classTable{
		classes: classes,
	}
}

// newClassTableFromSizes returns an immutable class table from generic Size
// values.
//
// This helper is useful at options/profile boundaries where user-facing
// configuration is expressed as Size. The values are converted to ClassSize after
// validation by ClassSizeFromSize.
func newClassTableFromSizes(sizes []Size) classTable {
	if len(sizes) == 0 {
		panic(errClassTableEmpty)
	}

	classSizes := make([]ClassSize, len(sizes))
	for index, size := range sizes {
		classSizes[index] = ClassSizeFromSize(size)
	}

	return newClassTable(classSizes)
}

// defaultClassTable returns the package default power-of-two class table.
//
// The default profile is intentionally defined here instead of size.go because
// these are not generic byte-size constants. They are a concrete size-class
// profile used by class-table construction.
func defaultClassTable() classTable {
	return newClassTable(defaultClassSizeProfile())
}

// defaultClassSizeProfile returns a copy of the default power-of-two class-size
// profile.
//
// The profile ranges from 64 B through 32 MiB:
//
//	64 B, 128 B, 256 B, 512 B,
//	1 KiB, 2 KiB, ...,
//	32 MiB.
//
// The returned slice is a copy and may be modified by callers before building a
// custom class table.
func defaultClassSizeProfile() []ClassSize {
	profile := make([]ClassSize, len(defaultClassSizes))
	copy(profile, defaultClassSizes[:])
	return profile
}

// len returns the number of classes in the table.
func (t classTable) len() int {
	return len(t.classes)
}

// isEmpty reports whether the table contains no classes.
func (t classTable) isEmpty() bool {
	return len(t.classes) == 0
}

// classesCopy returns a copy of the table's size-class descriptors.
//
// The copy preserves immutability of the table's internal slice.
func (t classTable) classesCopy() []SizeClass {
	classes := make([]SizeClass, len(t.classes))
	copy(classes, t.classes)
	return classes
}

// classSizes returns a copy of the table's class sizes.
func (t classTable) classSizes() []ClassSize {
	sizes := make([]ClassSize, len(t.classes))
	for index, class := range t.classes {
		sizes[index] = class.Size()
	}

	return sizes
}

// first returns the smallest class in the table.
func (t classTable) first() (SizeClass, bool) {
	if len(t.classes) == 0 {
		return SizeClass{}, false
	}

	return t.classes[0], true
}

// last returns the largest class in the table.
func (t classTable) last() (SizeClass, bool) {
	if len(t.classes) == 0 {
		return SizeClass{}, false
	}

	return t.classes[len(t.classes)-1], true
}

// classByID returns the class with the given identifier.
//
// ClassID is an ordinal assigned by table construction. A valid id is in the
// inclusive range [0, len(table)-1].
func (t classTable) classByID(id ClassID) (SizeClass, bool) {
	index := id.Index()
	if index < 0 || index >= len(t.classes) {
		return SizeClass{}, false
	}

	return t.classes[index], true
}

// classForExactSize returns the class whose ClassSize exactly equals size.
//
// Exact lookup is useful for validation, ownership records, and tests. Ordinary
// request normalization should use classForRequest.
func (t classTable) classForExactSize(size ClassSize) (SizeClass, bool) {
	if len(t.classes) == 0 || size.IsZero() {
		return SizeClass{}, false
	}

	index := sort.Search(len(t.classes), func(index int) bool {
		return t.classes[index].Size() >= size
	})

	if index >= len(t.classes) {
		return SizeClass{}, false
	}

	class := t.classes[index]
	if class.Size() != size {
		return SizeClass{}, false
	}

	return class, true
}

// classForRequest returns the smallest class that can serve requestedSize.
//
// This is a ceil lookup. A zero requested size maps to the smallest class. The
// public API or admission layer may decide to treat zero-length requests
// specially, but the class-table relation itself is capacity-based.
//
// If requestedSize is larger than the largest configured class, the method
// returns false.
func (t classTable) classForRequest(requestedSize Size) (SizeClass, bool) {
	if len(t.classes) == 0 {
		return SizeClass{}, false
	}

	index := sort.Search(len(t.classes), func(index int) bool {
		return t.classes[index].CanServe(requestedSize)
	})

	if index >= len(t.classes) {
		return SizeClass{}, false
	}

	return t.classes[index], true
}

// classForCapacity returns the largest class that can be safely served by
// capacity.
//
// This is a floor lookup. It is intended for classifying returned buffer
// capacities when no stricter ownership-origin class is available.
//
// If capacity is smaller than the smallest configured class, the method returns
// false.
//
// This method does not enforce max retained buffer capacity. Admission must still
// check max retained capacity, pressure, ownership growth, and shard credit
// before retaining a returned buffer.
func (t classTable) classForCapacity(capacity Size) (SizeClass, bool) {
	if len(t.classes) == 0 || capacity.IsZero() {
		return SizeClass{}, false
	}

	index := sort.Search(len(t.classes), func(index int) bool {
		return t.classes[index].ByteSize() > capacity
	})

	if index == 0 {
		return SizeClass{}, false
	}

	return t.classes[index-1], true
}

// supportsRequest reports whether requestedSize can be normalized to a class.
func (t classTable) supportsRequest(requestedSize Size) bool {
	_, ok := t.classForRequest(requestedSize)
	return ok
}

// supportsCapacity reports whether capacity can be classified to at least one
// configured class.
func (t classTable) supportsCapacity(capacity Size) bool {
	_, ok := t.classForCapacity(capacity)
	return ok
}

// validateClassSizes validates class-table construction input.
func validateClassSizes(sizes []ClassSize) {
	if len(sizes) == 0 {
		panic(errClassTableEmpty)
	}

	if len(sizes) > maxClassTableClasses {
		panic(errClassTableTooManyClasses)
	}

	previous := ClassSize(0)
	for index, size := range sizes {
		if size.IsZero() {
			panic(errClassTableZeroClassSize)
		}

		if index > 0 && !previous.Less(size) {
			panic(errClassTableUnorderedClassSizes)
		}

		previous = size
	}
}

// defaultClassSizes is the default power-of-two class-size profile.
//
// It is intentionally stored in class_table.go rather than size.go. These values
// are not generic size constants; together they define one concrete class-table
// profile.
var defaultClassSizes = [defaultClassCount]ClassSize{
	ClassSize(64 * Byte),
	ClassSize(128 * Byte),
	ClassSize(256 * Byte),
	ClassSize(512 * Byte),

	ClassSize(1 * KiB),
	ClassSize(2 * KiB),
	ClassSize(4 * KiB),
	ClassSize(8 * KiB),
	ClassSize(16 * KiB),
	ClassSize(32 * KiB),
	ClassSize(64 * KiB),
	ClassSize(128 * KiB),
	ClassSize(256 * KiB),
	ClassSize(512 * KiB),

	ClassSize(1 * MiB),
	ClassSize(2 * MiB),
	ClassSize(4 * MiB),
	ClassSize(8 * MiB),
	ClassSize(16 * MiB),
	ClassSize(32 * MiB),
}
