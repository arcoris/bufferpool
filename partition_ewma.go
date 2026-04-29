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

import controlsmooth "arcoris.dev/bufferpool/internal/control/smooth"

// PoolPartitionEWMAConfig configures partition-local controller smoothing.
//
// EWMA is updated from window rates in the control plane. It is not updated in
// Pool.Get, Pool.Put, LeaseRegistry release paths, or any data-plane hot path.
type PoolPartitionEWMAConfig struct {
	// Alpha is the update weight for new observations and must be in (0, 1].
	Alpha float64
}

// Normalize fills the default EWMA alpha when Alpha is zero.
//
// The method intentionally does not validate the resulting value. Construction
// and controller code can normalize first and then call Validate so diagnostics
// still report invalid explicitly configured alpha values.
func (c PoolPartitionEWMAConfig) Normalize() PoolPartitionEWMAConfig {
	alpha := controlsmooth.AlphaConfig{Alpha: c.Alpha}.Normalize()
	c.Alpha = alpha.Alpha
	return c
}

// Validate checks that Alpha is finite and in the accepted range.
//
// The returned error is classified as ErrInvalidOptions because smoothing
// configuration is caller input, not a runtime policy publication failure.
func (c PoolPartitionEWMAConfig) Validate() error {
	if err := (controlsmooth.AlphaConfig{Alpha: c.Alpha}).Validate(); err != nil {
		return wrapError(ErrInvalidOptions, err, "bufferpool.PoolPartitionEWMAConfig: invalid alpha")
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
}

// WithUpdate returns state updated with rates and config.
//
// A zero-value state accepts the first rates sample as its baseline. A later
// update smooths each field independently with the same alpha so ratios and
// throughput remain comparable. The method normalizes config but leaves
// validation to callers that need an error instead of defensive behavior.
func (s PoolPartitionEWMAState) WithUpdate(config PoolPartitionEWMAConfig, rates PoolPartitionWindowRates) PoolPartitionEWMAState {
	config = config.Normalize()
	alpha := config.Alpha
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
	}
}

// partitionEWMAValue adapts one partition signal to the shared EWMA primitive.
//
// Keeping this adapter local preserves the root package field names while the
// generic smoothing package remains unaware of PoolPartition-specific signals.
func partitionEWMAValue(initialized bool, previous, alpha, value float64) float64 {
	return (controlsmooth.EWMA{Initialized: initialized, Value: previous}).Update(alpha, value).Value
}
