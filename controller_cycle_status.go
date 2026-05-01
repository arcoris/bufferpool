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
	"sync/atomic"
)

const (
	// controllerCycleReasonClosed reports a tick rejected by owner lifecycle.
	controllerCycleReasonClosed = "controller_cycle_closed"

	// controllerCycleReasonAlreadyRunning reports an explicitly rejected overlap.
	controllerCycleReasonAlreadyRunning = "controller_cycle_already_running"

	// controllerCycleReasonFailed reports a controller cycle error that is not
	// better represented by a more specific stable reason.
	controllerCycleReasonFailed = "controller_cycle_failed"

	// controllerCycleReasonUnpublished reports a diagnostic cycle that computed
	// targets but did not publish them.
	controllerCycleReasonUnpublished = "controller_cycle_unpublished"

	// controllerCycleReasonSkipped reports a non-error cycle with no publication
	// work to apply.
	controllerCycleReasonSkipped = "controller_cycle_skipped"

	// controllerCycleReasonNoWork reports an explicit no-work skip condition.
	controllerCycleReasonNoWork = "controller_cycle_no_work"
)

// ControllerCycleStatus identifies the outcome of one foreground controller
// attempt.
//
// The status vocabulary is intentionally scheduler-ready but does not implement
// scheduling. PoolPartition.TickInto and PoolGroup.TickInto publish these values
// for manual foreground calls only; Pool.Get and Pool.Put never read or update
// them.
type ControllerCycleStatus uint8

const (
	// ControllerCycleStatusUnset means no controller cycle status has been
	// published.
	ControllerCycleStatusUnset ControllerCycleStatus = iota

	// ControllerCycleStatusSkipped means the cycle found no publication work or
	// an equivalent non-error skip condition.
	ControllerCycleStatusSkipped

	// ControllerCycleStatusApplied means the cycle committed controller state or
	// accepted the required runtime publication for that owner.
	ControllerCycleStatusApplied

	// ControllerCycleStatusUnpublished means diagnostics were produced but the
	// required budget publication was not applied.
	ControllerCycleStatusUnpublished

	// ControllerCycleStatusFailed means the cycle attempted control work and hit
	// a validation, publication, or internal control error.
	ControllerCycleStatusFailed

	// ControllerCycleStatusClosed means the owner was closing or closed before
	// the cycle could run.
	ControllerCycleStatusClosed

	// ControllerCycleStatusAlreadyRunning means another foreground cycle for the
	// same owner was already in progress.
	ControllerCycleStatusAlreadyRunning
)

// String returns a stable diagnostic label for s.
func (s ControllerCycleStatus) String() string {
	switch s {
	case ControllerCycleStatusUnset:
		return "unset"
	case ControllerCycleStatusSkipped:
		return "skipped"
	case ControllerCycleStatusApplied:
		return "applied"
	case ControllerCycleStatusUnpublished:
		return "unpublished"
	case ControllerCycleStatusFailed:
		return "failed"
	case ControllerCycleStatusClosed:
		return "closed"
	case ControllerCycleStatusAlreadyRunning:
		return "already_running"
	default:
		return "unknown"
	}
}

// ControllerCycleStatusSnapshot is the retained lightweight status for the most
// recent foreground controller attempt.
//
// The snapshot is generation-based on purpose. It keeps no heavy report data,
// score diagnostics, samples, trim candidates, windows, maps, or slices. Full
// controller reports are still returned by TickInto; this value only answers
// whether the last manual cycle applied, skipped, failed, or was rejected before
// overlap.
type ControllerCycleStatusSnapshot struct {
	// Status is the last published foreground cycle outcome.
	Status ControllerCycleStatus

	// AttemptGeneration is the owner event generation attempted by the cycle.
	AttemptGeneration Generation

	// AppliedGeneration is the generation accepted by the runtime publication or
	// committed controller state. It is NoGeneration for skipped, unpublished,
	// failed, closed, and overlap-rejected cycles.
	AppliedGeneration Generation

	// LastSuccessfulGeneration is the most recent AppliedGeneration from an
	// applied cycle.
	LastSuccessfulGeneration Generation

	// ConsecutiveFailures counts adjacent failed cycles. Closed and overlap
	// rejections are reported separately and do not increment this counter.
	ConsecutiveFailures uint64

	// ConsecutiveSkipped counts adjacent no-work skipped cycles.
	ConsecutiveSkipped uint64

	// ConsecutiveUnpublished counts adjacent diagnostic-only cycles. These are
	// not failures, but they are visible because repeated non-publication is a
	// useful generation-based freshness signal for future foreground orchestration.
	ConsecutiveUnpublished uint64

	// FailureReason is empty on success and a stable machine-readable reason for
	// closed, failed, unpublished, skipped, or overlap-rejected cycles.
	FailureReason string
}

