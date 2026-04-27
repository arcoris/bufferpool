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

// handleClassRetainRejection applies the policy action for class-local retain
// rejection.
//
// UnsupportedClass is used when capacity routing cannot find a class at all.
// Once a returned buffer has reached classState, a class-level rejection means
// the routed class and returned capacity are incompatible, so ClassMismatch is
// the precise admission action.
func (p *Pool) handleClassRetainRejection(policy Policy, buffer []byte) error {
	return p.handleBufferAdmissionAction(
		policy,
		policy.Admission.ClassMismatch,
		ErrRetentionRejected,
		errPoolPutClassRejected,
		buffer,
		PoolDropReasonNone,
	)
}

// recordOwnerDrop records and handles an owner-side drop.
//
// This helper is for decisions made before class/shard accounting owns the
// outcome: closed-pool drops, disabled returned-buffer retention, oversized
// returns, unsupported class routing, and invalid runtime policy. Class/shard
// rejections should use handleDroppedBuffer with PoolDropReasonNone instead.
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

// zeroDroppedBuffer clears a returned buffer when ZeroDroppedBuffers is enabled.
//
// This is used for no-retain outcomes. It is intentionally separate from
// retained zeroing in the shard publication path so secure profiles can
// independently control retained and dropped buffer hygiene.
func zeroDroppedBuffer(buffer []byte, policy Policy) {
	if !policy.Admission.ZeroDroppedBuffers {
		return
	}

	zeroBufferCapacity(buffer)
}

// zeroBufferCapacity clears the full capacity range of buffer.
//
// The caller must pass a non-nil positive-capacity buffer. Public Put validation
// enforces that for Pool return paths, and class/shard retain paths call this
// only after class admission has accepted the buffer.
func zeroBufferCapacity(buffer []byte) {
	clear(buffer[:cap(buffer)])
}
