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
	"strings"
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

// Store-level status tests bypass owner TickInto deliberately. They isolate the
// lightweight retained state machine from partition/group runtime side effects.
func TestControllerCycleStatusStoreCountsConsecutiveUnpublished(t *testing.T) {
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

func TestControllerCycleStatusStoreAppliedResetsUnpublished(t *testing.T) {
	var store controllerCycleStatusStore
	store.publish(ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)

	status := store.publish(ControllerCycleStatusApplied, Generation(2), Generation(2), "")
	if status.ConsecutiveUnpublished != 0 || status.FailureReason != "" {
		t.Fatalf("applied status = %+v, want unpublished counter and reason reset", status)
	}
}

func TestControllerCycleStatusStoreSkippedResetsUnpublished(t *testing.T) {
	var store controllerCycleStatusStore
	store.publish(ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)

	status := store.publish(ControllerCycleStatusSkipped, Generation(2), NoGeneration, controllerCycleReasonNoWork)
	if status.ConsecutiveUnpublished != 0 || status.ConsecutiveSkipped != 1 {
		t.Fatalf("skipped status = %+v, want unpublished reset and skipped incremented", status)
	}
}

func TestControllerCycleStatusStoreFailedResetsUnpublished(t *testing.T) {
	var store controllerCycleStatusStore
	store.publish(ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)

	status := store.publish(ControllerCycleStatusFailed, Generation(2), NoGeneration, controllerCycleReasonFailed)
	if status.ConsecutiveUnpublished != 0 || status.ConsecutiveFailures != 1 {
		t.Fatalf("failed status = %+v, want unpublished reset and failure incremented", status)
	}
}

func TestControllerCycleStatusStoreClosedDoesNotChangeCounters(t *testing.T) {
	var store controllerCycleStatusStore
	store.publish(ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)

	status := store.publish(ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
	if status.ConsecutiveUnpublished != 1 || status.ConsecutiveFailures != 0 || status.ConsecutiveSkipped != 0 {
		t.Fatalf("closed status = %+v, want computation counters preserved", status)
	}
}

func TestControllerCycleStatusStoreAlreadyRunningDoesNotChangeCounters(t *testing.T) {
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
		assertControllerCycleStatus(t, report.Status, ControllerCycleStatusUnpublished, report.Generation, NoGeneration, policyUpdateFailureSkippedChild)
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

func TestPoolGroupUnpublishedStatusUsesSpecificBudgetReason(t *testing.T) {
	group, report := runGroupTickUnpublishedByClosedChild(t)

	assertControllerCycleStatus(t, report.Status, ControllerCycleStatusUnpublished, report.Generation, NoGeneration, policyUpdateFailureSkippedChild)
	assertControllerStatusMatchesReport(t, group.ControllerStatus(), report.Status)
}

func TestPoolGroupUnpublishedStatusFallsBackToGenericReason(t *testing.T) {
	if got := controllerCycleUnpublishedFailureReason(""); got != controllerCycleReasonUnpublished {
		t.Fatalf("unpublished reason fallback = %q, want %q", got, controllerCycleReasonUnpublished)
	}
}

func TestControllerCycleStatusStoreUnpublishedUsesSpecificBudgetReason(t *testing.T) {
	var store controllerCycleStatusStore

	// This is retained-status policy coverage rather than a partition TickInto
	// path: partition unpublished runtime coverage requires a non-error
	// publication refusal, while budget publication errors are Failed.
	status := store.publish(
		ControllerCycleStatusUnpublished,
		Generation(3),
		NoGeneration,
		controllerCycleUnpublishedFailureReason(budgetAllocationReasonMinimumsExceedParent),
	)
	assertControllerCycleStatus(t, status, ControllerCycleStatusUnpublished, Generation(3), NoGeneration, budgetAllocationReasonMinimumsExceedParent)
}

func TestControllerCycleStatusStoreFailureReasonPolicy(t *testing.T) {
	var store controllerCycleStatusStore

	applied := store.publish(ControllerCycleStatusApplied, Generation(1), Generation(1), "ignored")
	closed := store.publish(ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
	overlap := store.publish(ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)
	skipped := store.publish(ControllerCycleStatusSkipped, Generation(2), NoGeneration, controllerCycleReasonNoWork)
	unpublished := store.publish(ControllerCycleStatusUnpublished, Generation(3), NoGeneration, policyUpdateFailureSkippedChild)
	failed := store.publish(ControllerCycleStatusFailed, Generation(4), NoGeneration, controllerCycleFailureReasonForError(newError(ErrClosed, errPoolClosed), controllerCycleReasonFailed))

	if applied.FailureReason != "" ||
		closed.FailureReason != controllerCycleReasonClosed ||
		overlap.FailureReason != controllerCycleReasonAlreadyRunning ||
		skipped.FailureReason != controllerCycleReasonNoWork ||
		unpublished.FailureReason != policyUpdateFailureSkippedChild ||
		failed.FailureReason != controllerCycleReasonClosed {
		t.Fatalf("unexpected failure-reason policy states: applied=%+v closed=%+v overlap=%+v skipped=%+v unpublished=%+v failed=%+v",
			applied, closed, overlap, skipped, unpublished, failed)
	}
}

func TestControllerStatusDoesNotExposeRawErrorText(t *testing.T) {
	assertControllerStatusDoesNotExposeRawErrorText(t)
}

func TestControllerCycleReasonsAreStable(t *testing.T) {
	reasons := []string{
		controllerCycleReasonClosed,
		controllerCycleReasonAlreadyRunning,
		controllerCycleReasonFailed,
		controllerCycleReasonUnpublished,
		controllerCycleReasonSkipped,
		controllerCycleReasonNoWork,
	}
	for _, reason := range reasons {
		if !strings.HasPrefix(reason, "controller_cycle_") {
			t.Fatalf("controller cycle reason = %q, want stable controller_cycle_ prefix", reason)
		}
	}
}

func TestControllerStatusDoesNotUseRawErrorText(t *testing.T) {
	assertControllerStatusDoesNotExposeRawErrorText(t)
}

func TestBudgetPublicationReasonsDoNotUseRawErrorTextOnKnownErrorPaths(t *testing.T) {
	_, partitionReport, partitionErr := runPartitionTickBudgetPublicationError(t)
	requirePartitionErrorIs(t, partitionErr, ErrClosed)
	if partitionReport.BudgetPublication.FailureReason == partitionErr.Error() {
		t.Fatalf("partition budget reason exposed raw error: %q", partitionReport.BudgetPublication.FailureReason)
	}

	_, groupReport := runGroupTickUnpublishedByClosedChild(t)
	if groupReport.BudgetPublication.FailureReason == errGroupClosed {
		t.Fatalf("group budget reason exposed raw group error: %q", groupReport.BudgetPublication.FailureReason)
	}
}

func TestGroupSkippedPartitionReasonStable(t *testing.T) {
	_, report := runGroupTickUnpublishedByClosedChild(t)
	if len(report.SkippedPartitions) != 1 || report.SkippedPartitions[0].Reason != policyUpdateFailureClosed {
		t.Fatalf("SkippedPartitions = %+v, want one stable closed reason", report.SkippedPartitions)
	}
}

func TestPartitionPublicationReasonStable(t *testing.T) {
	_, report, err := runPartitionTickBudgetPublicationError(t)
	requirePartitionErrorIs(t, err, ErrClosed)
	if report.BudgetPublication.FailureReason != controllerCycleReasonClosed {
		t.Fatalf("partition FailureReason = %q, want %q", report.BudgetPublication.FailureReason, controllerCycleReasonClosed)
	}
}

func TestPoolPartitionTickReportStatusMatchesRetainedStatusApplied(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	assertControllerStatusMatchesReport(t, partition.ControllerStatus(), report.Status)
}

func TestPoolPartitionTickReportStatusMatchesRetainedStatusFailed(t *testing.T) {
	partition, report, err := runPartitionTickBudgetPublicationError(t)
	requirePartitionErrorIs(t, err, ErrClosed)

	assertControllerStatusMatchesReport(t, partition.ControllerStatus(), report.Status)
}

func TestControllerCycleStatusStoreReportStatusMatchesRetainedStatusUnpublished(t *testing.T) {
	var store controllerCycleStatusStore

	// Partition currently reaches non-publication through real failed or applied
	// TickInto paths; this keeps report/status equality coverage for the
	// retained unpublished status without adding production-only failure hooks.
	status := store.publish(ControllerCycleStatusUnpublished, Generation(7), NoGeneration, budgetAllocationReasonMinimumsExceedParent)
	report := PartitionControllerReport{Status: status}

	assertControllerStatusMatchesReport(t, store.load(), report.Status)
}

func TestPoolPartitionTickReportStatusMatchesRetainedStatusAlreadyRunning(t *testing.T) {
	partition, report := runPartitionTickOverlapReport(t)

	assertControllerStatusMatchesReport(t, partition.ControllerStatus(), report.Status)
}

func TestPoolGroupTickReportStatusMatchesRetainedStatusSkipped(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))

	assertControllerStatusMatchesReport(t, group.ControllerStatus(), report.Status)
}

func TestPoolGroupTickReportStatusMatchesRetainedStatusApplied(t *testing.T) {
	group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))

	assertControllerStatusMatchesReport(t, group.ControllerStatus(), report.Status)
}

