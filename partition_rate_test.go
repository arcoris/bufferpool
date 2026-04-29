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
	"testing"
	"time"
)

// TestPoolPartitionWindowRatesUseDeltaCounters verifies window-derived ratios.
func TestPoolPartitionWindowRatesUseDeltaCounters(t *testing.T) {
	window := PoolPartitionWindow{
		Delta: PoolPartitionCounterDelta{
			Hits:                             3,
			Misses:                           1,
			Allocations:                      1,
			Retains:                          2,
			Drops:                            2,
			LeaseReleases:                    6,
			LeaseInvalidReleases:             1,
			LeaseDoubleReleases:              2,
			LeaseOwnershipViolations:         1,
			LeasePoolReturnAttempts:          5,
			LeasePoolReturnSuccesses:         4,
			LeasePoolReturnFailures:          1,
			LeasePoolReturnClosedFailures:    1,
			LeasePoolReturnAdmissionFailures: 1,
		},
	}

	rates := NewPoolPartitionWindowRates(window)

	requirePartitionFloat64(t, rates.HitRatio, 0.75)
	requirePartitionFloat64(t, rates.MissRatio, 0.25)
	requirePartitionFloat64(t, rates.AllocationRatio, 0.25)
	requirePartitionFloat64(t, rates.RetainRatio, 0.5)
	requirePartitionFloat64(t, rates.DropRatio, 0.5)
	requirePartitionFloat64(t, rates.PoolReturnFailureRatio, 0.2)
	requirePartitionFloat64(t, rates.PoolReturnSuccessRatio, 0.8)
	requirePartitionFloat64(t, rates.PoolReturnClosedFailureRatio, 0.2)
	requirePartitionFloat64(t, rates.PoolReturnAdmissionFailureRatio, 0.2)
	requirePartitionFloat64(t, rates.LeaseInvalidReleaseRatio, 0.1)
	requirePartitionFloat64(t, rates.LeaseDoubleReleaseRatio, 0.2)
	requirePartitionFloat64(t, rates.LeaseOwnershipViolationRatio, 0.1)
}

// TestPoolPartitionWindowRatesFallbackDenominatorsUseSaturatingSums verifies
// fallback denominator overflow protection for synthetic or older windows.
func TestPoolPartitionWindowRatesFallbackDenominatorsUseSaturatingSums(t *testing.T) {
	window := PoolPartitionWindow{
		Delta: PoolPartitionCounterDelta{
			Hits:        math.MaxUint64,
			Misses:      1,
			Allocations: math.MaxUint64,
			Retains:     math.MaxUint64,
			Drops:       1,
		},
	}

	rates := NewPoolPartitionWindowRates(window)

	requirePartitionFloat64(t, rates.AllocationRatio, 1)
	requirePartitionFloat64(t, rates.RetainRatio, 1)
	if rates.DropRatio <= 0 || math.IsInf(rates.DropRatio, 0) || math.IsNaN(rates.DropRatio) {
		t.Fatalf("DropRatio = %v, want positive finite fallback ratio", rates.DropRatio)
	}
}

// TestPoolPartitionWindowRatesRetainDropRatiosUsePutAttemptsWhenAvailable
// verifies that valid Put attempts are the primary denominator.
func TestPoolPartitionWindowRatesRetainDropRatiosUsePutAttemptsWhenAvailable(t *testing.T) {
	window := PoolPartitionWindow{
		Delta: PoolPartitionCounterDelta{
			Puts:    10,
			Retains: 2,
			Drops:   3,
		},
	}

	rates := NewPoolPartitionWindowRates(window)

	requirePartitionFloat64(t, rates.RetainRatio, 0.2)
	requirePartitionFloat64(t, rates.DropRatio, 0.3)
}

// TestPoolPartitionWindowRatesRetainDropRatiosFallbackToOutcomes verifies
// retained/drop outcomes remain usable when Put attempts are unavailable.
func TestPoolPartitionWindowRatesRetainDropRatiosFallbackToOutcomes(t *testing.T) {
	window := PoolPartitionWindow{
		Delta: PoolPartitionCounterDelta{
			Retains: 2,
			Drops:   3,
		},
	}

	rates := NewPoolPartitionWindowRates(window)

	requirePartitionFloat64(t, rates.RetainRatio, 0.4)
	requirePartitionFloat64(t, rates.DropRatio, 0.6)
}

