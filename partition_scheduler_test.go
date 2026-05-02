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
	"strings"
	"testing"
	"time"
)

// TestPoolPartitionSchedulerDisabledByDefault pins the default partition mode
// as manual-only. A newly constructed partition must not start the scheduler
// unless policy opts in explicitly.
func TestPoolPartitionSchedulerDisabledByDefault(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	if partition.controllerScheduler.isRunning() {
		t.Fatal("default partition unexpectedly started controller scheduler")
	}
	if policy := partition.Policy().Controller; policy.Enabled || policy.TickInterval != 0 {
		t.Fatalf("default controller policy = %+v, want manual scheduler-free policy", policy)
	}
}

// TestPoolPartitionSchedulerPolicyEnablesScheduler verifies that an enabled
// controller policy starts the owner-local scheduler after construction has
// completed all partition initialization.
func TestPoolPartitionSchedulerPolicyEnablesScheduler(t *testing.T) {
	partition, _ := newScheduledPartitionWithManualTicker(t, time.Second, "primary")

	if !partition.controllerScheduler.isRunning() {
		t.Fatal("enabled partition controller policy did not start scheduler")
	}
}

// TestPoolPartitionSchedulerRejectsIntervalWithoutEnabled keeps disabled policy
// honest: a dormant interval would imply automatic work that the partition will
// not start.
func TestPoolPartitionSchedulerRejectsIntervalWithoutEnabled(t *testing.T) {
	policy := DefaultPartitionPolicy()
	policy.Controller.TickInterval = time.Second

	err := policy.Validate()
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)
}

// TestPoolPartitionSchedulerUsesDefaultIntervalWhenEnabled proves that
// Normalize completes the scheduler cadence only for explicit opt-in policy and
// that the constructor passes the same cadence to the runtime ticker.
func TestPoolPartitionSchedulerUsesDefaultIntervalWhenEnabled(t *testing.T) {
	ticker := newManualControllerSchedulerTicker()
	var observed time.Duration
	config := schedulerPartitionConfig(0, "primary")

	partition, err := newPoolPartitionWithControllerSchedulerTickerFactory(config, func(interval time.Duration) controllerSchedulerTicker {
		observed = interval
		return ticker
	})
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	if observed != defaultPartitionControllerTickInterval {
		t.Fatalf("scheduler interval = %s, want default %s", observed, defaultPartitionControllerTickInterval)
	}
	if policy := partition.Policy().Controller; policy.TickInterval != defaultPartitionControllerTickInterval {
		t.Fatalf("runtime controller policy = %+v, want default interval", policy)
	}
}

// TestPoolPartitionSchedulerRunsTick drives the scheduler with a manual ticker
// and verifies that the scheduled path reaches the same applied status as a
// foreground TickInto call.
func TestPoolPartitionSchedulerRunsTick(t *testing.T) {
	partition, ticker := newScheduledPartitionWithManualTicker(t, time.Second, "primary")

	ticker.tick()
	status := waitForPartitionSchedulerStatus(t, partition, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusApplied
	})

	if status.AttemptGeneration.IsZero() || status.AppliedGeneration != status.AttemptGeneration {
		t.Fatalf("scheduled tick status = %+v, want applied generation", status)
	}
}

// TestPoolPartitionSchedulerUpdatesControllerStatus verifies that scheduler
// dispatch publishes the retained lightweight ControllerStatus instead of
// retaining the full report.
func TestPoolPartitionSchedulerUpdatesControllerStatus(t *testing.T) {
	partition, ticker := newScheduledPartitionWithManualTicker(t, time.Second, "primary")

	ticker.tick()
	first := waitForPartitionSchedulerStatus(t, partition, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusApplied
	})
	ticker.tick()
	second := waitForPartitionSchedulerStatus(t, partition, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusApplied &&
			status.AttemptGeneration.After(first.AttemptGeneration)
	})

	if second.LastSuccessfulGeneration != second.AppliedGeneration {
		t.Fatalf("second scheduled status = %+v, want last success to track applied generation", second)
	}
}

