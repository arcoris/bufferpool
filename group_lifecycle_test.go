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

// TestPoolGroupClose verifies ordinary hard close and idempotent repeat close.
func TestPoolGroupClose(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha", "beta"))
	requireGroupNoError(t, err)

	requireGroupNoError(t, group.Close())
	if !group.IsClosed() {
		t.Fatalf("group should be closed")
	}
	requireGroupNoError(t, group.Close())
}

// TestPoolGroupCloseConcurrent verifies that only one caller owns child cleanup.
func TestPoolGroupCloseConcurrent(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha", "beta"))
	requireGroupNoError(t, err)

	const callers = 32
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- group.Close()
		}()
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		requireGroupNoError(t, err)
	}
	if !group.IsClosed() {
		t.Fatalf("group should be closed after concurrent Close")
	}
	for _, name := range group.PartitionNames() {
		snapshot, ok := group.PartitionSnapshot(name)
		if !ok {
			t.Fatalf("missing partition snapshot for %q", name)
		}
		if snapshot.Lifecycle != LifecycleClosed {
			t.Fatalf("partition %q lifecycle = %s, want closed", name, snapshot.Lifecycle)
		}
	}
}

// TestPoolGroupCloseIdempotentAfterConcurrentClose verifies post-race idempotency.
func TestPoolGroupCloseIdempotentAfterConcurrentClose(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha", "beta"))
	requireGroupNoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			requireGroupNoError(t, group.Close())
		}()
	}
	wg.Wait()

	requireGroupNoError(t, group.Close())
	if !group.IsClosed() {
		t.Fatalf("group should remain closed")
	}
}

// TestPoolGroupTickAfterCloseRejected locks foreground work rejection after close.
func TestPoolGroupTickAfterCloseRejected(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha"))
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.Close())

	_, err = group.Tick()
	requireGroupErrorIs(t, err, ErrClosed)
}

// TestPoolGroupDiagnosticsAfterClose verifies post-close diagnostic availability.
func TestPoolGroupDiagnosticsAfterClose(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha"))
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.Close())

	sample := group.Sample()
	if sample.Lifecycle != LifecycleClosed {
		t.Fatalf("Sample lifecycle = %s, want closed", sample.Lifecycle)
	}
	metrics := group.Metrics()
	if metrics.Lifecycle != LifecycleClosed {
		t.Fatalf("Metrics lifecycle = %s, want closed", metrics.Lifecycle)
	}
	snapshot := group.Snapshot()
	if snapshot.Lifecycle != LifecycleClosed {
		t.Fatalf("Snapshot lifecycle = %s, want closed", snapshot.Lifecycle)
	}
	if _, ok := group.PartitionSnapshot("alpha"); !ok {
		t.Fatalf("PartitionSnapshot after close not found")
	}
	if _, ok := group.PartitionMetrics("alpha"); !ok {
		t.Fatalf("PartitionMetrics after close not found")
	}
}

// TestPoolGroupTickIntoConcurrentWithClose exercises observation racing shutdown.
func TestPoolGroupTickIntoConcurrentWithClose(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha", "beta"))
	requireGroupNoError(t, err)

	start := make(chan struct{})
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		var report PoolGroupCoordinatorReport
		for i := 0; i < 64; i++ {
			err := group.TickInto(&report)
			if err != nil && !errors.Is(err, ErrClosed) {
				errs <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		if closeErr := group.Close(); closeErr != nil {
			errs <- closeErr
		}
	}()

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		requireGroupNoError(t, err)
	}
	if !group.IsClosed() {
		t.Fatalf("group should be closed")
	}
}
