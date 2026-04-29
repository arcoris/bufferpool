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
	"sync"
	"testing"
)

// TestPoolGroupAcquireRelease verifies group-routed ownership lifecycle.
func TestPoolGroupAcquireRelease(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	lease, err := group.Acquire("alpha", "alpha-pool", 300)
	requireGroupNoError(t, err)
	if lease.Buffer() == nil {
		t.Fatalf("Acquire returned nil buffer")
	}
	requireGroupNoError(t, group.Release("alpha", lease, lease.Buffer()))

	metrics := group.Metrics()
	if metrics.LeaseAcquisitions != 1 || metrics.LeaseReleases != 1 {
		t.Fatalf("lease metrics = acquisitions %d releases %d, want 1/1", metrics.LeaseAcquisitions, metrics.LeaseReleases)
	}
	if metrics.ActiveLeases != 0 || metrics.CurrentActiveBytes != 0 {
		t.Fatalf("active metrics = leases %d bytes %d, want zero", metrics.ActiveLeases, metrics.CurrentActiveBytes)
	}
}

// TestPoolGroupAcquireSize verifies Size-typed routing mirrors Acquire.
func TestPoolGroupAcquireSize(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	lease, err := group.AcquireSize("alpha", "alpha-pool", SizeFromBytes(300))
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.Release("alpha", lease, lease.Buffer()))
}

// TestPoolGroupAcquireMissingPartition verifies group-local lookup errors.
func TestPoolGroupAcquireMissingPartition(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	_, err := group.Acquire("missing", "alpha-pool", 300)
	requireGroupErrorIs(t, err, ErrInvalidOptions)
}

// TestPoolGroupAcquireMissingPool verifies partition-local pool errors pass through.
func TestPoolGroupAcquireMissingPool(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	_, err := group.Acquire("alpha", "missing", 300)
	requireGroupErrorIs(t, err, ErrInvalidOptions)
}

// TestPoolGroupAcquireAfterCloseRejected locks acquisition gating after close.
func TestPoolGroupAcquireAfterCloseRejected(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha"))
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.Close())

	_, err = group.Acquire("alpha", "alpha-pool", 300)
	requireGroupErrorIs(t, err, ErrClosed)

	_, err = group.AcquireSize("alpha", "alpha-pool", SizeFromBytes(300))
	requireGroupErrorIs(t, err, ErrClosed)
}

// TestPoolGroupReleaseMissingPartition verifies release lookup stays group-local.
func TestPoolGroupReleaseMissingPartition(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	err := group.Release("missing", Lease{}, nil)
	requireGroupErrorIs(t, err, ErrInvalidOptions)
}

// TestPoolGroupReleaseAfterCloseCompletesLease preserves late release semantics.
func TestPoolGroupReleaseAfterCloseCompletesLease(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha"))
	requireGroupNoError(t, err)

	lease, err := group.Acquire("alpha", "alpha-pool", 300)
	requireGroupNoError(t, err)
	buffer := lease.Buffer()
	requireGroupNoError(t, group.Close())
	requireGroupNoError(t, group.Release("alpha", lease, buffer))

	metrics := group.Metrics()
	if metrics.ActiveLeases != 0 {
		t.Fatalf("ActiveLeases = %d, want 0", metrics.ActiveLeases)
	}
	if metrics.LeaseReleases != 1 {
		t.Fatalf("LeaseReleases = %d, want 1", metrics.LeaseReleases)
	}
	if metrics.PoolReturnFailures != 1 {
		t.Fatalf("PoolReturnFailures = %d, want 1 after closed-pool handoff", metrics.PoolReturnFailures)
	}
}

