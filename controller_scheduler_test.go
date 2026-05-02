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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestControllerSchedulerRuntimeStopBeforeStart(t *testing.T) {
	var runtime controllerSchedulerRuntime

	runtime.Stop()

	assertControllerSchedulerStopped(t, &runtime)
}

func TestControllerSchedulerRuntimeStartStop(t *testing.T) {
	var runtime controllerSchedulerRuntime
	ticker := newManualControllerSchedulerTicker()

	requireControllerSchedulerNoError(t, runtime.Start(controllerSchedulerTestInput(ticker, func() error {
		return nil
	})))
	if !runtime.isRunning() {
		t.Fatal("scheduler should report running after Start")
	}

	runtime.Stop()

	assertControllerSchedulerStopped(t, &runtime)
	assertManualControllerSchedulerTickerStopped(t, ticker)
}

func TestControllerSchedulerRuntimeStartRejectsInvalidInterval(t *testing.T) {
	var runtime controllerSchedulerRuntime

	err := runtime.Start(controllerSchedulerStartInput{
		Interval:      0,
		Tick:          func() error { return nil },
		TickerFactory: func(time.Duration) controllerSchedulerTicker { return newManualControllerSchedulerTicker() },
	})

	requireControllerSchedulerErrorIs(t, err, ErrInvalidOptions)
	if !strings.Contains(err.Error(), errControllerSchedulerInvalidInterval) {
		t.Fatalf("Start(invalid interval) error = %v, want stable reason %q", err, errControllerSchedulerInvalidInterval)
	}
	assertControllerSchedulerStopped(t, &runtime)
}

func TestControllerSchedulerRuntimeStartWhileRunning(t *testing.T) {
	var runtime controllerSchedulerRuntime
	var factoryCalls atomic.Int64
	ticker := newManualControllerSchedulerTicker()
	input := controllerSchedulerStartInput{
		Interval: time.Second,
		Tick:     func() error { return nil },
		TickerFactory: func(time.Duration) controllerSchedulerTicker {
			factoryCalls.Add(1)
			return ticker
		},
	}

	requireControllerSchedulerNoError(t, runtime.Start(input))
	err := runtime.Start(input)
	runtime.Stop()

	requireControllerSchedulerErrorIs(t, err, ErrInvalidOptions)
	if !strings.Contains(err.Error(), errControllerSchedulerAlreadyRunning) {
		t.Fatalf("Start(already running) error = %v, want stable reason %q", err, errControllerSchedulerAlreadyRunning)
	}
	if calls := factoryCalls.Load(); calls != 1 {
		t.Fatalf("ticker factory calls = %d, want 1 so rejected Start does not allocate a second ticker", calls)
	}
}

func TestControllerSchedulerRuntimeStopIsIdempotent(t *testing.T) {
	var runtime controllerSchedulerRuntime
	ticker := newManualControllerSchedulerTicker()

	runtime.Stop()
	requireControllerSchedulerNoError(t, runtime.Start(controllerSchedulerTestInput(ticker, func() error {
		return nil
	})))
	runtime.Stop()
	runtime.Stop()

	assertControllerSchedulerStopped(t, &runtime)
	assertManualControllerSchedulerTickerStopped(t, ticker)
}

func TestControllerSchedulerRuntimeRunsTickOnManualTicker(t *testing.T) {
	var runtime controllerSchedulerRuntime
	ticker := newManualControllerSchedulerTicker()
	ticked := make(chan struct{}, 1)

	requireControllerSchedulerNoError(t, runtime.Start(controllerSchedulerTestInput(ticker, func() error {
		ticked <- struct{}{}
		return nil
	})))
	ticker.tick()
	requireControllerSchedulerSignal(t, ticked, "manual tick dispatch")

	runtime.Stop()
}

func TestControllerSchedulerRuntimeContinuesAfterTickError(t *testing.T) {
	var runtime controllerSchedulerRuntime
	ticker := newManualControllerSchedulerTicker()
	ticks := make(chan int64, 2)
	var calls atomic.Int64

	requireControllerSchedulerNoError(t, runtime.Start(controllerSchedulerTestInput(ticker, func() error {
		call := calls.Add(1)
		ticks <- call
		if call == 1 {
			return errors.New("transient controller diagnostic")
		}
		return nil
	})))
	ticker.tick()
	if got := requireControllerSchedulerValue(t, ticks, "first tick"); got != 1 {
		t.Fatalf("first tick value = %d, want 1", got)
	}
	ticker.tick()
	if got := requireControllerSchedulerValue(t, ticks, "second tick"); got != 2 {
		t.Fatalf("second tick value = %d, want 2 after transient error", got)
	}

	runtime.Stop()
}

func TestControllerSchedulerRuntimeStopsAfterClosedError(t *testing.T) {
	var runtime controllerSchedulerRuntime
	ticker := newManualControllerSchedulerTicker()
	ticked := make(chan struct{}, 1)

	requireControllerSchedulerNoError(t, runtime.Start(controllerSchedulerTestInput(ticker, func() error {
		ticked <- struct{}{}
		return newError(ErrClosed, errPoolClosed)
	})))
	ticker.tick()
	requireControllerSchedulerSignal(t, ticked, "closed-error tick")

	runtime.Stop()

	assertControllerSchedulerStopped(t, &runtime)
	assertManualControllerSchedulerTickerStopped(t, ticker)
}

