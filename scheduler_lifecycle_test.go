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
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestManualTickStillWorksWithoutScheduler verifies the opt-in scheduler
// integration does not change the manual foreground control contract. Default
// owners keep their scheduler runtimes stopped, while TickInto still executes
// through the retained controller/coordinator status path.
func TestManualTickStillWorksWithoutScheduler(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	var partitionReport PartitionControllerReport

	requirePartitionNoError(t, partition.TickInto(&partitionReport))
	assertPoolPartitionSchedulerStopped(t, partition, "manual partition")
	if partitionReport.Status.Status == ControllerCycleStatusUnset {
		t.Fatalf("partition TickInto status = %+v, want a published manual cycle status", partitionReport.Status)
	}

	group := testNewPoolGroup(t, "alpha")
	var groupReport PoolGroupCoordinatorReport

	requireGroupNoError(t, group.TickInto(&groupReport))
	assertPoolGroupSchedulerStopped(t, group, "manual group")
	if groupReport.Status.Status == ControllerCycleStatusUnset {
		t.Fatalf("group TickInto status = %+v, want a published manual cycle status", groupReport.Status)
	}
}

// TestSchedulerRejectsIntervalWithoutEnabled keeps both reserved scheduler
// policy surfaces honest: an interval without Enabled would imply background
// work that neither owner starts in manual mode.
func TestSchedulerRejectsIntervalWithoutEnabled(t *testing.T) {
	partitionPolicy := DefaultPartitionPolicy()
	partitionPolicy.Controller.TickInterval = time.Second
	requirePartitionErrorIs(t, partitionPolicy.Validate(), ErrInvalidPolicy)

	groupPolicy := DefaultPoolGroupPolicy()
	groupPolicy.Coordinator.TickInterval = time.Second
	requireGroupErrorIs(t, groupPolicy.Validate(), ErrInvalidPolicy)
}

// TestSchedulerRejectsNegativeInterval verifies both opt-in scheduler policies
// reject negative cadences instead of passing invalid intervals to the internal
// ticker runtime.
func TestSchedulerRejectsNegativeInterval(t *testing.T) {
	partitionPolicy := DefaultPartitionPolicy()
	partitionPolicy.Controller.Enabled = true
	partitionPolicy.Controller.TickInterval = -time.Second
	requirePartitionErrorIs(t, partitionPolicy.Validate(), ErrInvalidPolicy)

	groupPolicy := DefaultPoolGroupPolicy()
	groupPolicy.Coordinator.Enabled = true
	groupPolicy.Coordinator.TickInterval = -time.Second
	requireGroupErrorIs(t, groupPolicy.Validate(), ErrInvalidPolicy)
}

// TestPoolPartitionCloseDoesNotRewriteLastControllerStatus verifies Close owns
// partition lifecycle state, not retained controller-cycle history. A previously
// applied TickInto status remains visible until another foreground TickInto
// attempt publishes a new controller outcome.
func TestPoolPartitionCloseDoesNotRewriteLastControllerStatus(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))
	before := partition.ControllerStatus()

	requirePartitionNoError(t, partition.Close())

	if after := partition.ControllerStatus(); after != before {
		t.Fatalf("ControllerStatus after Close = %+v, want unchanged last cycle %+v", after, before)
	}
	if !partition.IsClosed() || partition.Lifecycle() != LifecycleClosed {
		t.Fatalf("partition lifecycle after Close = %s closed=%v, want closed lifecycle", partition.Lifecycle(), partition.IsClosed())
	}
}

// TestPoolGroupCloseDoesNotRewriteLastControllerStatus verifies Close owns group
// lifecycle state, not retained coordinator-cycle history. A skipped or applied
// coordinator status remains the last controller-cycle status until TickInto
// observes the closed lifecycle.
func TestPoolGroupCloseDoesNotRewriteLastControllerStatus(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	before := group.ControllerStatus()

	requireGroupNoError(t, group.Close())

	if after := group.ControllerStatus(); after != before {
		t.Fatalf("ControllerStatus after Close = %+v, want unchanged last cycle %+v", after, before)
	}
	if !group.IsClosed() || group.Lifecycle() != LifecycleClosed {
		t.Fatalf("group lifecycle after Close = %s closed=%v, want closed lifecycle", group.Lifecycle(), group.IsClosed())
	}
}

