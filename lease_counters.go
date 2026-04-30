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
	"errors"
	"sync/atomic"
)

// LeaseCountersSnapshot is an observational counter projection for a
// LeaseRegistry.
//
// Lifetime counters are monotonic. ActiveLeases and ActiveBytes are gauges.
// Pool-return counters describe the best-effort retained-storage handoff after
// ownership release; they are separate from ownership validation failures
// because a Pool handoff failure does not make a released lease active again.
//
// The registry stores raw facts only. Rates such as pool-return failure rate,
// active memory ratio, or release failure rate should be computed from sampled
// counters by metrics/controller code rather than maintained as hot-path
// counters.
type LeaseCountersSnapshot struct {
	// Acquisitions is the number of leases successfully created.
	Acquisitions uint64

	// RequestedBytes is the sum of logical requested sizes for acquisitions.
	RequestedBytes uint64

	// AcquiredBytes is the sum of backing capacities checked out from Pool.
	AcquiredBytes uint64

	// Releases is the number of ownership releases that completed.
	Releases uint64

	// ReleasedBytes is the sum of capacities handed to Pool after release.
	ReleasedBytes uint64

	// ActiveLeases is the current checked-out lease count.
	ActiveLeases uint64

	// ActiveBytes is the current checked-out backing capacity gauge.
	ActiveBytes uint64

	// InvalidReleases counts malformed releases that are not strict identity or
	// managed capacity-growth violations.
	InvalidReleases uint64

	// DoubleReleases counts release attempts after ownership already completed.
	DoubleReleases uint64

	// OwnershipViolations counts ownership validation failures.
	OwnershipViolations uint64

	// ForeignBufferReleases counts strict releases with the wrong slice base.
	ForeignBufferReleases uint64

	// CapacityGrowthViolations counts managed capacity-growth failures.
	CapacityGrowthViolations uint64

	// PoolReturnAttempts counts best-effort Pool handoffs after ownership completion.
	PoolReturnAttempts uint64

	// PoolReturnSuccesses counts Pool handoffs that returned nil.
	PoolReturnSuccesses uint64

	// PoolReturnFailures counts Pool handoffs that returned an error.
	PoolReturnFailures uint64

	// PoolReturnClosedFailures counts Pool handoffs rejected by Pool lifecycle.
	PoolReturnClosedFailures uint64

	// PoolReturnAdmissionFailures counts non-closed Pool handoff failures.
	PoolReturnAdmissionFailures uint64
}

// IsZero reports whether s contains no activity and no active leases.
func (s LeaseCountersSnapshot) IsZero() bool {
	return s.Acquisitions == 0 && s.RequestedBytes == 0 && s.AcquiredBytes == 0 &&
		s.Releases == 0 && s.ReleasedBytes == 0 && s.ActiveLeases == 0 && s.ActiveBytes == 0 &&
		s.InvalidReleases == 0 && s.DoubleReleases == 0 && s.OwnershipViolations == 0 &&
		s.ForeignBufferReleases == 0 && s.CapacityGrowthViolations == 0 &&
		s.PoolReturnAttempts == 0 && s.PoolReturnSuccesses == 0 && s.PoolReturnFailures == 0 &&
		s.PoolReturnClosedFailures == 0 && s.PoolReturnAdmissionFailures == 0
}

// ReleaseAttempts returns successful ownership releases plus rejected ownership
// release attempts.
//
// It intentionally excludes PoolReturnAttempts. Pool return handoff happens only
// after ownership completion, so it is tracked as a separate diagnostic stream.
func (s LeaseCountersSnapshot) ReleaseAttempts() uint64 {
	return s.Releases + s.InvalidReleases + s.DoubleReleases + s.OwnershipViolations
}

type leaseCounters struct {
	// acquisitions is the lifetime count of successful lease acquisition.
	acquisitions atomic.Uint64

	// requestedBytes is the lifetime sum of logical requested sizes.
	requestedBytes atomic.Uint64

	// acquiredBytes is the lifetime sum of checked-out backing capacities.
	acquiredBytes atomic.Uint64

	// releases is the lifetime count of successful ownership releases.
	releases atomic.Uint64

	// releasedBytes is the lifetime sum of capacities released to Pool handoff.
	releasedBytes atomic.Uint64

	// activeLeases is the current checked-out lease gauge.
	activeLeases atomic.Uint64

	// activeBytes is the current checked-out backing-capacity gauge.
	activeBytes atomic.Uint64

	// invalidReleases counts malformed releases that do not fit a more specific
	// ownership violation bucket.
	invalidReleases atomic.Uint64

	// doubleReleases counts releases after the record is already released.
	doubleReleases atomic.Uint64

	// ownershipViolations counts strict ownership validation failures.
	ownershipViolations atomic.Uint64

	// foreignBufferReleases counts strict base-pointer mismatches.
	foreignBufferReleases atomic.Uint64

	// capacityGrowthViolations counts capacity growth limit failures.
	capacityGrowthViolations atomic.Uint64

	// poolReturnAttempts counts Pool retained-storage handoff attempts after release.
	poolReturnAttempts atomic.Uint64

	// poolReturnSuccesses counts successful Pool retained-storage handoffs.
	poolReturnSuccesses atomic.Uint64

	// poolReturnFailures counts failed Pool retained-storage handoffs.
	poolReturnFailures atomic.Uint64

	// poolReturnClosedFailures counts Pool handoffs rejected by lifecycle.
	poolReturnClosedFailures atomic.Uint64

	// poolReturnAdmissionFailures counts non-lifecycle Pool handoff errors.
	poolReturnAdmissionFailures atomic.Uint64
}