func TestControllerSchedulerRuntimeStopsTicker(t *testing.T) {
	var runtime controllerSchedulerRuntime
	ticker := newManualControllerSchedulerTicker()

	requireControllerSchedulerNoError(t, runtime.Start(controllerSchedulerTestInput(ticker, func() error {
		return nil
	})))
	runtime.Stop()

	assertManualControllerSchedulerTickerStopped(t, ticker)
}

func TestControllerSchedulerRuntimeDoesNotRetainReports(t *testing.T) {
	runtimeType := reflect.TypeOf(controllerSchedulerRuntime{})
	for index := 0; index < runtimeType.NumField(); index++ {
		field := runtimeType.Field(index)
		if strings.Contains(strings.ToLower(field.Name), "report") {
			t.Fatalf("controllerSchedulerRuntime field %q suggests retained report storage", field.Name)
		}
		if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Map {
			t.Fatalf("controllerSchedulerRuntime field %q has type %s; scheduler must not retain report diagnostics", field.Name, field.Type)
		}
	}
}

func TestControllerSchedulerRuntimeDoesNotLeakDoneOnStop(t *testing.T) {
	var runtime controllerSchedulerRuntime
	ticker := newManualControllerSchedulerTicker()

	requireControllerSchedulerNoError(t, runtime.Start(controllerSchedulerTestInput(ticker, func() error {
		return nil
	})))
	runtime.Stop()

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.running || runtime.stopping || runtime.stop != nil || runtime.done != nil {
		t.Fatalf("scheduler state after Stop = running:%v stopping:%v stop:%v done:%v; want fully released",
			runtime.running, runtime.stopping, runtime.stop, runtime.done)
	}
}

func BenchmarkControllerSchedulerRuntimeStartStop(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var runtime controllerSchedulerRuntime
		ticker := newManualControllerSchedulerTicker()
		err := runtime.Start(controllerSchedulerTestInput(ticker, func() error {
			return nil
		}))
		if err != nil {
			b.Fatalf("Start() error = %v", err)
		}
		runtime.Stop()
	}
}

func BenchmarkControllerSchedulerRuntimeManualTickDispatch(b *testing.B) {
	var runtime controllerSchedulerRuntime
	ticker := newManualControllerSchedulerTicker()
	ticked := make(chan struct{}, 1)
	err := runtime.Start(controllerSchedulerTestInput(ticker, func() error {
		ticked <- struct{}{}
		return nil
	}))
	if err != nil {
		b.Fatalf("Start() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ticker.tick()
		<-ticked
	}
	b.StopTimer()
	runtime.Stop()
}

// manualControllerSchedulerTicker is a deterministic ticker for scheduler
// tests. It never starts a timer or goroutine; tests explicitly enqueue ticks
// through tick and observe Stop through stopped.
type manualControllerSchedulerTicker struct {
	ch       chan time.Time
	stopped  chan struct{}
	stopOnce sync.Once
}

func newManualControllerSchedulerTicker() *manualControllerSchedulerTicker {
	return newManualControllerSchedulerTickerWithCapacity(16)
}

func newManualControllerSchedulerTickerWithCapacity(capacity int) *manualControllerSchedulerTicker {
	if capacity < 1 {
		capacity = 1
	}
	return &manualControllerSchedulerTicker{
		ch:      make(chan time.Time, capacity),
		stopped: make(chan struct{}),
	}
}

func (t *manualControllerSchedulerTicker) C() <-chan time.Time {
	return t.ch
}

func (t *manualControllerSchedulerTicker) Stop() {
	t.stopOnce.Do(func() {
		close(t.stopped)
	})
}

func (t *manualControllerSchedulerTicker) tick() {
	select {
	case t.ch <- time.Unix(0, 0):
	case <-t.stopped:
	}
}

func controllerSchedulerTestInput(ticker controllerSchedulerTicker, tick func() error) controllerSchedulerStartInput {
	return controllerSchedulerStartInput{
		Interval:      time.Second,
		Tick:          tick,
		TickerFactory: func(time.Duration) controllerSchedulerTicker { return ticker },
	}
}

func requireControllerSchedulerNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("scheduler error = %v, want nil", err)
	}
}

func requireControllerSchedulerErrorIs(t *testing.T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("scheduler error = %v, want errors.Is %v", err, target)
	}
}

func requireControllerSchedulerSignal(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-t.Context().Done():
		t.Fatalf("timed out waiting for %s", name)
	}
}

func requireControllerSchedulerValue(t *testing.T, ch <-chan int64, name string) int64 {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-t.Context().Done():
		t.Fatalf("timed out waiting for %s", name)
		return 0
	}
}

func assertControllerSchedulerStopped(t *testing.T, runtime *controllerSchedulerRuntime) {
	t.Helper()
	if runtime.isRunning() {
		t.Fatal("scheduler should be stopped")
	}
}

func assertManualControllerSchedulerTickerStopped(t *testing.T, ticker *manualControllerSchedulerTicker) {
	t.Helper()
	select {
	case <-ticker.stopped:
	default:
		t.Fatal("manual ticker was not stopped")
	}
}