// PoolPartitionControllerStatus is the lightweight retained status for
// PoolPartition foreground controller cycles.
type PoolPartitionControllerStatus = ControllerCycleStatusSnapshot

// PoolGroupControllerStatus is the lightweight retained status for PoolGroup
// foreground coordinator cycles.
type PoolGroupControllerStatus = ControllerCycleStatusSnapshot

// controllerCycleGate rejects overlapping foreground controller cycles without
// replacing the owner's lifecycle gates or controller mutex.
type controllerCycleGate struct {
	running atomic.Bool
}

// begin admits one foreground controller cycle when no peer cycle is running.
func (g *controllerCycleGate) begin() bool {
	return g.running.CompareAndSwap(false, true)
}

// end releases a controller cycle admitted by begin.
func (g *controllerCycleGate) end() {
	g.running.Store(false)
}

// isRunning reports whether a foreground cycle is currently admitted.
func (g *controllerCycleGate) isRunning() bool {
	return g.running.Load()
}

// controllerCycleStatusStore owns the retained lightweight controller status.
//
// FailureReason follows a compact policy: applied cycles clear it, lifecycle and
// overlap rejections use controller-cycle reasons, returned errors use stable
// error mappings, and unpublished cycles may carry a more specific nested budget
// publication reason when one is available. The detailed returned error remains
// separate from this machine-readable status field.
type controllerCycleStatusStore struct {
	mu       sync.RWMutex
	snapshot ControllerCycleStatusSnapshot
}

// load returns a copy of the retained status snapshot.
func (s *controllerCycleStatusStore) load() ControllerCycleStatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshot
}

// publish stores and returns the next lightweight controller status snapshot.
func (s *controllerCycleStatusStore) publish(
	status ControllerCycleStatus,
	attemptGeneration Generation,
	appliedGeneration Generation,
	reason string,
) ControllerCycleStatusSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := ControllerCycleStatusSnapshot{
		Status:                   status,
		AttemptGeneration:        attemptGeneration,
		AppliedGeneration:        appliedGeneration,
		LastSuccessfulGeneration: s.snapshot.LastSuccessfulGeneration,
		ConsecutiveFailures:      s.snapshot.ConsecutiveFailures,
		ConsecutiveSkipped:       s.snapshot.ConsecutiveSkipped,
		ConsecutiveUnpublished:   s.snapshot.ConsecutiveUnpublished,
		FailureReason:            reason,
	}

	switch status {
	case ControllerCycleStatusApplied:
		next.LastSuccessfulGeneration = appliedGeneration
		next.ConsecutiveFailures = 0
		next.ConsecutiveSkipped = 0
		next.ConsecutiveUnpublished = 0
		next.FailureReason = ""
	case ControllerCycleStatusSkipped:
		next.ConsecutiveSkipped++
		next.ConsecutiveFailures = 0
		next.ConsecutiveUnpublished = 0
	case ControllerCycleStatusFailed:
		next.ConsecutiveFailures++
		next.ConsecutiveSkipped = 0
		next.ConsecutiveUnpublished = 0
	case ControllerCycleStatusUnpublished:
		next.ConsecutiveUnpublished++
		next.ConsecutiveFailures = 0
		next.ConsecutiveSkipped = 0
	case ControllerCycleStatusClosed, ControllerCycleStatusAlreadyRunning:
		// Closed and overlap-rejected calls are lifecycle/orchestration outcomes,
		// not controller computation failures, skips, or unpublished cycles. They
		// leave the computation streak counters intact.
	default:
	}

	s.snapshot = next
	return next
}

// controllerCycleFailureReasonForError maps returned errors to stable status
// reasons while preserving the original error for the caller.
func controllerCycleFailureReasonForError(err error, fallback string) string {
	if fallback == "" {
		fallback = controllerCycleReasonFailed
	}
	if err == nil {
		return fallback
	}
	if errors.Is(err, ErrClosed) {
		return controllerCycleReasonClosed
	}
	if errors.Is(err, ErrInvalidPolicy) || errors.Is(err, ErrInvalidOptions) {
		return policyUpdateFailureInvalid
	}
	return fallback
}

// controllerCycleBudgetPublicationFailureReasonForError maps budget publication
// errors into stable retained status/report reasons. The returned error still
// carries detailed wrapping for errors.Is checks; report fields use this compact
// machine-readable value.
func controllerCycleBudgetPublicationFailureReasonForError(err error, fallback string) string {
	return controllerCycleFailureReasonForError(err, fallback)
}

// controllerCycleUnpublishedFailureReason returns the most specific stable
// reason available for a diagnostic-only non-publication.
func controllerCycleUnpublishedFailureReason(reason string) string {
	if reason != "" {
		return reason
	}
	return controllerCycleReasonUnpublished
}
