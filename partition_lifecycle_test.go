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
	"runtime"
	"sync"
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

// TestPoolPartitionCloseConcurrent verifies one hard cleanup under racing Close.
func TestPoolPartitionCloseConcurrent(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary", "secondary"))
	requirePartitionNoError(t, err)

	const callers = 32
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- partition.Close()
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		requirePartitionNoError(t, err)
	}
	if !partition.IsClosed() {
		t.Fatalf("partition should be closed")
	}
	for _, name := range partition.PoolNames() {
		snapshot, ok := partition.PoolSnapshot(name)
		if !ok {
			t.Fatalf("missing pool snapshot for %q", name)
		}
		if snapshot.Lifecycle != LifecycleClosed {
			t.Fatalf("pool %q lifecycle = %s, want closed", name, snapshot.Lifecycle)
		}
	}
}

// TestPoolPartitionCloseIdempotentAfterConcurrentClose verifies post-race close.
func TestPoolPartitionCloseIdempotentAfterConcurrentClose(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			requirePartitionNoError(t, partition.Close())
		}()
	}
	wg.Wait()

	requirePartitionNoError(t, partition.Close())
	if !partition.IsClosed() {
		t.Fatalf("partition should remain closed")
	}
}

// TestPoolPartitionCloseDoesNotDuplicateCleanupGeneration verifies one close event.
func TestPoolPartitionCloseDoesNotDuplicateCleanupGeneration(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	before := partition.Sample().Generation
	requirePartitionNoError(t, partition.Close())
	after := partition.Sample().Generation
	requirePartitionNoError(t, partition.Close())
	repeated := partition.Sample().Generation

	if after != before.Next() {
		t.Fatalf("generation after close = %s, want %s", after, before.Next())
	}
	if repeated != after {
		t.Fatalf("generation after repeated close = %s, want %s", repeated, after)
	}
}

// TestPoolPartitionAcquireConcurrentWithClose exercises acquisition during shutdown.
func TestPoolPartitionAcquireConcurrentWithClose(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	const workers = 4
	const iterations = 64
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for iteration := 0; iteration < iterations; iteration++ {
				lease, err := partition.Acquire("primary", 128)
				if err != nil {
					if errors.Is(err, ErrClosed) {
						return
					}
					errs <- err
					return
				}
				if err := partition.Release(lease, lease.Buffer()); err != nil {
					errs <- err
					return
				}
			}
		}()
	}

	close(start)
	requirePartitionNoError(t, partition.Close())
	wg.Wait()
	close(errs)
	for err := range errs {
		requirePartitionNoError(t, err)
	}
	if !partition.IsClosed() {
		t.Fatalf("partition should be closed")
	}
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

// TestPoolPartitionReleaseAfterBeginCloseBeforeClosed verifies graceful-timeout release.
func TestPoolPartitionReleaseAfterBeginCloseBeforeClosed(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)

	result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{})
	requirePartitionNoError(t, err)
	if !result.TimedOut || partition.Lifecycle() != LifecycleClosing {
		t.Fatalf("CloseGracefully result/lifecycle = %+v/%s, want timeout/closing", result, partition.Lifecycle())
	}

	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
	metrics := partition.Metrics()
	if metrics.ActiveLeases != 0 || metrics.PoolReturnSuccesses != 1 {
		t.Fatalf("metrics after closing release = active %d successes %d, want 0/1", metrics.ActiveLeases, metrics.PoolReturnSuccesses)
	}
}

// TestPoolPartitionReleaseAfterClosedCompletesLease verifies hard-close release.
func TestPoolPartitionReleaseAfterClosedCompletesLease(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Close())
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))

	metrics := partition.Metrics()
	if metrics.ActiveLeases != 0 || metrics.PoolReturnFailures != 1 {
		t.Fatalf("metrics after closed release = active %d failures %d, want 0/1", metrics.ActiveLeases, metrics.PoolReturnFailures)
	}
}

