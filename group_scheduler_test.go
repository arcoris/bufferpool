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
	"context"
	"errors"
	"reflect"
	"runtime"
	"testing"
	"time"
)

// TestDefaultPoolGroupDoesNotStartScheduler is the final-stage name for the
// default manual-mode invariant. It intentionally repeats the assertion instead
// of calling another Test function so failure output points at this contract.
func TestDefaultPoolGroupDoesNotStartScheduler(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	assertPoolGroupSchedulerStopped(t, group, "default group")
}

// TestPoolGroupSchedulerDisabledByDefault pins the default group mode as
// manual-only. A newly constructed group must not start the coordinator
// scheduler unless policy opts in explicitly.
func TestPoolGroupSchedulerDisabledByDefault(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")

	assertPoolGroupSchedulerStopped(t, group, "default group")
	if policy := group.Policy().Coordinator; policy.Enabled || policy.TickInterval != 0 {
		t.Fatalf("default coordinator policy = %+v, want manual scheduler-free policy", policy)
	}
}

// TestGroupSchedulerOptInOnly proves disabled construction stays manual and
// enabled construction starts exactly the group-level scheduler requested by
// PoolGroupPolicy.Coordinator.
func TestGroupSchedulerOptInOnly(t *testing.T) {
	manual := testNewPoolGroup(t, "alpha")
	assertPoolGroupSchedulerStopped(t, manual, "manual group")

	scheduled, _ := newScheduledGroupWithManualTicker(t, time.Second, "alpha")
	if !scheduled.coordinatorScheduler.isRunning() {
		t.Fatal("enabled group coordinator policy did not start scheduler")
	}
}

// TestPoolGroupSchedulerPolicyEnablesScheduler verifies that an enabled
// coordinator policy starts the owner-local scheduler after construction has
// completed all group initialization.
func TestPoolGroupSchedulerPolicyEnablesScheduler(t *testing.T) {
	group, _ := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	if !group.coordinatorScheduler.isRunning() {
		t.Fatal("enabled group coordinator policy did not start scheduler")
	}
}

// TestPoolGroupSchedulerRejectsIntervalWithoutEnabled keeps disabled policy
// honest: a dormant interval would imply automatic work that the group will not
// start.
func TestPoolGroupSchedulerRejectsIntervalWithoutEnabled(t *testing.T) {
	policy := DefaultPoolGroupPolicy()
	policy.Coordinator.TickInterval = time.Second

	err := policy.Validate()
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
}

// TestPoolGroupSchedulerRejectsNegativeInterval verifies an enabled scheduler
// cannot use a negative cadence after normalization.
func TestPoolGroupSchedulerRejectsNegativeInterval(t *testing.T) {
	policy := DefaultPoolGroupPolicy()
	policy.Coordinator.Enabled = true
	policy.Coordinator.TickInterval = -time.Second

	err := policy.Validate()
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
}

// TestPoolGroupSchedulerUsesDefaultIntervalWhenEnabled proves that Normalize
// completes the scheduler cadence only for explicit opt-in policy and that the
// constructor passes the same cadence to the runtime ticker.
func TestPoolGroupSchedulerUsesDefaultIntervalWhenEnabled(t *testing.T) {
	ticker := newManualControllerSchedulerTicker()
	var observed time.Duration
	config := schedulerGroupConfig(0, "alpha")

	group, err := newPoolGroupWithCoordinatorSchedulerTickerFactory(config, func(interval time.Duration) controllerSchedulerTicker {
		observed = interval
		return ticker
	})
	requireGroupNoError(t, err)
	t.Cleanup(func() { requireGroupNoError(t, group.Close()) })

	if observed != defaultGroupCoordinatorTickInterval {
		t.Fatalf("scheduler interval = %s, want default %s", observed, defaultGroupCoordinatorTickInterval)
	}
	if policy := group.Policy().Coordinator; policy.TickInterval != defaultGroupCoordinatorTickInterval {
		t.Fatalf("runtime coordinator policy = %+v, want default interval", policy)
	}
}

