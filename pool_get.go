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
	// errPoolGetNegativeSize is used when Get receives a negative requested
	// size.
	errPoolGetNegativeSize = "bufferpool.Pool.Get: requested size must not be negative"

	// errPoolGetZeroSizeRejected is used when policy rejects zero-size requests.
	errPoolGetZeroSizeRejected = "bufferpool.Pool.Get: zero-size request rejected by policy"

	// errPoolGetRequestTooLarge is used when a request exceeds the configured
	// request-side limit.
	errPoolGetRequestTooLarge = "bufferpool.Pool.Get: requested size exceeds max request size"

	// errPoolGetUnsupportedClass is used when the class table cannot normalize a
	// requested size to a configured class.
	errPoolGetUnsupportedClass = "bufferpool.Pool.Get: requested size is not supported by configured classes"

	// errPoolGetSizeExceedsInt is used when a valid Size value cannot be passed
	// to Go slice allocation APIs on the current platform.
	errPoolGetSizeExceedsInt = "bufferpool.Pool.Get: requested size exceeds int range"

	// errPoolGetClassSizeExceedsInt is used when a configured class size cannot
	// be used as a Go slice capacity on the current platform.
	errPoolGetClassSizeExceedsInt = "bufferpool.Pool.Get: class size exceeds int range"
)

// Get returns a byte slice with length size.
//
// On reuse hit, Get returns a retained buffer resized to len == size. On miss,
// Get allocates a new buffer with len == size and capacity equal to the selected
// normalized class size. Allocation is recorded against the shard selected for
// the miss.
//
// Get never returns retained buffers for requests that policy rejects or that
// cannot be represented by the configured class table.
func (p *Pool) Get(size int) ([]byte, error) {
	p.mustBeInitialized()

	if size < 0 {
		return nil, newError(ErrInvalidSize, errPoolGetNegativeSize)
	}

	return p.getSize(SizeFromInt(size))
}

// GetSize is the Size-typed form of Get.
//
// Use GetSize when the caller already works with bufferpool Size values and
// wants to avoid converting through int at the call site.
func (p *Pool) GetSize(requestedSize Size) ([]byte, error) {
	p.mustBeInitialized()

	return p.getSize(requestedSize)
}

// getSize runs the common acquisition path after receiver validation.
//
// Both Get and GetSize enter here so the receiver is checked once. The method
// loads the runtime snapshot before planning and uses that same snapshot for the
// whole operation, even if a future controller publishes a newer snapshot while
// the call is running.
//
// Pure validation errors are returned before lifecycle admission. Valid
// acquisition requests, including zero-size empty-buffer requests, enter the
// lifecycle gate so Close consistently rejects acquisition after shutdown.
func (p *Pool) getSize(requestedSize Size) ([]byte, error) {
	runtime := p.currentRuntimeSnapshot()

	plan, err := p.planGet(requestedSize, runtime)
	if err != nil {
		return nil, err
	}

	if err := p.beginAcquireOperation(); err != nil {
		return nil, err
	}

	if plan.empty {
		p.endOperation()
		return []byte{}, nil
	}

	buffer, err := p.get(plan)
	p.endOperation()

	return buffer, err
}

// get executes an already lifecycle-admitted acquisition.
//
// This method owns retained reuse attempt, allocation on miss, and allocation
// accounting. Pure request validation and class routing are done before lifecycle
// admission by planGet.
func (p *Pool) get(plan poolGetPlan) ([]byte, error) {
	state := p.mustClassStateFor(plan.class)
	result := state.tryGetSelected(p.shardSelectorFor(plan.class), plan.requestedSize)
	if result.Hit() {
		return result.Buffer()[:plan.requestedSizeInt], nil
	}

	buffer := make([]byte, plan.requestedSizeInt, plan.classSizeInt)
	state.recordAllocation(result.ShardIndex, uint64(cap(buffer)))

	return buffer, nil
}

