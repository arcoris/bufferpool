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
	"sync"
	"testing"
)

// TestNewLeaseRegistryWithZeroConfig verifies zero-config construction.
func TestNewLeaseRegistryWithZeroConfig(t *testing.T) {
	t.Parallel()

	registry, err := NewLeaseRegistry(LeaseConfig{})
	if err != nil {
		t.Fatalf("NewLeaseRegistry(LeaseConfig{}) returned error: %v", err)
	}
	defer closeLeaseRegistryForTest(t, registry)

	if registry.Lifecycle() != LifecycleActive {
		t.Fatalf("Lifecycle() = %s, want %s", registry.Lifecycle(), LifecycleActive)
	}
	if registry.IsClosed() {
		t.Fatal("new registry reported closed")
	}
	if registry.Config().Ownership.Mode != OwnershipModeStrict {
		t.Fatalf("Config().Ownership.Mode = %s, want %s", registry.Config().Ownership.Mode, OwnershipModeStrict)
	}
	if registry.OwnershipPolicy().Mode != OwnershipModeStrict {
		t.Fatalf("OwnershipPolicy().Mode = %s, want %s", registry.OwnershipPolicy().Mode, OwnershipModeStrict)
	}

	snapshot := registry.Snapshot()
	if snapshot.Lifecycle != LifecycleActive {
		t.Fatalf("Snapshot().Lifecycle = %s, want %s", snapshot.Lifecycle, LifecycleActive)
	}
	if snapshot.Generation != InitialGeneration {
		t.Fatalf("Snapshot().Generation = %s, want %s", snapshot.Generation, InitialGeneration)
	}
	if !snapshot.IsEmpty() {
		t.Fatalf("new registry snapshot is not empty: %#v", snapshot)
	}
}

// TestNewLeaseRegistryRejectsInvalidConfig verifies construction error
// classification.
func TestNewLeaseRegistryRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	registry, err := NewLeaseRegistry(LeaseConfig{Ownership: OwnershipPolicy{Mode: OwnershipModeNone}})
	if err == nil {
		t.Fatal("NewLeaseRegistry() returned nil error for invalid config")
	}
	if registry != nil {
		t.Fatalf("NewLeaseRegistry() returned non-nil registry for invalid config: %#v", registry)
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("NewLeaseRegistry() error does not match ErrInvalidOptions: %v", err)
	}
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("NewLeaseRegistry() error does not unwrap ErrInvalidPolicy: %v", err)
	}
}

// TestMustNewLeaseRegistryPanics verifies the programmer-error constructor.
func TestMustNewLeaseRegistryPanics(t *testing.T) {
	t.Parallel()

	assertPoolPanicMatches(t, func(recovered any) bool {
		err, ok := recovered.(error)
		return ok && errors.Is(err, ErrInvalidOptions) && errors.Is(err, ErrInvalidPolicy)
	}, func() {
		_ = MustNewLeaseRegistry(LeaseConfig{Ownership: OwnershipPolicy{Mode: OwnershipModeNone}})
	})
}

// TestLeaseRegistryClosePreventsAcquireButAllowsRelease verifies the shutdown
// boundary expected by future PoolPartition/PoolGroup owners.
func TestLeaseRegistryClosePreventsAcquireButAllowsRelease(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}

	if err := registry.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
	if !registry.IsClosed() {
		t.Fatal("registry did not report closed")
	}

	_, err = registry.Acquire(pool, 128)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Acquire() after Close error = %v, want ErrClosed", err)
	}

	if err := lease.ReleaseUnchanged(); err != nil {
		t.Fatalf("ReleaseUnchanged() after registry Close returned error: %v", err)
	}

	snapshot := registry.Snapshot()
	if snapshot.ActiveCount() != 0 {
		t.Fatalf("ActiveCount() after release = %d, want 0", snapshot.ActiveCount())
	}
	if snapshot.Counters.Releases != 1 {
		t.Fatalf("Releases = %d, want 1", snapshot.Counters.Releases)
	}
	if snapshot.Counters.ActiveLeases != 0 {
		t.Fatalf("ActiveLeases = %d, want 0", snapshot.Counters.ActiveLeases)
	}
	if snapshot.Counters.ActiveBytes != 0 {
		t.Fatalf("ActiveBytes = %d, want 0", snapshot.Counters.ActiveBytes)
	}
	if snapshot.Counters.PoolReturnSuccesses != 1 {
		t.Fatalf("PoolReturnSuccesses = %d, want 1", snapshot.Counters.PoolReturnSuccesses)
	}
}