func TestPoolGroupTickReportStatusMatchesRetainedStatusUnpublished(t *testing.T) {
	group, report := runGroupTickUnpublishedByClosedChild(t)

	assertControllerStatusMatchesReport(t, group.ControllerStatus(), report.Status)
}

func TestPoolGroupTickReportStatusMatchesRetainedStatusAlreadyRunning(t *testing.T) {
	group, report := runGroupTickOverlapReport(t)

	assertControllerStatusMatchesReport(t, group.ControllerStatus(), report.Status)
}

func TestControllerCycleStatusStoreSequenceAppliedFailedApplied(t *testing.T) {
	var store controllerCycleStatusStore

	// This is a status-store unit sequence, not a runtime TickInto sequence. It
	// keeps the counter and LastSuccessfulGeneration contract focused on the
	// retained lightweight state machine.
	first := store.publish(ControllerCycleStatusApplied, Generation(1), Generation(1), "")
	failed := store.publish(ControllerCycleStatusFailed, Generation(2), NoGeneration, controllerCycleReasonFailed)
	applied := store.publish(ControllerCycleStatusApplied, Generation(3), Generation(3), "")

	assertControllerCycleStatus(t, first, ControllerCycleStatusApplied, Generation(1), Generation(1), "")
	assertControllerCycleStatus(t, failed, ControllerCycleStatusFailed, Generation(2), NoGeneration, controllerCycleReasonFailed)
	assertControllerCycleStatus(t, applied, ControllerCycleStatusApplied, Generation(3), Generation(3), "")
	if applied.LastSuccessfulGeneration != Generation(3) || applied.ConsecutiveFailures != 0 {
		t.Fatalf("final status = %+v, want successful generation 3 and reset failures", applied)
	}
}