// TestPoolGroupSchedulerRunsTick drives the scheduler with a manual ticker and
// verifies that the scheduled path reaches the same status publication path as a
// foreground TickInto call.
func TestPoolGroupSchedulerRunsTick(t *testing.T) {
	group, ticker := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	ticker.tick()
	status := waitForGroupSchedulerStatus(t, group, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusSkipped
	})

	if status.AttemptGeneration.IsZero() || !status.AppliedGeneration.IsZero() {
		t.Fatalf("scheduled tick status = %+v, want skipped attempt without applied generation", status)
	}
}

// TestPoolGroupSchedulerUpdatesControllerStatus verifies that scheduler
// dispatch publishes retained lightweight ControllerStatus instead of retaining
// the full report.
func TestPoolGroupSchedulerUpdatesControllerStatus(t *testing.T) {
	group, ticker := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	ticker.tick()
	first := waitForGroupSchedulerStatus(t, group, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusSkipped
	})
	ticker.tick()
	second := waitForGroupSchedulerStatus(t, group, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusSkipped &&
			status.AttemptGeneration.After(first.AttemptGeneration)
	})

	if second.ConsecutiveSkipped <= first.ConsecutiveSkipped {
		t.Fatalf("second scheduled status = %+v, want skipped streak to advance after scheduler tick", second)
	}
}

// TestPoolGroupSchedulerDoesNotRetainFullReport checks the owner shape after a
// scheduled tick. The scheduler must remain a dispatch mechanism; heavy reports
// stay caller-owned or stack-local inside schedulerTick.
func TestPoolGroupSchedulerDoesNotRetainFullReport(t *testing.T) {
	group, ticker := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	ticker.tick()
	_ = waitForGroupSchedulerStatus(t, group, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusSkipped
	})

	assertPoolGroupDoesNotHaveReportFields(t)
}

// TestPoolGroupSchedulerContinuesAfterTickFailure verifies the internal
// scheduler runtime treats non-closed Tick errors as already-reported
// diagnostics and keeps dispatching later ticks.
func TestPoolGroupSchedulerContinuesAfterTickFailure(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	ticker := newManualControllerSchedulerTicker()
	ticks := make(chan int, 2)
	var calls int

	// Ordinary group TickInto errors that are not closed-owner errors are
	// difficult to trigger without corrupting internal state. This test starts
	// the same owner-local scheduler runtime with a Tick wrapper that performs a
	// real schedulerTick and then returns a synthetic non-closed error. The
	// scheduler must ignore that error class and dispatch the next tick.
	requireControllerSchedulerNoError(t, group.coordinatorScheduler.Start(controllerSchedulerStartInput{
		Interval: time.Second,
		Tick: func() error {
			calls++
			err := group.schedulerTick()
			ticks <- calls
			if err != nil {
				return err
			}
			return errors.New("transient group scheduler test error")
		},
		IsClosedError: func(err error) bool { return errors.Is(err, ErrClosed) },
		TickerFactory: func(time.Duration) controllerSchedulerTicker {
			return ticker
		},
	}))
	t.Cleanup(group.stopCoordinatorScheduler)

	ticker.tick()
	if got := requireGroupSchedulerValue(t, ticks, "first scheduled failure"); got != 1 {
		t.Fatalf("first scheduled tick = %d, want 1", got)
	}
	ticker.tick()
	if got := requireGroupSchedulerValue(t, ticks, "second scheduled failure"); got != 2 {
		t.Fatalf("second scheduled tick = %d, want 2 after transient failure", got)
	}
}