// TestPoolGroupAcquireReleaseUpdatesMetrics verifies real counters reach group metrics.
func TestPoolGroupAcquireReleaseUpdatesMetrics(t *testing.T) {
	group := testNewPoolGroup(t, "alpha", "beta")

	alphaLease, err := group.Acquire("alpha", "alpha-pool", 300)
	requireGroupNoError(t, err)
	betaLease, err := group.Acquire("beta", "beta-pool", 600)
	requireGroupNoError(t, err)
	betaReleased := false
	defer func() {
		if !betaReleased {
			requireGroupNoError(t, group.Release("beta", betaLease, betaLease.Buffer()))
		}
	}()

	active := group.Sample()
	if active.Aggregate.LeaseCounters.Acquisitions != 2 || active.Aggregate.ActiveLeases != 2 {
		t.Fatalf("active sample lease counters = %+v active %d, want two active acquisitions", active.Aggregate.LeaseCounters, active.Aggregate.ActiveLeases)
	}
	if active.CurrentActiveBytes == 0 || active.CurrentOwnedBytes != poolSaturatingAdd(active.CurrentRetainedBytes, active.CurrentActiveBytes) {
		t.Fatalf("active bytes = retained %d active %d owned %d", active.CurrentRetainedBytes, active.CurrentActiveBytes, active.CurrentOwnedBytes)
	}

	requireGroupNoError(t, group.Release("alpha", alphaLease, alphaLease.Buffer()))
	sample := group.Sample()
	if sample.Aggregate.LeaseCounters.Releases != 1 {
		t.Fatalf("Lease releases = %d, want 1", sample.Aggregate.LeaseCounters.Releases)
	}
	if sample.Aggregate.ActiveLeases != 1 {
		t.Fatalf("ActiveLeases = %d, want 1", sample.Aggregate.ActiveLeases)
	}
	if sample.CurrentRetainedBytes == 0 || sample.CurrentActiveBytes == 0 {
		t.Fatalf("sample bytes = retained %d active %d, want both non-zero", sample.CurrentRetainedBytes, sample.CurrentActiveBytes)
	}
	if sample.CurrentOwnedBytes != poolSaturatingAdd(sample.CurrentRetainedBytes, sample.CurrentActiveBytes) {
		t.Fatalf("CurrentOwnedBytes = %d, want retained+active", sample.CurrentOwnedBytes)
	}

	requireGroupNoError(t, group.Release("beta", betaLease, betaLease.Buffer()))
	betaReleased = true
	metrics := group.Metrics()
	if metrics.LeaseAcquisitions != 2 || metrics.LeaseReleases != 2 {
		t.Fatalf("metrics lease counters = acquisitions %d releases %d, want 2/2", metrics.LeaseAcquisitions, metrics.LeaseReleases)
	}
	if metrics.ActiveLeases != 0 || metrics.CurrentActiveBytes != 0 {
		t.Fatalf("metrics active = leases %d bytes %d, want zero", metrics.ActiveLeases, metrics.CurrentActiveBytes)
	}
}

// TestPoolGroupSampleConcurrentWithAcquireRelease exercises sampling during churn.
func TestPoolGroupSampleConcurrentWithAcquireRelease(t *testing.T) {
	runPoolGroupConcurrentObservation(t, func(group *PoolGroup) {
		var sample PoolGroupSample
		group.SampleInto(&sample)
	})
}

// TestPoolGroupMetricsConcurrentWithAcquireRelease exercises metrics during churn.
func TestPoolGroupMetricsConcurrentWithAcquireRelease(t *testing.T) {
	runPoolGroupConcurrentObservation(t, func(group *PoolGroup) {
		_ = group.Metrics()
	})
}

// TestPoolGroupSnapshotConcurrentWithAcquireRelease exercises snapshots during churn.
func TestPoolGroupSnapshotConcurrentWithAcquireRelease(t *testing.T) {
	runPoolGroupConcurrentObservation(t, func(group *PoolGroup) {
		_ = group.Snapshot()
	})
}

// runPoolGroupConcurrentObservation runs one diagnostic observer beside lease churn.
func runPoolGroupConcurrentObservation(t *testing.T, observe func(*PoolGroup)) {
	t.Helper()

	group := testNewPoolGroup(t, "alpha")
	const workers = 4
	const iterations = 64
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for iteration := 0; iteration < iterations; iteration++ {
				lease, err := group.Acquire("alpha", "alpha-pool", 64+worker+iteration)
				if err != nil {
					errs <- err
					return
				}
				if err := group.Release("alpha", lease, lease.Buffer()); err != nil {
					errs <- err
					return
				}
			}
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iterations; i++ {
			observe(group)
		}
	}()

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		requireGroupNoError(t, err)
	}
}