// TestPoolPartitionWindowRatesReleaseAttemptsUseSaturatingSum verifies release
// attempt ratios cannot wrap when diagnostic counters are extreme.
func TestPoolPartitionWindowRatesReleaseAttemptsUseSaturatingSum(t *testing.T) {
	window := PoolPartitionWindow{
		Delta: PoolPartitionCounterDelta{
			LeaseReleases:            1,
			LeaseInvalidReleases:     math.MaxUint64,
			LeaseDoubleReleases:      1,
			LeaseOwnershipViolations: 1,
		},
	}

	rates := NewPoolPartitionWindowRates(window)

	requirePartitionFloat64(t, rates.LeaseInvalidReleaseRatio, 1)
	if math.IsInf(rates.LeaseDoubleReleaseRatio, 0) || math.IsNaN(rates.LeaseDoubleReleaseRatio) {
		t.Fatalf("LeaseDoubleReleaseRatio = %v, want finite", rates.LeaseDoubleReleaseRatio)
	}
}

// TestPoolPartitionWindowRatesZeroDenominators verifies empty-window behavior.
func TestPoolPartitionWindowRatesZeroDenominators(t *testing.T) {
	rates := NewPoolPartitionWindowRates(PoolPartitionWindow{})

	if rates != (PoolPartitionWindowRates{}) {
		t.Fatalf("zero window rates = %+v, want zero", rates)
	}
}

// TestPoolPartitionTimedWindowRates verifies optional throughput projection.
func TestPoolPartitionTimedWindowRates(t *testing.T) {
	window := PoolPartitionWindow{
		Delta: PoolPartitionCounterDelta{
			Gets:              10,
			Puts:              6,
			Allocations:       2,
			LeaseAcquisitions: 10,
			LeaseReleases:     6,
			ReturnedBytes:     2048,
			DroppedBytes:      512,
		},
	}

	zeroElapsed := NewPoolPartitionTimedWindowRates(window, 0)
	if zeroElapsed.GetsPerSecond != 0 || zeroElapsed.ReturnedBytesPerSecond != 0 {
		t.Fatalf("zero-elapsed throughput = %+v, want zero", zeroElapsed)
	}

	rates := NewPoolPartitionTimedWindowRates(window, 2*time.Second)
	requirePartitionFloat64(t, rates.GetsPerSecond, 5)
	requirePartitionFloat64(t, rates.PutsPerSecond, 3)
	requirePartitionFloat64(t, rates.AllocationsPerSecond, 1)
	requirePartitionFloat64(t, rates.LeaseAcquisitionsPerSecond, 5)
	requirePartitionFloat64(t, rates.LeaseReleasesPerSecond, 3)
	requirePartitionFloat64(t, rates.LeaseOpsPerSecond, 8)
	requirePartitionFloat64(t, rates.ReturnedBytesPerSecond, 1024)
	requirePartitionFloat64(t, rates.DroppedBytesPerSecond, 256)
}

// TestPoolPartitionWindowRatesLeaseThroughput documents that lease activity
// throughput counts successful acquisition/release operations, not invalid
// release attempts.
func TestPoolPartitionWindowRatesLeaseThroughput(t *testing.T) {
	window := PoolPartitionWindow{
		Delta: PoolPartitionCounterDelta{
			LeaseAcquisitions:        10,
			LeaseReleases:            6,
			LeaseInvalidReleases:     100,
			LeaseDoubleReleases:      100,
			LeaseOwnershipViolations: 100,
		},
	}

	rates := NewPoolPartitionTimedWindowRates(window, 2*time.Second)

	requirePartitionFloat64(t, rates.LeaseAcquisitionsPerSecond, 5)
	requirePartitionFloat64(t, rates.LeaseReleasesPerSecond, 3)
	requirePartitionFloat64(t, rates.LeaseOpsPerSecond, 8)
}

// requirePartitionFloat64 compares floating-point projections with tight tolerance.
func requirePartitionFloat64(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.000000001 {
		t.Fatalf("float64 value = %f, want %f", got, want)
	}
}
