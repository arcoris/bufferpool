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
	"math"
	"time"

	controlnumeric "arcoris.dev/bufferpool/internal/control/numeric"
	controlrate "arcoris.dev/bufferpool/internal/control/rate"
)

// poolClassKey identifies one class inside one partition-local Pool.
type poolClassKey struct {
	// PoolName is the partition-local Pool name.
	PoolName string

	// ClassID is the Pool-local class identifier.
	ClassID ClassID
}

// PoolPartitionClassActivity is the bounded-window activity input used by
// partition-local control projections for one Pool class.
//
// The value is derived from window deltas, not lifetime counters. It is a
// control-plane projection only: it does not publish policy, mutate runtime
// snapshots, execute trim, or participate in Pool.Get/Pool.Put.
type PoolPartitionClassActivity struct {
	// Activity is a finite weighted event score for this class window.
	Activity float64

	// GetsPerSecond is class get throughput over the window.
	GetsPerSecond float64

	// PutsPerSecond is class put throughput over the window.
	PutsPerSecond float64

	// AllocationsPerSecond is class allocation throughput over the window.
	AllocationsPerSecond float64

	// HitRatio is retained-storage hit ratio for this class window.
	HitRatio float64

	// MissRatio is retained-storage miss ratio for this class window.
	MissRatio float64

	// AllocationRatio is allocation pressure relative to gets.
	AllocationRatio float64

	// DropRatio is drop pressure relative to puts.
	DropRatio float64
}

// poolPartitionClassActivity is kept as the internal spelling used by the
// existing partition controller while typed pool/class scoring adopts the
// exported value name. The alias preserves current controller behavior.
type poolPartitionClassActivity = PoolPartitionClassActivity

// newPoolPartitionClassActivity converts one class window into finite activity
// signals.
func newPoolPartitionClassActivity(window PoolPartitionClassWindow, elapsed time.Duration) poolPartitionClassActivity {
	delta := window.Delta
	getAttempts := delta.Gets
	if getAttempts == 0 {
		getAttempts = poolSaturatingAdd(delta.Hits, delta.Misses)
	}
	putAttempts := delta.Puts
	if putAttempts == 0 {
		putAttempts = poolSaturatingAdd(delta.Retains, delta.Drops)
	}

	activity := float64(delta.Gets) +
		float64(delta.Hits) +
		2*float64(delta.Misses) +
		2*float64(delta.Allocations) +
		float64(delta.Retains) +
		float64(delta.Drops)

	result := poolPartitionClassActivity{
		Activity:        controlnumeric.FiniteOrZero(activity),
		HitRatio:        controlrate.HitRatio(delta.Hits, delta.Misses),
		MissRatio:       controlrate.MissRatio(delta.Hits, delta.Misses),
		AllocationRatio: controlrate.AllocationRatio(delta.Allocations, getAttempts),
		DropRatio:       controlrate.DropRatio(delta.Drops, putAttempts),
	}
	if elapsed <= 0 {
		return result
	}

	result.GetsPerSecond = controlrate.PerSecond(delta.Gets, elapsed)
	result.PutsPerSecond = controlrate.PerSecond(delta.Puts, elapsed)
	result.AllocationsPerSecond = controlrate.PerSecond(delta.Allocations, elapsed)
	return result
}

// poolPartitionClassScore returns the budget-allocation score for one Pool
// class.
//
// The score is intentionally compact: activity is increased by demand signals,
// gently rewarded for useful hits, and reduced for larger class sizes so equal
// event rates do not automatically move all memory to large buffers. It is a
// local partition-controller input, not a public policy.
func poolPartitionClassScore(class PoolPartitionClassSample, activity poolPartitionClassActivity, ewma PoolClassEWMAState) float64 {
	baseActivity := activity.Activity
	if ewma.Initialized {
		baseActivity = ewma.Activity
	}
	if baseActivity <= 0 {
		return 0
	}

	demandBoost := 1 + activity.MissRatio + activity.AllocationRatio + activity.DropRatio
	efficiencyBoost := 1 + 0.25*activity.HitRatio
	sizePenalty := poolPartitionClassSizePenalty(class.Class.Size())

	score := baseActivity * demandBoost * efficiencyBoost * sizePenalty
	return controlnumeric.FiniteOrZero(score)
}

// poolPartitionClassSizePenalty returns a bounded class-size adjustment.
func poolPartitionClassSizePenalty(size ClassSize) float64 {
	bytes := size.Bytes()
	if bytes == 0 {
		return 1
	}

	kib := float64(KiB.Bytes())
	sizeKiB := math.Max(1, float64(bytes)/kib)
	return 1 / math.Sqrt(sizeKiB)
}