// TestPoolGroupSchedulerRuntimeContinuesAfterSyntheticFailedTick verifies the
// scheduler runtime continues after a non-closed Tick error. The synthetic Tick
// publishes the same Failed status shape as a production TickInto failure, but
// it intentionally avoids production-only corruption hooks; real TickInto
// publication failures are covered by controller status tests.
func TestPoolGroupSchedulerRuntimeContinuesAfterSyntheticFailedTick(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	ticker := newManualControllerSchedulerTicker()
	statuses := make(chan ControllerCycleStatusSnapshot, 2)
	var attempts int

	// The normal group TickInto error paths require closed child control gates or
	// corrupted internal state. This owner-local runtime uses the same status
	// publication contract a failed TickInto path uses, then returns a non-closed
	// error so the scheduler continuation behavior is deterministic and does not
	// need production-only failure hooks.
	requireControllerSchedulerNoError(t, group.coordinatorScheduler.Start(controllerSchedulerStartInput{
		Interval: time.Second,
		Tick: func() error {
			attempts++
			generation := group.generation.Advance()
			status := group.coordinator.status.publish(
				ControllerCycleStatusFailed,
				generation,
				NoGeneration,
				controllerCycleReasonFailed,
			)
			statuses <- status
			return errors.New("synthetic group coordinator failure")
		},
		IsClosedError: func(err error) bool { return errors.Is(err, ErrClosed) },
		TickerFactory: func(time.Duration) controllerSchedulerTicker {
			return ticker
		},
	}))
	t.Cleanup(group.stopCoordinatorScheduler)

	ticker.tick()
	first := requireGroupSchedulerStatusValue(t, statuses, "first failed scheduled tick")
	if first.Status != ControllerCycleStatusFailed {
		t.Fatalf("first failed scheduled status = %+v, want Failed", first)
	}
	ticker.tick()
	second := requireGroupSchedulerStatusValue(t, statuses, "second failed scheduled tick")
	if second.Status != ControllerCycleStatusFailed || attempts != 2 {
		t.Fatalf("second failed scheduled status = %+v attempts=%d, want Failed after continuation", second, attempts)
	}
}

// TestPoolGroupSchedulerStopsAfterClosedTick verifies an ErrClosed result from
// the scheduled Tick path exits the scheduler loop and stops its ticker.
func TestPoolGroupSchedulerStopsAfterClosedTick(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	ticker := newManualControllerSchedulerTicker()

	requireControllerSchedulerNoError(t, group.coordinatorScheduler.Start(controllerSchedulerStartInput{
		Interval:      time.Second,
		Tick:          func() error { return newError(ErrClosed, errGroupClosed) },
		IsClosedError: func(err error) bool { return errors.Is(err, ErrClosed) },
		TickerFactory: func(time.Duration) controllerSchedulerTicker {
			return ticker
		},
	}))

	ticker.tick()
	waitForGroupSchedulerRuntimeStopped(t, group, "closed group tick")
	assertManualControllerSchedulerTickerStopped(t, ticker)
}

// TestPoolGroupSchedulerCloseWhileIdle verifies Close stops an enabled but idle
// scheduler and that extra manual ticks on the stopped ticker cannot advance
// coordinator status after Close.
func TestPoolGroupSchedulerCloseWhileIdle(t *testing.T) {
	group, ticker := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	before := group.ControllerStatus()
	requireGroupNoError(t, group.Close())
	ticker.tick()

	assertPoolGroupSchedulerStopped(t, group, "closed group")
	assertManualControllerSchedulerTickerStopped(t, ticker)
	if after := group.ControllerStatus(); after != before {
		t.Fatalf("controller status after close+manual ticker tick = %+v, want unchanged %+v", after, before)
	}
}

// TestPoolGroupSchedulerStopsOnClose verifies Close stops the scheduler before
// group child-partition cleanup proceeds.
func TestPoolGroupSchedulerStopsOnClose(t *testing.T) {
	group, ticker := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	requireGroupNoError(t, group.Close())

	if group.coordinatorScheduler.isRunning() {
		t.Fatal("scheduler still running after group Close")
	}
	assertManualControllerSchedulerTickerStopped(t, ticker)
}

// TestPoolGroupSchedulerCloseIsIdempotent keeps scheduler shutdown aligned with
// PoolGroup.Close: repeated hard-close calls must not panic or leave a running
// scheduler behind.
func TestPoolGroupSchedulerCloseIsIdempotent(t *testing.T) {
	group, ticker := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	requireGroupNoError(t, group.Close())
	requireGroupNoError(t, group.Close())

	if group.coordinatorScheduler.isRunning() {
		t.Fatal("scheduler still running after repeated Close")
	}
	assertManualControllerSchedulerTickerStopped(t, ticker)
}

