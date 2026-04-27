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

// handleAdmissionAction maps an admission action to the public error contract.
//
// This helper is for conditions that do not need buffer zeroing. Return-path
// paths that discard a concrete buffer should prefer handleBufferAdmissionAction
// so ZeroDroppedBuffers is applied consistently.
func (p *Pool) handleAdmissionAction(action AdmissionAction, kind error, message string) error {
	switch action {
	case AdmissionActionDrop, AdmissionActionIgnore:
		return nil

	case AdmissionActionError:
		return newError(kind, message)

	default:
		return newError(ErrInvalidPolicy, message)
	}
}

// handleBufferAdmissionAction applies admission action semantics for a concrete
// returned buffer.
//
// If the action does not retain the buffer and ZeroDroppedBuffers is enabled,
// the buffer's full capacity is cleared before the call returns. This applies to
// drop, ignore, and error actions because all three represent a no-retain
// outcome for the returned buffer.
//
// reason is recorded only for owner-side decisions. Class/shard retain failures
// pass PoolDropReasonNone because classState and shard counters have already
// counted the drop; recording another owner-side drop would double count the
// aggregate outcome.
func (p *Pool) handleBufferAdmissionAction(policy Policy, action AdmissionAction, kind error, message string, buffer []byte, reason PoolDropReason) error {
	switch action {
	case AdmissionActionDrop, AdmissionActionIgnore:
		p.handleDroppedBuffer(policy, reason, uint64(cap(buffer)), buffer)
		return nil

	case AdmissionActionError:
		p.handleDroppedBuffer(policy, reason, uint64(cap(buffer)), buffer)
		return newError(kind, message)

	default:
		p.handleDroppedBuffer(policy, PoolDropReasonInvalidPolicy, uint64(cap(buffer)), buffer)
		return newError(ErrInvalidPolicy, message)
	}
}

func (p *Pool) recordOwnerDrop(policy Policy, reason PoolDropReason, capacity uint64, buffer []byte) {
	p.ownerCounters.recordDrop(reason, capacity)
	zeroDroppedBuffer(buffer, policy)
}

// handleDroppedBuffer applies no-retain buffer hygiene and optional owner-side
// drop accounting.
//
// PoolDropReasonNone means a lower layer already recorded the drop. The buffer
// still needs dropped-buffer zeroing because it will not remain in retained
// storage, but pool-owner drop counters must not be incremented.
func (p *Pool) handleDroppedBuffer(policy Policy, reason PoolDropReason, capacity uint64, buffer []byte) {
	if reason != PoolDropReasonNone {
		p.ownerCounters.recordDrop(reason, capacity)
	}

	zeroDroppedBuffer(buffer, policy)
}

// zeroRetainedBuffer clears a returned buffer when ZeroRetainedBuffers is
// enabled.
//
// The full backing capacity is cleared, not just len(buffer). Retained memory is
// represented by backing array capacity, and stale data may live beyond the
// current slice length.
func (p *Pool) zeroRetainedBuffer(buffer []byte, policy Policy) {
	if !policy.Admission.ZeroRetainedBuffers {
		return
	}

	zeroBufferCapacity(buffer)
}

// zeroDroppedBuffer clears a returned buffer when ZeroDroppedBuffers is enabled.
//
// This is used for no-retain outcomes. It is intentionally separate from
// zeroRetainedBuffer so secure profiles can independently control retained and
// dropped buffer hygiene.
func zeroDroppedBuffer(buffer []byte, policy Policy) {
	if !policy.Admission.ZeroDroppedBuffers {
		return
	}

	zeroBufferCapacity(buffer)
}

// zeroBufferCapacity clears the full capacity range of buffer.
//
// The caller must pass a non-nil positive-capacity buffer. Public Put validation
// enforces that before return-path admission reaches this helper.
func zeroBufferCapacity(buffer []byte) {
	clear(buffer[:cap(buffer)])
}
