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
	"strconv"
	"sync/atomic"
)

const (
	// errUnknownLifecycleState is used when a raw lifecycle state value does not
	// correspond to any known LifecycleState constant.
	//
	// This should not happen through normal constructors or AtomicLifecycle
	// methods. If it does happen, it indicates memory corruption, direct invalid
	// Store usage from future code, or an internal bug.
	errUnknownLifecycleState = "bufferpool.LifecycleState: unknown lifecycle state"

	// errInvalidLifecycleTransition is used when a lifecycle transition violates
	// the allowed state machine.
	//
	// Lifecycle transitions are internal invariants. Invalid transitions should
	// fail fast because silently accepting them can make a pool, partition,
	// controller, or group appear usable after shutdown has already started.
	errInvalidLifecycleTransition = "bufferpool.LifecycleState: invalid lifecycle transition"

	// errNilAtomicLifecycle is used when AtomicLifecycle methods are called on a
	// nil receiver.
	//
	// A nil lifecycle holder is always an internal wiring error. The explicit
	// panic message gives a clearer failure than a generic nil-pointer panic from
	// sync/atomic internals.
	errNilAtomicLifecycle = "bufferpool.AtomicLifecycle: receiver must not be nil"
)

const (
	// LifecycleCreated is the zero lifecycle state.
	//
	// It represents a runtime object that has been allocated or constructed but
	// has not yet been activated. Keeping Created as the zero value makes embedded
	// AtomicLifecycle fields safe to allocate before constructors finish wiring
	// the rest of the object.
	LifecycleCreated LifecycleState = iota

	// LifecycleActive represents a runtime object that accepts ordinary work.
	//
	// For a Pool, this means acquire/release paths may proceed according to the
	// current policy and admission rules. For a Partition or Group, this means
	// the component is part of the active runtime topology.
	LifecycleActive

	// LifecycleClosing represents a runtime object whose shutdown has started.
	//
	// New work should be rejected once Closing is visible. Existing cleanup may
	// still be in progress. Higher-level components may use this state to make
	// Close idempotent and to distinguish "shutdown started" from "shutdown
	// completed".
	LifecycleClosing

	// LifecycleClosed represents a terminal runtime object.
	//
	// Closed is terminal. A closed component must not become Active again.
	LifecycleClosed
)

// LifecycleState describes the lifecycle phase of a runtime component.
//
// It is intentionally generic and does not mention concrete runtime objects.
// Pool, partition, group, controller, and future runtime entities can all use
// the same state machine while keeping their own higher-level shutdown logic.
//
// The canonical state flow is:
//
//	Created -> Active -> Closing -> Closed
//
// Some shortcuts are also valid:
//
//	Created -> Closing
//	Created -> Closed
//	Active  -> Closed
//	Closed  -> Closed
//
// Closed is terminal. Runtime code MUST NOT reopen a closed component.
type LifecycleState uint32

// Uint32 returns state as a raw uint32 value.
//
// This is useful for metrics, snapshots, logs, and tests. Runtime code should
// prefer semantic methods such as IsActive, IsClosing, IsClosed, and AllowsWork.
func (s LifecycleState) Uint32() uint32 {
	return uint32(s)
}

// String returns the stable diagnostic name of the lifecycle state.
func (s LifecycleState) String() string {
	switch s {
	case LifecycleCreated:
		return "created"
	case LifecycleActive:
		return "active"
	case LifecycleClosing:
		return "closing"
	case LifecycleClosed:
		return "closed"
	default:
		return "unknown(" + strconv.FormatUint(uint64(s), 10) + ")"
	}
}

// IsKnown reports whether state is one of the defined lifecycle states.
func (s LifecycleState) IsKnown() bool {
	switch s {
	case LifecycleCreated, LifecycleActive, LifecycleClosing, LifecycleClosed:
		return true
	default:
		return false
	}
}