// TestPoolPartitionSchedulerDoesNotRetainFullReport checks the owner shape
// after a scheduled tick. The scheduler must remain a dispatch mechanism; heavy
// reports stay caller-owned or stack-local inside schedulerTick.
func TestPoolPartitionSchedulerDoesNotRetainFullReport(t *testing.T) {
	partition, ticker := newScheduledPartitionWithManualTicker(t, time.Second, "primary")

	ticker.tick()
	_ = waitForPartitionSchedulerStatus(t, partition, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusApplied
	})

	// The partition scheduler may retain only the lightweight runtime and the
	// controller status already owned by partitionController. Full TickInto
	// reports, samples, windows, score slices, and trim candidates must stay
	// caller-owned or stack-local to schedulerTick.
	partitionType := reflect.TypeOf(PoolPartition{})
	for index := 0; index < partitionType.NumField(); index++ {
		field := partitionType.Field(index)
		if strings.Contains(strings.ToLower(field.Name), "report") {
			t.Fatalf("PoolPartition field %q suggests scheduler report retention", field.Name)
		}
	}
}

// TestPoolPartitionSchedulerContinuesAfterTickFailure verifies the internal
// scheduler runtime treats non-closed Tick errors as already-reported
// diagnostics and keeps dispatching later ticks.
func TestPoolPartitionSchedulerContinuesAfterTickFailure(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	ticker := newManualControllerSchedulerTicker()
	ticks := make(chan int, 2)
	var calls int

	// Ordinary partition TickInto errors that are not closed-owner errors are
	// difficult to trigger without corrupting internal indexes. This test starts
	// the same owner-local scheduler runtime with a Tick wrapper that performs a
	// real schedulerTick and then returns a synthetic non-closed error. The
	// scheduler must ignore that error class and dispatch the next tick.
	requireControllerSchedulerNoError(t, partition.controllerScheduler.Start(controllerSchedulerStartInput{
		Interval: time.Second,
		Tick: func() error {
			calls++
			err := partition.schedulerTick()
			ticks <- calls
			if err != nil {
				return err
			}
			return errors.New("transient partition scheduler test error")
		},
		IsClosedError: func(err error) bool { return errors.Is(err, ErrClosed) },
		TickerFactory: func(time.Duration) controllerSchedulerTicker {
			return ticker
		},
	}))
	t.Cleanup(partition.stopControllerScheduler)

	ticker.tick()
	if got := requirePartitionSchedulerValue(t, ticks, "first scheduled failure"); got != 1 {
		t.Fatalf("first scheduled tick = %d, want 1", got)
	}
	ticker.tick()
	if got := requirePartitionSchedulerValue(t, ticks, "second scheduled failure"); got != 2 {
		t.Fatalf("second scheduled tick = %d, want 2 after transient failure", got)
	}
}

// TestPoolPartitionSchedulerStopsOnClose verifies Close stops the scheduler
// before partition child cleanup can proceed.
func TestPoolPartitionSchedulerStopsOnClose(t *testing.T) {
	partition, ticker := newScheduledPartitionWithManualTicker(t, time.Second, "primary")

	requirePartitionNoError(t, partition.Close())

	if partition.controllerScheduler.isRunning() {
		t.Fatal("scheduler still running after partition Close")
	}
	assertManualControllerSchedulerTickerStopped(t, ticker)
}

// TestPoolPartitionSchedulerCloseIsIdempotent keeps scheduler shutdown aligned
// with PoolPartition.Close: repeated hard-close calls must not panic or leave a
// running scheduler behind.
func TestPoolPartitionSchedulerCloseIsIdempotent(t *testing.T) {
	partition, ticker := newScheduledPartitionWithManualTicker(t, time.Second, "primary")

	requirePartitionNoError(t, partition.Close())
	requirePartitionNoError(t, partition.Close())

	if partition.controllerScheduler.isRunning() {
		t.Fatal("scheduler still running after repeated Close")
	}
	assertManualControllerSchedulerTickerStopped(t, ticker)
}