// recordAcquire records one successful lease acquisition.
//
// Active gauges use acquired capacity, not requested length, because ownership
// accounts for checked-out backing memory.
func (c *leaseCounters) recordAcquire(requested Size, acquiredCapacity uint64) {
	c.acquisitions.Add(1)
	c.requestedBytes.Add(requested.Bytes())
	c.acquiredBytes.Add(acquiredCapacity)
	c.activeLeases.Add(1)
	c.activeBytes.Add(acquiredCapacity)
}

// recordRelease records one successful ownership release.
//
// ActiveBytes is decremented by acquiredCapacity, not returnedCapacity. The
// registry releases the memory it checked out, even if the returned slice header
// was clipped or canonicalized before Pool handoff.
func (c *leaseCounters) recordRelease(acquiredCapacity uint64, returnedCapacity uint64) {
	c.releases.Add(1)
	c.releasedBytes.Add(returnedCapacity)
	c.activeLeases.Add(^uint64(0))
	c.activeBytes.Add(^uint64(acquiredCapacity - 1))
}

// recordInvalidRelease records a rejected release attempt by diagnostic kind.
//
// The counters are intentionally split so snapshots can distinguish malformed
// handles, double release, strict foreign-buffer release, and strict growth
// violations without inspecting individual active lease records.
func (c *leaseCounters) recordInvalidRelease(kind OwnershipViolationKind) {
	switch kind {
	case OwnershipViolationDoubleRelease:
		c.doubleReleases.Add(1)
	case OwnershipViolationForeignBuffer:
		c.ownershipViolations.Add(1)
		c.foreignBufferReleases.Add(1)
	case OwnershipViolationCapacityGrowth:
		c.ownershipViolations.Add(1)
		c.capacityGrowthViolations.Add(1)
	default:
		c.invalidReleases.Add(1)
	}
}

// recordPoolReturnAttempt records that ownership completed and Pool handoff
// was attempted.
func (c *leaseCounters) recordPoolReturnAttempt() {
	c.poolReturnAttempts.Add(1)
}

// recordPoolReturnSuccess records a successful best-effort Pool handoff.
func (c *leaseCounters) recordPoolReturnSuccess() {
	c.poolReturnSuccesses.Add(1)
}

// recordPoolReturnFailure records a failed best-effort Pool handoff.
//
// Closed-pool failures get a dedicated bucket because shutdown ordering is an
// ownership-layer integration concern. Other errors are grouped as admission
// failures until Pool exposes a richer handoff-failure taxonomy.
func (c *leaseCounters) recordPoolReturnFailure(err error) {
	c.poolReturnFailures.Add(1)
	if errors.Is(err, ErrClosed) {
		c.poolReturnClosedFailures.Add(1)
		return
	}

	c.poolReturnAdmissionFailures.Add(1)
}

// snapshot returns a race-safe atomic sample of lease counters.
//
// The sample is observational, not a transaction across fields. This matches
// Pool and class/shard counter semantics.
func (c *leaseCounters) snapshot() LeaseCountersSnapshot {
	return LeaseCountersSnapshot{
		Acquisitions:                c.acquisitions.Load(),
		RequestedBytes:              c.requestedBytes.Load(),
		AcquiredBytes:               c.acquiredBytes.Load(),
		Releases:                    c.releases.Load(),
		ReleasedBytes:               c.releasedBytes.Load(),
		ActiveLeases:                c.activeLeases.Load(),
		ActiveBytes:                 c.activeBytes.Load(),
		InvalidReleases:             c.invalidReleases.Load(),
		DoubleReleases:              c.doubleReleases.Load(),
		OwnershipViolations:         c.ownershipViolations.Load(),
		ForeignBufferReleases:       c.foreignBufferReleases.Load(),
		CapacityGrowthViolations:    c.capacityGrowthViolations.Load(),
		PoolReturnAttempts:          c.poolReturnAttempts.Load(),
		PoolReturnSuccesses:         c.poolReturnSuccesses.Load(),
		PoolReturnFailures:          c.poolReturnFailures.Load(),
		PoolReturnClosedFailures:    c.poolReturnClosedFailures.Load(),
		PoolReturnAdmissionFailures: c.poolReturnAdmissionFailures.Load(),
	}
}
