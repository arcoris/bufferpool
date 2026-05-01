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
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestPoolPartitionControllerStatusInitial(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	status := partition.ControllerStatus()
	if status.Status != ControllerCycleStatusUnset {
		t.Fatalf("ControllerStatus = %+v, want unset", status)
	}
}

func TestPoolPartitionTickUpdatesControllerStatusOnAppliedCycle(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	assertControllerCycleStatus(t, report.Status, ControllerCycleStatusApplied, report.Generation, report.Generation, "")
	if report.Status.LastSuccessfulGeneration != report.Generation {
		t.Fatalf("LastSuccessfulGeneration = %s, want %s", report.Status.LastSuccessfulGeneration, report.Generation)
	}
	if status := partition.ControllerStatus(); !reflect.DeepEqual(status, report.Status) {
		t.Fatalf("ControllerStatus = %+v, want report status %+v", status, report.Status)
	}
}

func TestPoolPartitionTickUpdatesControllerStatusOnUnpublishedCycle(t *testing.T) {
	partition := testPartitionControllerScorePartition(t, "primary")
	pool, ok := partition.registry.pool("primary")
	if !ok {
		t.Fatal("primary pool not found")
	}
	requirePartitionNoError(t, pool.Close())

	beforeGeneration := partition.controller.generation.Load()
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	assertControllerCycleStatus(t, report.Status, ControllerCycleStatusUnpublished, report.Generation, NoGeneration, controllerCycleReasonClosed)
	if afterGeneration := partition.controller.generation.Load(); afterGeneration != beforeGeneration {
		t.Fatalf("controller generation = %s, want unchanged %s", afterGeneration, beforeGeneration)
	}
}

func TestPoolPartitionTickUpdatesControllerStatusOnClosedPartition(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Close())

	var report PartitionControllerReport
	requirePartitionErrorIs(t, partition.TickInto(&report), ErrClosed)

	assertControllerCycleStatus(t, report.Status, ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
	if status := partition.ControllerStatus(); !reflect.DeepEqual(status, report.Status) {
		t.Fatalf("ControllerStatus after close = %+v, want report status %+v", status, report.Status)
	}
}

func TestPoolPartitionTickRejectsOverlap(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	partition.controller.mu.Lock()
	defer partition.controller.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		var report PartitionControllerReport
		done <- partition.TickInto(&report)
	}()
	waitForControllerCycleGate(t, "partition", partition.controller.cycleGate.isRunning)

	var overlap PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&overlap))
	assertControllerCycleStatus(t, overlap.Status, ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)

	partition.controller.mu.Unlock()
	requirePartitionNoError(t, <-done)
	partition.controller.mu.Lock()
}

func TestPoolPartitionTickOverlapDoesNotCommitState(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	partition.controller.mu.Lock()
	defer partition.controller.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		var report PartitionControllerReport
		done <- partition.TickInto(&report)
	}()
	waitForControllerCycleGate(t, "partition", partition.controller.cycleGate.isRunning)

	beforeGeneration := partition.generation.Load()
	var overlap PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&overlap))
	if !overlap.Generation.IsZero() || len(overlap.PoolBudgetTargets) != 0 || overlap.TrimResult.Attempted {
		t.Fatalf("overlap report = %+v, want no attempted controller state", overlap)
	}
	if afterGeneration := partition.generation.Load(); afterGeneration != beforeGeneration {
		t.Fatalf("partition generation changed on overlap: got %s want %s", afterGeneration, beforeGeneration)
	}

	partition.controller.mu.Unlock()
	requirePartitionNoError(t, <-done)
	partition.controller.mu.Lock()
}

func TestPoolPartitionControllerStatusIsCopySafe(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	requirePartitionNoError(t, partition.TickInto(&PartitionControllerReport{}))

	status := partition.ControllerStatus()
	status.FailureReason = "mutated"
	if got := partition.ControllerStatus(); got.FailureReason == "mutated" {
		t.Fatalf("ControllerStatus returned mutable storage: %+v", got)
	}
}

func TestPoolPartitionControllerStatusSurvivesClose(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.TickInto(&PartitionControllerReport{}))
	requirePartitionNoError(t, partition.Close())

	status := partition.ControllerStatus()
	if status.Status != ControllerCycleStatusApplied {
		t.Fatalf("ControllerStatus after Close = %+v, want last applied status", status)
	}
}