// TestPoolPartitionReleaseConcurrentWithClose verifies active leases can finish.
func TestPoolPartitionReleaseConcurrentWithClose(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	const leases = 16
	acquired := make([]Lease, leases)
	for index := range acquired {
		lease, err := partition.Acquire("primary", 128)
		requirePartitionNoError(t, err)
		acquired[index] = lease
	}

	start := make(chan struct{})
	errs := make(chan error, leases+1)
	var wg sync.WaitGroup
	for _, lease := range acquired {
		lease := lease
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := partition.Release(lease, lease.Buffer()); err != nil {
				errs <- err
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		if err := partition.Close(); err != nil {
			errs <- err
		}
	}()

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		requirePartitionNoError(t, err)
	}

	metrics := partition.Metrics()
	if metrics.ActiveLeases != 0 || metrics.LeaseReleases != leases {
		t.Fatalf("metrics after release/close race = active %d releases %d, want 0/%d", metrics.ActiveLeases, metrics.LeaseReleases, leases)
	}
}

// TestPoolPartitionCloseGracefullyWithoutActiveLeases verifies successful drain close.
func TestPoolPartitionCloseGracefullyWithoutActiveLeases(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	before := partition.Sample().Generation

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
	if after := partition.Sample().Generation; after != before.Next() {
		t.Fatalf("generation after graceful close = %s, want %s", after, before.Next())
	}
	poolSnapshot, ok := partition.PoolSnapshot("primary")
	if !ok {
		t.Fatalf("missing primary pool snapshot")
	}
	if poolSnapshot.Lifecycle != LifecycleClosed {
		t.Fatalf("pool lifecycle after graceful close = %s, want closed", poolSnapshot.Lifecycle)
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

// TestPoolPartitionCloseGracefullyZeroTimeoutIsNonBlocking verifies immediate timeout semantics.
func TestPoolPartitionCloseGracefullyZeroTimeoutIsNonBlocking(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	buffer := lease.Buffer()

	result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{})
	requirePartitionNoError(t, err)

	if result.Completed || !result.TimedOut {
		t.Fatalf("CloseGracefully zero-timeout result = %+v, want timeout", result)
	}
	if result.ActiveLeasesBefore != 1 || result.ActiveLeasesAfter != 1 {
		t.Fatalf("CloseGracefully zero-timeout active counts = %+v, want before=1 after=1", result)
	}
	if partition.Lifecycle() != LifecycleClosing {
		t.Fatalf("Lifecycle after zero-timeout drain = %s, want closing", partition.Lifecycle())
	}
	poolSnapshot, ok := partition.PoolSnapshot("primary")
	if !ok {
		t.Fatalf("missing primary pool snapshot")
	}
	if poolSnapshot.Lifecycle != LifecycleActive {
		t.Fatalf("pool lifecycle after zero-timeout drain = %s, want active", poolSnapshot.Lifecycle)
	}
	if _, err := partition.Acquire("primary", 300); !errors.Is(err, ErrClosed) {
		t.Fatalf("Acquire after drain start error = %v, want ErrClosed", err)
	}

	requirePartitionNoError(t, partition.Release(lease, buffer))
	metrics := partition.Metrics()
	if metrics.PoolReturnSuccesses != 1 || metrics.PoolReturnFailures != 0 {
		t.Fatalf("release after graceful timeout handoff metrics = %+v, want one success", metrics)
	}
}

// TestPoolPartitionCloseGracefullyRetryAfterTimeout verifies retry after drain timeout.
func TestPoolPartitionCloseGracefullyRetryAfterTimeout(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	buffer := lease.Buffer()

	result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{})
	requirePartitionNoError(t, err)
	if !result.TimedOut || result.Completed {
		t.Fatalf("first CloseGracefully result = %+v, want timeout", result)
	}

	requirePartitionNoError(t, partition.Release(lease, buffer))
	result, err = partition.CloseGracefully(PoolPartitionDrainPolicy{})
	requirePartitionNoError(t, err)
	if !result.Completed || result.TimedOut {
		t.Fatalf("retry CloseGracefully result = %+v, want completed", result)
	}
	if !partition.IsClosed() {
		t.Fatalf("partition should be closed after retry")
	}
	poolSnapshot, ok := partition.PoolSnapshot("primary")
	if !ok {
		t.Fatalf("missing primary pool snapshot")
	}
	if poolSnapshot.Lifecycle != LifecycleClosed {
		t.Fatalf("pool lifecycle after retry = %s, want closed", poolSnapshot.Lifecycle)
	}
}