// TestLeaseRegistryCloseIsIdempotent verifies repeated close calls.
func TestLeaseRegistryCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	before := registry.Snapshot().Generation
	if err := registry.Close(); err != nil {
		t.Fatalf("first Close() returned error: %v", err)
	}
	afterFirst := registry.Snapshot().Generation
	if err := registry.Close(); err != nil {
		t.Fatalf("second Close() returned error: %v", err)
	}
	afterSecond := registry.Snapshot().Generation
	if registry.Lifecycle() != LifecycleClosed {
		t.Fatalf("Lifecycle() = %s, want %s", registry.Lifecycle(), LifecycleClosed)
	}
	if afterFirst != before.Next() {
		t.Fatalf("generation after first Close = %s, want %s", afterFirst, before.Next())
	}
	if afterSecond != afterFirst {
		t.Fatalf("generation after second Close = %s, want %s", afterSecond, afterFirst)
	}
}

// TestLeaseRegistryCloseConcurrent verifies synchronous close idempotency.
func TestLeaseRegistryCloseConcurrent(t *testing.T) {
	t.Parallel()

	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	before := registry.Snapshot().Generation

	const callers = 32
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- registry.Close()
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Close() returned error: %v", err)
		}
	}
	if registry.Lifecycle() != LifecycleClosed {
		t.Fatalf("Lifecycle() = %s, want %s", registry.Lifecycle(), LifecycleClosed)
	}
	after := registry.Snapshot().Generation
	if after != before.Next() {
		t.Fatalf("generation after concurrent close = %s, want %s", after, before.Next())
	}
}

// TestLeaseRegistryGenerationAdvancesOnStateChanges verifies that snapshot
// generation moves when registry-visible ownership state changes.
func TestLeaseRegistryGenerationAdvancesOnStateChanges(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	registry := MustNewLeaseRegistry(DefaultLeaseConfig())

	initial := registry.Snapshot().Generation
	if initial != InitialGeneration {
		t.Fatalf("initial generation = %s, want %s", initial, InitialGeneration)
	}

	lease, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}
	afterAcquire := registry.Snapshot().Generation
	if !afterAcquire.After(initial) {
		t.Fatalf("generation after acquire = %s, want after %s", afterAcquire, initial)
	}

	if err := lease.Release(nil); !errors.Is(err, ErrNilBuffer) {
		t.Fatalf("Release(nil) error = %v, want ErrNilBuffer", err)
	}
	afterInvalid := registry.Snapshot().Generation
	if !afterInvalid.After(afterAcquire) {
		t.Fatalf("generation after invalid release = %s, want after %s", afterInvalid, afterAcquire)
	}

	if err := lease.ReleaseUnchanged(); err != nil {
		t.Fatalf("ReleaseUnchanged() returned error: %v", err)
	}
	afterRelease := registry.Snapshot().Generation
	if !afterRelease.After(afterInvalid) {
		t.Fatalf("generation after release = %s, want after %s", afterRelease, afterInvalid)
	}

	if err := registry.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
	afterClose := registry.Snapshot().Generation
	if !afterClose.After(afterRelease) {
		t.Fatalf("generation after close = %s, want after %s", afterClose, afterRelease)
	}
}

// TestLeaseRegistrySnapshotReportsActiveLeases verifies snapshot content for
// multiple active leases.
func TestLeaseRegistrySnapshotReportsActiveLeases(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	leaseA, err := registry.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("Acquire A returned error: %v", err)
	}
	leaseB, err := registry.Acquire(pool, 700)
	if err != nil {
		t.Fatalf("Acquire B returned error: %v", err)
	}

	snapshot := registry.Snapshot()
	if snapshot.ActiveCount() != 2 {
		t.Fatalf("ActiveCount() = %d, want 2", snapshot.ActiveCount())
	}
	if snapshot.Counters.Acquisitions != 2 {
		t.Fatalf("Acquisitions = %d, want 2", snapshot.Counters.Acquisitions)
	}
	if snapshot.Counters.ActiveLeases != 2 {
		t.Fatalf("ActiveLeases = %d, want 2", snapshot.Counters.ActiveLeases)
	}
	if snapshot.Counters.ActiveBytes != leaseA.AcquiredCapacity()+leaseB.AcquiredCapacity() {
		t.Fatalf("ActiveBytes = %d, want %d", snapshot.Counters.ActiveBytes, leaseA.AcquiredCapacity()+leaseB.AcquiredCapacity())
	}
	assertLeaseRegistrySnapshotActiveConsistency(t, snapshot)

	ids := map[LeaseID]bool{}
	for _, active := range snapshot.Active {
		if !active.IsActive() {
			t.Fatalf("active snapshot state = %s, want active", active.State)
		}
		ids[active.ID] = true
		if active.HoldDuration < 0 {
			t.Fatalf("HoldDuration = %s, want non-negative", active.HoldDuration)
		}
	}
	if !ids[leaseA.ID()] || !ids[leaseB.ID()] {
		t.Fatalf("active snapshots do not contain both lease ids: %#v", ids)
	}

	if err := leaseA.ReleaseUnchanged(); err != nil {
		t.Fatalf("Release A returned error: %v", err)
	}
	if err := leaseB.ReleaseUnchanged(); err != nil {
		t.Fatalf("Release B returned error: %v", err)
	}
}

