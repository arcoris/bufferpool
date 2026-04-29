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

import controlseries "arcoris.dev/bufferpool/internal/control/series"

// PoolPartitionCounterDelta is the raw counter movement between two samples.
//
// Delta values are controller inputs. Lifetime metrics remain diagnostic, while
// future EWMA or adaptive scoring should be built from bounded windows like
// this one. This type stores raw counter deltas only; it does not compute rates,
// scores, or policy decisions.
type PoolPartitionCounterDelta struct {
	// Gets is the Pool acquisition-attempt delta.
	Gets uint64

	// Hits is the retained-storage reuse delta.
	Hits uint64

	// Misses is the miss/allocation-path delta.
	Misses uint64

	// Allocations is the backing-allocation delta.
	Allocations uint64

	// Puts is the Pool return-attempt delta.
	Puts uint64

	// ReturnedBytes is the returned-buffer capacity delta.
	ReturnedBytes uint64

	// Retains is the retained-return delta.
	Retains uint64

	// RetainedBytes is the retained capacity delta. It is a monotonic accepted
	// byte counter, not the current retained-byte gauge.
	RetainedBytes uint64

	// Drops is the dropped-return delta.
	Drops uint64

	// DroppedBytes is the dropped returned-capacity delta.
	DroppedBytes uint64

	// ClosedPoolDrops is the owner-side closed-Pool drop reason delta.
	ClosedPoolDrops uint64

	// ReturnedBuffersDisabledDrops is the owner-side disabled-return drop delta.
	ReturnedBuffersDisabledDrops uint64

	// OversizedDrops is the owner-side oversized-buffer drop reason delta.
	OversizedDrops uint64

	// UnsupportedClassDrops is the owner-side unsupported-class drop reason delta.
	UnsupportedClassDrops uint64

	// InvalidPolicyDrops is the owner-side invalid-policy drop reason delta.
	InvalidPolicyDrops uint64

	// LeaseAcquisitions is the lease-acquisition delta.
	LeaseAcquisitions uint64

	// LeaseReleases is the successful ownership-release delta.
	LeaseReleases uint64

	// LeaseInvalidReleases is the malformed release-attempt delta.
	LeaseInvalidReleases uint64

	// LeaseDoubleReleases is the double-release delta.
	LeaseDoubleReleases uint64

	// LeaseOwnershipViolations is the strict ownership violation delta.
	LeaseOwnershipViolations uint64

	// LeasePoolReturnAttempts is the post-release Pool handoff attempt delta.
	LeasePoolReturnAttempts uint64

	// LeasePoolReturnSuccesses is the successful post-release Pool handoff delta.
	LeasePoolReturnSuccesses uint64

	// LeasePoolReturnFailures is the failed post-release Pool handoff delta.
	LeasePoolReturnFailures uint64

	// LeasePoolReturnClosedFailures is the closed-Pool handoff failure delta.
	LeasePoolReturnClosedFailures uint64

	// LeasePoolReturnAdmissionFailures is the non-closed handoff failure delta.
	LeasePoolReturnAdmissionFailures uint64
}

// IsZero reports whether d contains no observed counter movement.
func (d PoolPartitionCounterDelta) IsZero() bool {
	return d.Gets == 0 &&
		d.Hits == 0 &&
		d.Misses == 0 &&
		d.Allocations == 0 &&
		d.Puts == 0 &&
		d.ReturnedBytes == 0 &&
		d.Retains == 0 &&
		d.RetainedBytes == 0 &&
		d.Drops == 0 &&
		d.DroppedBytes == 0 &&
		d.ClosedPoolDrops == 0 &&
		d.ReturnedBuffersDisabledDrops == 0 &&
		d.OversizedDrops == 0 &&
		d.UnsupportedClassDrops == 0 &&
		d.InvalidPolicyDrops == 0 &&
		d.LeaseAcquisitions == 0 &&
		d.LeaseReleases == 0 &&
		d.LeaseInvalidReleases == 0 &&
		d.LeaseDoubleReleases == 0 &&
		d.LeaseOwnershipViolations == 0 &&
		d.LeasePoolReturnAttempts == 0 &&
		d.LeasePoolReturnSuccesses == 0 &&
		d.LeasePoolReturnFailures == 0 &&
		d.LeasePoolReturnClosedFailures == 0 &&
		d.LeasePoolReturnAdmissionFailures == 0
}

// PoolPartitionWindow captures two partition samples and their counter delta.
//
// Generation and PolicyGeneration come from Current. Window construction does
// not mutate Pool, LeaseRegistry, or PoolPartition state. If a counter appears
// to move backwards, the delta treats Current as the post-reset value. That
// makes reset or reconstruction visible as a fresh baseline instead of wrapping
// unsigned counters.
type PoolPartitionWindow struct {
	// Generation is the current sample partition generation.
	Generation Generation

	// PolicyGeneration is the current sample runtime-policy generation.
	PolicyGeneration Generation

	// Previous is the older sample used for delta computation.
	Previous PoolPartitionSample

	// Current is the newer sample used for delta computation.
	Current PoolPartitionSample

	// Delta is the raw counter movement from Previous to Current.
	Delta PoolPartitionCounterDelta
}

// NewPoolPartitionWindow returns a window from previous to current.
func NewPoolPartitionWindow(previous, current PoolPartitionSample) PoolPartitionWindow {
	var window PoolPartitionWindow
	window.Reset(previous, current)
	return window
}