// TestPoolPartitionSchedulerCloseWhileTickRunningDoesNotDeadlock pins the close
// lock order. Close waits for the scheduler without holding foreground locks
// that the in-flight scheduled TickInto may need.
func TestPoolPartitionSchedulerCloseWhileTickRunningDoesNotDeadlock(t *testing.T) {
	partition, ticker := newScheduledPartitionWithManualTicker(t, time.Second, "primary")

	partition.controller.mu.Lock()
	ticker.tick()
	waitForControllerCycleGate(t, "partition scheduler", partition.controller.cycleGate.isRunning)

	closed := make(chan error, 1)
	go func() {
		closed <- partition.Close()
	}()

	partition.controller.mu.Unlock()
	requirePartitionNoError(t, requirePartitionSchedulerError(t, closed, "Close while scheduled tick is running"))
	if partition.controllerScheduler.isRunning() {
		t.Fatal("scheduler still running after Close raced with scheduled tick")
	}
}

// TestPoolPartitionSchedulerManualTickMayOverlapAndIsRejected proves scheduled
// ticks and manual ticks share the same controller cycle gate. The second
// foreground attempt is reported as AlreadyRunning rather than running a second
// controller cycle.
func TestPoolPartitionSchedulerManualTickMayOverlapAndIsRejected(t *testing.T) {
	partition, ticker := newScheduledPartitionWithManualTicker(t, time.Second, "primary")

	partition.controller.mu.Lock()
	ticker.tick()
	waitForControllerCycleGate(t, "partition scheduler", partition.controller.cycleGate.isRunning)

	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))
	if report.Status.Status != ControllerCycleStatusAlreadyRunning {
		t.Fatalf("manual TickInto during scheduled tick status = %+v, want already running", report.Status)
	}

	partition.controller.mu.Unlock()
	_ = waitForPartitionSchedulerStatus(t, partition, func(status ControllerCycleStatusSnapshot) bool {
		return status.Status == ControllerCycleStatusApplied
	})
}

// TestPoolPartitionSchedulerDoesNotTouchPoolGetPutHotPath extends the AST
// hot-path boundary checks with scheduler-specific symbols. The opt-in
// scheduler is partition control-plane code and must not appear in Pool.Get or
// Pool.Put.
func TestPoolPartitionSchedulerDoesNotTouchPoolGetPutHotPath(t *testing.T) {
	for _, file := range []string{"pool_get.go", "pool_put.go"} {
		facts := parsePolicyPublicationASTFacts(t, file)
		for _, forbidden := range []string{
			"controllerSchedulerRuntime",
			"scheduler",
			"Ticker",
			"TickInterval",
			"ControllerStatus",
			"TickInto",
			"cycleGate",
			"EWMA",
			"activeRegistry",
		} {
			if facts.hasIdentifier(forbidden) || facts.hasSelector(forbidden) || facts.hasCall(forbidden) {
				t.Fatalf("%s AST references %q; Pool.Get/Put must not depend on partition scheduler code", file, forbidden)
			}
		}
	}
}

// TestPoolPartitionPublishPolicyDoesNotSilentlyStartScheduler verifies that
// PublishPolicy rejects a live enable attempt instead of starting goroutines
// after construction.
func TestPoolPartitionPublishPolicyDoesNotSilentlyStartScheduler(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	policy := partition.Policy()
	policy.Controller.Enabled = true
	policy.Controller.TickInterval = defaultPartitionControllerTickInterval

	result, err := partition.PublishPolicy(policy)
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)
	if result.FailureReason != policyUpdateFailureSchedulerChange || result.RuntimePublished {
		t.Fatalf("PublishPolicy(enable scheduler) = %+v, want scheduler-change rejection before runtime publication", result)
	}
	if partition.controllerScheduler.isRunning() {
		t.Fatal("PublishPolicy scheduler enable change started scheduler")
	}
}

// TestPoolPartitionPublishPolicyRejectsSchedulerEnableChange pins the
// compatibility decision separately from the runtime side-effect assertion:
// scheduler enablement is construction-time policy in this stage.
func TestPoolPartitionPublishPolicyRejectsSchedulerEnableChange(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	policy := partition.Policy()
	policy.Controller.Enabled = true
	policy.Controller.TickInterval = defaultPartitionControllerTickInterval

	result, err := partition.PublishPolicy(policy)
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)
	if result.FailureReason != policyUpdateFailureSchedulerChange || result.RuntimePublished {
		t.Fatalf("PublishPolicy(enable scheduler) = %+v, want scheduler-change rejection before runtime publication", result)
	}
}

