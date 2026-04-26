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

// classRetainDecision describes the class-local retain eligibility result for a
// returned buffer.
//
// This is the minimal static-core admission layer. It only protects the
// size-class invariant that retained buffers inside a class must be capable of
// serving that class. Additional owner-side checks and public result mapping
// belong outside this helper.
type classRetainDecision uint8

const (
	// classRetainAccept means the returned buffer is eligible to be attempted at
	// shard storage.
	classRetainAccept classRetainDecision = iota

	// classRetainRejectNilBuffer means a nil buffer cannot be retained.
	classRetainRejectNilBuffer

	// classRetainRejectZeroCapacity means a non-nil buffer has no reusable
	// backing capacity.
	classRetainRejectZeroCapacity

	// classRetainRejectCapacityBelowClassSize means the returned buffer cannot
	// serve the normalized class capacity.
	classRetainRejectCapacityBelowClassSize
)

// AllowsShardRetain reports whether the decision allows shard-level retention
// to be attempted.
func (d classRetainDecision) AllowsShardRetain() bool {
	return d == classRetainAccept
}

// String returns the stable diagnostic name of the decision.
func (d classRetainDecision) String() string {
	switch d {
	case classRetainAccept:
		return "accept"
	case classRetainRejectNilBuffer:
		return "reject_nil_buffer"
	case classRetainRejectZeroCapacity:
		return "reject_zero_capacity"
	case classRetainRejectCapacityBelowClassSize:
		return "reject_capacity_below_class_size"
	default:
		return "unknown"
	}
}

// evaluateClassRetain evaluates whether buffer is eligible for retention inside
// class before shard credit or bucket storage is touched.
func evaluateClassRetain(class SizeClass, buffer []byte) classRetainDecision {
	if buffer == nil {
		return classRetainRejectNilBuffer
	}

	capacity := cap(buffer)
	if capacity == 0 {
		return classRetainRejectZeroCapacity
	}

	if SizeFromInt(capacity) < class.ByteSize() {
		return classRetainRejectCapacityBelowClassSize
	}

	return classRetainAccept
}