// TestPoolGroupSchedulerCloseWhileTickRunningDoesNotDeadlock pins the close
// lock order. Close waits for the scheduler without holding runtimeMu, so the
// in-flight scheduled TickInto can finish and release the scheduler.
func TestPoolGroupSchedulerCloseWhileTickRunningDoesNotDeadlock(t *testing.T) {
	group, ticker := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	group.coordinator.mu.Lock()
	ticker.tick()
	waitForControllerCycleGate(t, "group scheduler", group.coordinator.cycleGate.isRunning)

	closed := make(chan error, 1)
	go func() {
		closed <- group.Close()
	}()

	group.coordinator.mu.Unlock()
	requireGroupNoError(t, requireGroupSchedulerError(t, closed, "Close while scheduled tick is running"))
	if group.coordinatorScheduler.isRunning() {
		t.Fatal("scheduler still running after Close raced with scheduled tick")
	}
}

// TestPoolGroupSchedulerCloseWhileManualTickRunning verifies scheduler-aware
// Close remains deadlock-free when the scheduler is idle but a manual TickInto
// already owns the foreground coordinator cycle.
func TestPoolGroupSchedulerCloseWhileManualTickRunning(t *testing.T) {
	group, _ := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	group.coordinator.mu.Lock()
	manual := make(chan error, 1)
	go func() {
		var report PoolGroupCoordinatorReport
		manual <- group.TickInto(&report)
	}()
	waitForControllerCycleGate(t, "manual group tick", group.coordinator.cycleGate.isRunning)

	closed := make(chan error, 1)
	go func() {
		closed <- group.Close()
	}()

	group.coordinator.mu.Unlock()
	requireGroupNoError(t, requireGroupSchedulerError(t, manual, "manual TickInto while Close waits"))
	requireGroupNoError(t, requireGroupSchedulerError(t, closed, "Close while manual TickInto is running"))
	assertPoolGroupSchedulerStopped(t, group, "closed group")
}

// TestPoolGroupSchedulerManualTickOverlap is the final-stage name for the
// scheduler/manual no-overlap contract. The scheduled tick owns the cycle gate;
// the manual tick observes AlreadyRunning and does not publish a second cycle.
func TestPoolGroupSchedulerManualTickOverlap(t *testing.T) {
	group, ticker := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	group.coordinator.mu.Lock()
	ticker.tick()
	waitForControllerCycleGate(t, "group scheduler", group.coordinator.cycleGate.isRunning)

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	if report.Status.Status != ControllerCycleStatusAlreadyRunning {
		t.Fatalf("manual TickInto during scheduled tick status = %+v, want already running", report.Status)
	}

	group.coordinator.mu.Unlock()
	_ = waitForGroupSchedulerStatus(t, group, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusSkipped
	})
}

// TestPoolGroupSchedulerManualTickMayOverlapAndIsRejected proves scheduled
// ticks and manual ticks share the same coordinator cycle gate. The second
// foreground attempt is reported as AlreadyRunning rather than running a second
// coordinator cycle.
func TestPoolGroupSchedulerManualTickMayOverlapAndIsRejected(t *testing.T) {
	group, ticker := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	group.coordinator.mu.Lock()
	ticker.tick()
	waitForControllerCycleGate(t, "group scheduler", group.coordinator.cycleGate.isRunning)

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	if report.Status.Status != ControllerCycleStatusAlreadyRunning {
		t.Fatalf("manual TickInto during scheduled tick status = %+v, want already running", report.Status)
	}

	group.coordinator.mu.Unlock()
	_ = waitForGroupSchedulerStatus(t, group, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusSkipped
	})
}

