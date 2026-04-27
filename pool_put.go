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
	// errPoolPutNilBuffer is used when Put receives a nil buffer.
	errPoolPutNilBuffer = "bufferpool.Pool.Put: buffer must not be nil"

	// errPoolPutZeroCapacity is used when Put receives a zero-capacity buffer.
	errPoolPutZeroCapacity = "bufferpool.Pool.Put: buffer capacity must be greater than zero"

	// errPoolPutReturnedBuffersDisabled is used when the effective policy has
	// disabled returned-buffer retention.
	errPoolPutReturnedBuffersDisabled = "bufferpool.Pool.Put: returned-buffer retention is disabled"

	// errPoolPutInvalidReturnedBufferPolicy is used when the effective policy has
	// an unknown returned-buffer mode.
	errPoolPutInvalidReturnedBufferPolicy = "bufferpool.Pool.Put: unknown returned-buffer policy"

	// errPoolPutBufferTooLarge is used when a returned buffer exceeds the
	// configured retained-buffer capacity limit.
	errPoolPutBufferTooLarge = "bufferpool.Pool.Put: buffer capacity exceeds max retained buffer capacity"

	// errPoolPutUnsupportedClass is used when a returned buffer capacity cannot
	// be classified to any configured size class.
	errPoolPutUnsupportedClass = "bufferpool.Pool.Put: buffer capacity is not supported by configured classes"

	// errPoolPutClassRejected is used when class-local admission rejects a
	// returned buffer after capacity classification.
	errPoolPutClassRejected = "bufferpool.Pool.Put: buffer rejected by class admission"

	// errPoolPutCreditExhausted is used when shard credit rejects a returned
	// buffer.
	errPoolPutCreditExhausted = "bufferpool.Pool.Put: shard retention credit exhausted"

	// errPoolPutBucketFull is used when shard credit accepts a returned buffer
	// but the physical bucket has no free retained-buffer slot.
	errPoolPutBucketFull = "bufferpool.Pool.Put: shard bucket is full"
)

// Put returns buffer to the Pool for possible retention.
//
// Put is a best-effort reuse operation governed by the effective Policy.
// Depending on admission actions, unsuitable returned buffers may be silently
// dropped or reported as errors.
//
// Put records valid public return attempts before lifecycle admission so
// post-close drop-return behavior remains visible in snapshots and metrics. nil
// and zero-capacity inputs are rejected before accounting because they are
// invalid API input rather than reusable buffer returns.
//
// Retention zeroing and dropped-buffer zeroing are intentionally separate:
// ZeroRetainedBuffers is applied only after class/shard storage accepts the
// buffer, while ZeroDroppedBuffers is applied only when this call will not
// retain the buffer.
//
// nil and zero-capacity buffers are invalid public input because they cannot
// contribute reusable retained capacity.
func (p *Pool) Put(buffer []byte) error {
	p.mustBeInitialized()

	if buffer == nil {
		return newError(ErrNilBuffer, errPoolPutNilBuffer)
	}

	if cap(buffer) == 0 {
		return newError(ErrZeroCapacity, errPoolPutZeroCapacity)
	}

	capacity := uint64(cap(buffer))
	p.ownerCounters.recordPut(capacity)

	runtime := p.currentRuntimeSnapshot()

	admitted, err := p.beginReturnOperation()
	if err != nil {
		return err
	}

	if !admitted {
		p.recordOwnerDrop(runtime.Policy, PoolDropReasonClosedPool, capacity, buffer)
		return nil
	}

	err = p.put(buffer, runtime)
	p.endOperation()

	return err
}

// put executes an already lifecycle-admitted return path.
//
// This method owns owner-side return admission, returned-capacity class routing,
// retained/dropped buffer zeroing, classState retention call, and mapping local
// class/shard outcomes to public error classes.
//
// Admission order is deliberately explicit:
//
//   - effective returned-buffer policy;
//   - maximum retained capacity;
//   - capacity-to-class floor routing;
//   - class/shard retention attempt;
//   - retained or dropped buffer hygiene;
//   - public error classification.
//
// Bare []byte Put can only perform capacity-based admission. Origin-class
// growth checks require an ownership-aware lease API and are not hidden here.
func (p *Pool) put(buffer []byte, runtime *poolRuntimeSnapshot) error {
	policy := runtime.Policy

	switch policy.Admission.ReturnedBuffers {
	case ReturnedBufferPolicyDrop:
		p.recordOwnerDrop(policy, PoolDropReasonReturnedBuffersDisabled, uint64(cap(buffer)), buffer)
		return nil

	case ReturnedBufferPolicyAdmit:

	default:
		p.recordOwnerDrop(policy, PoolDropReasonInvalidPolicy, uint64(cap(buffer)), buffer)
		return newError(ErrInvalidPolicy, errPoolPutInvalidReturnedBufferPolicy)
	}

	capacity := SizeFromInt(cap(buffer))

	if capacity > policy.Retention.MaxRetainedBufferCapacity {
		return p.handleBufferAdmissionAction(
			policy,
			policy.Admission.OversizedReturn,
			ErrBufferTooLarge,
			errPoolPutBufferTooLarge,
			buffer,
			PoolDropReasonOversized,
		)
	}

	class, ok := p.table.classForCapacity(capacity)
	if !ok {
		return p.handleBufferAdmissionAction(
			policy,
			policy.Admission.UnsupportedClass,
			ErrUnsupportedClass,
			errPoolPutUnsupportedClass,
			buffer,
			PoolDropReasonUnsupportedClass,
		)
	}

	result := p.mustClassStateFor(class).tryRetainSelected(p.shardSelectorFor(class), buffer)
	if result.Retained() {
		p.zeroRetainedBuffer(buffer, policy)
		return nil
	}

	if result.RejectedByClass() {
		return p.handleBufferAdmissionAction(
			policy,
			policy.Admission.UnsupportedClass,
			ErrRetentionRejected,
			errPoolPutClassRejected,
			buffer,
			PoolDropReasonNone,
		)
	}

	if result.RejectedByCredit() {
		return p.handleBufferAdmissionAction(
			policy,
			policy.Admission.CreditExhausted,
			ErrRetentionCreditExhausted,
			errPoolPutCreditExhausted,
			buffer,
			PoolDropReasonNone,
		)
	}

	if result.RejectedByBucket() {
		return p.handleBufferAdmissionAction(
			policy,
			policy.Admission.BucketFull,
			ErrRetentionStorageFull,
			errPoolPutBucketFull,
			buffer,
			PoolDropReasonNone,
		)
	}

	return nil
}
