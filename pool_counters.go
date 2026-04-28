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

import "sync/atomic"

// PoolDropReason identifies why Pool owner-side admission discarded a returned
// buffer before class/shard retention completed.
//
// These reasons are pool-owner facts. They cover decisions made outside
// classState and shard counters, so aggregate snapshots can include returned
// buffers that never reached lower retained-storage accounting.
type PoolDropReason uint8

const (
	// PoolDropReasonNone means no pool-owner drop reason is recorded.
	//
	// This value is used when classState or shard has already counted the drop
	// and Pool only needs to apply dropped-buffer hygiene.
	PoolDropReasonNone PoolDropReason = iota

	// PoolDropReasonClosedPool means a valid returned buffer arrived after Pool
	// shutdown had started and PoolConfig selected drop-return behavior.
	PoolDropReasonClosedPool

	// PoolDropReasonReturnedBuffersDisabled means the effective policy disabled
	// returned-buffer retention before class routing was attempted.
	PoolDropReasonReturnedBuffersDisabled

	// PoolDropReasonOversized means the returned buffer capacity exceeded the
	// effective maximum retained-buffer capacity.
	PoolDropReasonOversized

	// PoolDropReasonUnsupportedClass means the returned capacity could not be
	// mapped to any configured size class.
	PoolDropReasonUnsupportedClass

	// PoolDropReasonInvalidPolicy means the runtime policy view contained an
	// admission value Pool could not execute.
	PoolDropReasonInvalidPolicy
)

const poolDropReasonCount = int(PoolDropReasonInvalidPolicy) + 1

// String returns a stable diagnostic label for reason.
func (r PoolDropReason) String() string {
	switch r {
	case PoolDropReasonNone:
		return "none"
	case PoolDropReasonClosedPool:
		return "closed_pool"
	case PoolDropReasonReturnedBuffersDisabled:
		return "returned_buffers_disabled"
	case PoolDropReasonOversized:
		return "oversized"
	case PoolDropReasonUnsupportedClass:
		return "unsupported_class"
	case PoolDropReasonInvalidPolicy:
		return "invalid_policy"
	default:
		return "unknown"
	}
}

// PoolDropReasonCounters contains owner-side drop counts by reason.
//
// These counters intentionally exclude class/shard drops. Aggregate Pool drops
// combine owner-side drops with lower-layer drops, but reason counters here
// describe only decisions made before or outside classState/shard admission.
type PoolDropReasonCounters struct {
	// ClosedPool counts valid returns dropped because the Pool was closing or
	// closed and close policy selected drop-return behavior.
	ClosedPool uint64

	// ReturnedBuffersDisabled counts valid returns dropped because returned
	// retention is disabled by policy.
	ReturnedBuffersDisabled uint64

	// Oversized counts returns whose capacity exceeded policy limits.
	Oversized uint64

	// UnsupportedClass counts returns whose capacity could not be classified.
	UnsupportedClass uint64

	// InvalidPolicy counts runtime admission values Pool could not execute.
	InvalidPolicy uint64
}

// IsZero reports whether no owner-side drop reason has been observed.
func (c PoolDropReasonCounters) IsZero() bool {
	return c.ClosedPool == 0 &&
		c.ReturnedBuffersDisabled == 0 &&
		c.Oversized == 0 &&
		c.UnsupportedClass == 0 &&
		c.InvalidPolicy == 0
}

// Total returns the sum of all owner-side drop reason counts.
func (c PoolDropReasonCounters) Total() uint64 {
	return c.ClosedPool +
		c.ReturnedBuffersDisabled +
		c.Oversized +
		c.UnsupportedClass +
		c.InvalidPolicy
}

// poolOwnerCounters records pool-owner return-path decisions.
//
// classState and shard counters record operations that reached class/shard
// storage. These counters cover valid public Put attempts and owner-side drops
// that happen before or outside classState so aggregate snapshots do not miss
// disabled-return, oversized, unsupported, closed-drop, or invalid-policy
// outcomes.
//
// Puts means "valid public Put calls". It intentionally includes valid buffers
// later rejected by lifecycle in close-reject mode. PutOutcomes is Retains +
// Drops at the aggregate Pool level, so Puts may be greater than PutOutcomes.
type poolOwnerCounters struct {
	puts          atomic.Uint64
	returnedBytes atomic.Uint64

	drops        atomic.Uint64
	droppedBytes atomic.Uint64

	dropReasons [poolDropReasonCount]atomic.Uint64
}

type poolOwnerCountersSnapshot struct {
	// Puts and ReturnedBytes describe valid public Put attempts.
	//
	// They intentionally include attempts that are later dropped by owner-side
	// admission, class admission, shard credit, bucket storage, or rejected by
	// close policy.
	Puts          uint64
	ReturnedBytes uint64

	// Drops and DroppedBytes describe owner-side drops only.
	//
	// Pool aggregate snapshots add these to class/shard drops. They are kept
	// separate here to avoid double counting lower-layer outcomes.
	Drops        uint64
	DroppedBytes uint64
	DropReasons  PoolDropReasonCounters
}

// recordPut records one valid public Put attempt.
//
// The caller must pass buffer capacity, not len(buffer). Retention accounting is
// capacity-oriented because reusable value is the backing array size.
func (c *poolOwnerCounters) recordPut(capacity uint64) {
	c.puts.Add(1)
	c.returnedBytes.Add(capacity)
}

// recordDrop records one owner-side no-retain outcome.
//
// This method is not used for class/shard drops because those lower layers
// already maintain their own drop counters. Invalid or none-like reasons are
// folded into InvalidPolicy to keep the reason array bounded and fail closed in
// diagnostics.
func (c *poolOwnerCounters) recordDrop(reason PoolDropReason, capacity uint64) {
	c.drops.Add(1)
	c.droppedBytes.Add(capacity)

	index := int(reason)
	if index <= int(PoolDropReasonNone) || index >= len(c.dropReasons) {
		index = int(PoolDropReasonInvalidPolicy)
	}

	c.dropReasons[index].Add(1)
}

// snapshot returns an atomic sample of owner-side counters.
//
// The sample is race-safe but not transactional across fields. This matches the
// rest of Pool observability: public snapshots are diagnostic observations, not
// global stop-the-world measurements.
func (c *poolOwnerCounters) snapshot() poolOwnerCountersSnapshot {
	return poolOwnerCountersSnapshot{
		Puts:          c.puts.Load(),
		ReturnedBytes: c.returnedBytes.Load(),
		Drops:         c.drops.Load(),
		DroppedBytes:  c.droppedBytes.Load(),
		DropReasons: PoolDropReasonCounters{
			ClosedPool:              c.dropReasons[PoolDropReasonClosedPool].Load(),
			ReturnedBuffersDisabled: c.dropReasons[PoolDropReasonReturnedBuffersDisabled].Load(),
			Oversized:               c.dropReasons[PoolDropReasonOversized].Load(),
			UnsupportedClass:        c.dropReasons[PoolDropReasonUnsupportedClass].Load(),
			InvalidPolicy:           c.dropReasons[PoolDropReasonInvalidPolicy].Load(),
		},
	}
}