// TestPoolPartitionTickIntoAfterClosePublishesClosedStatus pins the path that
// does update ControllerStatus after shutdown: a foreground TickInto attempt
// observes closed lifecycle and publishes Closed while returning ErrClosed.
func TestPoolPartitionTickIntoAfterClosePublishesClosedStatus(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	requirePartitionNoError(t, partition.Close())

	var report PartitionControllerReport
	err := partition.TickInto(&report)

	requirePartitionErrorIs(t, err, ErrClosed)
	if report.Status.Status != ControllerCycleStatusClosed {
		t.Fatalf("TickInto after Close status = %+v, want Closed", report.Status)
	}
	if retained := partition.ControllerStatus(); retained != report.Status {
		t.Fatalf("retained status = %+v, want TickInto report status %+v", retained, report.Status)
	}
}

// TestPoolGroupTickIntoAfterClosePublishesClosedStatus pins the path that does
// update ControllerStatus after shutdown: a foreground TickInto attempt observes
// closed lifecycle and publishes Closed while returning ErrClosed.
func TestPoolGroupTickIntoAfterClosePublishesClosedStatus(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	requireGroupNoError(t, group.Close())

	var report PoolGroupCoordinatorReport
	err := group.TickInto(&report)

	requireGroupErrorIs(t, err, ErrClosed)
	if report.Status.Status != ControllerCycleStatusClosed {
		t.Fatalf("TickInto after Close status = %+v, want Closed", report.Status)
	}
	if retained := group.ControllerStatus(); retained != report.Status {
		t.Fatalf("retained status = %+v, want TickInto report status %+v", retained, report.Status)
	}
}

// TestPoolPartitionLifecycleIsClosedAfterSchedulerClose verifies scheduler
// shutdown remains subordinate to owner lifecycle shutdown: Close stops the
// opt-in scheduler and still marks the partition lifecycle closed.
func TestPoolPartitionLifecycleIsClosedAfterSchedulerClose(t *testing.T) {
	partition, _ := newScheduledPartitionWithManualTicker(t, time.Second, "primary")

	requirePartitionNoError(t, partition.Close())

	assertPoolPartitionSchedulerStopped(t, partition, "closed scheduled partition")
	if !partition.IsClosed() || partition.Lifecycle() != LifecycleClosed {
		t.Fatalf("scheduled partition lifecycle after Close = %s closed=%v, want closed lifecycle",
			partition.Lifecycle(), partition.IsClosed())
	}
}

// TestPoolGroupLifecycleIsClosedAfterSchedulerClose verifies scheduler shutdown
// remains subordinate to owner lifecycle shutdown: Close stops the opt-in
// scheduler and still marks the group lifecycle closed.
func TestPoolGroupLifecycleIsClosedAfterSchedulerClose(t *testing.T) {
	group, _ := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	requireGroupNoError(t, group.Close())

	assertPoolGroupSchedulerStopped(t, group, "closed scheduled group")
	if !group.IsClosed() || group.Lifecycle() != LifecycleClosed {
		t.Fatalf("scheduled group lifecycle after Close = %s closed=%v, want closed lifecycle",
			group.Lifecycle(), group.IsClosed())
	}
}

// TestPoolPartitionSchedulerDoesNotRetainFullReportState combines a structural
// owner/runtime check with a behavioral status-copy check. The scheduler wrapper
// discards its stack-local report, and mutating a separate direct TickInto report
// cannot affect retained ControllerStatus.
func TestPoolPartitionSchedulerDoesNotRetainFullReportState(t *testing.T) {
	partition, ticker := newScheduledPartitionWithManualTicker(t, time.Second, "primary")
	ticker.tick()
	_ = waitForPartitionSchedulerStatus(t, partition, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusApplied
	})
	waitForControllerCycleGateIdle(t, "partition scheduler report-retention tick", partition.controller.cycleGate.isRunning)

	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))
	retained := partition.ControllerStatus()
	report.Status.FailureReason = "mutated_report_status"
	report.Status.Status = ControllerCycleStatusFailed
	report.Generation = NoGeneration
	report.PoolBudgetTargets = append(report.PoolBudgetTargets, PoolBudgetTarget{})

	if after := partition.ControllerStatus(); after != retained {
		t.Fatalf("ControllerStatus after local report mutation = %+v, want retained copy %+v", after, retained)
	}
	assertPoolPartitionDoesNotHaveReportFields(t)
	assertTypeHasNoReportRetentionFields(t, reflect.TypeOf(controllerSchedulerRuntime{}), "controllerSchedulerRuntime", nil)
}