// TestPoolPartitionHardCloseAfterGracefulTimeoutAllowsLateDiagnosticRelease verifies fallback close.
func TestPoolPartitionHardCloseAfterGracefulTimeoutAllowsLateDiagnosticRelease(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	buffer := lease.Buffer()

	result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{})
	requirePartitionNoError(t, err)
	if !result.TimedOut || result.Completed {
		t.Fatalf("CloseGracefully result = %+v, want timeout", result)
	}

	requirePartitionNoError(t, partition.Close())
	requirePartitionNoError(t, partition.Release(lease, buffer))

	sample := partition.Sample()
	if sample.LeaseCounters.PoolReturnFailures != 1 || sample.LeaseCounters.PoolReturnClosedFailures != 1 {
		t.Fatalf("late release counters = %+v, want closed Pool handoff failure", sample.LeaseCounters)
	}
}

// TestPoolPartitionCloseGracefullyAfterHardCloseWithActiveLease documents terminal observation.
func TestPoolPartitionCloseGracefullyAfterHardCloseWithActiveLease(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Close())

	result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{})
	requirePartitionNoError(t, err)
	if result.Attempted || result.Completed || result.TimedOut {
		t.Fatalf("CloseGracefully after hard close with active lease = %+v, want not attempted/incomplete/no timeout", result)
	}
	if result.ActiveLeasesBefore != 1 || result.ActiveLeasesAfter != 1 {
		t.Fatalf("CloseGracefully active counts = %+v, want 1/1", result)
	}

	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
	sample := partition.Sample()
	if sample.ActiveLeases != 0 ||
		sample.LeaseCounters.PoolReturnFailures != 1 ||
		sample.LeaseCounters.PoolReturnClosedFailures != 1 {
		t.Fatalf("sample after late release = active %d counters %+v, want active=0 closed-pool failure", sample.ActiveLeases, sample.LeaseCounters)
	}
}

// TestPoolPartitionCloseGracefullyAfterHardCloseWithNoActiveLeases documents closed idle observation.
func TestPoolPartitionCloseGracefullyAfterHardCloseWithNoActiveLeases(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Close())
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
	before := partition.Sample().Generation

	result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{})
	requirePartitionNoError(t, err)
	if result.Attempted || !result.Completed || result.TimedOut {
		t.Fatalf("CloseGracefully after hard close with no active leases = %+v, want not attempted/completed/no timeout", result)
	}
	if result.ActiveLeasesBefore != 0 || result.ActiveLeasesAfter != 0 {
		t.Fatalf("CloseGracefully active counts = %+v, want 0/0", result)
	}
	if after := partition.Sample().Generation; after != before {
		t.Fatalf("generation after closed CloseGracefully = %s, want %s", after, before)
	}
}

// TestPoolPartitionCloseGracefullyAfterTimeoutHardCloseAndRelease documents retry semantics.
func TestPoolPartitionCloseGracefullyAfterTimeoutHardCloseAndRelease(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{})
	requirePartitionNoError(t, err)
	if !result.Attempted || result.Completed || !result.TimedOut {
		t.Fatalf("first CloseGracefully result = %+v, want attempted timeout", result)
	}

	requirePartitionNoError(t, partition.Close())
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
	result, err = partition.CloseGracefully(PoolPartitionDrainPolicy{})
	requirePartitionNoError(t, err)
	if result.Attempted || !result.Completed || result.TimedOut {
		t.Fatalf("CloseGracefully after timeout/hard-close/release = %+v, want not attempted/completed/no timeout", result)
	}
	if result.ActiveLeasesBefore != 0 || result.ActiveLeasesAfter != 0 {
		t.Fatalf("CloseGracefully active counts = %+v, want 0/0", result)
	}
}