func TestControllerCycleStatusStoreSequenceUnpublishedStreakApplied(t *testing.T) {
	var store controllerCycleStatusStore

	// Repeated unpublished cycles are diagnostic-only but still important for
	// future scheduler-readiness; the store tracks that streak until an applied
	// cycle proves the owner made forward progress again.
	store.publish(ControllerCycleStatusApplied, Generation(1), Generation(1), "")
	first := store.publish(ControllerCycleStatusUnpublished, Generation(2), NoGeneration, budgetAllocationReasonMinimumsExceedParent)
	second := store.publish(ControllerCycleStatusUnpublished, Generation(3), NoGeneration, budgetAllocationReasonMinimumsExceedParent)
	applied := store.publish(ControllerCycleStatusApplied, Generation(4), Generation(4), "")

	if first.ConsecutiveUnpublished != 1 || second.ConsecutiveUnpublished != 2 {
		t.Fatalf("unpublished streak = %d, %d; want 1, 2", first.ConsecutiveUnpublished, second.ConsecutiveUnpublished)
	}
	if applied.ConsecutiveUnpublished != 0 || applied.LastSuccessfulGeneration != Generation(4) {
		t.Fatalf("applied status = %+v, want reset unpublished streak and generation 4", applied)
	}
}

func TestControllerCycleStatusStoreSequenceOverlapPreservesCounters(t *testing.T) {
	var store controllerCycleStatusStore

	// AlreadyRunning reports orchestration overlap. It must not change the
	// computation counters because no controller sample, publication, or skip
	// decision actually ran.
	store.publish(ControllerCycleStatusFailed, Generation(1), NoGeneration, controllerCycleReasonFailed)

	overlap := store.publish(ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)
	if overlap.ConsecutiveFailures != 1 || overlap.ConsecutiveSkipped != 0 || overlap.ConsecutiveUnpublished != 0 {
		t.Fatalf("overlap status = %+v, want computation counters preserved", overlap)
	}
}

func TestPoolPartitionControllerStatusSequenceNilNoOp(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))
	beforeStatus := partition.ControllerStatus()
	beforeGeneration := partition.generation.Load()

	requirePartitionNoError(t, partition.TickInto(nil))

	assertControllerStatusMatchesReport(t, partition.ControllerStatus(), beforeStatus)
	if generation := partition.generation.Load(); generation != beforeGeneration {
		t.Fatalf("partition generation = %s, want unchanged %s", generation, beforeGeneration)
	}
}