// TestPoolGroupSchedulerDoesNotRetainFullReportState combines a structural
// owner/runtime check with a behavioral status-copy check. The scheduler wrapper
// discards its stack-local report, and mutating a separate direct TickInto report
// cannot affect retained ControllerStatus.
func TestPoolGroupSchedulerDoesNotRetainFullReportState(t *testing.T) {
	group, ticker := newScheduledGroupWithManualTicker(t, time.Second, "alpha")
	ticker.tick()
	_ = waitForGroupSchedulerStatus(t, group, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusSkipped
	})
	waitForControllerCycleGateIdle(t, "group scheduler report-retention tick", group.coordinator.cycleGate.isRunning)

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	retained := group.ControllerStatus()
	report.Status.FailureReason = "mutated_report_status"
	report.Status.Status = ControllerCycleStatusFailed
	report.Generation = NoGeneration
	report.PartitionBudgetTargets = append(report.PartitionBudgetTargets, PartitionBudgetTarget{})

	if after := group.ControllerStatus(); after != retained {
		t.Fatalf("ControllerStatus after local report mutation = %+v, want retained copy %+v", after, retained)
	}
	assertPoolGroupDoesNotHaveReportFields(t)
	assertTypeHasNoReportRetentionFields(t, reflect.TypeOf(controllerSchedulerRuntime{}), "controllerSchedulerRuntime", nil)
}

// TestControllerSchedulerRuntimeDoesNotRetainReportState is the final scheduler
// retention check for the internal primitive. The runtime can keep channels and
// booleans needed for lifecycle, but it must not keep report, sample, score,
// trim, diagnostic, map, or slice fields.
func TestControllerSchedulerRuntimeDoesNotRetainReportState(t *testing.T) {
	assertTypeHasNoReportRetentionFields(t, reflect.TypeOf(controllerSchedulerRuntime{}), "controllerSchedulerRuntime", nil)
}

// TestControllerSchedulerRuntimeCanRestartAfterStop documents the distinction
// between internal runtime mechanics and owner policy. The primitive can safely
// Start after Stop; PoolPartition and PoolGroup still reject live scheduler
// toggles through PublishPolicy.
func TestControllerSchedulerRuntimeCanRestartAfterStop(t *testing.T) {
	var runtime controllerSchedulerRuntime
	firstTicker := newManualControllerSchedulerTicker()
	secondTicker := newManualControllerSchedulerTicker()
	ticks := make(chan string, 2)

	requireControllerSchedulerNoError(t, runtime.Start(controllerSchedulerTestInput(firstTicker, func() error {
		ticks <- "first"
		return nil
	})))
	firstTicker.tick()
	if got := requireControllerSchedulerString(t, ticks, "first runtime tick"); got != "first" {
		t.Fatalf("first runtime tick = %q, want first", got)
	}
	runtime.Stop()
	assertManualControllerSchedulerTickerStopped(t, firstTicker)

	requireControllerSchedulerNoError(t, runtime.Start(controllerSchedulerTestInput(secondTicker, func() error {
		ticks <- "second"
		return nil
	})))
	secondTicker.tick()
	if got := requireControllerSchedulerString(t, ticks, "second runtime tick"); got != "second" {
		t.Fatalf("second runtime tick = %q, want second after Stop/Start", got)
	}
	runtime.Stop()
	assertManualControllerSchedulerTickerStopped(t, secondTicker)
}

// TestDefaultPartitionPolicySchedulerManual is the scheduler-specific name for
// the default partition policy invariant: no automatic cadence is filled unless
// the caller explicitly enables the partition scheduler.
func TestDefaultPartitionPolicySchedulerManual(t *testing.T) {
	policy := DefaultPartitionPolicy()
	if policy.Controller.Enabled || policy.Controller.TickInterval != 0 {
		t.Fatalf("default partition scheduler policy = %+v, want disabled zero interval", policy.Controller)
	}
	requirePartitionNoError(t, policy.Validate())
}

// TestDefaultPoolGroupPolicySchedulerManual is the scheduler-specific name for
// the default group policy invariant: no automatic cadence is filled unless the
// caller explicitly enables the group scheduler.
func TestDefaultPoolGroupPolicySchedulerManual(t *testing.T) {
	policy := DefaultPoolGroupPolicy()
	if policy.Coordinator.Enabled || policy.Coordinator.TickInterval != 0 {
		t.Fatalf("default group scheduler policy = %+v, want disabled zero interval", policy.Coordinator)
	}
	requireGroupNoError(t, policy.Validate())
}

