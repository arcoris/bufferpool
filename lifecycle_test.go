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
	"sync"
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestLifecycleStateConstants verifies the canonical lifecycle state values.
//
// LifecycleCreated must remain the zero value so an embedded AtomicLifecycle
// starts in a safe non-active state. A zero-value runtime component must not
// accidentally accept work before construction and activation complete.
func TestLifecycleStateConstants(t *testing.T) {
	t.Parallel()

	if LifecycleCreated != LifecycleState(0) {
		t.Fatalf("LifecycleCreated = %d, want 0", LifecycleCreated)
	}

	if LifecycleActive != LifecycleState(1) {
		t.Fatalf("LifecycleActive = %d, want 1", LifecycleActive)
	}

	if LifecycleClosing != LifecycleState(2) {
		t.Fatalf("LifecycleClosing = %d, want 2", LifecycleClosing)
	}

	if LifecycleClosed != LifecycleState(3) {
		t.Fatalf("LifecycleClosed = %d, want 3", LifecycleClosed)
	}
}

// TestLifecycleStateUint32 verifies raw numeric extraction.
//
// Uint32 is useful for metrics, snapshots, logs, and tests. Runtime code should
// still prefer semantic predicates for ordinary lifecycle decisions.
func TestLifecycleStateUint32(t *testing.T) {
	t.Parallel()

	if got := LifecycleClosing.Uint32(); got != 2 {
		t.Fatalf("LifecycleClosing.Uint32() = %d, want 2", got)
	}
}

// TestLifecycleStateString verifies stable diagnostic names.
//
// These names may appear in logs, metrics labels, snapshots, and test failures.
// Unknown states should remain visible rather than being collapsed to an empty
// string.
func TestLifecycleStateString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// state is the lifecycle value being formatted.
		state LifecycleState

		// want is the expected stable diagnostic representation.
		want string
	}{
		{
			name:  "created",
			state: LifecycleCreated,
			want:  "created",
		},
		{
			name:  "active",
			state: LifecycleActive,
			want:  "active",
		},
		{
			name:  "closing",
			state: LifecycleClosing,
			want:  "closing",
		},
		{
			name:  "closed",
			state: LifecycleClosed,
			want:  "closed",
		},
		{
			name:  "unknown",
			state: LifecycleState(99),
			want:  "unknown(99)",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.state.String()
			if got != tt.want {
				t.Fatalf("LifecycleState(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

// TestLifecycleStateIsKnown verifies known-state classification.
//
// Unknown raw values should be rejected by validation and atomic load paths.
// They usually indicate direct invalid storage, memory corruption, or an
// internal state-machine bug.
func TestLifecycleStateIsKnown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// state is the lifecycle value being classified.
		state LifecycleState

		// want is true only for declared lifecycle constants.
		want bool
	}{
		{
			name:  "created",
			state: LifecycleCreated,
			want:  true,
		},
		{
			name:  "active",
			state: LifecycleActive,
			want:  true,
		},
		{
			name:  "closing",
			state: LifecycleClosing,
			want:  true,
		},
		{
			name:  "closed",
			state: LifecycleClosed,
			want:  true,
		},
		{
			name:  "unknown",
			state: LifecycleState(99),
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.state.IsKnown()
			if got != tt.want {
				t.Fatalf("LifecycleState(%d).IsKnown() = %t, want %t", tt.state, got, tt.want)
			}
		})
	}
}

// TestLifecycleStatePredicates verifies single-state semantic predicates.
//
// These helpers make higher-level runtime checks readable and avoid spreading raw
// state comparisons across pool, partition, group, and controller code.
func TestLifecycleStatePredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// state is the lifecycle value being inspected.
		state LifecycleState

		wantCreated bool
		wantActive  bool
		wantClosing bool
		wantClosed  bool
	}{
		{
			name:        "created",
			state:       LifecycleCreated,
			wantCreated: true,
		},
		{
			name:       "active",
			state:      LifecycleActive,
			wantActive: true,
		},
		{
			name:        "closing",
			state:       LifecycleClosing,
			wantClosing: true,
		},
		{
			name:       "closed",
			state:      LifecycleClosed,
			wantClosed: true,
		},
		{
			name:  "unknown",
			state: LifecycleState(99),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.state.IsCreated(); got != tt.wantCreated {
				t.Fatalf("IsCreated() = %t, want %t", got, tt.wantCreated)
			}

			if got := tt.state.IsActive(); got != tt.wantActive {
				t.Fatalf("IsActive() = %t, want %t", got, tt.wantActive)
			}

			if got := tt.state.IsClosing(); got != tt.wantClosing {
				t.Fatalf("IsClosing() = %t, want %t", got, tt.wantClosing)
			}

			if got := tt.state.IsClosed(); got != tt.wantClosed {
				t.Fatalf("IsClosed() = %t, want %t", got, tt.wantClosed)
			}
		})
	}
}