// TestPoolPartitionCloseGracefullyConcurrentHardCloseWithActiveLease verifies handoff.
func TestPoolPartitionCloseGracefullyConcurrentHardCloseWithActiveLease(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	resultCh := make(chan partitionDrainAttempt, 1)
	go func() {
		result, closeErr := partition.CloseGracefully(PoolPartitionDrainPolicy{
			Timeout:      time.Second,
			PollInterval: time.Millisecond,
		})
		resultCh <- partitionDrainAttempt{result: result, err: closeErr}
	}()
	waitForPartitionLifecycle(t, partition, LifecycleClosing)

	requirePartitionNoError(t, partition.Close())
	if partition.Lifecycle() != LifecycleClosed {
		t.Fatalf("Lifecycle after hard Close = %s, want closed", partition.Lifecycle())
	}
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))

	attempt := receivePartitionDrainAttempt(t, resultCh)
	requirePartitionNoError(t, attempt.err)
	if !attempt.result.Attempted || !attempt.result.Completed || attempt.result.TimedOut {
		t.Fatalf("CloseGracefully concurrent hard close result = %+v, want attempted/completed/no timeout", attempt.result)
	}
	if attempt.result.ActiveLeasesBefore != 1 || attempt.result.ActiveLeasesAfter != 0 {
		t.Fatalf("CloseGracefully concurrent active counts = %+v, want 1/0", attempt.result)
	}

	metrics := partition.Metrics()
	if metrics.ActiveLeases != 0 || metrics.PoolReturnFailures != 1 {
		t.Fatalf("metrics after concurrent graceful/hard close = active %d failures %d, want 0/1", metrics.ActiveLeases, metrics.PoolReturnFailures)
	}
	sample := partition.Sample()
	if sample.LeaseCounters.PoolReturnClosedFailures != 1 {
		t.Fatalf("PoolReturnClosedFailures = %d, want 1", sample.LeaseCounters.PoolReturnClosedFailures)
	}
}

// TestPoolPartitionCloseGracefullyAlreadyClosedDoesNotAttempt verifies closed behavior.
func TestPoolPartitionCloseGracefullyAlreadyClosedDoesNotAttempt(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Close())
	before := partition.Sample().Generation

	result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{})
	requirePartitionNoError(t, err)

	if result.Attempted {
		t.Fatalf("CloseGracefully on closed partition attempted drain: %+v", result)
	}
	if !result.Completed || result.ActiveLeasesBefore != 0 || result.ActiveLeasesAfter != 0 {
		t.Fatalf("CloseGracefully on closed partition result = %+v, want completed zero-active observation", result)
	}
	if after := partition.Sample().Generation; after != before {
		t.Fatalf("generation after closed CloseGracefully = %s, want %s", after, before)
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

type partitionDrainAttempt struct {
	result PoolPartitionDrainResult
	err    error
}

func receivePartitionDrainAttempt(t *testing.T, ch <-chan partitionDrainAttempt) partitionDrainAttempt {
	t.Helper()

	select {
	case attempt := <-ch:
		return attempt
	case <-time.After(time.Second):
		// Deadlock guard only. Successful path is channel-driven.
		t.Fatalf("CloseGracefully did not return")
		return partitionDrainAttempt{}
	}
}

func waitForPartitionLifecycle(t *testing.T, partition *PoolPartition, state LifecycleState) {
	t.Helper()

	// Deadlock guard only. The loop exits as soon as the observed lifecycle moves.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if partition.Lifecycle() == state {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("partition lifecycle = %s, want %s", partition.Lifecycle(), state)
}