// IsCreated reports whether state is LifecycleCreated.
func (s LifecycleState) IsCreated() bool {
	return s == LifecycleCreated
}

// IsActive reports whether state is LifecycleActive.
func (s LifecycleState) IsActive() bool {
	return s == LifecycleActive
}

// IsClosing reports whether state is LifecycleClosing.
func (s LifecycleState) IsClosing() bool {
	return s == LifecycleClosing
}

// IsClosed reports whether state is LifecycleClosed.
func (s LifecycleState) IsClosed() bool {
	return s == LifecycleClosed
}

// IsTerminal reports whether state is terminal.
//
// Closed is the only terminal lifecycle state.
func (s LifecycleState) IsTerminal() bool {
	return s == LifecycleClosed
}

// AllowsWork reports whether ordinary runtime work may start in this state.
//
// Only Active allows new ordinary work. Created is not active yet. Closing and
// Closed must reject new work.
//
// Higher-level code may still perform cleanup while Closing or Closed, but such
// cleanup should be explicit and should not be treated as normal data-plane work.
func (s LifecycleState) AllowsWork() bool {
	return s == LifecycleActive
}

// AllowsCloseStart reports whether a Close operation may start from this state.
//
// Created and Active may start closing. Closing and Closed have already observed
// shutdown and should make Close idempotent rather than starting it again.
func (s LifecycleState) AllowsCloseStart() bool {
	return s == LifecycleCreated || s == LifecycleActive
}

// Validate panics if state is not a known lifecycle state.
func (s LifecycleState) Validate() {
	if !s.IsKnown() {
		panic(errUnknownLifecycleState)
	}
}

// CanTransitionTo reports whether the lifecycle state machine allows a
// transition from state to next.
//
// Self-transitions are valid for known states. This lets idempotent paths
// publish or observe the same lifecycle state repeatedly without being treated
// as invalid.
func (s LifecycleState) CanTransitionTo(next LifecycleState) bool {
	if !s.IsKnown() || !next.IsKnown() {
		return false
	}

	if s == next {
		return true
	}

	switch s {
	case LifecycleCreated:
		return next == LifecycleActive ||
			next == LifecycleClosing ||
			next == LifecycleClosed

	case LifecycleActive:
		return next == LifecycleClosing ||
			next == LifecycleClosed

	case LifecycleClosing:
		return next == LifecycleClosed

	default:
		return false
	}
}

// MustTransitionTo validates and returns next.
//
// This helper is useful for code that computes a next state and wants a concise
// invariant check before publishing it. It panics on unknown states or invalid
// transitions.
func (s LifecycleState) MustTransitionTo(next LifecycleState) LifecycleState {
	if !s.CanTransitionTo(next) {
		panic(errInvalidLifecycleTransition)
	}

	return next
}

// AtomicLifecycle is an atomic holder for LifecycleState.
//
// AtomicLifecycle is intended for runtime components that need lock-free
// lifecycle checks on hot or hot-adjacent paths, for example:
//
//   - Pool acquire/release admission checks;
//   - Partition controller state;
//   - PoolGroup shutdown state;
//   - background trim or coordination loops.
//
// The zero value is ready to use and starts in LifecycleCreated.
//
// AtomicLifecycle MUST NOT be copied after first use. It wraps sync/atomic
// state, and copying it after use would split the logical lifecycle into
// independent atomic cells.
type AtomicLifecycle struct {
	state atomic.Uint32
}

// Load atomically loads and returns the current lifecycle state.
func (l *AtomicLifecycle) Load() LifecycleState {
	l.mustNotBeNil()

	state := LifecycleState(l.state.Load())
	state.Validate()

	return state
}

// Store atomically stores next after validating that it is a known state.
//
// Store does not validate transition direction because it is intended only for
// controlled initialization, restoration, or tests. Ordinary runtime lifecycle
// movement should prefer Activate, BeginClose, MarkClosed, or Transition.
func (l *AtomicLifecycle) Store(next LifecycleState) {
	l.mustNotBeNil()
	next.Validate()

	l.state.Store(uint32(next))
}

