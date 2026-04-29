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
	"time"

	controlnumeric "arcoris.dev/bufferpool/internal/control/numeric"
	controlrate "arcoris.dev/bufferpool/internal/control/rate"
)

// PoolPartitionWindowRates projects ratios and optional throughput from a
// PoolPartitionWindow.
//
// Rates are derived from window deltas, not lifetime metrics. They do not apply
// EWMA, adaptive scoring, or policy decisions. Future adaptive control should
// build those layers on top of this bounded window projection.
type PoolPartitionWindowRates struct {
	// HitRatio is delta Hits divided by delta Hits plus Misses.
	HitRatio float64

	// MissRatio is delta Misses divided by delta Hits plus Misses.
	MissRatio float64

	// AllocationRatio is delta Allocations divided by valid acquisition attempts.
	AllocationRatio float64

	// RetainRatio is delta Retains divided by valid Put attempts when available.
	// Older or synthetic windows without Put attempts fall back to Retains plus
	// Drops with saturating arithmetic.
	RetainRatio float64

	// DropRatio is delta Drops divided by valid Put attempts when available.
	// Older or synthetic windows without Put attempts fall back to Retains plus
	// Drops with saturating arithmetic.
	DropRatio float64

	// PoolReturnFailureRatio is failed Pool handoffs divided by handoff attempts.
	PoolReturnFailureRatio float64

	// PoolReturnSuccessRatio is successful Pool handoffs divided by handoff attempts.
	PoolReturnSuccessRatio float64

	// PoolReturnClosedFailureRatio is closed-Pool handoff failures divided by handoff attempts.
	PoolReturnClosedFailureRatio float64

	// PoolReturnAdmissionFailureRatio is admission/runtime handoff failures divided by handoff attempts.
	PoolReturnAdmissionFailureRatio float64

	// LeaseInvalidReleaseRatio is invalid releases divided by release attempts.
	LeaseInvalidReleaseRatio float64

	// LeaseDoubleReleaseRatio is double releases divided by release attempts.
	LeaseDoubleReleaseRatio float64

	// LeaseOwnershipViolationRatio is ownership violations divided by release attempts.
	LeaseOwnershipViolationRatio float64

	// GetsPerSecond is delta Gets divided by elapsed seconds when elapsed is positive.
	GetsPerSecond float64

	// PutsPerSecond is delta Puts divided by elapsed seconds when elapsed is positive.
	PutsPerSecond float64

	// AllocationsPerSecond is delta Allocations divided by elapsed seconds.
	AllocationsPerSecond float64

	// LeaseAcquisitionsPerSecond is successful lease acquisitions per second.
	LeaseAcquisitionsPerSecond float64

	// LeaseReleasesPerSecond is successful lease releases per second.
	LeaseReleasesPerSecond float64

	// LeaseOpsPerSecond is successful lease acquisitions plus successful lease
	// releases per second. Invalid, double, and ownership-violation release
	// attempts remain risk/misuse signals and are intentionally excluded from
	// this activity throughput field.
	LeaseOpsPerSecond float64

	// ReturnedBytesPerSecond is delta ReturnedBytes divided by elapsed seconds.
	// It is a diagnostic rate and is not automatically combined into partition
	// activity because returned and dropped bytes have different control
	// meanings.
	ReturnedBytesPerSecond float64

	// DroppedBytesPerSecond is delta DroppedBytes divided by elapsed seconds.
	// It is a diagnostic rate and is not automatically combined into partition
	// activity because dropped bytes can indicate admission pressure rather than
	// useful byte movement.
	DroppedBytesPerSecond float64
}

// NewPoolPartitionWindowRates returns ratio projections for window.
func NewPoolPartitionWindowRates(window PoolPartitionWindow) PoolPartitionWindowRates {
	return NewPoolPartitionTimedWindowRates(window, 0)
}

// NewPoolPartitionTimedWindowRates returns ratio and throughput projections.
//
// If elapsed is zero or negative, throughput fields remain zero. The function
// does not observe wall-clock time and does not mutate partition state.
func NewPoolPartitionTimedWindowRates(window PoolPartitionWindow, elapsed time.Duration) PoolPartitionWindowRates {
	delta := window.Delta
	getAttempts := delta.Gets
	if getAttempts == 0 {
		getAttempts = controlnumeric.SaturatingSumUint64(delta.Hits, delta.Misses)
	}
	putAttempts := delta.Puts
	if putAttempts == 0 {
		putAttempts = controlnumeric.SaturatingSumUint64(delta.Retains, delta.Drops)
	}
	releaseAttempts := controlnumeric.SaturatingSumUint64(
		delta.LeaseReleases,
		delta.LeaseInvalidReleases,
		delta.LeaseDoubleReleases,
		delta.LeaseOwnershipViolations,
	)
	leaseActivityOps := controlnumeric.SaturatingSumUint64(delta.LeaseAcquisitions, delta.LeaseReleases)

	rates := PoolPartitionWindowRates{
		HitRatio:                        controlrate.HitRatio(delta.Hits, delta.Misses),
		MissRatio:                       controlrate.MissRatio(delta.Hits, delta.Misses),
		AllocationRatio:                 controlrate.AllocationRatio(delta.Allocations, getAttempts),
		RetainRatio:                     controlrate.RetainRatio(delta.Retains, putAttempts),
		DropRatio:                       controlrate.DropRatio(delta.Drops, putAttempts),
		PoolReturnFailureRatio:          controlrate.FailureRatio(delta.LeasePoolReturnFailures, delta.LeasePoolReturnAttempts),
		PoolReturnSuccessRatio:          controlrate.SuccessRatio(delta.LeasePoolReturnSuccesses, delta.LeasePoolReturnAttempts),
		PoolReturnClosedFailureRatio:    controlrate.FailureRatio(delta.LeasePoolReturnClosedFailures, delta.LeasePoolReturnAttempts),
		PoolReturnAdmissionFailureRatio: controlrate.FailureRatio(delta.LeasePoolReturnAdmissionFailures, delta.LeasePoolReturnAttempts),
		LeaseInvalidReleaseRatio:        controlrate.InvalidRatio(delta.LeaseInvalidReleases, releaseAttempts),
		LeaseDoubleReleaseRatio:         controlrate.InvalidRatio(delta.LeaseDoubleReleases, releaseAttempts),
		LeaseOwnershipViolationRatio:    controlrate.InvalidRatio(delta.LeaseOwnershipViolations, releaseAttempts),
	}
	if elapsed <= 0 {
		return rates
	}

	rates.GetsPerSecond = controlrate.PerSecond(delta.Gets, elapsed)
	rates.PutsPerSecond = controlrate.PerSecond(delta.Puts, elapsed)
	rates.AllocationsPerSecond = controlrate.PerSecond(delta.Allocations, elapsed)
	rates.LeaseAcquisitionsPerSecond = controlrate.PerSecond(delta.LeaseAcquisitions, elapsed)
	rates.LeaseReleasesPerSecond = controlrate.PerSecond(delta.LeaseReleases, elapsed)
	rates.LeaseOpsPerSecond = controlrate.PerSecond(leaseActivityOps, elapsed)
	rates.ReturnedBytesPerSecond = controlrate.BytesPerSecond(delta.ReturnedBytes, elapsed)
	rates.DroppedBytesPerSecond = controlrate.BytesPerSecond(delta.DroppedBytes, elapsed)
	return rates
}
