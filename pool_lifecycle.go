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

const (
	// errNilPool is used when a Pool method is called on a nil receiver.
	//
	// A nil Pool is always a caller or wiring bug. Treating nil as an empty
	// disabled pool would hide construction errors and make cleanup paths appear
	// successful when no runtime owner exists.
	errNilPool = "bufferpool.Pool: receiver must not be nil"

	// errUninitializedPool is used when a Pool value was not constructed through
	// New.
	//
	// The zero value of Pool is intentionally not usable. Pool construction must
	// wire close coordination, effective policy, class table, class states, and
	// shard selector before any operation can run.
	errUninitializedPool = "bufferpool.Pool: pool must be constructed with New"

	// errPoolClosed is used when ordinary data-plane work is attempted after the
	// Pool no longer allows work.
	//
	// Pool treats both Closing and Closed as closed for acquisition. Return paths
	// may either reject or drop late returned buffers depending on PoolConfig.
	errPoolClosed = "bufferpool.Pool: pool is closed"

	// errPoolOperationCounterUnderflow is used when the operation gate observes
	// more completed operations than admitted operations.
	//
	// This should be impossible when callers use beginAcquireOperation or
	// beginReturnOperation with a matching endOperation. If it happens, the Pool
	// lifecycle gate has been corrupted by an internal bug.
	errPoolOperationCounterUnderflow = "bufferpool.Pool: active operation counter underflow"
)

// Lifecycle returns the current Pool lifecycle state.
//
// The returned value is observational. Callers MUST NOT use it as an external
// synchronization primitive because the state may change immediately after it is
// read. Public operations perform their own lifecycle admission checks.
func (p *Pool) Lifecycle() LifecycleState {
	p.mustBeInitialized()

	return p.lifecycle.Load()
}

// IsActive reports whether the Pool currently accepts ordinary data-plane work.
func (p *Pool) IsActive() bool {
	p.mustBeInitialized()

	return p.lifecycle.IsActive()
}

// IsClosing reports whether Pool shutdown has started but has not necessarily
// completed.
//
// While Closing is visible, new acquisition work is rejected. Return work is
// rejected or treated as a no-retain drop according to PoolConfig.
func (p *Pool) IsClosing() bool {
	p.mustBeInitialized()

	return p.lifecycle.IsClosing()
}

// IsClosed reports whether the Pool has reached its terminal lifecycle state.
func (p *Pool) IsClosed() bool {
	p.mustBeInitialized()

	return p.lifecycle.IsClosed()
}

// Close stops the Pool and performs configured close-time cleanup.
//
// Close is synchronous and idempotent:
//
//   - the first caller moves lifecycle to Closing;
//   - new ordinary acquisition work is rejected after Closing is visible;
//   - return work is rejected or dropped according to PoolConfig;
//   - operations already admitted by the lifecycle gate are allowed to finish;
//   - retained storage is cleared if PoolConfig requests close-time cleanup;
//   - lifecycle is marked Closed;
//   - concurrent Close callers wait until the first close sequence reaches
//     Closed.
//
// Close does not start or stop controllers. Standalone Pool owns no controller
// loop. Future PoolPartition or PoolGroup components must own controller
// lifecycle separately.
func (p *Pool) Close() error {
	p.mustBeInitialized()

	if !p.lifecycle.BeginClose() {
		p.waitForClosed()
		return nil
	}

	p.waitForOperations()

	if p.config.ShouldClearRetainedOnClose() {
		p.clearRetainedStorage()
	}

	p.lifecycle.MarkClosed()
	p.notifyLifecycleWaiters()

	return nil
}

// beginAcquireOperation admits one Get-like operation into the Pool lifecycle
// gate.
//
// The gate uses a double-check pattern:
//
//   - first check: avoid incrementing activeOperations when the Pool is already
//     closing or closed;
//   - increment: mark that an operation may touch runtime state;
//   - second check: detect a Close that started between the first check and the
//     increment.
//
// If the second check observes Closing or Closed, the method rolls back the
// active operation count and rejects the operation. This prevents Close from
// clearing retained storage while an already-admitted acquisition path is still
// executing.
//
// Admitted hot paths use explicit endOperation calls instead of defer to keep
// overhead low. This is a deliberate data-plane trade-off: internal invariant
// panics are treated as fatal runtime corruption, not recoverable application
// errors. If caller code recovers from an internal Pool panic and keeps using
// the same Pool, lifecycle drain correctness is no longer guaranteed for the
// corrupted operation because the active-operation counter may not have been
// decremented.
func (p *Pool) beginAcquireOperation() error {
	for {
		if !p.lifecycle.AllowsWork() {
			return newError(ErrClosed, errPoolClosed)
		}

		p.activeOperations.Add(1)

		if p.lifecycle.AllowsWork() {
			return nil
		}

		p.endOperation()
	}
}