// TestLifecycleStateIsTerminal verifies terminal-state classification.
//
// Closed is the only terminal state. Closing is not terminal because cleanup may
// still be in progress and the lifecycle must still be able to move to Closed.
func TestLifecycleStateIsTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// state is the lifecycle value being inspected.
		state LifecycleState

		// want is true only for LifecycleClosed.
		want bool
	}{
		{
			name:  "created",
			state: LifecycleCreated,
			want:  false,
		},
		{
			name:  "active",
			state: LifecycleActive,
			want:  false,
		},
		{
			name:  "closing",
			state: LifecycleClosing,
			want:  false,
		},
		{
			name:  "closed",
			state: LifecycleClosed,
			want:  true,
		},
		{
			name:  "unknown",
			state: LifecycleState(99),
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.state.IsTerminal()
			if got != tt.want {
				t.Fatalf("LifecycleState(%d).IsTerminal() = %t, want %t", tt.state, got, tt.want)
			}
		})
	}
}

// TestLifecycleStateAllowsWork verifies the ordinary-work admission predicate.
//
// Only Active may start normal runtime work. Created has not been activated yet.
// Closing and Closed must reject new work.
func TestLifecycleStateAllowsWork(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// state is the lifecycle value being inspected.
		state LifecycleState

		// want is true only for LifecycleActive.
		want bool
	}{
		{
			name:  "created rejects work",
			state: LifecycleCreated,
			want:  false,
		},
		{
			name:  "active allows work",
			state: LifecycleActive,
			want:  true,
		},
		{
			name:  "closing rejects work",
			state: LifecycleClosing,
			want:  false,
		},
		{
			name:  "closed rejects work",
			state: LifecycleClosed,
			want:  false,
		},
		{
			name:  "unknown rejects work",
			state: LifecycleState(99),
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.state.AllowsWork()
			if got != tt.want {
				t.Fatalf("LifecycleState(%d).AllowsWork() = %t, want %t", tt.state, got, tt.want)
			}
		})
	}
}

// TestLifecycleStateAllowsCloseStart verifies idempotent close-start semantics.
//
// Created and Active may start closing. Closing and Closed have already observed
// shutdown and should not start cleanup again.
func TestLifecycleStateAllowsCloseStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// state is the lifecycle value being inspected.
		state LifecycleState

		// want is true when BeginClose may start a close sequence.
		want bool
	}{
		{
			name:  "created allows close start",
			state: LifecycleCreated,
			want:  true,
		},
		{
			name:  "active allows close start",
			state: LifecycleActive,
			want:  true,
		},
		{
			name:  "closing does not start close again",
			state: LifecycleClosing,
			want:  false,
		},
		{
			name:  "closed does not start close again",
			state: LifecycleClosed,
			want:  false,
		},
		{
			name:  "unknown does not start close",
			state: LifecycleState(99),
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.state.AllowsCloseStart()
			if got != tt.want {
				t.Fatalf("LifecycleState(%d).AllowsCloseStart() = %t, want %t", tt.state, got, tt.want)
			}
		})
	}
}

// TestLifecycleStateValidate verifies known-state validation.
//
// Validate should accept declared states and reject unknown raw values. Unknown
// lifecycle states should not be allowed to enter runtime state machines.
func TestLifecycleStateValidate(t *testing.T) {
	t.Parallel()

	for _, state := range []LifecycleState{
		LifecycleCreated,
		LifecycleActive,
		LifecycleClosing,
		LifecycleClosed,
	} {
		state := state

		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()

			state.Validate()
		})
	}

	t.Run("unknown state panics", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errUnknownLifecycleState, func() {
			LifecycleState(99).Validate()
		})
	})
}