func TestPoolPartitionTickConcurrentWithPublishPolicy(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	runPartitionControllerConcurrentPair(t, func() error {
		var report PartitionControllerReport
		return partition.TickInto(&report)
	}, func() error {
		_, err := partition.PublishPolicy(partition.Policy())
		return err
	})
}

func TestPoolPartitionTickConcurrentWithPublishPressure(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	runPartitionControllerConcurrentPair(t, func() error {
		var report PartitionControllerReport
		return partition.TickInto(&report)
	}, func() error {
		_, err := partition.PublishPressure(PressureLevelHigh)
		return err
	})
}

func TestPoolPartitionTickConcurrentWithClose(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	runControllerCloseRace(t, func() error {
		var report PartitionControllerReport
		return partition.TickInto(&report)
	}, partition.Close)
}

func TestPoolGroupControllerStatusInitial(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	status := group.ControllerStatus()
	if status.Status != ControllerCycleStatusUnset {
		t.Fatalf("ControllerStatus = %+v, want unset", status)
	}
}

func TestPoolGroupTickUpdatesControllerStatusOnAppliedCycle(t *testing.T) {
	group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))

	assertControllerCycleStatus(t, report.Status, ControllerCycleStatusApplied, report.Generation, report.Generation, "")
	if report.Status.LastSuccessfulGeneration != report.Generation {
		t.Fatalf("LastSuccessfulGeneration = %s, want %s", report.Status.LastSuccessfulGeneration, report.Generation)
	}
	if status := group.ControllerStatus(); !reflect.DeepEqual(status, report.Status) {
		t.Fatalf("ControllerStatus = %+v, want report status %+v", status, report.Status)
	}
}

func TestPoolGroupTickUpdatesControllerStatusOnUnpublishedOrSkippedCycle(t *testing.T) {
	t.Run("unpublished", func(t *testing.T) {
		group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")
		beta, ok := group.partition("beta")
		if !ok {
			t.Fatal("partition beta not found")
		}
		requirePartitionNoError(t, beta.Close())

		var report PoolGroupCoordinatorReport
		requireGroupNoError(t, group.TickInto(&report))
		assertControllerCycleStatus(t, report.Status, ControllerCycleStatusUnpublished, report.Generation, NoGeneration, controllerCycleReasonUnpublished)
	})

	t.Run("skipped", func(t *testing.T) {
		group := testNewPoolGroup(t, "alpha")

		var report PoolGroupCoordinatorReport
		requireGroupNoError(t, group.TickInto(&report))
		assertControllerCycleStatus(t, report.Status, ControllerCycleStatusSkipped, report.Generation, NoGeneration, controllerCycleReasonNoWork)
		if report.Status.ConsecutiveSkipped != 1 {
			t.Fatalf("ConsecutiveSkipped = %d, want 1", report.Status.ConsecutiveSkipped)
		}
	})
}

func TestPoolGroupTickUpdatesControllerStatusOnClosedGroup(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha"))
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.Close())

	var report PoolGroupCoordinatorReport
	requireGroupErrorIs(t, group.TickInto(&report), ErrClosed)

	assertControllerCycleStatus(t, report.Status, ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
	if status := group.ControllerStatus(); !reflect.DeepEqual(status, report.Status) {
		t.Fatalf("ControllerStatus after close = %+v, want report status %+v", status, report.Status)
	}
}

func TestPoolGroupTickRejectsOverlap(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	group.coordinator.mu.Lock()
	defer group.coordinator.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		var report PoolGroupCoordinatorReport
		done <- group.TickInto(&report)
	}()
	waitForControllerCycleGate(t, "group", group.coordinator.cycleGate.isRunning)

	var overlap PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&overlap))
	assertControllerCycleStatus(t, overlap.Status, ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)

	group.coordinator.mu.Unlock()
	requireGroupNoError(t, <-done)
	group.coordinator.mu.Lock()
}

