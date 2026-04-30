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
	controlsmooth "arcoris.dev/bufferpool/internal/control/smooth"
)

const (
	// defaultPartitionEWMAHalfLife is the default real-time half-life for
	// partition controller smoothing.
	//
	// One default manual tick interval halves the previous signal weight. This
	// keeps smoothing responsive without replaying fake ticks when a foreground
	// controller call is delayed.
	defaultPartitionEWMAHalfLife = defaultPartitionControllerTickInterval
)

// PoolPartitionEWMAConfig configures partition-local controller smoothing.
//
// EWMA is updated from window rates in the control plane. It is not updated in
// Pool.Get, Pool.Put, LeaseRegistry release paths, or any data-plane hot path.
type PoolPartitionEWMAConfig struct {
	// HalfLife is the elapsed time after which the previous smoothed value carries
	// half its former weight.
	HalfLife time.Duration

	// MinAlpha clamps the previous-value weight after half-life decay. Zero leaves
	// the lower bound open.
	MinAlpha float64

	// MaxAlpha clamps the previous-value weight after half-life decay. Zero means
	// the default upper bound of one.
	MaxAlpha float64
}

// Normalize fills default half-life and alpha bounds.
//
// The method intentionally does not validate the resulting value. Construction
// and controller code can normalize first and then call Validate so diagnostics
// still report invalid explicitly configured values.
func (c PoolPartitionEWMAConfig) Normalize() PoolPartitionEWMAConfig {
	if c.HalfLife == 0 {
		c.HalfLife = defaultPartitionEWMAHalfLife
	}
	if c.MaxAlpha == 0 {
		c.MaxAlpha = 1
	}
	return c
}

// Validate checks half-life and alpha clamp values.
//
// The returned error is classified as ErrInvalidOptions because smoothing
// configuration is caller input, not a runtime policy publication failure.
func (c PoolPartitionEWMAConfig) Validate() error {
	c = c.Normalize()
	if c.HalfLife <= 0 {
		return newError(ErrInvalidOptions, "bufferpool.PoolPartitionEWMAConfig: half-life must be positive")
	}
	if !partitionEWMAFinite(c.MinAlpha) || !partitionEWMAFinite(c.MaxAlpha) {
		return newError(ErrInvalidOptions, "bufferpool.PoolPartitionEWMAConfig: alpha bounds must be finite")
	}
	if c.MinAlpha < 0 || c.MinAlpha > 1 || c.MaxAlpha < 0 || c.MaxAlpha > 1 {
		return newError(ErrInvalidOptions, "bufferpool.PoolPartitionEWMAConfig: alpha bounds must be in [0, 1]")
	}
	if c.MinAlpha > c.MaxAlpha {
		return newError(ErrInvalidOptions, "bufferpool.PoolPartitionEWMAConfig: minimum alpha must not exceed maximum alpha")
	}
	return nil
}

// PoolPartitionEWMAState stores smoothed partition controller signals.
//
// The first update initializes every field from the current window rates.
// Later updates apply EWMA using the configured alpha. This type is a pure
// projection and never mutates policy or publishes runtime snapshots.
type PoolPartitionEWMAState struct {
	// Initialized reports whether the state contains at least one observation.
	Initialized bool

	// HitRatio is the smoothed reuse hit ratio.
	HitRatio float64

	// MissRatio is the smoothed reuse miss ratio.
	MissRatio float64

	// AllocationRatio is the smoothed allocation ratio.
	AllocationRatio float64

	// RetainRatio is the smoothed retained-return ratio.
	RetainRatio float64

	// DropRatio is the smoothed dropped-return ratio.
	DropRatio float64

	// PoolReturnFailureRatio is the smoothed failed Pool handoff ratio.
	PoolReturnFailureRatio float64

	// LeaseInvalidReleaseRatio is the smoothed invalid-release ratio.
	LeaseInvalidReleaseRatio float64

	// LeaseDoubleReleaseRatio is the smoothed double-release ratio.
	LeaseDoubleReleaseRatio float64

	// LeaseOwnershipViolationRatio is the smoothed ownership-violation ratio.
	LeaseOwnershipViolationRatio float64

	// GetsPerSecond is the smoothed acquisition throughput.
	GetsPerSecond float64

	// PutsPerSecond is the smoothed return throughput.
	PutsPerSecond float64

	// AllocationsPerSecond is the smoothed allocation throughput.
	AllocationsPerSecond float64

	// LeaseOpsPerSecond is the smoothed successful lease operation throughput.
	LeaseOpsPerSecond float64
}

// PoolClassEWMAState stores smoothed controller signals for one Pool class.
//
// Partition TickInto owns this state under partitionController.mu. Pool.Get,
// Pool.Put, classState, shard, and bucket code never read or mutate it.
type PoolClassEWMAState struct {
	// Initialized reports whether the state contains at least one class window.
	Initialized bool

	// Activity is the smoothed class activity score before demand and size shaping.
	Activity float64

	// GetsPerSecond is the smoothed class get throughput.
	GetsPerSecond float64

	// PutsPerSecond is the smoothed class put throughput.
	PutsPerSecond float64

	// AllocationsPerSecond is the smoothed class allocation throughput.
	AllocationsPerSecond float64

	// DropRatio is the smoothed class drop ratio.
	DropRatio float64
}

