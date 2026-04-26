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
	// errSizeClassZeroSize is used when a SizeClass is constructed with a zero
	// ClassSize.
	//
	// A size class represents reusable buffer capacity. A zero-capacity class
	// cannot serve positive requests and would corrupt class-budget, shard-credit,
	// admission, and ownership calculations.
	errSizeClassZeroSize = "bufferpool.SizeClass: class size must be greater than zero"
)

// ClassID identifies a size class inside a class table.
//
// ClassID is an ordinal identifier, not a byte size. ClassID(0) is a valid class
// identifier and usually represents the smallest class in a class table.
//
// The identifier is intentionally separate from ClassSize because multiple
// runtime structures need a compact stable index-like value:
//
//   - class state arrays;
//   - class counters;
//   - class budgets;
//   - shard arrays per class;
//   - ownership origin-class records;
//   - class metrics and snapshots.
//
// ClassID values are assigned by class-table construction. Code outside
// class-table setup should not manufacture arbitrary ClassID values and assume
// they are present in a specific table.
type ClassID uint16

// Uint16 returns id as a raw uint16 value.
func (id ClassID) Uint16() uint16 {
	return uint16(id)
}

// Index returns id as an int suitable for indexing class-owned slices.
//
// The conversion is safe because ClassID is backed by uint16.
func (id ClassID) Index() int {
	return int(id)
}

// String returns the stable diagnostic representation of id.
func (id ClassID) String() string {
	return "class#" + strconv.FormatUint(uint64(id), 10)
}

// SizeClass describes one normalized reusable buffer size class.
//
// SizeClass is a small immutable descriptor. It binds a ClassID to the
// ClassSize selected by a class table.
//
// It does not own runtime state. It does not contain counters, buckets, shards,
// budget targets, workload score, retention policy, or admission state. Those
// belong to higher layers:
//
//   - class_table.go owns ordered lookup and validation;
//   - class_state.go owns class runtime state;
//   - class_counters.go owns class-level counters;
//   - class_budget.go owns class-level targets;
//   - shard.go owns per-class shards;
//   - bucket.go owns retained buffer storage.
//
// Construction:
//
// Use NewSizeClass when building validated class descriptors. The zero value of
// SizeClass is an unset value and is not a valid enabled class because its size
// is zero.
type SizeClass struct {
	// id is the ordinal class identifier inside a class table.
	id ClassID

	// size is the normalized reusable capacity for this class.
	size ClassSize
}

// NewSizeClass returns a validated size-class descriptor.
//
// id may be zero. ClassID(0) is a valid identifier. size MUST be greater than
// zero.
func NewSizeClass(id ClassID, size ClassSize) SizeClass {
	if size.IsZero() {
		panic(errSizeClassZeroSize)
	}

	return SizeClass{
		id:   id,
		size: size,
	}
}

// ID returns the class identifier.
func (c SizeClass) ID() ClassID {
	return c.id
}

// Size returns the normalized class capacity.
func (c SizeClass) Size() ClassSize {
	return c.size
}

// ByteSize returns the class capacity as a generic Size.
func (c SizeClass) ByteSize() Size {
	return c.size.Size()
}

// Bytes returns the class capacity as a raw byte count.
func (c SizeClass) Bytes() uint64 {
	return c.size.Bytes()
}

// Int returns the class capacity as int.
//
// The method panics if the class size cannot be represented as int on the
// current platform.
func (c SizeClass) Int() int {
	return c.size.Int()
}

// IsZero reports whether c is the zero unset descriptor.
//
// A valid enabled class constructed by NewSizeClass is never zero.
func (c SizeClass) IsZero() bool {
	return c.id == 0 && c.size.IsZero()
}

// IsValid reports whether c has a non-zero class size.
func (c SizeClass) IsValid() bool {
	return c.size.IsPositive()
}

// IsPowerOfTwo reports whether the class size is a power of two.
//
// This is useful for validating power-of-two class profiles. It is not a
// universal requirement for every class table.
func (c SizeClass) IsPowerOfTwo() bool {
	return c.size.IsPowerOfTwo()
}

// CanServe reports whether this class can serve requestedSize.
//
// A class can serve a request when its normalized capacity is greater than or
// equal to the requested size.
func (c SizeClass) CanServe(requestedSize Size) bool {
	return c.size.CanServe(requestedSize)
}

// WasteFor returns the internal fragmentation introduced when requestedSize is
// served by this class.
//
// The result is expressed in bytes. Ratio calculations belong to workload
// scoring or metrics code.
func (c SizeClass) WasteFor(requestedSize Size) Size {
	return c.size.WasteFor(requestedSize)
}

// SameID reports whether c and other have the same ClassID.
func (c SizeClass) SameID(other SizeClass) bool {
	return c.id == other.id
}

// SameSize reports whether c and other have the same ClassSize.
func (c SizeClass) SameSize(other SizeClass) bool {
	return c.size == other.size
}

// Equal reports whether c and other describe the same class identifier and class
// capacity.
func (c SizeClass) Equal(other SizeClass) bool {
	return c.id == other.id && c.size == other.size
}

// String returns a stable diagnostic representation of the size class.
func (c SizeClass) String() string {
	if c.IsZero() {
		return "class#0:0 B"
	}

	return c.id.String() + ":" + c.size.String()
}