func TestPoolGroupControllerStatusReportsSkippedAndAppliedPaths(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	var skipped PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&skipped))
	var skippedAgain PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&skippedAgain))

	// Applied group publication requires a budgeted group, while the skipped
	// streak above intentionally uses a no-budget group. This test covers both
	// owner-level paths without pretending they form one same-owner sequence.
	appliedGroup := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")
	var applied PoolGroupCoordinatorReport
	requireGroupNoError(t, appliedGroup.TickInto(&applied))

	if skippedAgain.Status.ConsecutiveSkipped != 2 {
		t.Fatalf("second skipped status = %+v, want skipped streak 2", skippedAgain.Status)
	}
	assertControllerCycleStatus(t, applied.Status, ControllerCycleStatusApplied, applied.Generation, applied.Generation, "")
}

func TestPoolGroupControllerStatusReportsUnpublishedAndAppliedPaths(t *testing.T) {
	_, unpublished := runGroupTickUnpublishedByClosedChild(t)
	assertControllerCycleStatus(t, unpublished.Status, ControllerCycleStatusUnpublished, unpublished.Generation, NoGeneration, policyUpdateFailureSkippedChild)

	// The unpublished path closes a child partition. A fresh budgeted group keeps
	// the applied-path assertion explicit without adding test-only repair hooks.
	appliedGroup := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")
	var applied PoolGroupCoordinatorReport
	requireGroupNoError(t, appliedGroup.TickInto(&applied))
	assertControllerCycleStatus(t, applied.Status, ControllerCycleStatusApplied, applied.Generation, applied.Generation, "")
}

func TestPoolGroupControllerStatusRuntimeSkippedResetsSeededFailure(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	// Seed the retained status store with a previous failed cycle, then drive a
	// real no-work TickInto on the same group to prove skipped resets failures.
	group.coordinator.status.publish(ControllerCycleStatusFailed, Generation(1), NoGeneration, controllerCycleReasonFailed)
	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))

	if report.Status.ConsecutiveFailures != 0 || report.Status.Status != ControllerCycleStatusSkipped {
		t.Fatalf("report status = %+v, want skipped cycle resetting previous failure", report.Status)
	}
}

func TestControllerCycleStatusStoreSequenceOverlapPreservesUnpublishedCounters(t *testing.T) {
	var store controllerCycleStatusStore

	store.publish(ControllerCycleStatusUnpublished, Generation(1), NoGeneration, controllerCycleReasonUnpublished)
	overlap := store.publish(ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)
	if overlap.ConsecutiveFailures != 0 || overlap.ConsecutiveSkipped != 0 || overlap.ConsecutiveUnpublished != 1 {
		t.Fatalf("overlap status = %+v, want unpublished counter preserved", overlap)
	}
}

func TestPoolGroupControllerStatusSequenceNilNoOp(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	beforeStatus := group.ControllerStatus()
	beforeGeneration := group.generation.Load()

	requireGroupNoError(t, group.TickInto(nil))

	assertControllerStatusMatchesReport(t, group.ControllerStatus(), beforeStatus)
	if generation := group.generation.Load(); generation != beforeGeneration {
		t.Fatalf("group generation = %s, want unchanged %s", generation, beforeGeneration)
	}
}

func TestPoolPartitionControllerStatusDoesNotRetainReportDiagnostics(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))
	before := partition.ControllerStatus()

	report.Sample.Pools = append(report.Sample.Pools, PoolPartitionPoolSample{Name: "mutated"})
	report.Window.Pools = nil
	report.PoolScores = nil
	report.ClassScores = nil
	report.TrimResult.CandidatePools = nil

	assertControllerStatusMatchesReport(t, partition.ControllerStatus(), before)
}

func TestPoolGroupControllerStatusDoesNotRetainReportDiagnostics(t *testing.T) {
	group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")
	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	before := group.ControllerStatus()

	report.Sample.Partitions = append(report.Sample.Partitions, PoolGroupPartitionSample{Name: "mutated"})
	report.Window.Current.Partitions = nil
	report.Window.Previous.Partitions = nil
	report.PartitionScores = nil
	report.PartitionBudgetTargets = nil

	assertControllerStatusMatchesReport(t, group.ControllerStatus(), before)
}