// beginReturnOperation admits one Put-like operation into the Pool lifecycle
// gate.
//
// Return paths are different from acquisition paths after close starts. A
// configured Pool may choose tolerant behavior for late returned buffers:
//
//   - reject: return ErrClosed;
//   - drop returns: accept the call as a no-op and do not retain the buffer.
//
// The returned bool reports whether the caller should continue with ordinary
// return-path logic. A false/nil result means "drop without touching retained
// storage".
func (p *Pool) beginReturnOperation() (bool, error) {
	for {
		if !p.lifecycle.AllowsWork() {
			if p.config.ShouldDropReturnedBuffersAfterClose() {
				return false, nil
			}

			return false, newError(ErrClosed, errPoolClosed)
		}

		p.activeOperations.Add(1)

		if p.lifecycle.AllowsWork() {
			return true, nil
		}

		p.endOperation()

		if p.config.ShouldDropReturnedBuffersAfterClose() {
			return false, nil
		}

		return false, newError(ErrClosed, errPoolClosed)
	}
}

// endOperation completes one operation previously admitted through the lifecycle
// gate.
//
// When the last active operation exits after close has started, Close waiters are
// woken so the shutdown sequence can continue to close-time cleanup.
func (p *Pool) endOperation() {
	remaining := p.activeOperations.Add(-1)
	if remaining < 0 {
		panic(errPoolOperationCounterUnderflow)
	}

	if remaining == 0 && !p.lifecycle.AllowsWork() {
		p.notifyLifecycleWaiters()
	}
}

// waitForOperations waits until every operation admitted by the lifecycle gate
// has finished.
//
// activeOperations is atomic, but the condition variable prevents busy waiting.
// The loop is required because condition variables can wake spuriously.
func (p *Pool) waitForOperations() {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()

	for p.activeOperations.Load() != 0 {
		p.closeCond.Wait()
	}
}

// waitForClosed waits until the first Close caller completes the shutdown
// sequence.
//
// This makes concurrent Close calls synchronous: a caller that arrives while
// another goroutine is already closing the Pool returns only after lifecycle is
// Closed, not merely after observing Closing.
func (p *Pool) waitForClosed() {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()

	for !p.lifecycle.IsClosed() {
		p.closeCond.Wait()
	}
}

// notifyLifecycleWaiters wakes goroutines waiting for operation drain or final
// closed state.
//
// The condition variable protects waiting, not Pool state itself. Lifecycle and
// active operation count are atomic. Broadcasting under closeMu keeps the wait
// loops simple and avoids missed wakeups around close coordination.
func (p *Pool) notifyLifecycleWaiters() {
	p.closeMu.Lock()
	p.closeCond.Broadcast()
	p.closeMu.Unlock()
}

// clearRetainedStorage removes retained buffers from every Pool-owned class.
//
// This method is called only after Close has published Closing and waited for
// admitted operations to drain. classState.clear is intentionally not a global
// transaction by itself; Pool lifecycle supplies the owner-level shutdown
// barrier that makes close-time clearing safe.
func (p *Pool) clearRetainedStorage() {
	for index := range p.classes {
		p.classes[index].clear()
	}
}

// mustBeInitialized verifies that the receiver is a constructed Pool.
//
// The check is stricter than a nil check. A zero-value Pool has a usable
// zero-value AtomicLifecycle, but it does not have the condition variable,
// normalized config, policy, class table, class states, or shard selector
// required for correct owner behavior.
func (p *Pool) mustBeInitialized() {
	if p == nil {
		panic(errNilPool)
	}

	if p.closeCond == nil {
		panic(errUninitializedPool)
	}
}