// TestLeaseRegistrySnapshotCountersMatchActiveState verifies that active-map
// mutation, active gauges, and generation are published at one registry boundary
// for ordinary acquire/release transitions.
func TestLeaseRegistrySnapshotCountersMatchActiveState(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	initial := registry.Snapshot().Generation
	lease, err := registry.Acquire(pool, 700)
	if err != nil {
		t.Fatalf("Acquire() returned error: %v", err)
	}

	afterAcquire := registry.Snapshot()
	if !afterAcquire.Generation.After(initial) {
		t.Fatalf("generation after acquire = %s, want after %s", afterAcquire.Generation, initial)
	}
	assertLeaseRegistrySnapshotActiveConsistency(t, afterAcquire)

	afterAcquireGeneration := afterAcquire.Generation
	if err := lease.ReleaseUnchanged(); err != nil {
		t.Fatalf("ReleaseUnchanged() returned error: %v", err)
	}

	afterRelease := registry.Snapshot()
	if !afterRelease.Generation.After(afterAcquireGeneration) {
		t.Fatalf("generation after release = %s, want after %s", afterRelease.Generation, afterAcquireGeneration)
	}
	if afterRelease.ActiveCount() != 0 {
		t.Fatalf("ActiveCount() = %d, want 0", afterRelease.ActiveCount())
	}
	if afterRelease.Counters.ActiveLeases != 0 {
		t.Fatalf("ActiveLeases = %d, want 0", afterRelease.Counters.ActiveLeases)
	}
	if afterRelease.Counters.ActiveBytes != 0 {
		t.Fatalf("ActiveBytes = %d, want 0", afterRelease.Counters.ActiveBytes)
	}
}

// TestLeaseRegistrySnapshotConcurrent exercises snapshotting while ownership
// operations are mutating the active map and per-record state.
func TestLeaseRegistrySnapshotConcurrent(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	const workers = 4
	const iterations = 64

	var workersWG sync.WaitGroup
	var snapshotWG sync.WaitGroup
	stop := make(chan struct{})

	snapshotWG.Add(1)
	go func() {
		defer snapshotWG.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = registry.Snapshot()
			}
		}
	}()

	for worker := 0; worker < workers; worker++ {
		workersWG.Add(1)
		go func(worker int) {
			defer workersWG.Done()
			for i := 0; i < iterations; i++ {
				lease, err := registry.Acquire(pool, 128+(worker%2)*64)
				if err != nil {
					t.Errorf("Acquire() returned error: %v", err)
					return
				}
				if err := lease.ReleaseUnchanged(); err != nil {
					t.Errorf("ReleaseUnchanged() returned error: %v", err)
					return
				}
			}
		}(worker)
	}
	workersWG.Wait()
	close(stop)
	snapshotWG.Wait()

	snapshot := registry.Snapshot()
	if snapshot.ActiveCount() != 0 {
		t.Fatalf("ActiveCount() = %d, want 0", snapshot.ActiveCount())
	}
	if snapshot.Counters.Acquisitions != uint64(workers*iterations) {
		t.Fatalf("Acquisitions = %d, want %d", snapshot.Counters.Acquisitions, workers*iterations)
	}
}