// TestLifecycleStateCanTransitionTo verifies the complete transition matrix.
//
// The state machine deliberately permits same-state transitions for Created,
// Active, Closing, and Closed. This allows idempotent observations or controlled
// Store-like behavior while still forbidding reopening closed or closing
// components.
func TestLifecycleStateCanTransitionTo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// from is the current lifecycle state.
		from LifecycleState

		// to is the requested next lifecycle state.
		to LifecycleState

		// want is true when the state machine allows the transition.
		want bool
	}{
		{
			name: "created to created",
			from: LifecycleCreated,
			to:   LifecycleCreated,
			want: true,
		},
		{
			name: "created to active",
			from: LifecycleCreated,
			to:   LifecycleActive,
			want: true,
		},
		{
			name: "created to closing",
			from: LifecycleCreated,
			to:   LifecycleClosing,
			want: true,
		},
		{
			name: "created to closed",
			from: LifecycleCreated,
			to:   LifecycleClosed,
			want: true,
		},
		{
			name: "active to created is invalid",
			from: LifecycleActive,
			to:   LifecycleCreated,
			want: false,
		},
		{
			name: "active to active",
			from: LifecycleActive,
			to:   LifecycleActive,
			want: true,
		},
		{
			name: "active to closing",
			from: LifecycleActive,
			to:   LifecycleClosing,
			want: true,
		},
		{
			name: "active to closed",
			from: LifecycleActive,
			to:   LifecycleClosed,
			want: true,
		},
		{
			name: "closing to created is invalid",
			from: LifecycleClosing,
			to:   LifecycleCreated,
			want: false,
		},
		{
			name: "closing to active is invalid",
			from: LifecycleClosing,
			to:   LifecycleActive,
			want: false,
		},
		{
			name: "closing to closing",
			from: LifecycleClosing,
			to:   LifecycleClosing,
			want: true,
		},
		{
			name: "closing to closed",
			from: LifecycleClosing,
			to:   LifecycleClosed,
			want: true,
		},
		{
			name: "closed to created is invalid",
			from: LifecycleClosed,
			to:   LifecycleCreated,
			want: false,
		},
		{
			name: "closed to active is invalid",
			from: LifecycleClosed,
			to:   LifecycleActive,
			want: false,
		},
		{
			name: "closed to closing is invalid",
			from: LifecycleClosed,
			to:   LifecycleClosing,
			want: false,
		},
		{
			name: "closed to closed",
			from: LifecycleClosed,
			to:   LifecycleClosed,
			want: true,
		},
		{
			name: "unknown source is invalid",
			from: LifecycleState(99),
			to:   LifecycleClosed,
			want: false,
		},
		{
			name: "unknown target is invalid",
			from: LifecycleCreated,
			to:   LifecycleState(99),
			want: false,
		},
		{
			name: "unknown source and target are invalid",
			from: LifecycleState(98),
			to:   LifecycleState(99),
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.from.CanTransitionTo(tt.to)
			if got != tt.want {
				t.Fatalf("LifecycleState(%d).CanTransitionTo(%d) = %t, want %t", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

// TestLifecycleStateMustTransitionTo verifies transition validation with a
// returned next state.
//
// MustTransitionTo is useful for call sites that compute a next state and want a
// compact invariant check before publishing it.
func TestLifecycleStateMustTransitionTo(t *testing.T) {
	t.Parallel()

	got := LifecycleActive.MustTransitionTo(LifecycleClosing)
	if got != LifecycleClosing {
		t.Fatalf("MustTransitionTo returned %s, want %s", got, LifecycleClosing)
	}

	testutil.MustPanicWithMessage(t, errInvalidLifecycleTransition, func() {
		_ = LifecycleClosed.MustTransitionTo(LifecycleActive)
	})
}

// TestAtomicLifecycleZeroValue verifies that AtomicLifecycle is usable without
// explicit initialization.
//
// A zero-value AtomicLifecycle starts in Created, which is safe because it does
// not allow ordinary runtime work before activation.
func TestAtomicLifecycleZeroValue(t *testing.T) {
	t.Parallel()

	var lifecycle AtomicLifecycle

	if got := lifecycle.Load(); got != LifecycleCreated {
		t.Fatalf("zero-value AtomicLifecycle Load() = %s, want %s", got, LifecycleCreated)
	}

	if lifecycle.AllowsWork() {
		t.Fatal("zero-value AtomicLifecycle AllowsWork() = true, want false")
	}
}

// TestAtomicLifecycleStoreAndLoad verifies controlled state publication.
//
// Store is intentionally available for initialization, restoration, and tests.
// Ordinary runtime movement should prefer Activate, BeginClose, MarkClosed, or
// Transition.
func TestAtomicLifecycleStoreAndLoad(t *testing.T) {
	t.Parallel()

	var lifecycle AtomicLifecycle

	lifecycle.Store(LifecycleActive)

	if got := lifecycle.Load(); got != LifecycleActive {
		t.Fatalf("Load after Store(active) = %s, want active", got)
	}
}

// TestAtomicLifecycleStorePanicsForUnknownState verifies Store validation.
//
// Even though Store does not validate transition direction, it must not allow raw
// unknown lifecycle states to enter the atomic holder.
func TestAtomicLifecycleStorePanicsForUnknownState(t *testing.T) {
	t.Parallel()

	var lifecycle AtomicLifecycle

	testutil.MustPanicWithMessage(t, errUnknownLifecycleState, func() {
		lifecycle.Store(LifecycleState(99))
	})
}

// TestAtomicLifecycleLoadPanicsForUnknownState verifies load-side protection.
//
// This test directly writes an invalid raw value because tests are in package
// bufferpool and can access the internal atomic field. Normal callers cannot do
// this, but Load should still fail fast if invalid state is observed.
func TestAtomicLifecycleLoadPanicsForUnknownState(t *testing.T) {
	t.Parallel()

	var lifecycle AtomicLifecycle

	lifecycle.state.Store(uint32(99))

	testutil.MustPanicWithMessage(t, errUnknownLifecycleState, func() {
		_ = lifecycle.Load()
	})
}

// TestAtomicLifecycleTransition verifies compare-and-swap lifecycle movement.
//
// Transition applies a valid state-machine edge only when the current state
// matches the expected state.
func TestAtomicLifecycleTransition(t *testing.T) {
	t.Parallel()

	var lifecycle AtomicLifecycle

	if transitioned := lifecycle.Transition(LifecycleCreated, LifecycleActive); !transitioned {
		t.Fatal("Transition(created, active) = false, want true")
	}

	if got := lifecycle.Load(); got != LifecycleActive {
		t.Fatalf("Load after Transition(created, active) = %s, want active", got)
	}

	if transitioned := lifecycle.Transition(LifecycleCreated, LifecycleClosed); transitioned {
		t.Fatal("Transition(created, closed) from active state = true, want false")
	}

	if got := lifecycle.Load(); got != LifecycleActive {
		t.Fatalf("Load after failed Transition = %s, want active", got)
	}
}

// TestAtomicLifecycleTransitionPanicsForInvalidTransition verifies that
// Transition validates the requested state-machine edge before CAS.
//
// Reopening a closed component or moving a closing component back to active is a
// runtime invariant violation and must fail fast.
func TestAtomicLifecycleTransitionPanicsForInvalidTransition(t *testing.T) {
	t.Parallel()

	var lifecycle AtomicLifecycle

	lifecycle.Store(LifecycleClosed)

	testutil.MustPanicWithMessage(t, errInvalidLifecycleTransition, func() {
		_ = lifecycle.Transition(LifecycleClosed, LifecycleActive)
	})
}

// TestAtomicLifecycleTransitionAllowsSelfTransition verifies idempotent CAS
// movement for known states.
//
// Self-transitions are valid at the LifecycleState layer so higher-level code
// can publish or observe the same lifecycle state without treating that as a
// state-machine violation.
func TestAtomicLifecycleTransitionAllowsSelfTransition(t *testing.T) {
	t.Parallel()

	var lifecycle AtomicLifecycle
	lifecycle.Store(LifecycleActive)

	if transitioned := lifecycle.Transition(LifecycleActive, LifecycleActive); !transitioned {
		t.Fatal("Transition(active, active) = false, want true")
	}

	if got := lifecycle.Load(); got != LifecycleActive {
		t.Fatalf("Load after Transition(active, active) = %s, want active", got)
	}
}

// TestAtomicLifecycleActivate verifies activation behavior.
//
// Activation is a one-way transition from Created to Active. Calling Activate
// again while already active is idempotent and returns false.
func TestAtomicLifecycleActivate(t *testing.T) {
	t.Parallel()

	var lifecycle AtomicLifecycle

	if activated := lifecycle.Activate(); !activated {
		t.Fatal("first Activate() = false, want true")
	}

	if got := lifecycle.Load(); got != LifecycleActive {
		t.Fatalf("Load after first Activate() = %s, want active", got)
	}

	if activated := lifecycle.Activate(); activated {
		t.Fatal("second Activate() = true, want false")
	}

	if got := lifecycle.Load(); got != LifecycleActive {
		t.Fatalf("Load after second Activate() = %s, want active", got)
	}
}

// TestAtomicLifecycleActivatePanicsAfterShutdownStarts verifies that components
// cannot be reactivated after Closing or Closed becomes visible.
func TestAtomicLifecycleActivatePanicsAfterShutdownStarts(t *testing.T) {
	t.Parallel()

	t.Run("closing", func(t *testing.T) {
		t.Parallel()

		var lifecycle AtomicLifecycle
		lifecycle.Store(LifecycleClosing)

		testutil.MustPanicWithMessage(t, errInvalidLifecycleTransition, func() {
			_ = lifecycle.Activate()
		})
	})

	t.Run("closed", func(t *testing.T) {
		t.Parallel()

		var lifecycle AtomicLifecycle
		lifecycle.Store(LifecycleClosed)

		testutil.MustPanicWithMessage(t, errInvalidLifecycleTransition, func() {
			_ = lifecycle.Activate()
		})
	})
}

// TestAtomicLifecycleMethodsPanicForUnknownLoadedState verifies high-level
// helpers preserve load-side state validation.
//
// The test writes invalid raw state directly because package tests can access
// the atomic field. Normal callers cannot do this, but public lifecycle helpers
// should still fail fast if invalid state is ever observed.
func TestAtomicLifecycleMethodsPanicForUnknownLoadedState(t *testing.T) {
	t.Parallel()

	t.Run("Activate", func(t *testing.T) {
		t.Parallel()

		var lifecycle AtomicLifecycle
		lifecycle.state.Store(uint32(99))

		testutil.MustPanicWithMessage(t, errUnknownLifecycleState, func() {
			_ = lifecycle.Activate()
		})
	})

	t.Run("BeginClose", func(t *testing.T) {
		t.Parallel()

		var lifecycle AtomicLifecycle
		lifecycle.state.Store(uint32(99))

		testutil.MustPanicWithMessage(t, errUnknownLifecycleState, func() {
			_ = lifecycle.BeginClose()
		})
	})

	t.Run("MarkClosed", func(t *testing.T) {
		t.Parallel()

		var lifecycle AtomicLifecycle
		lifecycle.state.Store(uint32(99))

		testutil.MustPanicWithMessage(t, errUnknownLifecycleState, func() {
			_ = lifecycle.MarkClosed()
		})
	})
}

// TestAtomicLifecycleBeginClose verifies idempotent close-start behavior.
//
// BeginClose returns true only for the first caller that moves Created or Active
// into Closing. Later calls observe that shutdown already started and return
// false.
func TestAtomicLifecycleBeginClose(t *testing.T) {
	t.Parallel()

	t.Run("from created", func(t *testing.T) {
		t.Parallel()

		var lifecycle AtomicLifecycle

		if started := lifecycle.BeginClose(); !started {
			t.Fatal("BeginClose() from created = false, want true")
		}

		if got := lifecycle.Load(); got != LifecycleClosing {
			t.Fatalf("Load after BeginClose() = %s, want closing", got)
		}

		if started := lifecycle.BeginClose(); started {
			t.Fatal("second BeginClose() = true, want false")
		}
	})

	t.Run("from active", func(t *testing.T) {
		t.Parallel()

		var lifecycle AtomicLifecycle
		lifecycle.Store(LifecycleActive)

		if started := lifecycle.BeginClose(); !started {
			t.Fatal("BeginClose() from active = false, want true")
		}

		if got := lifecycle.Load(); got != LifecycleClosing {
			t.Fatalf("Load after BeginClose() = %s, want closing", got)
		}
	})

	t.Run("from closed", func(t *testing.T) {
		t.Parallel()

		var lifecycle AtomicLifecycle
		lifecycle.Store(LifecycleClosed)

		if started := lifecycle.BeginClose(); started {
			t.Fatal("BeginClose() from closed = true, want false")
		}

		if got := lifecycle.Load(); got != LifecycleClosed {
			t.Fatalf("Load after BeginClose() from closed = %s, want closed", got)
		}
	})

	t.Run("from closing", func(t *testing.T) {
		t.Parallel()

		var lifecycle AtomicLifecycle
		lifecycle.Store(LifecycleClosing)

		if started := lifecycle.BeginClose(); started {
			t.Fatal("BeginClose() from closing = true, want false")
		}

		if got := lifecycle.Load(); got != LifecycleClosing {
			t.Fatalf("Load after BeginClose() from closing = %s, want closing", got)
		}
	})
}

// TestAtomicLifecycleMarkClosed verifies terminal close publication.
//
// MarkClosed accepts Created, Active, and Closing as sources. It returns false
// only when the lifecycle was already Closed.
func TestAtomicLifecycleMarkClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// initial is the state stored before MarkClosed.
		initial LifecycleState

		// wantChanged indicates whether this call should perform the transition.
		wantChanged bool
	}{
		{
			name:        "from created",
			initial:     LifecycleCreated,
			wantChanged: true,
		},
		{
			name:        "from active",
			initial:     LifecycleActive,
			wantChanged: true,
		},
		{
			name:        "from closing",
			initial:     LifecycleClosing,
			wantChanged: true,
		},
		{
			name:        "from closed",
			initial:     LifecycleClosed,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var lifecycle AtomicLifecycle
			lifecycle.Store(tt.initial)

			changed := lifecycle.MarkClosed()
			if changed != tt.wantChanged {
				t.Fatalf("MarkClosed() from %s = %t, want %t", tt.initial, changed, tt.wantChanged)
			}

			if got := lifecycle.Load(); got != LifecycleClosed {
				t.Fatalf("Load after MarkClosed() from %s = %s, want closed", tt.initial, got)
			}
		})
	}
}

// TestAtomicLifecyclePredicates verifies atomic semantic predicates.
//
// These helpers are the public shape higher-level runtime code will use for
// admission checks and shutdown checks.
func TestAtomicLifecyclePredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// state is the lifecycle value stored in the atomic holder.
		state LifecycleState

		wantActive     bool
		wantClosing    bool
		wantClosed     bool
		wantAllowsWork bool
	}{
		{
			name:           "created",
			state:          LifecycleCreated,
			wantActive:     false,
			wantClosing:    false,
			wantClosed:     false,
			wantAllowsWork: false,
		},
		{
			name:           "active",
			state:          LifecycleActive,
			wantActive:     true,
			wantClosing:    false,
			wantClosed:     false,
			wantAllowsWork: true,
		},
		{
			name:           "closing",
			state:          LifecycleClosing,
			wantActive:     false,
			wantClosing:    true,
			wantClosed:     false,
			wantAllowsWork: false,
		},
		{
			name:           "closed",
			state:          LifecycleClosed,
			wantActive:     false,
			wantClosing:    false,
			wantClosed:     true,
			wantAllowsWork: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var lifecycle AtomicLifecycle
			lifecycle.Store(tt.state)

			if got := lifecycle.IsActive(); got != tt.wantActive {
				t.Fatalf("IsActive() = %t, want %t", got, tt.wantActive)
			}

			if got := lifecycle.IsClosing(); got != tt.wantClosing {
				t.Fatalf("IsClosing() = %t, want %t", got, tt.wantClosing)
			}

			if got := lifecycle.IsClosed(); got != tt.wantClosed {
				t.Fatalf("IsClosed() = %t, want %t", got, tt.wantClosed)
			}

			if got := lifecycle.AllowsWork(); got != tt.wantAllowsWork {
				t.Fatalf("AllowsWork() = %t, want %t", got, tt.wantAllowsWork)
			}
		})
	}
}

