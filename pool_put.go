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

	// errPoolReturnOutcomeMissingErrorKind is used when an internal return
	// outcome asks Pool to report an error without selecting a public error
	// class.
	errPoolReturnOutcomeMissingErrorKind = "bufferpool.Pool.Put: return outcome error kind must not be nil"
)

// Put returns buffer to the Pool for possible retention.
//
// Put is a best-effort reuse operation governed by the effective Policy.
// Depending on admission actions, unsuitable returned buffers may be silently
// dropped or reported as errors.
//
// Bare []byte returns are capacity-admitted, not ownership-verified. The
// current API cannot prove that a buffer originated from this Pool, cannot
// detect double release, cannot enforce origin-class capacity growth, and cannot
// track in-use bytes or buffers. Those guarantees require a lease-aware
// ownership layer above or beside the bare []byte API; Pool does not fake them
// in the hot path.
//
// Put records valid public return attempts before lifecycle admission so
// post-close drop-return behavior remains visible in snapshots and metrics. nil
// and zero-capacity inputs are rejected before accounting because they are
// invalid API input rather than reusable buffer returns.
//
// Close reject mode returns ErrClosed after the valid Put attempt is counted,
// but it does not accept the buffer as a dropped return. In that mode the caller
// remains responsible for the buffer and ZeroDroppedBuffers is not applied.
//
// Retention zeroing and dropped-buffer zeroing are intentionally separate:
// ZeroRetainedBuffers is applied inside the shard retain path before bucket
// publication, while ZeroDroppedBuffers is applied only when this call will not
// retain the buffer.
//
// nil and zero-capacity buffers are invalid public input because they cannot
// contribute reusable retained capacity.
func (p *Pool) Put(buffer []byte) error {
	p.mustBeInitialized()

	input, err := p.validateReturnInput(buffer)
	if err != nil {
		return err
	}

	p.ownerCounters.recordPut(input.Capacity)

	runtime := p.currentRuntimeSnapshot()

	admitted, err := p.beginReturnOperation()
	if err != nil {
		return err
	}

	if !admitted {
		return p.applyReturnOutcome(
			runtime.Policy,
			input,
			poolReturnOutcomeDrop(
				AdmissionActionDrop,
				nil,
				"",
				PoolDropReasonClosedPool,
				input.Capacity,
			),
		)
	}

	outcome := p.returnBuffer(input, runtime)
	err = p.applyReturnOutcome(runtime.Policy, input, outcome)
	p.endOperation()

	return err
}

// validateReturnInput validates public Put input and records the capacity value
// used by every later return-path stage.
//
// nil and zero-capacity buffers are invalid API input. They are rejected before
// owner counters because they are not valid returned-buffer attempts.
func (p *Pool) validateReturnInput(buffer []byte) (poolReturnInput, error) {
	if buffer == nil {
		return poolReturnInput{}, newError(ErrNilBuffer, errPoolPutNilBuffer)
	}

	if cap(buffer) == 0 {
		return poolReturnInput{}, newError(ErrZeroCapacity, errPoolPutZeroCapacity)
	}

	return poolReturnInput{
		Buffer:   buffer,
		Capacity: uint64(cap(buffer)),
	}, nil
}

// returnBuffer executes owner-side and class/shard return admission after
// lifecycle admission.
//
// The method decides the outcome but does not apply dropped-buffer hygiene or
// owner-side drop accounting. Keeping decision and application separate makes
// the Put pipeline reviewable:
//
//	validate input
//	-> record valid public Put attempt
//	-> lifecycle gate
//	-> owner-side policy admission
//	-> class routing
//	-> class/shard retain
//	-> apply outcome
//
// Bare []byte Put can only perform capacity-based admission. Origin-class
// growth checks, double-release detection, and in-use accounting require an
// ownership-aware lease API and are not hidden here.
func (p *Pool) returnBuffer(input poolReturnInput, runtime *poolRuntimeSnapshot) poolReturnOutcome {
	policy := runtime.Policy

	switch policy.Admission.ReturnedBuffers {
	case ReturnedBufferPolicyDrop:
		return poolReturnOutcomeDrop(
			AdmissionActionDrop,
			nil,
			"",
			PoolDropReasonReturnedBuffersDisabled,
			input.Capacity,
		)

	case ReturnedBufferPolicyAdmit:

	default:
		return poolReturnOutcomeDrop(
			AdmissionActionError,
			ErrInvalidPolicy,
			errPoolPutInvalidReturnedBufferPolicy,
			PoolDropReasonInvalidPolicy,
			input.Capacity,
		)
	}

	capacity := SizeFromBytes(input.Capacity)
	if capacity > policy.Retention.MaxRetainedBufferCapacity {
		return poolReturnOutcomeDrop(
			policy.Admission.OversizedReturn,
			ErrBufferTooLarge,
			errPoolPutBufferTooLarge,
			PoolDropReasonOversized,
			input.Capacity,
		)
	}

	class, ok := p.table.classForCapacity(capacity)
	if !ok {
		return poolReturnOutcomeDrop(
			policy.Admission.UnsupportedClass,
			ErrUnsupportedClass,
			errPoolPutUnsupportedClass,
			PoolDropReasonUnsupportedClass,
			input.Capacity,
		)
	}

	result := p.mustClassStateFor(class).tryRetainSelectedWithOptions(
		p.shardSelectorFor(class),
		input.Buffer,
		classRetainOptions{
			ZeroBeforeRetain: policy.Admission.ZeroRetainedBuffers,
		},
	)
	if result.Retained() {
		return poolReturnOutcomeRetained(input.Capacity)
	}

	if result.RejectedByClass() {
		return poolReturnOutcomeDrop(
			policy.Admission.ClassMismatch,
			ErrRetentionRejected,
			errPoolPutClassRejected,
			PoolDropReasonNone,
			input.Capacity,
		)
	}

	if result.RejectedByCredit() {
		return poolReturnOutcomeDrop(
			policy.Admission.CreditExhausted,
			ErrRetentionCreditExhausted,
			errPoolPutCreditExhausted,
			PoolDropReasonNone,
			input.Capacity,
		)
	}

	if result.RejectedByBucket() {
		return poolReturnOutcomeDrop(
			policy.Admission.BucketFull,
			ErrRetentionStorageFull,
			errPoolPutBucketFull,
			PoolDropReasonNone,
			input.Capacity,
		)
	}

	return poolReturnOutcomeDrop(
		AdmissionActionDrop,
		nil,
		"",
		PoolDropReasonNone,
		input.Capacity,
	)
}