// Reset rebuilds w from previous to current while reusing stored sample slices.
//
// Reset owns copies of Previous and Current so caller mutations after Reset do
// not affect the window. It allocates only when the existing stored Pool sample
// capacity is too small for the input samples.
func (w *PoolPartitionWindow) Reset(previous, current PoolPartitionSample) {
	if w == nil {
		return
	}
	w.Generation = current.Generation
	w.PolicyGeneration = current.PolicyGeneration
	w.Previous = copyPoolPartitionSampleInto(w.Previous, previous)
	w.Current = copyPoolPartitionSampleInto(w.Current, current)
	w.Delta = newPoolPartitionCounterDelta(previous, current)
}

// newPoolPartitionCounterDelta computes raw counter movement between samples.
func newPoolPartitionCounterDelta(previous, current PoolPartitionSample) PoolPartitionCounterDelta {
	return PoolPartitionCounterDelta{
		Gets:            controlseries.DeltaValue(previous.PoolCounters.Gets, current.PoolCounters.Gets),
		Hits:            controlseries.DeltaValue(previous.PoolCounters.Hits, current.PoolCounters.Hits),
		Misses:          controlseries.DeltaValue(previous.PoolCounters.Misses, current.PoolCounters.Misses),
		Allocations:     controlseries.DeltaValue(previous.PoolCounters.Allocations, current.PoolCounters.Allocations),
		Puts:            controlseries.DeltaValue(previous.PoolCounters.Puts, current.PoolCounters.Puts),
		ReturnedBytes:   controlseries.DeltaValue(previous.PoolCounters.ReturnedBytes, current.PoolCounters.ReturnedBytes),
		Retains:         controlseries.DeltaValue(previous.PoolCounters.Retains, current.PoolCounters.Retains),
		RetainedBytes:   controlseries.DeltaValue(previous.PoolCounters.RetainedBytes, current.PoolCounters.RetainedBytes),
		Drops:           controlseries.DeltaValue(previous.PoolCounters.Drops, current.PoolCounters.Drops),
		DroppedBytes:    controlseries.DeltaValue(previous.PoolCounters.DroppedBytes, current.PoolCounters.DroppedBytes),
		ClosedPoolDrops: controlseries.DeltaValue(previous.PoolCounters.DropReasons.ClosedPool, current.PoolCounters.DropReasons.ClosedPool),
		ReturnedBuffersDisabledDrops: controlseries.DeltaValue(
			previous.PoolCounters.DropReasons.ReturnedBuffersDisabled,
			current.PoolCounters.DropReasons.ReturnedBuffersDisabled,
		),
		OversizedDrops:           controlseries.DeltaValue(previous.PoolCounters.DropReasons.Oversized, current.PoolCounters.DropReasons.Oversized),
		UnsupportedClassDrops:    controlseries.DeltaValue(previous.PoolCounters.DropReasons.UnsupportedClass, current.PoolCounters.DropReasons.UnsupportedClass),
		InvalidPolicyDrops:       controlseries.DeltaValue(previous.PoolCounters.DropReasons.InvalidPolicy, current.PoolCounters.DropReasons.InvalidPolicy),
		LeaseAcquisitions:        controlseries.DeltaValue(previous.LeaseCounters.Acquisitions, current.LeaseCounters.Acquisitions),
		LeaseReleases:            controlseries.DeltaValue(previous.LeaseCounters.Releases, current.LeaseCounters.Releases),
		LeaseInvalidReleases:     controlseries.DeltaValue(previous.LeaseCounters.InvalidReleases, current.LeaseCounters.InvalidReleases),
		LeaseDoubleReleases:      controlseries.DeltaValue(previous.LeaseCounters.DoubleReleases, current.LeaseCounters.DoubleReleases),
		LeaseOwnershipViolations: controlseries.DeltaValue(previous.LeaseCounters.OwnershipViolations, current.LeaseCounters.OwnershipViolations),
		LeasePoolReturnAttempts:  controlseries.DeltaValue(previous.LeaseCounters.PoolReturnAttempts, current.LeaseCounters.PoolReturnAttempts),
		LeasePoolReturnSuccesses: controlseries.DeltaValue(previous.LeaseCounters.PoolReturnSuccesses, current.LeaseCounters.PoolReturnSuccesses),
		LeasePoolReturnFailures:  controlseries.DeltaValue(previous.LeaseCounters.PoolReturnFailures, current.LeaseCounters.PoolReturnFailures),
		LeasePoolReturnClosedFailures: controlseries.DeltaValue(
			previous.LeaseCounters.PoolReturnClosedFailures,
			current.LeaseCounters.PoolReturnClosedFailures,
		),
		LeasePoolReturnAdmissionFailures: controlseries.DeltaValue(
			previous.LeaseCounters.PoolReturnAdmissionFailures,
			current.LeaseCounters.PoolReturnAdmissionFailures,
		),
	}
}

// clonePoolPartitionSample returns a sample copy with owned per-Pool slice storage.
func clonePoolPartitionSample(sample PoolPartitionSample) PoolPartitionSample {
	return copyPoolPartitionSampleInto(PoolPartitionSample{}, sample)
}

// copyPoolPartitionSampleInto copies src into dst while reusing dst.Pools capacity.
func copyPoolPartitionSampleInto(dst, src PoolPartitionSample) PoolPartitionSample {
	pools := dst.Pools[:0]
	if src.Pools != nil {
		if cap(pools) < len(src.Pools) {
			pools = make([]PoolPartitionPoolSample, len(src.Pools))
		} else {
			pools = pools[:len(src.Pools)]
		}
		copy(pools, src.Pools)
	} else {
		pools = nil
	}
	dst = src
	dst.Pools = pools
	return dst
}