// Transition atomically moves from expected to next.
//
// The transition is attempted only if the current state equals expected. The
// state-machine transition expected -> next must be valid. The method returns
// true when the transition was applied.
//
// Transition is useful when higher-level code needs a precise compare-and-swap
// lifecycle movement.
func (l *AtomicLifecycle) Transition(expected, next LifecycleState) bool {
	l.mustNotBeNil()

	expected.MustTransitionTo(next)

	return l.state.CompareAndSwap(uint32(expected), uint32(next))
}

// Activate moves the lifecycle from Created to Active.
//
// It returns true when this call performed the activation. If the lifecycle is
// already Active, it returns false. If the lifecycle is Closing or Closed, it
// panics because a component must not be reactivated after shutdown starts.
//
// Load validates the observed state before the switch below, so every switch
// iteration sees a known lifecycle state.
func (l *AtomicLifecycle) Activate() bool {
	l.mustNotBeNil()

	for {
		current := l.Load()

		switch current {
		case LifecycleCreated:
			if l.state.CompareAndSwap(uint32(LifecycleCreated), uint32(LifecycleActive)) {
				return true
			}

		case LifecycleActive:
			return false

		case LifecycleClosing, LifecycleClosed:
			panic(errInvalidLifecycleTransition)
		}
	}
}

// BeginClose moves the lifecycle to Closing if shutdown has not already started.
//
// It returns true when this call started the close sequence. It returns false
// when the lifecycle was already Closing or Closed.
//
// BeginClose is useful for idempotent Close implementations:
//
//	if lifecycle.BeginClose() {
//	    cleanup()
//	    lifecycle.MarkClosed()
//	}
//
// Load validates the observed state before the switch below, so the switch only
// handles known lifecycle states.
func (l *AtomicLifecycle) BeginClose() bool {
	l.mustNotBeNil()

	for {
		current := l.Load()

		switch current {
		case LifecycleCreated, LifecycleActive:
			if l.state.CompareAndSwap(uint32(current), uint32(LifecycleClosing)) {
				return true
			}

		case LifecycleClosing, LifecycleClosed:
			return false
		}
	}
}

// MarkClosed moves the lifecycle to Closed.
//
// MarkClosed is idempotent when the lifecycle is already Closed. It accepts
// Created, Active, or Closing as sources so callers can close components during
// construction failure, direct shutdown, or after an explicit Closing phase.
//
// Load validates the observed state before the switch below, so the switch only
// handles known lifecycle states.
func (l *AtomicLifecycle) MarkClosed() bool {
	l.mustNotBeNil()

	for {
		current := l.Load()

		switch current {
		case LifecycleCreated, LifecycleActive, LifecycleClosing:
			if l.state.CompareAndSwap(uint32(current), uint32(LifecycleClosed)) {
				return true
			}

		case LifecycleClosed:
			return false
		}
	}
}

// IsActive reports whether the current lifecycle state is Active.
func (l *AtomicLifecycle) IsActive() bool {
	return l.Load().IsActive()
}

// IsClosing reports whether the current lifecycle state is Closing.
func (l *AtomicLifecycle) IsClosing() bool {
	return l.Load().IsClosing()
}

// IsClosed reports whether the current lifecycle state is Closed.
func (l *AtomicLifecycle) IsClosed() bool {
	return l.Load().IsClosed()
}

// AllowsWork reports whether ordinary runtime work may start now.
func (l *AtomicLifecycle) AllowsWork() bool {
	return l.Load().AllowsWork()
}

// mustNotBeNil validates the receiver before accessing atomic state.
//
// Methods on nil pointer receivers are legal to call in Go. Providing an
// explicit panic message makes internal wiring failures easier to diagnose.
func (l *AtomicLifecycle) mustNotBeNil() {
	if l == nil {
		panic(errNilAtomicLifecycle)
	}
}