// TestPoolGroupSchedulerDoesNotTouchPoolGetPutHotPath extends the AST hot-path
// boundary checks with group scheduler symbols. The opt-in scheduler is group
// control-plane code and must not appear in Pool.Get or Pool.Put.
func TestPoolGroupSchedulerDoesNotTouchPoolGetPutHotPath(t *testing.T) {
	for _, file := range []string{"pool_get.go", "pool_put.go"} {
		facts := parsePolicyPublicationASTFacts(t, file)
		for _, forbidden := range []string{
			"controllerSchedulerRuntime",
			"coordinatorScheduler",
			"scheduler",
			"Ticker",
			"TickInterval",
			"ControllerStatus",
			"TickInto",
			"cycleGate",
			"coordinator",
			"PublishPolicy",
			"PublishPressure",
			"PlanTrim",
			"TrimPlan",
			"EWMA",
			"activeRegistry",
		} {
			if facts.hasIdentifier(forbidden) || facts.hasSelector(forbidden) || facts.hasCall(forbidden) {
				t.Fatalf("%s AST references %q; Pool.Get/Put must not depend on group scheduler code", file, forbidden)
			}
		}
	}
}

// TestPoolGroupSchedulerDoesNotScanPoolInternals verifies the scheduler wrapper
// stays at the group coordinator boundary. Pool shard/class/bucket access
// remains owned by Pool and partition control paths.
func TestPoolGroupSchedulerDoesNotScanPoolInternals(t *testing.T) {
	facts := parsePolicyPublicationASTFacts(t, "group_scheduler.go")
	for _, forbidden := range []string{"classState", "bucket", "shards", "PoolShard", "mustClassStateFor"} {
		if facts.hasIdentifier(forbidden) || facts.hasSelector(forbidden) || facts.hasCall(forbidden) {
			t.Fatalf("group_scheduler.go AST references %q; group scheduler must not scan Pool internals", forbidden)
		}
	}
}

// TestPoolGroupSchedulerDoesNotCallPoolTrim verifies the scheduler wrapper does
// not execute physical Pool trim directly. Trim remains partition/Pool-local
// control-plane work.
func TestPoolGroupSchedulerDoesNotCallPoolTrim(t *testing.T) {
	facts := parsePolicyPublicationASTFacts(t, "group_scheduler.go")
	for _, forbidden := range []string{"Trim", "TrimClass", "TrimShard"} {
		if facts.hasCall(forbidden) || facts.hasSelector(forbidden) {
			t.Fatalf("group_scheduler.go AST references %q; group scheduler must not execute Pool trim", forbidden)
		}
	}
}

// TestPoolGroupSchedulerCallsOnlyTickIntoAST verifies the scheduler wrapper
// stays a thin owner dispatch path. Group coordinator logic and partition target
// publication remain behind TickInto.
func TestPoolGroupSchedulerCallsOnlyTickIntoAST(t *testing.T) {
	facts := parsePolicyPublicationASTFacts(t, "group_scheduler.go")
	if !facts.hasCall("TickInto") {
		t.Fatal("group_scheduler.go must call TickInto")
	}
	for _, forbidden := range []string{
		"Get",
		"Put",
		"Trim",
		"TrimClass",
		"TrimShard",
		"PublishPolicy",
		"PublishPressure",
		"Acquire",
		"Release",
		"classState",
		"bucket",
		"shard",
		"shards",
		"mustClassStateFor",
	} {
		if facts.hasCall(forbidden) || facts.hasSelector(forbidden) {
			t.Fatalf("group_scheduler.go AST references %q; scheduler wrapper must dispatch only through TickInto", forbidden)
		}
	}
}

// TestPoolGroupSchedulerDoesNotStartPartitionSchedulers verifies group
// scheduler opt-in does not cascade into partition scheduler opt-in. Each
// partition remains governed by its own PartitionPolicy.Controller.Enabled.
func TestPoolGroupSchedulerDoesNotStartPartitionSchedulers(t *testing.T) {
	group, _ := newScheduledGroupWithManualTicker(t, time.Second, "alpha", "beta")

	for _, entry := range group.registry.entries {
		if entry.partition.controllerScheduler.isRunning() {
			t.Fatalf("partition %q scheduler is running; group scheduler must not start child schedulers", entry.name)
		}
	}
}

