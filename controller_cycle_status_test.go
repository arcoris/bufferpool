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

func TestControllerCycleStatusCountsConsecutiveUnpublished(t *testing.T) {
	var store controllerCycleStatusStore

	first := store.publish(ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)
	second := store.publish(ControllerCycleStatusUnpublished, Generation(2), NoGeneration, controllerCycleReasonUnpublished)

	if first.ConsecutiveUnpublished != 1 || second.ConsecutiveUnpublished != 2 {
		t.Fatalf("unpublished counters = %d, %d; want 1, 2", first.ConsecutiveUnpublished, second.ConsecutiveUnpublished)
	}
	if second.ConsecutiveFailures != 0 || second.ConsecutiveSkipped != 0 {
		t.Fatalf("unpublished status counters = %+v, want failure/skip counters reset", second)
	}
}

func TestControllerCycleStatusAppliedResetsUnpublished(t *testing.T) {
	var store controllerCycleStatusStore
	store.publish(ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)

	status := store.publish(ControllerCycleStatusApplied, Generation(2), Generation(2), "")
	if status.ConsecutiveUnpublished != 0 || status.FailureReason != "" {
		t.Fatalf("applied status = %+v, want unpublished counter and reason reset", status)
	}
}

func TestControllerCycleStatusSkippedResetsUnpublished(t *testing.T) {
	var store controllerCycleStatusStore
	store.publish(ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)

	status := store.publish(ControllerCycleStatusSkipped, Generation(2), NoGeneration, controllerCycleReasonNoWork)
	if status.ConsecutiveUnpublished != 0 || status.ConsecutiveSkipped != 1 {
		t.Fatalf("skipped status = %+v, want unpublished reset and skipped incremented", status)
	}
}

func TestControllerCycleStatusFailedResetsUnpublished(t *testing.T) {
	var store controllerCycleStatusStore
	store.publish(ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)

	status := store.publish(ControllerCycleStatusFailed, Generation(2), NoGeneration, controllerCycleReasonFailed)
	if status.ConsecutiveUnpublished != 0 || status.ConsecutiveFailures != 1 {
		t.Fatalf("failed status = %+v, want unpublished reset and failure incremented", status)
	}
}

