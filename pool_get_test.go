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
	"testing"
)

// TestPoolGetAllocatesClassSizedBufferOnMiss verifies miss allocation behavior.
//
// Pool owns allocation on miss. classState records allocation facts but does not
// allocate buffers itself.
func TestPoolGetAllocatesClassSizedBufferOnMiss(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	buffer, err := pool.Get(700)
	if err != nil {
		t.Fatalf("Get(700) returned error: %v", err)
	}

	if len(buffer) != 700 {
		t.Fatalf("len(Get(700)) = %d, want 700", len(buffer))
	}

	if cap(buffer) != KiB.Int() {
		t.Fatalf("cap(Get(700)) = %d, want %d", cap(buffer), KiB.Int())
	}

	class, ok := pool.table.classForRequest(SizeFromInt(700))
	if !ok {
		t.Fatal("test setup: 700 B request is unsupported")
	}

	state := pool.mustClassStateFor(class).state()
	if state.Counters.Allocations != 1 {
		t.Fatalf("allocations = %d, want 1", state.Counters.Allocations)
	}

	if state.Counters.AllocatedBytes != KiB.Bytes() {
		t.Fatalf("allocated bytes = %d, want %d", state.Counters.AllocatedBytes, KiB.Bytes())
	}
}

// TestPoolGetReusesReturnedBuffer verifies retained reuse on the acquisition
// path.
func TestPoolGetReusesReturnedBuffer(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	returned := make([]byte, 123, KiB.Int())
	returned[0] = 42

	if err := pool.Put(returned); err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}

	reused, err := pool.Get(900)
	if err != nil {
		t.Fatalf("Get(900) returned error: %v", err)
	}

	if len(reused) != 900 {
		t.Fatalf("len(reused) = %d, want 900", len(reused))
	}

	if cap(reused) != KiB.Int() {
		t.Fatalf("cap(reused) = %d, want %d", cap(reused), KiB.Int())
	}

	if &reused[:1][0] != &returned[:1][0] {
		t.Fatal("Get did not reuse the previously returned buffer")
	}

	if usage := poolTestRetainedUsage(pool); usage.buffers != 0 || usage.bytes != 0 {
		t.Fatalf("retained usage after reuse = %+v, want zero", usage)
	}
}

// TestPoolGetAcquisitionFallbackUsesSecondaryShard verifies Pool.Get can reuse
// retained storage through bounded fallback after the primary selected shard
// misses.
func TestPoolGetAcquisitionFallbackUsesSecondaryShard(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	policy.Shards.ShardsPerClass = 2
	policy.Shards.AcquisitionFallbackShards = 1

	pool := MustNew(PoolConfig{Policy: policy})
	defer closePoolForTest(t, pool)

	class, ok := pool.table.classForRequest(SizeFromInt(300))
	if !ok {
		t.Fatal("test setup: 300 B request is unsupported")
	}
	state := pool.mustClassStateFor(class)
	retain := state.tryRetain(1, make([]byte, 0, 512))
	if !retain.Retained() {
		t.Fatalf("test setup retain failed: %s", retain.CreditDecision())
	}

	buffer, err := pool.Get(300)
	if err != nil {
		t.Fatalf("Get(300) returned error: %v", err)
	}
	if len(buffer) != 300 || cap(buffer) != 512 {
		t.Fatalf("Get(300) len/cap = %d/%d, want 300/512", len(buffer), cap(buffer))
	}

	snapshot := pool.Snapshot()
	if snapshot.Counters.Hits != 1 {
		t.Fatalf("pool hits = %d, want 1", snapshot.Counters.Hits)
	}
	if snapshot.Counters.Misses != 0 {
		t.Fatalf("pool misses = %d, want zero for fallback-served get", snapshot.Counters.Misses)
	}
	if snapshot.Classes[0].Shards[0].Counters.Misses != 1 {
		t.Fatalf("primary shard misses = %d, want 1", snapshot.Classes[0].Shards[0].Counters.Misses)
	}
	if snapshot.Classes[0].Shards[1].Counters.Hits != 1 {
		t.Fatalf("fallback shard hits = %d, want 1", snapshot.Classes[0].Shards[1].Counters.Hits)
	}
	if poolTestRetainedUsage(pool).buffers != 0 {
		t.Fatal("retained buffer was not consumed by fallback hit")
	}
}

