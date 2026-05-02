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
	"time"
)

const (
	errControllerSchedulerAlreadyRunning  = "bufferpool.controllerSchedulerRuntime: scheduler already running"
	errControllerSchedulerInvalidInterval = "bufferpool.controllerSchedulerRuntime: interval must be positive"
	errControllerSchedulerNilTick         = "bufferpool.controllerSchedulerRuntime: tick function must not be nil"
	errControllerSchedulerNilTicker       = "bufferpool.controllerSchedulerRuntime: ticker factory returned nil"
)

// controllerSchedulerTicker is the small timer surface the scheduler loop
// needs. Production uses time.NewTicker through controllerSchedulerTimeTicker;
// tests provide a manual ticker so scheduler behavior can be driven without
// sleeps, real timers, or nondeterministic cadence.
type controllerSchedulerTicker interface {
	C() <-chan time.Time
	Stop()
}

// controllerSchedulerTickerFactory constructs a ticker for one scheduler start.
//
// Factories are start-scoped rather than stored globally so future owners can
// supply deterministic tickers in tests while production code keeps the default
// time.NewTicker-backed implementation.
type controllerSchedulerTickerFactory func(time.Duration) controllerSchedulerTicker

// controllerSchedulerStartInput contains the cold control-plane dependencies
// needed to start one internal scheduler runtime.
//
// This is not public policy. PoolPartition and PoolGroup policy decide whether
// construction starts the runtime. Tick is intentionally the only owner
// callback; the scheduler does not allocate or retain controller reports, score
// diagnostics, trim candidates, samples, or snapshots.
type controllerSchedulerStartInput struct {
	// Interval is the fixed ticker cadence for this scheduler run.
	Interval time.Duration

	// Tick performs one owner foreground controller cycle. It should normally be
	// an owner Tick or TickInto wrapper, so all lifecycle gates, no-overlap
	// behavior, status publication, and report construction remain owned by the
	// existing controller path.
	Tick func() error

	// IsClosedError classifies owner-close errors. Closed-owner errors stop the
	// scheduler because no future foreground cycles can be accepted. Other errors
	// are ignored here because Tick/TickInto already publish retained lightweight
	// status and return full diagnostics to direct callers.
	IsClosedError func(error) bool

	// TickerFactory overrides the production ticker for deterministic tests. A
	// nil factory uses time.NewTicker through newControllerSchedulerTicker.
	TickerFactory controllerSchedulerTickerFactory
}

// controllerSchedulerRuntime is an owner-local internal scheduler primitive.
//
// It only owns start/stop mechanics and ticker dispatch. It does not interpret
// policy, does not publish runtime snapshots, does not retain full reports, and
// does not decide whether a PoolPartition or PoolGroup should enable
// scheduling. Owner integration calls Start explicitly after construction policy
// validation and calls Stop before cleanup locks that Tick may need.
//
// The primitive can be started again after Stop because that is a lifecycle-safe
// property of the internal runtime. Owner-level live scheduler toggles remain a
// separate policy contract and are intentionally rejected by PublishPolicy in
// the current integration.
type controllerSchedulerRuntime struct {
	mu       sync.Mutex
	running  bool
	stopping bool
	stop     chan struct{}
	done     chan struct{}
}

