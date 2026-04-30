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
// EWMA and adaptive scoring are built from bounded windows like this one. This
// type stores raw counter deltas only; it does not compute rates, scores, or
// policy decisions.
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

// PoolPartitionPoolWindow captures one Pool's counter delta inside a partition
// window.
type PoolPartitionPoolWindow struct {
	// Name is the partition-local Pool name.
	Name string

	// Delta is the Pool counter movement from previous to current.
	Delta PoolPartitionCounterDelta

	// Classes contains class-level counter movement for this Pool.
	Classes []PoolPartitionClassWindow
}

// PoolPartitionClassWindow captures one Pool class's counter delta.
type PoolPartitionClassWindow struct {
	// Class is the immutable size-class descriptor.
	Class SizeClass

	// ClassID is the Pool-local class identifier.
	ClassID ClassID

	// Previous is the older class sample used for delta computation.
	Previous PoolPartitionClassSample

	// Current is the newer class sample used for delta computation.
	Current PoolPartitionClassSample

	// Delta contains class counter movement from Previous to Current.
	Delta ClassCountersDelta
}

// ClassCountersDelta describes class activity between two samples.
type ClassCountersDelta = classCountersDelta

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

	// Pools contains per-Pool and per-class counter movement.
	Pools []PoolPartitionPoolWindow
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
	w.Pools = copyPoolPartitionPoolWindowsInto(w.Pools, previous, current)
}

// newPoolPartitionCounterDelta computes raw counter movement between samples.
func newPoolPartitionCounterDelta(previous, current PoolPartitionSample) PoolPartitionCounterDelta {
	return newPoolPartitionPoolCounterDelta(previous.PoolCounters, current.PoolCounters, previous.LeaseCounters, current.LeaseCounters)
}

// newPoolPartitionPoolCounterDelta computes raw Pool counter movement.
func newPoolPartitionPoolCounterDelta(previous, current PoolCountersSnapshot, previousLease, currentLease LeaseCountersSnapshot) PoolPartitionCounterDelta {
	return PoolPartitionCounterDelta{
		Gets:            controlseries.DeltaValue(previous.Gets, current.Gets),
		Hits:            controlseries.DeltaValue(previous.Hits, current.Hits),
		Misses:          controlseries.DeltaValue(previous.Misses, current.Misses),
		Allocations:     controlseries.DeltaValue(previous.Allocations, current.Allocations),
		Puts:            controlseries.DeltaValue(previous.Puts, current.Puts),
		ReturnedBytes:   controlseries.DeltaValue(previous.ReturnedBytes, current.ReturnedBytes),
		Retains:         controlseries.DeltaValue(previous.Retains, current.Retains),
		RetainedBytes:   controlseries.DeltaValue(previous.RetainedBytes, current.RetainedBytes),
		Drops:           controlseries.DeltaValue(previous.Drops, current.Drops),
		DroppedBytes:    controlseries.DeltaValue(previous.DroppedBytes, current.DroppedBytes),
		ClosedPoolDrops: controlseries.DeltaValue(previous.DropReasons.ClosedPool, current.DropReasons.ClosedPool),
		ReturnedBuffersDisabledDrops: controlseries.DeltaValue(
			previous.DropReasons.ReturnedBuffersDisabled,
			current.DropReasons.ReturnedBuffersDisabled,
		),
		OversizedDrops:           controlseries.DeltaValue(previous.DropReasons.Oversized, current.DropReasons.Oversized),
		UnsupportedClassDrops:    controlseries.DeltaValue(previous.DropReasons.UnsupportedClass, current.DropReasons.UnsupportedClass),
		InvalidPolicyDrops:       controlseries.DeltaValue(previous.DropReasons.InvalidPolicy, current.DropReasons.InvalidPolicy),
		LeaseAcquisitions:        controlseries.DeltaValue(previousLease.Acquisitions, currentLease.Acquisitions),
		LeaseReleases:            controlseries.DeltaValue(previousLease.Releases, currentLease.Releases),
		LeaseInvalidReleases:     controlseries.DeltaValue(previousLease.InvalidReleases, currentLease.InvalidReleases),
		LeaseDoubleReleases:      controlseries.DeltaValue(previousLease.DoubleReleases, currentLease.DoubleReleases),
		LeaseOwnershipViolations: controlseries.DeltaValue(previousLease.OwnershipViolations, currentLease.OwnershipViolations),
		LeasePoolReturnAttempts:  controlseries.DeltaValue(previousLease.PoolReturnAttempts, currentLease.PoolReturnAttempts),
		LeasePoolReturnSuccesses: controlseries.DeltaValue(previousLease.PoolReturnSuccesses, currentLease.PoolReturnSuccesses),
		LeasePoolReturnFailures:  controlseries.DeltaValue(previousLease.PoolReturnFailures, currentLease.PoolReturnFailures),
		LeasePoolReturnClosedFailures: controlseries.DeltaValue(
			previousLease.PoolReturnClosedFailures,
			currentLease.PoolReturnClosedFailures,
		),
		LeasePoolReturnAdmissionFailures: controlseries.DeltaValue(
			previousLease.PoolReturnAdmissionFailures,
			currentLease.PoolReturnAdmissionFailures,
		),
	}
}

