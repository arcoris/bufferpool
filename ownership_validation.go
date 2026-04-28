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
	"math/bits"
	"unsafe"
)

const (
	errLeaseDoubleRelease         = "bufferpool.Lease.Release: lease already released"
	errLeaseReleaseNilBuffer      = "bufferpool.Lease.Release: buffer must not be nil"
	errLeaseReleaseZeroCapacity   = "bufferpool.Lease.Release: buffer capacity must be greater than zero"
	errLeaseReleaseForeignBuffer  = "bufferpool.Lease.Release: returned buffer does not preserve acquired slice base"
	errLeaseReleaseCapacityGrowth = "bufferpool.Lease.Release: returned capacity exceeds ownership growth limit"
)

// validateLeaseRelease validates one release attempt against the lease record and
// ownership policy.
//
// The function does not mutate registry state. Callers decide whether a failed
// validation consumes the lease. Current strict behavior leaves the lease active
// so the caller may retry with the correct buffer.
//
// Strict mode compares the acquired slice base pointer. Sharing some backing
// allocation is not enough: buffer[1:] is rejected because the slice base
// changed. After successful strict validation, release canonicalizes the handoff
// back to the originally acquired capacity before calling Pool.Put. This
// protects Pool's capacity-based class routing from clipped-capacity views.
//
// Accounting mode intentionally skips strict backing identity checks. It tracks
// lease lifecycle and active bytes, but replacement or foreign buffers can
// complete ownership in that mode by design.
func validateLeaseRelease(record *leaseRecord, buffer []byte, policy OwnershipPolicy) (OwnershipViolationKind, error) {
	if buffer == nil {
		return OwnershipViolationNilBuffer, newError(ErrNilBuffer, errLeaseReleaseNilBuffer)
	}
	if cap(buffer) == 0 {
		return OwnershipViolationZeroCapacity, newError(ErrZeroCapacity, errLeaseReleaseZeroCapacity)
	}
	if ownershipModeEnforcesStrictBufferIdentity(policy) && bufferBackingData(buffer) != record.data {
		return OwnershipViolationForeignBuffer, newError(ErrOwnershipViolation, errLeaseReleaseForeignBuffer)
	}
	if ownershipModeEnforcesCapacityGrowth(policy) {
		maxCapacity := ownershipMaxReturnedCapacity(record.originClass, policy.MaxReturnedCapacityGrowth)
		if uint64(cap(buffer)) > maxCapacity {
			return OwnershipViolationCapacityGrowth, newError(ErrOwnershipViolation, errLeaseReleaseCapacityGrowth)
		}
	}
	return OwnershipViolationNone, nil
}

// bufferBackingData returns the first addressable byte for this slice view.
//
// The function requires cap(buffer) > 0. It reslices to full capacity so strict
// ownership can compare base identity even when len(buffer) is zero. The result
// is the base of the provided slice view, not proof of whole backing-array
// ownership.
func bufferBackingData(buffer []byte) *byte {
	if cap(buffer) == 0 {
		return nil
	}
	return unsafe.SliceData(buffer[:cap(buffer)])
}

// ownershipMaxReturnedCapacity returns the maximum returned capacity permitted
// by origin class and growth ratio.
//
// The multiplication is saturating. If origin*ratio cannot be represented before
// division by PolicyRatioScale, the result is MaxUint64, which effectively means
// the policy does not reject by capacity growth at this scale.
func ownershipMaxReturnedCapacity(origin ClassSize, ratio PolicyRatio) uint64 {
	if ratio == 0 {
		return origin.Bytes()
	}
	hi, lo := bits.Mul64(origin.Bytes(), uint64(ratio))
	scale := uint64(PolicyRatioScale)
	if hi >= scale {
		return ^uint64(0)
	}
	value, _ := bits.Div64(hi, lo, scale)
	if value == 0 {
		return 1
	}
	return value
}