// WithUpdate returns state updated with rates, elapsed time, and config.
//
// A zero-value state accepts the first rates sample as its baseline. A later
// update smooths each field independently with the same elapsed-time decay so
// ratios and throughput remain comparable. The method normalizes config but
// leaves validation to callers that need an error instead of defensive behavior.
func (s PoolPartitionEWMAState) WithUpdate(config PoolPartitionEWMAConfig, elapsed time.Duration, rates PoolPartitionWindowRates) PoolPartitionEWMAState {
	config = config.Normalize()
	alpha := config.DecayAlpha(elapsed)
	return PoolPartitionEWMAState{
		Initialized:                  true,
		HitRatio:                     partitionEWMAValue(s.Initialized, s.HitRatio, alpha, rates.HitRatio),
		MissRatio:                    partitionEWMAValue(s.Initialized, s.MissRatio, alpha, rates.MissRatio),
		AllocationRatio:              partitionEWMAValue(s.Initialized, s.AllocationRatio, alpha, rates.AllocationRatio),
		RetainRatio:                  partitionEWMAValue(s.Initialized, s.RetainRatio, alpha, rates.RetainRatio),
		DropRatio:                    partitionEWMAValue(s.Initialized, s.DropRatio, alpha, rates.DropRatio),
		PoolReturnFailureRatio:       partitionEWMAValue(s.Initialized, s.PoolReturnFailureRatio, alpha, rates.PoolReturnFailureRatio),
		LeaseInvalidReleaseRatio:     partitionEWMAValue(s.Initialized, s.LeaseInvalidReleaseRatio, alpha, rates.LeaseInvalidReleaseRatio),
		LeaseDoubleReleaseRatio:      partitionEWMAValue(s.Initialized, s.LeaseDoubleReleaseRatio, alpha, rates.LeaseDoubleReleaseRatio),
		LeaseOwnershipViolationRatio: partitionEWMAValue(s.Initialized, s.LeaseOwnershipViolationRatio, alpha, rates.LeaseOwnershipViolationRatio),
		GetsPerSecond:                partitionEWMAValue(s.Initialized, s.GetsPerSecond, alpha, rates.GetsPerSecond),
		PutsPerSecond:                partitionEWMAValue(s.Initialized, s.PutsPerSecond, alpha, rates.PutsPerSecond),
		AllocationsPerSecond:         partitionEWMAValue(s.Initialized, s.AllocationsPerSecond, alpha, rates.AllocationsPerSecond),
		LeaseOpsPerSecond:            partitionEWMAValue(s.Initialized, s.LeaseOpsPerSecond, alpha, rates.LeaseOpsPerSecond),
	}
}

// WithUpdate returns class EWMA state updated from one class activity window.
func (s PoolClassEWMAState) WithUpdate(config PoolPartitionEWMAConfig, elapsed time.Duration, activity poolPartitionClassActivity) PoolClassEWMAState {
	config = config.Normalize()
	alpha := config.DecayAlpha(elapsed)
	return PoolClassEWMAState{
		Initialized:          true,
		Activity:             partitionEWMAValue(s.Initialized, s.Activity, alpha, activity.Activity),
		GetsPerSecond:        partitionEWMAValue(s.Initialized, s.GetsPerSecond, alpha, activity.GetsPerSecond),
		PutsPerSecond:        partitionEWMAValue(s.Initialized, s.PutsPerSecond, alpha, activity.PutsPerSecond),
		AllocationsPerSecond: partitionEWMAValue(s.Initialized, s.AllocationsPerSecond, alpha, activity.AllocationsPerSecond),
		DropRatio:            partitionEWMAValue(s.Initialized, s.DropRatio, alpha, activity.DropRatio),
	}
}

// DecayAlpha returns the previous-value weight for elapsed.
//
// The half-life formula is:
//
//	alpha = 2 ^ (-elapsed / halfLife)
//
// The returned alpha is then clamped to [MinAlpha, MaxAlpha]. elapsed <= 0 keeps
// the previous value at full weight for already-initialized state. First updates
// still initialize directly from the observed value.
func (c PoolPartitionEWMAConfig) DecayAlpha(elapsed time.Duration) float64 {
	c = c.Normalize()
	alpha := 1.0
	if elapsed > 0 && c.HalfLife > 0 {
		alpha = math.Pow(2, -float64(elapsed)/float64(c.HalfLife))
	}
	alpha = controlnumeric.Clamp(alpha, c.MinAlpha, c.MaxAlpha)
	return controlnumeric.FiniteOrZero(alpha)
}

// partitionEWMAValue adapts one partition signal to the shared EWMA primitive.
//
// Keeping this adapter local preserves the root package field names while the
// generic smoothing package remains unaware of PoolPartition-specific signals.
func partitionEWMAValue(initialized bool, previous, decayAlpha, value float64) float64 {
	observationWeight := 1 - decayAlpha
	if !initialized {
		observationWeight = 1
	}
	return (controlsmooth.EWMA{Initialized: initialized, Value: previous}).Update(observationWeight, value).Value
}

// partitionEWMAFinite reports whether value can safely participate in smoothing
// configuration.
func partitionEWMAFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