// TestPoolGetAcquisitionFallbackImprovesControlledHitRatio compares identical
// seeded storage with and without one fallback probe.
func TestPoolGetAcquisitionFallbackImprovesControlledHitRatio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fallback int
		wantHit  bool
	}{
		{name: "fallback disabled", fallback: 0, wantHit: false},
		{name: "fallback one", fallback: 1, wantHit: true},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy := poolTestSmallSingleShardPolicy()
			policy.Shards.ShardsPerClass = 2
			policy.Shards.AcquisitionFallbackShards = tt.fallback

			pool := MustNew(PoolConfig{Policy: policy})
			defer closePoolForTest(t, pool)

			class, ok := pool.table.classForRequest(SizeFromInt(300))
			if !ok {
				t.Fatal("test setup: 300 B request is unsupported")
			}
			state := pool.mustClassStateFor(class)
			if retain := state.tryRetain(1, make([]byte, 0, 512)); !retain.Retained() {
				t.Fatalf("test setup retain failed: %s", retain.CreditDecision())
			}

			buffer, err := pool.Get(300)
			if err != nil {
				t.Fatalf("Get(300) returned error: %v", err)
			}
			if len(buffer) != 300 || cap(buffer) != 512 {
				t.Fatalf("Get(300) len/cap = %d/%d, want 300/512", len(buffer), cap(buffer))
			}

			gotHit := pool.Snapshot().Counters.Hits == 1
			if gotHit != tt.wantHit {
				t.Fatalf("hit = %v, want %v", gotHit, tt.wantHit)
			}
		})
	}
}

// TestPoolGetZeroSizePolicy verifies policy-owned zero-size behavior.
func TestPoolGetZeroSizePolicy(t *testing.T) {
	t.Parallel()

	t.Run("empty buffer", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSingleShardPolicy()
		policy.Admission.ZeroSizeRequests = ZeroSizeRequestEmptyBuffer

		pool := MustNew(PoolConfig{Policy: policy})
		defer closePoolForTest(t, pool)

		buffer, err := pool.Get(0)
		if err != nil {
			t.Fatalf("Get(0) returned error: %v", err)
		}

		if len(buffer) != 0 {
			t.Fatalf("len(Get(0)) = %d, want 0", len(buffer))
		}
		if buffer == nil {
			t.Fatal("Get(0) returned nil buffer for empty-buffer policy")
		}

		if usage := poolTestRetainedUsage(pool); usage.buffers != 0 || usage.bytes != 0 {
			t.Fatalf("retained usage after empty zero-size request = %+v, want zero", usage)
		}
		if active := pool.activeOperations.Load(); active != 0 {
			t.Fatalf("active operations after empty zero-size request = %d, want 0", active)
		}

		snapshot := pool.Snapshot()
		if !snapshot.Counters.IsZero() {
			t.Fatalf("empty zero-size request touched counters: %#v", snapshot.Counters)
		}
	})

	t.Run("smallest class", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSingleShardPolicy()
		policy.Admission.ZeroSizeRequests = ZeroSizeRequestSmallestClass

		pool := MustNew(PoolConfig{Policy: policy})
		defer closePoolForTest(t, pool)

		buffer, err := pool.Get(0)
		if err != nil {
			t.Fatalf("Get(0) returned error: %v", err)
		}

		if len(buffer) != 0 {
			t.Fatalf("len(Get(0)) = %d, want 0", len(buffer))
		}

		if cap(buffer) != 256 {
			t.Fatalf("cap(Get(0)) = %d, want smallest class 256", cap(buffer))
		}
	})

	t.Run("reject", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSingleShardPolicy()
		policy.Admission.ZeroSizeRequests = ZeroSizeRequestReject

		pool := MustNew(PoolConfig{Policy: policy})
		defer closePoolForTest(t, pool)

		buffer, err := pool.Get(0)
		if err == nil {
			t.Fatal("Get(0) returned nil error")
		}

		if buffer != nil {
			t.Fatalf("Get(0) returned buffer %#v", buffer)
		}

		if !errors.Is(err, ErrInvalidSize) {
			t.Fatalf("Get(0) error does not match ErrInvalidSize: %v", err)
		}
	})
}

