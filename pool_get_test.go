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