func TestPoolPartitionControllerStatusUnaffectedByReportMutation(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))
	before := partition.ControllerStatus()

	report.Status.FailureReason = "mutated"
	report.Status.ConsecutiveFailures = 99

	assertControllerStatusMatchesReport(t, partition.ControllerStatus(), before)
}

func TestPoolGroupControllerStatusUnaffectedByReportMutation(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	before := group.ControllerStatus()

	report.Status.FailureReason = "mutated"
	report.Status.ConsecutiveSkipped = 99

	assertControllerStatusMatchesReport(t, group.ControllerStatus(), before)
}

func TestPoolPartitionTickOverlapDoesNotAdvanceOwnerGeneration(t *testing.T) {
	partition, report, beforeOwner, _, _ := runPartitionTickOverlapReportWithState(t)

	assertControllerCycleStatus(t, report.Status, ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)
	if got := partition.generation.Load(); got != beforeOwner {
		t.Fatalf("partition generation = %s, want unchanged %s", got, beforeOwner)
	}
}

func TestPoolPartitionTickOverlapDoesNotAdvanceControllerGeneration(t *testing.T) {
	partition, _, _, beforeController, _ := runPartitionTickOverlapReportWithState(t)

	if got := partition.controller.generation.Load(); got != beforeController {
		t.Fatalf("controller generation = %s, want unchanged %s", got, beforeController)
	}
}

func TestPoolPartitionTickOverlapDoesNotCommitPreviousSample(t *testing.T) {
	partition, _, _, _, beforeHasPrevious := runPartitionTickOverlapReportWithState(t)

	if partition.controller.hasPreviousSample != beforeHasPrevious {
		t.Fatalf("hasPreviousSample = %v, want %v", partition.controller.hasPreviousSample, beforeHasPrevious)
	}
}

func TestPoolPartitionTickOverlapDoesNotPublishBudgetOrTrim(t *testing.T) {
	_, report, _, _, _ := runPartitionTickOverlapReportWithState(t)

	if len(report.PoolBudgetTargets) != 0 || report.BudgetPublication.Published || report.TrimResult.Attempted {
		t.Fatalf("overlap report = %+v, want no budget publication or trim", report)
	}
}

func TestPoolGroupTickOverlapDoesNotAdvanceOwnerGeneration(t *testing.T) {
	group, report, beforeOwner, _, _ := runGroupTickOverlapReportWithState(t)

	assertControllerCycleStatus(t, report.Status, ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)
	if got := group.generation.Load(); got != beforeOwner {
		t.Fatalf("group generation = %s, want unchanged %s", got, beforeOwner)
	}
}

func TestPoolGroupTickOverlapDoesNotAdvanceCoordinatorGeneration(t *testing.T) {
	group, _, _, beforeCoordinator, _ := runGroupTickOverlapReportWithState(t)

	if got := group.coordinator.generation.Load(); got != beforeCoordinator {
		t.Fatalf("coordinator generation = %s, want unchanged %s", got, beforeCoordinator)
	}
}

func TestPoolGroupTickOverlapDoesNotCommitPreviousSample(t *testing.T) {
	group, _, _, _, beforeHasPrevious := runGroupTickOverlapReportWithState(t)

	if group.coordinator.hasPreviousSample != beforeHasPrevious {
		t.Fatalf("hasPreviousSample = %v, want %v", group.coordinator.hasPreviousSample, beforeHasPrevious)
	}
}

func TestPoolGroupTickOverlapDoesNotPublishBudget(t *testing.T) {
	_, report, _, _, _ := runGroupTickOverlapReportWithState(t)

	if len(report.PartitionBudgetTargets) != 0 || report.BudgetPublication.Published {
		t.Fatalf("overlap report = %+v, want no budget publication", report)
	}
}

func TestPoolPartitionTickDelegatesStatusConsistently(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	report, err := partition.Tick()
	requirePartitionNoError(t, err)

	assertControllerStatusMatchesReport(t, partition.ControllerStatus(), report.Status)
}

func TestPoolGroupTickDelegatesStatusConsistently(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	report, err := group.Tick()
	requireGroupNoError(t, err)

	assertControllerStatusMatchesReport(t, group.ControllerStatus(), report.Status)
}

func TestControllerStatusAccessorDoesNotRunCycle(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	before := partition.controller.cycles

	_ = partition.ControllerStatus()

	if partition.controller.cycles != before {
		t.Fatalf("controller cycles = %d, want unchanged %d", partition.controller.cycles, before)
	}
}