// TestAtomicLifecycleConcurrentActivate verifies that only one concurrent caller
// performs the Created -> Active transition.
//
// The rest should observe the already-active state and return false.
func TestAtomicLifecycleConcurrentActivate(t *testing.T) {
	t.Parallel()

	const goroutines = 32

	var lifecycle AtomicLifecycle
	var wg sync.WaitGroup

	results := make(chan bool, goroutines)

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			results <- lifecycle.Activate()
		}()
	}

	wg.Wait()
	close(results)

	var activated int
	for result := range results {
		if result {
			activated++
		}
	}

	if activated != 1 {
		t.Fatalf("concurrent Activate true count = %d, want 1", activated)
	}

	if got := lifecycle.Load(); got != LifecycleActive {
		t.Fatalf("Load after concurrent Activate = %s, want active", got)
	}
}

// TestAtomicLifecycleConcurrentBeginClose verifies that only one concurrent
// caller starts shutdown.
//
// This is the core idempotent Close pattern: the first caller performs cleanup,
// while the rest observe that close has already started.
func TestAtomicLifecycleConcurrentBeginClose(t *testing.T) {
	t.Parallel()

	const goroutines = 32

	var lifecycle AtomicLifecycle
	lifecycle.Store(LifecycleActive)

	var wg sync.WaitGroup

	results := make(chan bool, goroutines)

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			results <- lifecycle.BeginClose()
		}()
	}

	wg.Wait()
	close(results)

	var started int
	for result := range results {
		if result {
			started++
		}
	}

	if started != 1 {
		t.Fatalf("concurrent BeginClose true count = %d, want 1", started)
	}

	if got := lifecycle.Load(); got != LifecycleClosing {
		t.Fatalf("Load after concurrent BeginClose = %s, want closing", got)
	}
}