// TestPoolGetZeroSizeEmptyPolicyRespectsClose verifies valid zero-size
// acquisition still enters the lifecycle gate.
func TestPoolGetZeroSizeEmptyPolicyRespectsClose(t *testing.T) {
	t.Parallel()

	policy := poolTestSingleShardPolicy()
	policy.Admission.ZeroSizeRequests = ZeroSizeRequestEmptyBuffer

	pool := MustNew(PoolConfig{Policy: policy})
	closePoolForTest(t, pool)

	buffer, err := pool.Get(0)
	if err == nil {
		t.Fatal("Get(0) after close returned nil error")
	}
	if buffer != nil {
		t.Fatalf("Get(0) after close returned buffer %#v", buffer)
	}
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Get(0) after close error does not match ErrClosed: %v", err)
	}
}

// TestPoolGetZeroSizeRejectPrecedesLifecycle documents error precedence for
// invalid zero-size requests.
func TestPoolGetZeroSizeRejectPrecedesLifecycle(t *testing.T) {
	t.Parallel()

	policy := poolTestSingleShardPolicy()
	policy.Admission.ZeroSizeRequests = ZeroSizeRequestReject

	pool := MustNew(PoolConfig{Policy: policy})
	closePoolForTest(t, pool)

	buffer, err := pool.Get(0)
	if err == nil {
		t.Fatal("Get(0) after close with reject policy returned nil error")
	}
	if buffer != nil {
		t.Fatalf("Get(0) returned buffer %#v", buffer)
	}
	if !errors.Is(err, ErrInvalidSize) {
		t.Fatalf("Get(0) error = %v, want ErrInvalidSize", err)
	}
}

// TestPoolGetRejectsInvalidRequests verifies request-side public error classes.
func TestPoolGetRejectsInvalidRequests(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	t.Run("negative size", func(t *testing.T) {
		t.Parallel()

		buffer, err := pool.Get(-1)
		if err == nil {
			t.Fatal("Get(-1) returned nil error")
		}

		if buffer != nil {
			t.Fatalf("Get(-1) returned buffer %#v", buffer)
		}

		if !errors.Is(err, ErrInvalidSize) {
			t.Fatalf("Get(-1) error does not match ErrInvalidSize: %v", err)
		}
		if active := pool.activeOperations.Load(); active != 0 {
			t.Fatalf("active operations after rejected Get(-1) = %d, want 0", active)
		}
	})

	t.Run("request too large", func(t *testing.T) {
		t.Parallel()

		buffer, err := pool.Get((MiB + Byte).Int())
		if err == nil {
			t.Fatal("Get(max+1) returned nil error")
		}

		if buffer != nil {
			t.Fatalf("Get(max+1) returned buffer %#v", buffer)
		}

		if !errors.Is(err, ErrRequestTooLarge) {
			t.Fatalf("Get(max+1) error does not match ErrRequestTooLarge: %v", err)
		}
		if active := pool.activeOperations.Load(); active != 0 {
			t.Fatalf("active operations after rejected oversized Get = %d, want 0", active)
		}
	})
}

// TestPoolSizeIntRejectsOverflow verifies allocation length conversion is
// checked before make is called.
func TestPoolSizeIntRejectsOverflow(t *testing.T) {
	t.Parallel()

	if _, ok := poolSizeInt(Size(^uint64(0))); ok {
		t.Fatal("poolSizeInt(max uint64) reported success")
	}
}

// TestPoolGetUnsupportedClass verifies lookup failure when policy validation is
// explicitly disabled to allow a request limit larger than the class table.
func TestPoolGetUnsupportedClass(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	policy.Retention.MaxRequestSize = 2 * MiB

	pool := MustNew(PoolConfig{
		Policy:           policy,
		PolicyValidation: PoolPolicyValidationModeDisabled,
	})
	defer closePoolForTest(t, pool)

	buffer, err := pool.Get((MiB + Byte).Int())
	if err == nil {
		t.Fatal("Get() returned nil error for unsupported class")
	}

	if buffer != nil {
		t.Fatalf("Get() returned buffer %#v for unsupported class", buffer)
	}

	if !errors.Is(err, ErrUnsupportedClass) {
		t.Fatalf("Get() error does not match ErrUnsupportedClass: %v", err)
	}
}