// poolGetPlan is the immutable acquisition plan for one Get/GetSize call.
//
// Planning happens before lifecycle admission because it is pure validation and
// routing: it reads the runtime policy snapshot and class table but does not
// touch retained storage. This keeps invalid requests from briefly entering the
// active-operation drain counter during Close.
type poolGetPlan struct {
	// requestedSize is the normalized logical length requested by the caller.
	requestedSize Size

	// requestedSizeInt is requestedSize converted for Go slice operations.
	requestedSizeInt int

	// class is the size class selected by ceil request routing.
	class SizeClass

	// classSizeInt is class.ByteSize converted for allocation capacity.
	classSizeInt int

	// empty means policy resolved the request without retained storage or
	// allocation. The public call returns a non-nil empty slice for this case.
	empty bool
}

// planGet validates and routes one acquisition request against runtime.
//
// The returned plan is stable for the operation even if a future controller
// publishes a newer runtime snapshot concurrently. The method performs only
// pure checks, so it is safe to run before beginAcquireOperation.
func (p *Pool) planGet(requestedSize Size, runtime *poolRuntimeSnapshot) (poolGetPlan, error) {
	requestedSize, shouldContinue, err := p.normalizeGetRequest(requestedSize, runtime.Policy)
	if err != nil || !shouldContinue {
		return poolGetPlan{empty: !shouldContinue}, err
	}

	if requestedSize > runtime.Policy.Retention.MaxRequestSize {
		return poolGetPlan{}, newError(ErrRequestTooLarge, errPoolGetRequestTooLarge)
	}

	class, ok := p.table.classForRequest(requestedSize)
	if !ok {
		return poolGetPlan{}, newError(ErrUnsupportedClass, errPoolGetUnsupportedClass)
	}

	requestedSizeInt, ok := poolSizeInt(requestedSize)
	if !ok {
		return poolGetPlan{}, newError(ErrInvalidSize, errPoolGetSizeExceedsInt)
	}

	classSizeInt, ok := poolSizeInt(class.ByteSize())
	if !ok {
		return poolGetPlan{}, newError(ErrInvalidPolicy, errPoolGetClassSizeExceedsInt)
	}

	return poolGetPlan{
		requestedSize:    requestedSize,
		requestedSizeInt: requestedSizeInt,
		class:            class,
		classSizeInt:     classSizeInt,
	}, nil
}

// normalizeGetRequest applies zero-size request policy.
//
// Zero-size behavior is policy-owned because different users may want different
// semantics:
//
//   - smallest class: produce a reusable zero-length buffer with class capacity;
//   - empty buffer: return a zero-length non-retained buffer and avoid counters;
//   - reject: report ErrInvalidSize.
//
// For the empty-buffer policy, the returned boolean is false. getSize then
// returns a non-nil empty slice without entering the lifecycle gate or touching
// retained storage.
func (p *Pool) normalizeGetRequest(requestedSize Size, policy Policy) (Size, bool, error) {
	if !requestedSize.IsZero() {
		return requestedSize, true, nil
	}

	switch policy.Admission.ZeroSizeRequests {
	case ZeroSizeRequestSmallestClass:
		return 0, true, nil

	case ZeroSizeRequestEmptyBuffer:
		return 0, false, nil

	case ZeroSizeRequestReject:
		return 0, false, newError(ErrInvalidSize, errPoolGetZeroSizeRejected)

	default:
		return 0, false, newError(ErrInvalidPolicy, errPoolGetZeroSizeRejected)
	}
}

// poolSizeInt converts Size to int for Go slice APIs.
//
// Size is uint64-backed, but make([]byte, len, cap) requires int lengths and
// capacities. This helper lets Pool reject impossible policy/request values with
// classified errors instead of relying on a runtime panic or architecture-
// dependent truncation.
func poolSizeInt(size Size) (int, bool) {
	maxInt := uint64(^uint(0) >> 1)
	if size.Bytes() > maxInt {
		return 0, false
	}

	return int(size), true
}