// TestAtomicLifecycleConcurrentMarkClosed verifies that only one concurrent
// caller performs the terminal transition when the lifecycle is not already
// closed.
func TestAtomicLifecycleConcurrentMarkClosed(t *testing.T) {
	t.Parallel()

	const goroutines = 32

	var lifecycle AtomicLifecycle
	lifecycle.Store(LifecycleClosing)

	var wg sync.WaitGroup

	results := make(chan bool, goroutines)

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			results <- lifecycle.MarkClosed()
		}()
	}

	wg.Wait()
	close(results)

	var closed int
	for result := range results {
		if result {
			closed++
		}
	}

	if closed != 1 {
		t.Fatalf("concurrent MarkClosed true count = %d, want 1", closed)
	}

	if got := lifecycle.Load(); got != LifecycleClosed {
		t.Fatalf("Load after concurrent MarkClosed = %s, want closed", got)
	}
}

// TestAtomicLifecycleNilReceiverPanics verifies explicit nil-receiver
// diagnostics.
//
// Methods on nil pointer receivers are legal to call in Go. AtomicLifecycle
// deliberately checks this case and panics with a stable package-specific
// message instead of allowing a generic nil-pointer panic.
func TestAtomicLifecycleNilReceiverPanics(t *testing.T) {
	t.Parallel()

	var lifecycle *AtomicLifecycle

	t.Run("Load", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicLifecycle, func() {
			_ = lifecycle.Load()
		})
	})

	t.Run("Store", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicLifecycle, func() {
			lifecycle.Store(LifecycleActive)
		})
	})

	t.Run("Transition", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicLifecycle, func() {
			_ = lifecycle.Transition(LifecycleCreated, LifecycleActive)
		})
	})

	t.Run("Activate", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicLifecycle, func() {
			_ = lifecycle.Activate()
		})
	})

	t.Run("BeginClose", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicLifecycle, func() {
			_ = lifecycle.BeginClose()
		})
	})

	t.Run("MarkClosed", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicLifecycle, func() {
			_ = lifecycle.MarkClosed()
		})
	})

	t.Run("IsActive", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicLifecycle, func() {
			_ = lifecycle.IsActive()
		})
	})

	t.Run("IsClosing", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicLifecycle, func() {
			_ = lifecycle.IsClosing()
		})
	})

	t.Run("IsClosed", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicLifecycle, func() {
			_ = lifecycle.IsClosed()
		})
	})

	t.Run("AllowsWork", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicLifecycle, func() {
			_ = lifecycle.AllowsWork()
		})
	})
}