// TestPoolPartitionPublishPolicyRejectsSchedulerIntervalChange verifies that a
// running partition scheduler cannot be retimed through live policy publication
// in this stage.
func TestPoolPartitionPublishPolicyRejectsSchedulerIntervalChange(t *testing.T) {
	partition, _ := newScheduledPartitionWithManualTicker(t, time.Second, "primary")
	policy := partition.Policy()
	policy.Controller.TickInterval = 2 * time.Second

	result, err := partition.PublishPolicy(policy)
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)
	if result.FailureReason != policyUpdateFailureSchedulerChange || result.RuntimePublished {
		t.Fatalf("PublishPolicy(interval change) = %+v, want scheduler-change rejection before runtime publication", result)
	}
	if !partition.controllerScheduler.isRunning() {
		t.Fatal("PublishPolicy interval change stopped existing scheduler")
	}
}

// TestPoolPartitionPublishPolicyDoesNotSilentlyStopScheduler verifies that
// PublishPolicy rejects a live disable attempt and leaves the construction-time
// scheduler state unchanged.
func TestPoolPartitionPublishPolicyDoesNotSilentlyStopScheduler(t *testing.T) {
	partition, _ := newScheduledPartitionWithManualTicker(t, time.Second, "primary")
	policy := partition.Policy()
	policy.Controller = PartitionControllerPolicy{}

	result, err := partition.PublishPolicy(policy)
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)
	if result.FailureReason != policyUpdateFailureSchedulerChange || result.RuntimePublished {
		t.Fatalf("PublishPolicy(disable scheduler) = %+v, want scheduler-change rejection before runtime publication", result)
	}
	if !partition.controllerScheduler.isRunning() {
		t.Fatal("PublishPolicy scheduler disable change stopped scheduler")
	}
}

// BenchmarkPoolPartitionSchedulerDisabledConstruction measures the default
// constructor path where scheduler policy is disabled and no scheduler goroutine
// is started.
func BenchmarkPoolPartitionSchedulerDisabledConstruction(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		partition, err := NewPoolPartition(testPartitionConfig("primary"))
		if err != nil {
			b.Fatalf("NewPoolPartition() error = %v", err)
		}
		if err := partition.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

// BenchmarkPoolPartitionSchedulerManualTickDispatch measures one manual-ticker
// dispatch through the opt-in scheduler plus the retained status observation
// needed to confirm the tick was applied.
func BenchmarkPoolPartitionSchedulerManualTickDispatch(b *testing.B) {
	partition, ticker := newScheduledPartitionWithManualTicker(b, time.Second, "primary")
	defer func() { requirePartitionSchedulerNoError(b, partition.Close()) }()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		before := partition.ControllerStatus().AttemptGeneration
		ticker.tick()
		_ = waitForPartitionSchedulerStatus(b, partition, func(status ControllerCycleStatusSnapshot) bool {
			return status.Status == ControllerCycleStatusApplied &&
				status.AttemptGeneration.After(before)
		})
	}
}

// BenchmarkPoolPartitionSchedulerStop measures construction of an enabled
// partition followed by scheduler-aware Close. It keeps the measured unit
// explicit because Stop is owned by Close in production.
func BenchmarkPoolPartitionSchedulerStop(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		partition, _ := newScheduledPartitionWithManualTickerNoCleanup(b, time.Second, "primary")
		if err := partition.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

// BenchmarkPoolPartitionSchedulerStatusReadWhileRunning measures the retained
// lightweight ControllerStatus accessor while the opt-in scheduler runtime is
// active.
func BenchmarkPoolPartitionSchedulerStatusReadWhileRunning(b *testing.B) {
	partition, _ := newScheduledPartitionWithManualTicker(b, time.Second, "primary")
	defer func() { requirePartitionSchedulerNoError(b, partition.Close()) }()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		controllerStatusBenchmarkSink = partition.ControllerStatus()
	}
}

