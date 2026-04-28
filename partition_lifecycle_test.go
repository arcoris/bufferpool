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
	"testing"
	"time"
)

// TestPoolPartitionLifecycleStartsActiveAndCloseIsIdempotent verifies lifecycle basics.
func TestPoolPartitionLifecycleStartsActiveAndCloseIsIdempotent(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	if got := partition.Lifecycle(); got != LifecycleActive {
		t.Fatalf("Lifecycle() = %s, want active", got)
	}
	if partition.IsClosed() {
		t.Fatalf("new partition reports closed")
	}

	requirePartitionNoError(t, partition.Close())
	if !partition.IsClosed() {
		t.Fatalf("partition should be closed after Close")
	}
	requirePartitionNoError(t, partition.Close())
}

// TestPoolPartitionAcquireAfterCloseIsRejected verifies hard-close acquisition gating.
func TestPoolPartitionAcquireAfterCloseIsRejected(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Close())

	_, err = partition.Acquire("primary", 300)
	requirePartitionErrorIs(t, err, ErrClosed)

	_, err = partition.AcquireSize("primary", SizeFromBytes(300))
	requirePartitionErrorIs(t, err, ErrClosed)
}

// TestPoolPartitionCloseAllowsActiveLeaseRelease verifies late release diagnostics.
func TestPoolPartitionCloseAllowsActiveLeaseRelease(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	buffer := lease.Buffer()

	requirePartitionNoError(t, partition.Close())
	requirePartitionNoError(t, partition.Release(lease, buffer))

	metrics := partition.Metrics()
	if metrics.ActiveLeases != 0 {
		t.Fatalf("ActiveLeases = %d, want 0", metrics.ActiveLeases)
	}
	if metrics.CurrentActiveBytes != 0 {
		t.Fatalf("CurrentActiveBytes = %d, want 0", metrics.CurrentActiveBytes)
	}
	if metrics.PoolReturnFailures != 1 {
		t.Fatalf("PoolReturnFailures = %d, want 1 after pools are closed by partition Close", metrics.PoolReturnFailures)
	}
	if metrics.PoolReturnSuccesses != 0 {
		t.Fatalf("PoolReturnSuccesses = %d, want 0 after closed-pool handoff", metrics.PoolReturnSuccesses)
	}
	sample := partition.Sample()
	if sample.LeaseCounters.PoolReturnClosedFailures != 1 {
		t.Fatalf("PoolReturnClosedFailures = %d, want 1 after hard close", sample.LeaseCounters.PoolReturnClosedFailures)
	}

	err = partition.Release(lease, buffer)
	if !errors.Is(err, ErrDoubleRelease) {
		t.Fatalf("second Release error = %v, want ErrDoubleRelease", err)
	}
}

// TestPoolPartitionCloseGracefullyWithoutActiveLeases verifies successful drain close.
func TestPoolPartitionCloseGracefullyWithoutActiveLeases(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{Timeout: 10 * time.Millisecond})
	requirePartitionNoError(t, err)

	if !result.Attempted || !result.Completed || result.TimedOut {
		t.Fatalf("CloseGracefully result = %+v, want completed without timeout", result)
	}
	if result.ActiveLeasesBefore != 0 || result.ActiveLeasesAfter != 0 {
		t.Fatalf("CloseGracefully active counts = %+v, want zero", result)
	}
	if !partition.IsClosed() {
		t.Fatalf("partition should be closed after graceful close")
	}
	if _, err := partition.Acquire("primary", 300); !errors.Is(err, ErrClosed) {
		t.Fatalf("Acquire after graceful close error = %v, want ErrClosed", err)
	}
}

// TestPoolPartitionCloseGracefullyWaitsForActiveRelease verifies bounded drain success.
func TestPoolPartitionCloseGracefullyWaitsForActiveRelease(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	buffer := lease.Buffer()
	releaseDone := make(chan error, 1)
	go func() {
		time.Sleep(2 * time.Millisecond)
		releaseDone <- partition.Release(lease, buffer)
	}()

	result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{Timeout: 100 * time.Millisecond, PollInterval: time.Millisecond})
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, <-releaseDone)

	if !result.Completed || result.TimedOut {
		t.Fatalf("CloseGracefully result = %+v, want completed", result)
	}
	if result.ActiveLeasesBefore != 1 || result.ActiveLeasesAfter != 0 {
		t.Fatalf("CloseGracefully active counts = %+v, want before=1 after=0", result)
	}
	if !partition.IsClosed() {
		t.Fatalf("partition should be closed after graceful close")
	}
}

// TestPoolPartitionCloseGracefullyTimeoutDoesNotForceRelease verifies timeout semantics.
func TestPoolPartitionCloseGracefullyTimeoutDoesNotForceRelease(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	buffer := lease.Buffer()

	result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{Timeout: time.Millisecond, PollInterval: time.Millisecond})
	requirePartitionNoError(t, err)

	if result.Completed || !result.TimedOut {
		t.Fatalf("CloseGracefully result = %+v, want timeout without completion", result)
	}
	if result.ActiveLeasesBefore != 1 || result.ActiveLeasesAfter != 1 {
		t.Fatalf("CloseGracefully active counts = %+v, want before=1 after=1", result)
	}
	if partition.Lifecycle() != LifecycleClosing {
		t.Fatalf("Lifecycle after drain timeout = %s, want closing", partition.Lifecycle())
	}
	if lease.Buffer() == nil {
		t.Fatalf("active lease buffer was force-reclaimed")
	}
	if _, err := partition.Acquire("primary", 300); !errors.Is(err, ErrClosed) {
		t.Fatalf("Acquire during closing drain error = %v, want ErrClosed", err)
	}

	requirePartitionNoError(t, partition.Release(lease, buffer))
	requirePartitionNoError(t, partition.Close())
	if !partition.IsClosed() {
		t.Fatalf("hard Close after drain timeout should close partition")
	}
}

// TestPoolPartitionCloseGracefullyRejectsInvalidPolicy verifies timing validation.
func TestPoolPartitionCloseGracefullyRejectsInvalidPolicy(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	_, err := partition.CloseGracefully(PoolPartitionDrainPolicy{Timeout: -time.Nanosecond})
	requirePartitionErrorIs(t, err, ErrInvalidOptions)

	_, err = partition.CloseGracefully(PoolPartitionDrainPolicy{PollInterval: -time.Nanosecond})
	requirePartitionErrorIs(t, err, ErrInvalidOptions)
}

// TestPoolPartitionNilReceiverPanics verifies zero-value receiver invariants.
func TestPoolPartitionNilReceiverPanics(t *testing.T) {
	var partition *PoolPartition
	requirePartitionPanic(t, func() { _ = partition.Name() })
	requirePartitionPanic(t, func() { _ = partition.Metrics() })
	requirePartitionPanic(t, func() { _ = partition.Snapshot() })
	requirePartitionPanic(t, func() { partition.SampleInto(nil) })
	requirePartitionPanic(t, func() { _ = partition.TickInto(nil) })
	requirePartitionPanic(t, func() { _, _ = partition.CloseGracefully(PoolPartitionDrainPolicy{}) })
}