// TestPartitionSchedulerPolicyRejectsDormantInterval duplicates the exact
// partition scheduler-policy contract with final-stage naming. A disabled
// scheduler may not carry a dormant interval because construction will not start
// background work for it.
func TestPartitionSchedulerPolicyRejectsDormantInterval(t *testing.T) {
	policy := DefaultPartitionPolicy()
	policy.Controller.TickInterval = time.Second
	requirePartitionErrorIs(t, policy.Validate(), ErrInvalidPolicy)
}

// TestPoolGroupSchedulerPolicyRejectsDormantInterval duplicates the exact group
// scheduler-policy contract with final-stage naming. A disabled scheduler may
// not carry a dormant interval because construction will not start background
// work for it.
func TestPoolGroupSchedulerPolicyRejectsDormantInterval(t *testing.T) {
	policy := DefaultPoolGroupPolicy()
	policy.Coordinator.TickInterval = time.Second
	requireGroupErrorIs(t, policy.Validate(), ErrInvalidPolicy)
}

// TestPartitionSchedulerPolicyNormalizesEnabledInterval verifies explicit
// partition scheduler opt-in is the only path that receives the default cadence.
func TestPartitionSchedulerPolicyNormalizesEnabledInterval(t *testing.T) {
	policy := DefaultPartitionPolicy()
	policy.Controller.Enabled = true

	normalized := policy.Normalize()

	if normalized.Controller.TickInterval != defaultPartitionControllerTickInterval {
		t.Fatalf("normalized partition scheduler interval = %s, want %s",
			normalized.Controller.TickInterval, defaultPartitionControllerTickInterval)
	}
	requirePartitionNoError(t, normalized.Validate())
}

// TestPoolGroupSchedulerPolicyNormalizesEnabledInterval verifies explicit group
// scheduler opt-in is the only path that receives the default cadence.
func TestPoolGroupSchedulerPolicyNormalizesEnabledInterval(t *testing.T) {
	policy := DefaultPoolGroupPolicy()
	policy.Coordinator.Enabled = true

	normalized := policy.Normalize()

	if normalized.Coordinator.TickInterval != defaultGroupCoordinatorTickInterval {
		t.Fatalf("normalized group scheduler interval = %s, want %s",
			normalized.Coordinator.TickInterval, defaultGroupCoordinatorTickInterval)
	}
	requireGroupNoError(t, normalized.Validate())
}

// TestPartitionSchedulerPolicyRejectsNegativeInterval verifies enabled
// partition scheduler policy still rejects negative intervals after Normalize.
func TestPartitionSchedulerPolicyRejectsNegativeInterval(t *testing.T) {
	policy := DefaultPartitionPolicy()
	policy.Controller.Enabled = true
	policy.Controller.TickInterval = -time.Second
	requirePartitionErrorIs(t, policy.Validate(), ErrInvalidPolicy)
}

// TestPoolGroupSchedulerPolicyRejectsNegativeInterval verifies enabled group
// scheduler policy still rejects negative intervals after Normalize.
func TestPoolGroupSchedulerPolicyRejectsNegativeInterval(t *testing.T) {
	policy := DefaultPoolGroupPolicy()
	policy.Coordinator.Enabled = true
	policy.Coordinator.TickInterval = -time.Second
	requireGroupErrorIs(t, policy.Validate(), ErrInvalidPolicy)
}

func requireControllerSchedulerString(t *testing.T, ch <-chan string, name string) string {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-t.Context().Done():
		t.Fatalf("timed out waiting for %s", name)
		return ""
	}
}

func assertTypeHasNoReportRetentionFields(
	t *testing.T,
	typ reflect.Type,
	ownerName string,
	allowedFields map[string]struct{},
) {
	t.Helper()

	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if _, ok := allowedFields[field.Name]; ok {
			continue
		}

		lowerName := strings.ToLower(field.Name)
		for _, forbidden := range []string{"report", "diagnostic", "sample", "window", "score", "trim", "candidate"} {
			if strings.Contains(lowerName, forbidden) {
				t.Fatalf("%s field %q suggests retained scheduler report state", ownerName, field.Name)
			}
		}
		if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Map {
			t.Fatalf("%s field %q has type %s; scheduler status must not retain report diagnostics",
				ownerName, field.Name, field.Type)
		}
	}
}

func waitForControllerCycleGateIdle(t *testing.T, name string, running func() bool) {
	t.Helper()

	for {
		if !running() {
			return
		}

		select {
		case <-t.Context().Done():
			t.Fatalf("timed out waiting for %s controller cycle to leave no-overlap gate", name)
		default:
			runtime.Gosched()
		}
	}
}