func TestControllerStatusAccessorDoesNotAdvanceGeneration(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	before := group.generation.Load()

	_ = group.ControllerStatus()

	if got := group.generation.Load(); got != before {
		t.Fatalf("group generation = %s, want unchanged %s", got, before)
	}
}

func TestControllerCycleStatusStoreSuccessClearsFailureReason(t *testing.T) {
	var store controllerCycleStatusStore
	store.publish(ControllerCycleStatusFailed, Generation(1), NoGeneration, controllerCycleReasonFailed)

	status := store.publish(ControllerCycleStatusApplied, Generation(2), Generation(2), "")
	if status.FailureReason != "" {
		t.Fatalf("applied status reason = %q, want empty", status.FailureReason)
	}
}

func TestControllerCycleStatusStoreClosedReasonStable(t *testing.T) {
	var store controllerCycleStatusStore

	status := store.publish(ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
	assertControllerCycleStatus(t, status, ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
}

func TestControllerCycleStatusStoreAlreadyRunningReasonStable(t *testing.T) {
	var store controllerCycleStatusStore

	status := store.publish(ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)
	assertControllerCycleStatus(t, status, ControllerCycleStatusAlreadyRunning, NoGeneration, NoGeneration, controllerCycleReasonAlreadyRunning)
}

func TestControllerCycleStatusStoreNoWorkReasonStable(t *testing.T) {
	var store controllerCycleStatusStore

	status := store.publish(ControllerCycleStatusSkipped, Generation(1), NoGeneration, controllerCycleReasonNoWork)
	assertControllerCycleStatus(t, status, ControllerCycleStatusSkipped, Generation(1), NoGeneration, controllerCycleReasonNoWork)
}

func TestControllerCycleStatusStoreUnpublishedReasonStable(t *testing.T) {
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

func assertControllerStatusMatchesReport(t *testing.T, retained, reported ControllerCycleStatusSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(retained, reported) {
		t.Fatalf("retained status = %+v, want report status %+v", retained, reported)
	}
}

func assertControllerStatusDoesNotExposeRawErrorText(t *testing.T) {
	t.Helper()

	err := newError(ErrClosed, errPoolClosed)
	reason := controllerCycleFailureReasonForError(err, controllerCycleReasonFailed)
	if reason == err.Error() {
		t.Fatalf("status reason exposed raw error text: %q", reason)
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

func runGroupTickUnpublishedByClosedChild(t *testing.T) (*PoolGroup, PoolGroupCoordinatorReport) {
	t.Helper()

	group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")
	beta, ok := group.partition("beta")
	if !ok {
		t.Fatal("partition beta not found")
	}
	requirePartitionNoError(t, beta.Close())

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	return group, report
}

func runPartitionTickOverlapReport(t *testing.T) (*PoolPartition, PartitionControllerReport) {
	t.Helper()
	partition, report, _, _, _ := runPartitionTickOverlapReportWithState(t)
	return partition, report
}

func runPartitionTickOverlapReportWithState(t *testing.T) (*PoolPartition, PartitionControllerReport, Generation, Generation, bool) {
	t.Helper()

	partition := testNewPoolPartition(t, "primary")
	partition.controller.cycleGate.running.Store(true)
	defer partition.controller.cycleGate.running.Store(false)

	beforeOwner := partition.generation.Load()
	beforeController := partition.controller.generation.Load()
	beforeHasPrevious := partition.controller.hasPreviousSample
	var overlap PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&overlap))

	return partition, overlap, beforeOwner, beforeController, beforeHasPrevious
}

func runGroupTickOverlapReport(t *testing.T) (*PoolGroup, PoolGroupCoordinatorReport) {
	t.Helper()
	group, report, _, _, _ := runGroupTickOverlapReportWithState(t)
	return group, report
}

func runGroupTickOverlapReportWithState(t *testing.T) (*PoolGroup, PoolGroupCoordinatorReport, Generation, Generation, bool) {
	t.Helper()

	group := testNewPoolGroup(t, "alpha")
	group.coordinator.cycleGate.running.Store(true)
	defer group.coordinator.cycleGate.running.Store(false)

	beforeOwner := group.generation.Load()
	beforeCoordinator := group.coordinator.generation.Load()
	beforeHasPrevious := group.coordinator.hasPreviousSample
	var overlap PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&overlap))

	return group, overlap, beforeOwner, beforeCoordinator, beforeHasPrevious
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