func TestPoolGroupTickOverlapDoesNotCommitState(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	group.coordinator.mu.Lock()
	defer group.coordinator.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		var report PoolGroupCoordinatorReport
		done <- group.TickInto(&report)
	}()
	waitForControllerCycleGate(t, "group", group.coordinator.cycleGate.isRunning)

	beforeGeneration := group.generation.Load()
	var overlap PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&overlap))
	if !overlap.Generation.IsZero() || len(overlap.PartitionBudgetTargets) != 0 {
		t.Fatalf("overlap report = %+v, want no attempted coordinator state", overlap)
	}
	if afterGeneration := group.generation.Load(); afterGeneration != beforeGeneration {
		t.Fatalf("group generation changed on overlap: got %s want %s", afterGeneration, beforeGeneration)
	}

	group.coordinator.mu.Unlock()
	requireGroupNoError(t, <-done)
	group.coordinator.mu.Lock()
}

func TestPoolGroupControllerStatusIsCopySafe(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	requireGroupNoError(t, group.TickInto(&PoolGroupCoordinatorReport{}))

	status := group.ControllerStatus()
	status.FailureReason = "mutated"
	if got := group.ControllerStatus(); got.FailureReason == "mutated" {
		t.Fatalf("ControllerStatus returned mutable storage: %+v", got)
	}
}

func TestPoolGroupControllerStatusSurvivesClose(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha"))
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.TickInto(&PoolGroupCoordinatorReport{}))
	requireGroupNoError(t, group.Close())

	status := group.ControllerStatus()
	if status.Status != ControllerCycleStatusSkipped {
		t.Fatalf("ControllerStatus after Close = %+v, want last skipped status", status)
	}
}

func TestPoolGroupTickConcurrentWithPublishPolicy(t *testing.T) {
	group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")
	runGroupControllerConcurrentPair(t, func() error {
		var report PoolGroupCoordinatorReport
		return group.TickInto(&report)
	}, func() error {
		_, err := group.PublishPolicy(group.Policy())
		return err
	})
}

func TestPoolGroupTickConcurrentWithPublishPressure(t *testing.T) {
	group := testNewPoolGroup(t, "alpha", "beta")
	runGroupControllerConcurrentPair(t, func() error {
		var report PoolGroupCoordinatorReport
		return group.TickInto(&report)
	}, func() error {
		_, err := group.PublishPressure(PressureLevelHigh)
		return err
	})
}

func TestPoolGroupTickConcurrentWithClose(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha", "beta"))
	requireGroupNoError(t, err)
	runControllerCloseRace(t, func() error {
		var report PoolGroupCoordinatorReport
		return group.TickInto(&report)
	}, group.Close)
}

func assertControllerCycleStatus(
	t *testing.T,
	status ControllerCycleStatusSnapshot,
	wantStatus ControllerCycleStatus,
	wantAttempt Generation,
	wantApplied Generation,
	wantReason string,
) {
	t.Helper()
	if status.Status != wantStatus ||
		status.AttemptGeneration != wantAttempt ||
		status.AppliedGeneration != wantApplied ||
		status.FailureReason != wantReason {
		t.Fatalf("status = %+v, want status=%s attempt=%s applied=%s reason=%q",
			status, wantStatus, wantAttempt, wantApplied, wantReason)
	}
}

func waitForControllerCycleGate(t *testing.T, name string, running func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !running() {
		if time.Now().After(deadline) {
			t.Fatalf("%s controller cycle did not enter no-overlap gate", name)
		}
		runtime.Gosched()
	}
}

func runPartitionControllerConcurrentPair(t *testing.T, left func() error, right func() error) {
	t.Helper()
	for _, err := range runControllerConcurrentPair(left, right) {
		if err != nil {
			requirePartitionNoError(t, err)
		}
	}
}

func runGroupControllerConcurrentPair(t *testing.T, left func() error, right func() error) {
	t.Helper()
	for _, err := range runControllerConcurrentPair(left, right) {
		if err != nil {
			requireGroupNoError(t, err)
		}
	}
}

func runControllerCloseRace(t *testing.T, tick func() error, closeOwner func() error) {
	t.Helper()
	for _, err := range runControllerConcurrentPair(tick, closeOwner) {
		if err != nil && !errors.Is(err, ErrClosed) {
			t.Fatalf("unexpected close-race error: %v", err)
		}
	}
}

func runControllerConcurrentPair(left func() error, right func() error) []error {
	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errs[0] = left()
	}()
	go func() {
		defer wg.Done()
		<-start
		errs[1] = right()
	}()
	close(start)
	wg.Wait()
	return errs
}