func TestControllerCycleStatusClosedDoesNotChangeCounters(t *testing.T) {
	var store controllerCycleStatusStore
	store.publish(ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)

	status := store.publish(ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
	if status.ConsecutiveUnpublished != 1 || status.ConsecutiveFailures != 0 || status.ConsecutiveSkipped != 0 {
		t.Fatalf("closed status = %+v, want computation counters preserved", status)
	}
}

func TestControllerCycleStatusAlreadyRunningDoesNotChangeCounters(t *testing.T) {
	var store controllerCycleStatusStore
	store.publish(ControllerCycleStatusFailed, Generation(1), NoGeneration, controllerCycleReasonFailed)

	status := store.publish(ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)
	if status.ConsecutiveFailures != 1 || status.ConsecutiveSkipped != 0 || status.ConsecutiveUnpublished != 0 {
		t.Fatalf("already-running status = %+v, want computation counters preserved", status)
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

func TestPoolPartitionTickBudgetPublicationErrorStatusFailed(t *testing.T) {
	partition := testPartitionControllerScorePartition(t, "primary")
	pool, ok := partition.registry.pool("primary")
	if !ok {
		t.Fatal("primary pool not found")
	}
	requirePartitionNoError(t, pool.Close())

	var report PartitionControllerReport
	requirePartitionErrorIs(t, partition.TickInto(&report), ErrClosed)

	assertControllerCycleStatus(t, report.Status, ControllerCycleStatusFailed, report.Generation, NoGeneration, controllerCycleReasonClosed)
}

func TestPoolPartitionTickBudgetPublicationErrorReturnsError(t *testing.T) {
	_, _, err := runPartitionTickBudgetPublicationError(t)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("TickInto error = %v, want ErrClosed", err)
	}
}

func TestPoolPartitionTickBudgetPublicationErrorDoesNotCommitControllerState(t *testing.T) {
	partition, report, err := runPartitionTickBudgetPublicationError(t)
	requirePartitionErrorIs(t, err, ErrClosed)

	if partition.controller.hasPreviousSample {
		t.Fatal("controller has previous sample after failed budget publication")
	}
	if got := partition.controller.generation.Load(); got != InitialGeneration {
		t.Fatalf("controller generation = %s, want unchanged initial generation %s", got, InitialGeneration)
	}
	if report.Status.LastSuccessfulGeneration != NoGeneration {
		t.Fatalf("LastSuccessfulGeneration = %s, want NoGeneration", report.Status.LastSuccessfulGeneration)
	}
}

func TestPoolPartitionTickBudgetPublicationErrorDoesNotExecuteTrim(t *testing.T) {
	_, report, err := runPartitionTickBudgetPublicationError(t)
	requirePartitionErrorIs(t, err, ErrClosed)

	if report.TrimResult.Attempted {
		t.Fatalf("TrimResult = %+v, want no trim after budget publication error", report.TrimResult)
	}
}

func TestPoolPartitionTickBudgetPublicationErrorUsesStableReason(t *testing.T) {
	_, report, err := runPartitionTickBudgetPublicationError(t)
	requirePartitionErrorIs(t, err, ErrClosed)

	if report.BudgetPublication.FailureReason != controllerCycleReasonClosed {
		t.Fatalf("BudgetPublication.FailureReason = %q, want %q", report.BudgetPublication.FailureReason, controllerCycleReasonClosed)
	}
	if report.BudgetPublication.FailureReason == err.Error() {
		t.Fatalf("BudgetPublication.FailureReason exposed raw error text: %q", report.BudgetPublication.FailureReason)
	}
}

func TestPoolPartitionTickBudgetPublicationErrorReportKeepsDiagnostics(t *testing.T) {
	_, report, err := runPartitionTickBudgetPublicationError(t)
	requirePartitionErrorIs(t, err, ErrClosed)

	if report.Generation.IsZero() || report.PolicyGeneration.IsZero() {
		t.Fatalf("report generations = generation %s policy %s, want diagnostic generations", report.Generation, report.PolicyGeneration)
	}
	if len(report.Sample.Pools) == 0 || len(report.Window.Pools) == 0 {
		t.Fatalf("report sample/window missing diagnostics: sample=%+v window=%+v", report.Sample, report.Window)
	}
	if len(report.PoolBudgetTargets) == 0 || len(report.PoolScores) == 0 || len(report.ClassScores) == 0 {
		t.Fatalf("report budget diagnostics missing: targets=%d poolScores=%d classScores=%d",
			len(report.PoolBudgetTargets), len(report.PoolScores), len(report.ClassScores))
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

func TestPoolPartitionTickIntoNilDoesNotChangeControllerStatus(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	beforeStatus := partition.ControllerStatus()
	beforeGeneration := partition.generation.Load()

	requirePartitionNoError(t, partition.TickInto(nil))

	if status := partition.ControllerStatus(); !reflect.DeepEqual(status, beforeStatus) {
		t.Fatalf("ControllerStatus after nil TickInto = %+v, want %+v", status, beforeStatus)
	}
	if generation := partition.generation.Load(); generation != beforeGeneration {
		t.Fatalf("partition generation after nil TickInto = %s, want %s", generation, beforeGeneration)
	}
}

func TestPoolPartitionTickIntoNilDoesNotEnterCycleGate(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	requirePartitionNoError(t, partition.TickInto(nil))
	if partition.controller.cycleGate.isRunning() {
		t.Fatal("nil TickInto left partition cycle gate running")
	}
}

func TestPoolPartitionTickIntoNilClosedStatusSemantics(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Close())

	requirePartitionErrorIs(t, partition.TickInto(nil), ErrClosed)
	assertControllerCycleStatus(t, partition.ControllerStatus(), ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
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

func TestPoolGroupTickNoBudgetTargetsStatusSkipped(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))

	assertControllerCycleStatus(t, report.Status, ControllerCycleStatusSkipped, report.Generation, NoGeneration, controllerCycleReasonNoWork)
	if len(report.PartitionBudgetTargets) != 0 {
		t.Fatalf("PartitionBudgetTargets = %+v, want no group-level budget targets", report.PartitionBudgetTargets)
	}
}

func TestPoolPartitionTickNoBudgetTargetsCanStatusAppliedWhenControllerStateCommitted(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	requirePartitionNoError(t, partition.activeRegistry.markInactiveIndex(0))
	partition.activeRegistry.resetDirty()
	partition.controller.cycles = 1

	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	assertControllerCycleStatus(t, report.Status, ControllerCycleStatusApplied, report.Generation, report.Generation, "")
	if len(report.PoolBudgetTargets) != 0 {
		t.Fatalf("PoolBudgetTargets = %+v, want no partition-to-Pool budget targets", report.PoolBudgetTargets)
	}
	if !partition.controller.hasPreviousSample {
		t.Fatal("partition controller did not commit observation state")
	}
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

func TestPoolGroupTickIntoNilDoesNotChangeControllerStatus(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	beforeStatus := group.ControllerStatus()
	beforeGeneration := group.generation.Load()

	requireGroupNoError(t, group.TickInto(nil))

	if status := group.ControllerStatus(); !reflect.DeepEqual(status, beforeStatus) {
		t.Fatalf("ControllerStatus after nil TickInto = %+v, want %+v", status, beforeStatus)
	}
	if generation := group.generation.Load(); generation != beforeGeneration {
		t.Fatalf("group generation after nil TickInto = %s, want %s", generation, beforeGeneration)
	}
}

func TestPoolGroupTickIntoNilDoesNotEnterCycleGate(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	requireGroupNoError(t, group.TickInto(nil))
	if group.coordinator.cycleGate.isRunning() {
		t.Fatal("nil TickInto left group cycle gate running")
	}
}

func TestPoolGroupTickIntoNilClosedStatusSemantics(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha"))
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.Close())

	requireGroupErrorIs(t, group.TickInto(nil), ErrClosed)
	assertControllerCycleStatus(t, group.ControllerStatus(), ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
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

func TestControllerCycleStatusSuccessClearsFailureReason(t *testing.T) {
	var store controllerCycleStatusStore
	store.publish(ControllerCycleStatusFailed, Generation(1), NoGeneration, controllerCycleReasonFailed)

	status := store.publish(ControllerCycleStatusApplied, Generation(2), Generation(2), "")
	if status.FailureReason != "" {
		t.Fatalf("applied status reason = %q, want empty", status.FailureReason)
	}
}

func TestControllerCycleStatusClosedReasonStable(t *testing.T) {
	var store controllerCycleStatusStore

	status := store.publish(ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
	assertControllerCycleStatus(t, status, ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
}

func TestControllerCycleStatusAlreadyRunningReasonStable(t *testing.T) {
	var store controllerCycleStatusStore

	status := store.publish(ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)
	assertControllerCycleStatus(t, status, ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)
}

func TestControllerCycleStatusNoWorkReasonStable(t *testing.T) {
	var store controllerCycleStatusStore

	status := store.publish(ControllerCycleStatusSkipped, Generation(1), NoGeneration, controllerCycleReasonNoWork)
	assertControllerCycleStatus(t, status, ControllerCycleStatusSkipped, Generation(1), NoGeneration, controllerCycleReasonNoWork)
}

func TestControllerCycleStatusUnpublishedReasonStable(t *testing.T) {
	var store controllerCycleStatusStore

	status := store.publish(ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)
	assertControllerCycleStatus(t, status, ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)
}

func TestControllerCycleFailureReasonMapping(t *testing.T) {
	if got := controllerCycleFailureReasonForError(newError(ErrClosed, errPoolClosed), "fallback"); got != controllerCycleReasonClosed {
		t.Fatalf("closed reason = %q, want %q", got, controllerCycleReasonClosed)
	}
	if got := controllerCycleFailureReasonForError(newError(ErrInvalidPolicy, "invalid"), "fallback"); got != policyUpdateFailureInvalid {
		t.Fatalf("invalid policy reason = %q, want %q", got, policyUpdateFailureInvalid)
	}
	if got := controllerCycleFailureReasonForError(errors.New("raw detailed text"), "fallback"); got != "fallback" {
		t.Fatalf("unknown reason = %q, want fallback", got)
	}
}

func TestControllerCycleBudgetPublicationFailureReasonDoesNotExposeRawError(t *testing.T) {
	err := newError(ErrClosed, errPoolClosed)

	reason := controllerCycleBudgetPublicationFailureReasonForError(err, controllerCycleReasonFailed)
	if reason != controllerCycleReasonClosed {
		t.Fatalf("budget publication reason = %q, want %q", reason, controllerCycleReasonClosed)
	}
	if reason == err.Error() {
		t.Fatalf("budget publication reason exposed raw error text: %q", reason)
	}
}

func TestControllerCycleBudgetPublicationFailureReasonPreservesReturnedError(t *testing.T) {
	_, _, err := runPartitionTickBudgetPublicationError(t)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("returned error = %v, want errors.Is ErrClosed", err)
	}
}

func TestPoolPartitionBudgetPublicationFailureReasonStable(t *testing.T) {
	_, report, err := runPartitionTickBudgetPublicationError(t)
	requirePartitionErrorIs(t, err, ErrClosed)

	if report.BudgetPublication.FailureReason != controllerCycleReasonClosed {
		t.Fatalf("FailureReason = %q, want %q", report.BudgetPublication.FailureReason, controllerCycleReasonClosed)
	}
}

func TestPoolGroupBudgetPublicationFailureReasonStableIfErrorPathExists(t *testing.T) {
	group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")
	beta, ok := group.partition("beta")
	if !ok {
		t.Fatal("partition beta not found")
	}
	requirePartitionNoError(t, beta.Close())

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	if report.BudgetPublication.FailureReason != policyUpdateFailureSkippedChild {
		t.Fatalf("group budget FailureReason = %q, want %q", report.BudgetPublication.FailureReason, policyUpdateFailureSkippedChild)
	}
	if len(report.SkippedPartitions) != 1 || report.SkippedPartitions[0].Reason != policyUpdateFailureClosed {
		t.Fatalf("SkippedPartitions = %+v, want stable closed child reason", report.SkippedPartitions)
	}
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

func runPartitionTickBudgetPublicationError(t *testing.T) (*PoolPartition, PartitionControllerReport, error) {
	t.Helper()

	partition := testPartitionControllerScorePartition(t, "primary")
	pool, ok := partition.registry.pool("primary")
	if !ok {
		t.Fatal("primary pool not found")
	}
	requirePartitionNoError(t, pool.Close())

	var report PartitionControllerReport
	err := partition.TickInto(&report)
	return partition, report, err
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