// TestPoolGroupPublishPolicyDoesNotSilentlyStartScheduler verifies that
// PublishPolicy rejects a live enable attempt instead of starting goroutines
// after construction.
func TestPoolGroupPublishPolicyDoesNotSilentlyStartScheduler(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	policy := group.Policy()
	policy.Coordinator.Enabled = true
	policy.Coordinator.TickInterval = defaultGroupCoordinatorTickInterval

	result, err := group.PublishPolicy(policy)
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.FailureReason != policyUpdateFailureSchedulerChange || result.RuntimePublished {
		t.Fatalf("PublishPolicy(enable scheduler) = %+v, want scheduler-change rejection before runtime publication", result)
	}
	if group.coordinatorScheduler.isRunning() {
		t.Fatal("PublishPolicy scheduler enable change started scheduler")
	}
}

// TestPoolGroupPublishPolicyRejectsSchedulerEnableChange pins the compatibility
// decision separately from the runtime side-effect assertion: scheduler
// enablement is construction-time policy in this stage.
func TestPoolGroupPublishPolicyRejectsSchedulerEnableChange(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	policy := group.Policy()
	policy.Coordinator.Enabled = true
	policy.Coordinator.TickInterval = defaultGroupCoordinatorTickInterval

	result, err := group.PublishPolicy(policy)
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.FailureReason != policyUpdateFailureSchedulerChange || result.RuntimePublished {
		t.Fatalf("PublishPolicy(enable scheduler) = %+v, want scheduler-change rejection before runtime publication", result)
	}
}

// TestPoolGroupPublishPolicyRejectsSchedulerModeChange verifies any live
// scheduler mode change is rejected before runtime publication.
func TestPoolGroupPublishPolicyRejectsSchedulerModeChange(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	policy := group.Policy()
	policy.Coordinator.Enabled = true
	policy.Coordinator.TickInterval = defaultGroupCoordinatorTickInterval

	result, err := group.PublishPolicy(policy)
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.FailureReason != policyUpdateFailureSchedulerChange || result.RuntimePublished {
		t.Fatalf("PublishPolicy(mode change) = %+v, want scheduler-change rejection before runtime publication", result)
	}
}

// TestPoolGroupPublishPolicyDoesNotStartScheduler is the final-stage name for
// the live-enable side-effect invariant.
func TestPoolGroupPublishPolicyDoesNotStartScheduler(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	policy := group.Policy()
	policy.Coordinator.Enabled = true
	policy.Coordinator.TickInterval = defaultGroupCoordinatorTickInterval

	_, err := group.PublishPolicy(policy)
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	assertPoolGroupSchedulerStopped(t, group, "PublishPolicy rejected enable")
}

// TestPoolGroupPublishPolicyRejectsSchedulerIntervalChange verifies that a
// running group scheduler cannot be retimed through live policy publication in
// this stage.
func TestPoolGroupPublishPolicyRejectsSchedulerIntervalChange(t *testing.T) {
	group, _ := newScheduledGroupWithManualTicker(t, time.Second, "alpha")
	policy := group.Policy()
	policy.Coordinator.TickInterval = 2 * time.Second

	result, err := group.PublishPolicy(policy)
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.FailureReason != policyUpdateFailureSchedulerChange || result.RuntimePublished {
		t.Fatalf("PublishPolicy(interval change) = %+v, want scheduler-change rejection before runtime publication", result)
	}
	if !group.coordinatorScheduler.isRunning() {
		t.Fatal("PublishPolicy interval change stopped existing scheduler")
	}
}

// TestPoolGroupPublishPolicyDoesNotSilentlyStopScheduler verifies that
// PublishPolicy rejects a live disable attempt and leaves the construction-time
// scheduler state unchanged.
func TestPoolGroupPublishPolicyDoesNotSilentlyStopScheduler(t *testing.T) {
	group, _ := newScheduledGroupWithManualTicker(t, time.Second, "alpha")
	policy := group.Policy()
	policy.Coordinator = PoolGroupCoordinatorPolicy{}

	result, err := group.PublishPolicy(policy)
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.FailureReason != policyUpdateFailureSchedulerChange || result.RuntimePublished {
		t.Fatalf("PublishPolicy(disable scheduler) = %+v, want scheduler-change rejection before runtime publication", result)
	}
	if !group.coordinatorScheduler.isRunning() {
		t.Fatal("PublishPolicy scheduler disable change stopped scheduler")
	}
}

