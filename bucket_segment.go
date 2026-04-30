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

// bucketSegment owns one lazily allocated group of bucket slots.
//
// Segments form a stack through previous. The bucket head points at the most
// recently allocated segment, so LIFO push/pop never searches through older
// segments. A segment is dropped as soon as it becomes empty.
type bucketSegment struct {
	// previous points at the next older segment in the bucket stack.
	previous *bucketSegment

	// slots stores retained buffers in stack order for this segment.
	//
	// Only slots in [0, count) are occupied. Free slots must stay nil so removed
	// buffers do not remain reachable through stale slice references.
	slots [][]byte

	// count is the number of occupied slots in this segment.
	count int
}

// newBucketSegment allocates slot metadata for one lazy bucket segment.
func newBucketSegment(slotLimit int, previous *bucketSegment) *bucketSegment {
	if slotLimit <= 0 {
		panic(errBucketInvalidSegmentSlotLimit)
	}

	return &bucketSegment{
		previous: previous,
		slots:    make([][]byte, slotLimit),
	}
}

// isFull reports whether no free slot remains in this segment.
func (s *bucketSegment) isFull() bool {
	s.mustHaveValidCount()

	return s.count == len(s.slots)
}

// clearOccupiedSlots clears all occupied slots in this segment.
//
// The method validates occupied slots before clearing them. Invalid occupied
// slots indicate corruption in bucket accounting and should panic instead of
// silently hiding a stale or zero-capacity entry.
func (s *bucketSegment) clearOccupiedSlots() {
	s.mustHaveValidCount()

	for index := 0; index < s.count; index++ {
		_ = s.mustHaveOccupiedSlot(index)
		s.slots[index] = nil
	}

	s.count = 0
}

// mustHaveValidCount verifies this segment's local count invariant.
func (s *bucketSegment) mustHaveValidCount() {
	if s == nil || s.count < 0 || s.count > len(s.slots) {
		panic(errBucketInvalidCount)
	}
}

// mustHaveFreeSlot verifies that index is a writable free slot.
func (s *bucketSegment) mustHaveFreeSlot(index int) {
	s.mustHaveValidCount()
	if index < 0 || index >= len(s.slots) {
		panic(errBucketInvalidCount)
	}
	if s.slots[index] != nil {
		panic(errBucketNonEmptyFreeSlot)
	}
}

// mustHaveOccupiedSlot returns the occupied buffer at index.
func (s *bucketSegment) mustHaveOccupiedSlot(index int) []byte {
	s.mustHaveValidCount()
	if index < 0 || index >= s.count {
		panic(errBucketInvalidCount)
	}

	buffer := s.slots[index]
	if !bucketCanRetain(buffer) {
		panic(errBucketEmptyOccupiedSlot)
	}

	return buffer
}