// copyPoolPartitionPoolWindowsInto rebuilds per-Pool windows while reusing dst.
func copyPoolPartitionPoolWindowsInto(dst []PoolPartitionPoolWindow, previous, current PoolPartitionSample) []PoolPartitionPoolWindow {
	dst = dst[:0]
	if cap(dst) < len(current.Pools) {
		dst = make([]PoolPartitionPoolWindow, 0, len(current.Pools))
	}

	for _, currentPool := range current.Pools {
		previousPool, _ := findPoolPartitionPoolSample(previous, currentPool.Name)
		var poolWindow PoolPartitionPoolWindow
		if len(dst) < cap(dst) {
			poolWindow = dst[:cap(dst)][len(dst)]
		}
		poolWindow.Name = currentPool.Name
		poolWindow.Delta = newPoolPartitionPoolCounterDelta(previousPool.Counters, currentPool.Counters, LeaseCountersSnapshot{}, LeaseCountersSnapshot{})
		poolWindow.Classes = copyPoolPartitionClassWindowsInto(poolWindow.Classes, previousPool, currentPool)
		dst = append(dst, poolWindow)
	}

	return dst
}

// copyPoolPartitionClassWindowsInto rebuilds per-class windows while reusing dst.
func copyPoolPartitionClassWindowsInto(dst []PoolPartitionClassWindow, previous, current PoolPartitionPoolSample) []PoolPartitionClassWindow {
	dst = dst[:0]
	if cap(dst) < len(current.Classes) {
		dst = make([]PoolPartitionClassWindow, 0, len(current.Classes))
	}

	for _, currentClass := range current.Classes {
		previousClass, _ := findPoolPartitionClassSample(previous, currentClass.ClassID)
		dst = append(dst, PoolPartitionClassWindow{
			Class:    currentClass.Class,
			ClassID:  currentClass.ClassID,
			Previous: previousClass,
			Current:  currentClass,
			Delta:    currentClass.Counters.deltaSince(previousClass.Counters),
		})
	}

	return dst
}

// findPoolPartitionPoolSample returns a Pool sample by partition-local name.
func findPoolPartitionPoolSample(sample PoolPartitionSample, name string) (PoolPartitionPoolSample, bool) {
	for _, pool := range sample.Pools {
		if pool.Name == name {
			return pool, true
		}
	}
	return PoolPartitionPoolSample{}, false
}

// findPoolPartitionClassSample returns a class sample by ClassID.
func findPoolPartitionClassSample(sample PoolPartitionPoolSample, classID ClassID) (PoolPartitionClassSample, bool) {
	for _, class := range sample.Classes {
		if class.ClassID == classID {
			return class, true
		}
	}
	return PoolPartitionClassSample{}, false
}

// clonePoolPartitionSample returns a sample copy with owned per-Pool slice storage.
func clonePoolPartitionSample(sample PoolPartitionSample) PoolPartitionSample {
	return copyPoolPartitionSampleInto(PoolPartitionSample{}, sample)
}

// copyPoolPartitionSampleInto copies src into dst while reusing nested sample
// capacity.
func copyPoolPartitionSampleInto(dst, src PoolPartitionSample) PoolPartitionSample {
	pools := dst.Pools[:0]
	if src.Pools != nil {
		if cap(pools) < len(src.Pools) {
			pools = make([]PoolPartitionPoolSample, len(src.Pools))
		} else {
			pools = pools[:len(src.Pools)]
		}
		for index := range src.Pools {
			pools[index] = copyPoolPartitionPoolSampleInto(pools[index], src.Pools[index])
		}
	} else {
		pools = nil
	}
	dst = src
	dst.Pools = pools
	return dst
}

// copyPoolPartitionPoolSampleInto copies src while reusing nested class storage.
func copyPoolPartitionPoolSampleInto(dst, src PoolPartitionPoolSample) PoolPartitionPoolSample {
	classes := dst.Classes[:0]
	if src.Classes != nil {
		if cap(classes) < len(src.Classes) {
			classes = make([]PoolPartitionClassSample, len(src.Classes))
		} else {
			classes = classes[:len(src.Classes)]
		}
		copy(classes, src.Classes)
	} else {
		classes = nil
	}

	dst = src
	dst.Classes = classes
	return dst
}