// TestPoolGroupPublishPolicyDoesNotStopScheduler is the final-stage name for
// the live-disable side-effect invariant.
func TestPoolGroupPublishPolicyDoesNotStopScheduler(t *testing.T) {
	group, _ := newScheduledGroupWithManualTicker(t, time.Second, "alpha")
	policy := group.Policy()
	policy.Coordinator = PoolGroupCoordinatorPolicy{}

	_, err := group.PublishPolicy(policy)
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if !group.coordinatorScheduler.isRunning() {
		t.Fatal("PublishPolicy scheduler disable rejection stopped scheduler")
	}
}

// TestPoolGroupSchedulerDoesNotRetainFullReports is the final-stage plural name
// for the report-retention invariant.
func TestPoolGroupSchedulerDoesNotRetainFullReports(t *testing.T) {
	group, ticker := newScheduledGroupWithManualTicker(t, time.Second, "alpha")

	ticker.tick()
	_ = waitForGroupSchedulerStatus(t, group, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusSkipped
	})
	assertPoolGroupDoesNotHaveReportFields(t)
}

// BenchmarkPoolGroupSchedulerDisabledConstruction measures the default
// constructor path where scheduler policy is disabled and no scheduler goroutine
// is started.
func BenchmarkPoolGroupSchedulerDisabledConstruction(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		group, err := NewPoolGroup(testGroupConfig("alpha"))
		if err != nil {
			b.Fatalf("NewPoolGroup() error = %v", err)
		}
		if err := group.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

// BenchmarkPoolGroupSchedulerManualTickDispatch measures one manual-ticker
// dispatch through the opt-in scheduler plus the retained status observation
// needed to confirm the tick completed.
func BenchmarkPoolGroupSchedulerManualTickDispatch(b *testing.B) {
	group, ticker := newScheduledGroupWithManualTicker(b, time.Second, "alpha")
	defer func() { requireGroupSchedulerNoError(b, group.Close()) }()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		before := group.ControllerStatus().AttemptGeneration
		ticker.tick()
		_ = waitForGroupSchedulerStatus(b, group, func(status ControllerCycleStatusSnapshot) bool {
			return status.Status == ControllerCycleStatusSkipped &&
				status.AttemptGeneration.After(before)
		})
	}
}

// BenchmarkPoolGroupSchedulerStop measures construction of an enabled group
// followed by scheduler-aware Close. It keeps the measured unit explicit because
// Stop is owned by Close in production.
func BenchmarkPoolGroupSchedulerStop(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		group, _ := newScheduledGroupWithManualTickerNoCleanup(b, time.Second, "alpha")
		if err := group.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

// BenchmarkPoolGroupSchedulerStatusReadWhileRunning measures the retained
// lightweight ControllerStatus accessor while the opt-in scheduler runtime is
// active.
func BenchmarkPoolGroupSchedulerStatusReadWhileRunning(b *testing.B) {
	group, _ := newScheduledGroupWithManualTicker(b, time.Second, "alpha")
	defer func() { requireGroupSchedulerNoError(b, group.Close()) }()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		controllerStatusBenchmarkSink = group.ControllerStatus()
	}
}

// groupSchedulerTest is the shared test/benchmark surface needed by scheduler
// helpers. It keeps helper code deterministic without depending on *testing.T
// specifically.
type groupSchedulerTest interface {
	Helper()
	Fatalf(string, ...any)
	Fatal(...any)
	Cleanup(func())
	Context() context.Context
}

// schedulerGroupConfig returns a minimal group config with opt-in scheduler
// policy enabled. A zero interval intentionally exercises Normalize's default
// cadence path.
func schedulerGroupConfig(interval time.Duration, partitionNames ...string) PoolGroupConfig {
	config := testGroupConfig(partitionNames...)
	config.Policy.Coordinator.Enabled = true
	config.Policy.Coordinator.TickInterval = interval
	return config
}

// newScheduledGroupWithManualTicker constructs an enabled group and registers
// Close cleanup. Tests use the returned manual ticker to drive scheduled ticks
// without real timers.
func newScheduledGroupWithManualTicker(
	t groupSchedulerTest,
	interval time.Duration,
	partitionNames ...string,
) (*PoolGroup, *manualControllerSchedulerTicker) {
	t.Helper()

	group, ticker := newScheduledGroupWithManualTickerNoCleanup(t, interval, partitionNames...)
	t.Cleanup(func() {
		requireGroupSchedulerNoError(t, group.Close())
	})

	return group, ticker
}