// applyReturnOutcome applies the already-decided return outcome.
//
// Retained outcomes need no Pool-side zeroing because ZeroRetainedBuffers runs
// inside shard publication before bucket visibility. No-retain outcomes apply
// ZeroDroppedBuffers here after the lower layer has recorded its own drop, or
// after owner-side accounting has recorded an owner drop.
func (p *Pool) applyReturnOutcome(policy Policy, input poolReturnInput, outcome poolReturnOutcome) error {
	if outcome.Retained {
		return nil
	}

	reason := outcome.DropReason
	if !poolAdmissionActionKnown(outcome.Action) {
		if reason != PoolDropReasonNone {
			reason = PoolDropReasonInvalidPolicy
		}
	}

	if reason != PoolDropReasonNone {
		p.ownerCounters.recordDrop(reason, outcome.Capacity)
	}

	zeroDroppedBuffer(input.Buffer, policy)

	switch outcome.Action {
	case AdmissionActionDrop, AdmissionActionIgnore:
		return nil

	case AdmissionActionError:
		if outcome.Kind == nil {
			return newError(ErrInvalidPolicy, errPoolReturnOutcomeMissingErrorKind)
		}

		return newError(outcome.Kind, outcome.Message)

	default:
		return newError(ErrInvalidPolicy, outcome.Message)
	}
}

// poolReturnInput is the validated public Put input carried through the return
// pipeline.
type poolReturnInput struct {
	// Buffer is the caller-provided slice. Retention and zeroing operate on its
	// full backing capacity.
	Buffer []byte

	// Capacity is cap(Buffer) captured once after validation.
	Capacity uint64
}

// poolReturnOutcome describes the final return-path decision before Pool
// applies no-retain hygiene and public error mapping.
type poolReturnOutcome struct {
	// Retained is true only when class/shard storage accepted and published the
	// returned buffer.
	Retained bool

	// Action is the policy-selected action to apply for a no-retain outcome.
	Action AdmissionAction

	// Kind is the public error class used when Action is AdmissionActionError.
	Kind error

	// Message is the operation-specific diagnostic error message.
	Message string

	// DropReason is recorded only for owner-side drops. PoolDropReasonNone means
	// classState or shard already counted the drop.
	DropReason PoolDropReason

	// Capacity is the returned backing capacity used for drop accounting.
	Capacity uint64
}

// poolReturnOutcomeRetained constructs the successful return outcome.
func poolReturnOutcomeRetained(capacity uint64) poolReturnOutcome {
	return poolReturnOutcome{
		Retained: true,
		Capacity: capacity,
	}
}

// poolReturnOutcomeDrop constructs a no-retain outcome.
//
// reason must be PoolDropReasonNone for class/shard drops because those lower
// layers already update drop counters. Owner-side decisions pass a concrete
// reason so aggregate Pool counters include returns that never reached storage.
func poolReturnOutcomeDrop(action AdmissionAction, kind error, message string, reason PoolDropReason, capacity uint64) poolReturnOutcome {
	return poolReturnOutcome{
		Action:     action,
		Kind:       kind,
		Message:    message,
		DropReason: reason,
		Capacity:   capacity,
	}
}

// poolAdmissionActionKnown reports whether Pool can execute action while
// applying a return outcome.
//
// Policy validation should reject unknown actions before construction, but the
// runtime snapshot boundary is defensive. Unknown actions are treated as invalid
// policy and still run dropped-buffer hygiene for the returned buffer.
func poolAdmissionActionKnown(action AdmissionAction) bool {
	switch action {
	case AdmissionActionDrop, AdmissionActionIgnore, AdmissionActionError:
		return true
	default:
		return false
	}
}