// Start launches one scheduler goroutine for input.
//
// Start validates only internal runtime wiring: positive interval, non-nil
// Tick, and a usable ticker. It returns a stable ErrInvalidOptions-classified
// error when called while a scheduler is already running. The already-running
// path is explicit because silently starting a second loop would overlap owner
// controller cycles and would make shutdown ownership ambiguous.
func (r *controllerSchedulerRuntime) Start(input controllerSchedulerStartInput) error {
	if input.Interval <= 0 {
		return newError(ErrInvalidOptions, errControllerSchedulerInvalidInterval)
	}
	if input.Tick == nil {
		return newError(ErrInvalidOptions, errControllerSchedulerNilTick)
	}
	tickerFactory := input.TickerFactory
	if tickerFactory == nil {
		tickerFactory = newControllerSchedulerTicker
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return newError(ErrInvalidOptions, errControllerSchedulerAlreadyRunning)
	}

	ticker := tickerFactory(input.Interval)
	if ticker == nil {
		return newError(ErrInvalidOptions, errControllerSchedulerNilTicker)
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	r.running = true
	r.stopping = false
	r.stop = stop
	r.done = done

	go r.run(input.Tick, input.IsClosedError, ticker, stop, done)
	return nil
}

// Stop requests scheduler shutdown and waits for the scheduler goroutine to
// exit. Stop is idempotent and can be called before Start.
//
// The method closes only the scheduler-owned stop channel while holding mu, then
// releases mu before waiting on done. That ordering is important for future
// owner integration: callers must not hold owner locks that Tick may need while
// waiting for Stop, and Stop itself must not hold the scheduler mutex across the
// goroutine exit path.
func (r *controllerSchedulerRuntime) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	if !r.stopping {
		close(r.stop)
		r.stopping = true
	}
	done := r.done
	r.mu.Unlock()

	<-done
}

// isRunning reports whether a scheduler goroutine is currently active.
//
// This is intentionally internal. It exists to support deterministic unit tests
// and future owner assertions without exposing scheduler state as public API.
func (r *controllerSchedulerRuntime) isRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.running
}

// run owns the scheduler goroutine.
//
// The loop dispatches Tick only from ticker events and ignores ordinary tick
// errors. Closed-owner errors stop the loop because the owner can no longer
// accept foreground controller work. The loop always stops its ticker and
// publishes done exactly once, including when the owner closes itself during a
// Tick call.
func (r *controllerSchedulerRuntime) run(
	tick func() error,
	isClosedError func(error) bool,
	ticker controllerSchedulerTicker,
	stop <-chan struct{},
	done chan struct{},
) {
	tickerC := ticker.C()
	defer r.finish(ticker, done)

	for {
		select {
		case _, ok := <-tickerC:
			if !ok {
				return
			}
			if err := tick(); err != nil && controllerSchedulerIsClosedError(isClosedError, err) {
				return
			}
		case <-stop:
			return
		}
	}
}

// finish marks the runtime stopped, stops the ticker, and releases Stop waiters.
//
// The state update and done close happen while holding mu so a concurrent Stop
// either observes a running scheduler and waits on this done channel, or observes
// a fully stopped runtime. That avoids the small race where Stop could return
// before the goroutine finished its shutdown bookkeeping.
func (r *controllerSchedulerRuntime) finish(ticker controllerSchedulerTicker, done chan struct{}) {
	ticker.Stop()

	r.mu.Lock()
	if r.done == done {
		r.running = false
		r.stopping = false
		r.stop = nil
		r.done = nil
	}
	close(done)
	r.mu.Unlock()
}

// controllerSchedulerIsClosedError applies the owner-supplied closed classifier.
//
// A nil classifier falls back to errors.Is(err, ErrClosed), which matches the
// existing owner lifecycle contract while keeping the start input compact for
// tests that do not need custom classification.
func controllerSchedulerIsClosedError(isClosedError func(error) bool, err error) bool {
	if err == nil {
		return false
	}
	if isClosedError != nil {
		return isClosedError(err)
	}
	return errors.Is(err, ErrClosed)
}

// controllerSchedulerTimeTicker adapts time.Ticker to controllerSchedulerTicker.
type controllerSchedulerTimeTicker struct {
	ticker *time.Ticker
}

// newControllerSchedulerTicker creates the production ticker.
//
// This is the only production timer construction in the scheduler foundation.
// Owner constructors reach it only through explicit opt-in scheduler policy; the
// default manual policy path never constructs a ticker or starts a goroutine.
func newControllerSchedulerTicker(interval time.Duration) controllerSchedulerTicker {
	return controllerSchedulerTimeTicker{ticker: time.NewTicker(interval)}
}

// C returns the production ticker channel.
func (t controllerSchedulerTimeTicker) C() <-chan time.Time {
	return t.ticker.C
}

// Stop releases the production ticker.
func (t controllerSchedulerTimeTicker) Stop() {
	t.ticker.Stop()
}