// partitionSchedulerTest is the shared test/benchmark surface needed by
// scheduler helpers. It keeps helper code deterministic without depending on
// *testing.T specifically.
type partitionSchedulerTest interface {
	Helper()
	Fatalf(string, ...any)
	Fatal(...any)
	Cleanup(func())
	Context() context.Context
}

// schedulerPartitionConfig returns a minimal partition config with opt-in
// scheduler policy enabled. A zero interval intentionally exercises Normalize's
// default cadence path.
func schedulerPartitionConfig(interval time.Duration, poolNames ...string) PoolPartitionConfig {
	config := testPartitionConfig(poolNames...)
	config.Policy.Controller.Enabled = true
	config.Policy.Controller.TickInterval = interval
	return config
}

// newScheduledPartitionWithManualTicker constructs an enabled partition and
// registers Close cleanup. Tests use the returned manual ticker to drive
// scheduled ticks without real timers.
func newScheduledPartitionWithManualTicker(
	t partitionSchedulerTest,
	interval time.Duration,
	poolNames ...string,
) (*PoolPartition, *manualControllerSchedulerTicker) {
	t.Helper()

	partition, ticker := newScheduledPartitionWithManualTickerNoCleanup(t, interval, poolNames...)
	t.Cleanup(func() {
		requirePartitionSchedulerNoError(t, partition.Close())
	})

	return partition, ticker
}

// newScheduledPartitionWithManualTickerNoCleanup constructs an enabled
// partition without registering cleanup. Benchmarks use it when Close is the
// measured operation.
func newScheduledPartitionWithManualTickerNoCleanup(
	t partitionSchedulerTest,
	interval time.Duration,
	poolNames ...string,
) (*PoolPartition, *manualControllerSchedulerTicker) {
	t.Helper()

	ticker := newManualControllerSchedulerTicker()
	partition, err := newPoolPartitionWithControllerSchedulerTickerFactory(
		schedulerPartitionConfig(interval, poolNames...),
		func(time.Duration) controllerSchedulerTicker {
			return ticker
		},
	)
	requirePartitionSchedulerNoError(t, err)

	return partition, ticker
}

// waitForPartitionSchedulerStatus spins cooperatively until the retained
// ControllerStatus matches the caller's predicate. It avoids sleeps because the
// manual ticker and t.Context cancellation make the wait deterministic.
func waitForPartitionSchedulerStatus(
	t partitionSchedulerTest,
	partition *PoolPartition,
	matches func(ControllerCycleStatusSnapshot) bool,
) ControllerCycleStatusSnapshot {
	t.Helper()

	for {
		status := partition.ControllerStatus()
		if matches(status) {
			return status
		}

		select {
		case <-t.Context().Done():
			t.Fatalf("timed out waiting for partition scheduler status; last status = %+v", status)
		default:
			runtime.Gosched()
		}
	}
}

// requirePartitionSchedulerValue receives one integer diagnostic from a helper
// channel and fails through the shared testing surface if the value is not
// delivered before the test context ends.
func requirePartitionSchedulerValue(t partitionSchedulerTest, values <-chan int, name string) int {
	t.Helper()

	select {
	case value := <-values:
		return value
	case <-t.Context().Done():
		t.Fatalf("timed out waiting for %s", name)
		return 0
	}
}

// requirePartitionSchedulerError receives one error result from an asynchronous
// scheduler/close helper and reports deterministic timeout failures.
func requirePartitionSchedulerError(t partitionSchedulerTest, values <-chan error, name string) error {
	t.Helper()

	select {
	case value := <-values:
		return value
	case <-t.Context().Done():
		t.Fatalf("timed out waiting for %s", name)
		return nil
	}
}

// requirePartitionSchedulerNoError is the scheduler-test equivalent of the
// partition test helper, generalized for both *testing.T and *testing.B.
func requirePartitionSchedulerNoError(t partitionSchedulerTest, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected partition scheduler error: %v", err)
	}
}