// TestLeaseRegistryConcurrentAcquireRelease exercises registry locking, record
// locking, counters, and Pool integration under concurrent ownership traffic.
func TestLeaseRegistryConcurrentAcquireRelease(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	registry := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registry)

	const goroutines = 8
	const iterations = 128

	var wg sync.WaitGroup
	for worker := 0; worker < goroutines; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				lease, err := registry.Acquire(pool, 128+(worker%4)*32)
				if err != nil {
					t.Errorf("Acquire() returned error: %v", err)
					return
				}
				buffer := lease.Buffer()
				if len(buffer) == 0 {
					t.Error("Acquire() returned empty buffer")
					return
				}
				buffer[0] = byte(worker)
				if err := lease.Release(buffer); err != nil {
					t.Errorf("Release() returned error: %v", err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()

	snapshot := registry.Snapshot()
	want := uint64(goroutines * iterations)
	if snapshot.ActiveCount() != 0 {
		t.Fatalf("ActiveCount() = %d, want 0", snapshot.ActiveCount())
	}
	if snapshot.Counters.Acquisitions != want {
		t.Fatalf("Acquisitions = %d, want %d", snapshot.Counters.Acquisitions, want)
	}
	if snapshot.Counters.Releases != want {
		t.Fatalf("Releases = %d, want %d", snapshot.Counters.Releases, want)
	}
	if snapshot.Counters.ActiveLeases != 0 {
		t.Fatalf("ActiveLeases = %d, want 0", snapshot.Counters.ActiveLeases)
	}
	if snapshot.Counters.ActiveBytes != 0 {
		t.Fatalf("ActiveBytes = %d, want 0", snapshot.Counters.ActiveBytes)
	}
}

// TestLeaseRegistryMultipleRegistriesSharePoolWithSeparateAccounting verifies
// that LeaseRegistry is an ownership scope, not a Pool-global registry.
func TestLeaseRegistryMultipleRegistriesSharePoolWithSeparateAccounting(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	registryA := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registryA)
	registryB := MustNewLeaseRegistry(DefaultLeaseConfig())
	defer closeLeaseRegistryForTest(t, registryB)

	leaseA, err := registryA.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("registryA Acquire() returned error: %v", err)
	}
	leaseB, err := registryB.Acquire(pool, 128)
	if err != nil {
		t.Fatalf("registryB Acquire() returned error: %v", err)
	}

	if registryA.Snapshot().ActiveCount() != 1 {
		t.Fatalf("registryA ActiveCount() = %d, want 1", registryA.Snapshot().ActiveCount())
	}
	if registryB.Snapshot().ActiveCount() != 1 {
		t.Fatalf("registryB ActiveCount() = %d, want 1", registryB.Snapshot().ActiveCount())
	}

	err = registryB.Release(leaseA, leaseA.Buffer())
	if !errors.Is(err, ErrInvalidLease) {
		t.Fatalf("registryB Release(leaseA) error = %v, want ErrInvalidLease", err)
	}
	if registryB.Snapshot().Counters.InvalidReleases != 1 {
		t.Fatalf("registryB InvalidReleases = %d, want 1", registryB.Snapshot().Counters.InvalidReleases)
	}
	if registryA.Snapshot().ActiveCount() != 1 {
		t.Fatalf("registryA ActiveCount() after wrong-registry release = %d, want 1", registryA.Snapshot().ActiveCount())
	}

	if err := leaseA.ReleaseUnchanged(); err != nil {
		t.Fatalf("registryA ReleaseUnchanged() returned error: %v", err)
	}
	if err := leaseB.ReleaseUnchanged(); err != nil {
		t.Fatalf("registryB ReleaseUnchanged() returned error: %v", err)
	}
}

// TestLeaseRegistryNilReceiverPanics verifies fail-fast receiver validation.
func TestLeaseRegistryNilReceiverPanics(t *testing.T) {
	t.Parallel()

	assertPoolPanic(t, errNilLeaseRegistry, func() {
		var registry *LeaseRegistry
		_ = registry.Lifecycle()
	})
}

func closeLeaseRegistryForTest(t *testing.T, registry *LeaseRegistry) {
	t.Helper()
	if err := registry.Close(); err != nil {
		t.Fatalf("LeaseRegistry.Close() returned error: %v", err)
	}
}

func assertLeaseRegistrySnapshotActiveConsistency(t *testing.T, snapshot LeaseRegistrySnapshot) {
	t.Helper()

	if uint64(snapshot.ActiveCount()) != snapshot.Counters.ActiveLeases {
		t.Fatalf("ActiveCount()/ActiveLeases = %d/%d", snapshot.ActiveCount(), snapshot.Counters.ActiveLeases)
	}

	var activeBytes uint64
	for _, lease := range snapshot.Active {
		activeBytes += lease.AcquiredCapacity
	}
	if activeBytes != snapshot.Counters.ActiveBytes {
		t.Fatalf("sum active capacity/ActiveBytes = %d/%d", activeBytes, snapshot.Counters.ActiveBytes)
	}
}