// newScheduledGroupWithManualTickerNoCleanup constructs an enabled group
// without registering cleanup. Benchmarks use it when Close is the measured
// operation.
func newScheduledGroupWithManualTickerNoCleanup(
	t groupSchedulerTest,
	interval time.Duration,
	partitionNames ...string,
) (*PoolGroup, *manualControllerSchedulerTicker) {
	t.Helper()

	ticker := newManualControllerSchedulerTicker()
	group, err := newPoolGroupWithCoordinatorSchedulerTickerFactory(
		schedulerGroupConfig(interval, partitionNames...),
		func(time.Duration) controllerSchedulerTicker {
			return ticker
		},
	)
	requireGroupSchedulerNoError(t, err)

	return group, ticker
}

// waitForGroupSchedulerStatus spins cooperatively until the retained
// ControllerStatus matches the caller's predicate. It avoids sleeps because the
// manual ticker and t.Context cancellation make the wait deterministic.
func waitForGroupSchedulerStatus(
	t groupSchedulerTest,
	group *PoolGroup,
	matches func(ControllerCycleStatusSnapshot) bool,
) ControllerCycleStatusSnapshot {
	t.Helper()

	for {
		status := group.ControllerStatus()
		if matches(status) {
			return status
		}

		select {
		case <-t.Context().Done():
			t.Fatalf("timed out waiting for group scheduler status; last status = %+v", status)
		default:
			runtime.Gosched()
		}
	}
}

// requireGroupSchedulerValue receives one integer diagnostic from a helper
// channel and fails through the shared testing surface if the value is not
// delivered before the test context ends.
func requireGroupSchedulerValue(t groupSchedulerTest, values <-chan int, name string) int {
	t.Helper()

	select {
	case value := <-values:
		return value
	case <-t.Context().Done():
		t.Fatalf("timed out waiting for %s", name)
		return 0
	}
}

// requireGroupSchedulerError receives one error result from an asynchronous
// scheduler/close helper and reports deterministic timeout failures.
func requireGroupSchedulerError(t groupSchedulerTest, values <-chan error, name string) error {
	t.Helper()

	select {
	case value := <-values:
		return value
	case <-t.Context().Done():
		t.Fatalf("timed out waiting for %s", name)
		return nil
	}
}

func requireGroupSchedulerStatusValue(
	t groupSchedulerTest,
	values <-chan ControllerCycleStatusSnapshot,
	name string,
) ControllerCycleStatusSnapshot {
	t.Helper()

	select {
	case value := <-values:
		return value
	case <-t.Context().Done():
		t.Fatalf("timed out waiting for %s", name)
		return ControllerCycleStatusSnapshot{}
	}
}

func waitForGroupSchedulerRuntimeStopped(t groupSchedulerTest, group *PoolGroup, name string) {
	t.Helper()

	for {
		if !group.coordinatorScheduler.isRunning() {
			return
		}

		select {
		case <-t.Context().Done():
			t.Fatalf("timed out waiting for %s scheduler stop", name)
		default:
			runtime.Gosched()
		}
	}
}

// requireGroupSchedulerNoError is the scheduler-test equivalent of the group
// test helper, generalized for both *testing.T and *testing.B.
func requireGroupSchedulerNoError(t groupSchedulerTest, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected group scheduler error: %v", err)
	}
}

func assertPoolGroupSchedulerStopped(t *testing.T, group *PoolGroup, name string) {
	t.Helper()
	if group.coordinatorScheduler.isRunning() {
		t.Fatalf("%s unexpectedly has running coordinator scheduler", name)
	}
}

func assertPoolGroupDoesNotHaveReportFields(t *testing.T) {
	t.Helper()

	assertTypeHasNoReportRetentionFields(
		t,
		reflect.TypeOf(PoolGroup{}),
		"PoolGroup",
		map[string]struct{}{"scoreEvaluator": {}},
	)
}
