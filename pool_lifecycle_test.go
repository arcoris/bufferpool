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
	"testing"
	"time"
)

// TestPoolCloseIsIdempotent verifies that Close can be called repeatedly and
// leaves the Pool in the terminal lifecycle state.
func TestPoolCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})

	if err := pool.Close(); err != nil {
		t.Fatalf("first Close() returned error: %v", err)
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("second Close() returned error: %v", err)
	}

	if pool.Lifecycle() != LifecycleClosed {
		t.Fatalf("Lifecycle() = %s, want %s", pool.Lifecycle(), LifecycleClosed)
	}

	if pool.IsActive() {
		t.Fatal("IsActive() = true after close")
	}

	if pool.IsClosing() {
		t.Fatal("IsClosing() = true after close completed")
	}

	if !pool.IsClosed() {
		t.Fatal("IsClosed() = false after close")
	}
}

// TestPoolCloseClearsRetainedStorage verifies production close cleanup.
//
// Close must publish Closing, wait for active operations, and then clear retained
// storage if PoolConfig requests close-time cleanup.
func TestPoolCloseClearsRetainedStorage(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})

	if err := pool.Put(make([]byte, 0, KiB.Int())); err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}

	if usage := poolTestRetainedUsage(pool); usage.buffers != 1 || usage.bytes != KiB.Bytes() {
		t.Fatalf("retained usage before close = %+v, want 1 buffer / 1 KiB", usage)
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	if usage := poolTestRetainedUsage(pool); usage.buffers != 0 || usage.bytes != 0 {
		t.Fatalf("retained usage after close = %+v, want zero", usage)
	}
}

// TestPoolCloseCanKeepRetainedStorage verifies the diagnostic close mode that
// preserves retained storage.
func TestPoolCloseCanKeepRetainedStorage(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{
		Policy:    poolTestSingleShardPolicy(),
		CloseMode: PoolCloseModeKeepRetained,
	})

	if err := pool.Put(make([]byte, 0, KiB.Int())); err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	if usage := poolTestRetainedUsage(pool); usage.buffers != 1 || usage.bytes != KiB.Bytes() {
		t.Fatalf("retained usage after keep-retained close = %+v, want 1 buffer / 1 KiB", usage)
	}
}

// TestPoolOperationsAfterClose verifies post-close operation policy.
func TestPoolOperationsAfterClose(t *testing.T) {
	t.Parallel()

	t.Run("default rejects get and put", func(t *testing.T) {
		t.Parallel()

		pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
		closePoolForTest(t, pool)

		buffer, err := pool.Get(128)
		if err == nil {
			t.Fatal("Get() after close returned nil error")
		}

		if buffer != nil {
			t.Fatalf("Get() after close returned buffer %#v", buffer)
		}

		if !errors.Is(err, ErrClosed) {
			t.Fatalf("Get() after close error does not match ErrClosed: %v", err)
		}

		err = pool.Put(make([]byte, 0, 256))
		if err == nil {
			t.Fatal("Put() after close returned nil error")
		}

		if !errors.Is(err, ErrClosed) {
			t.Fatalf("Put() after close error does not match ErrClosed: %v", err)
		}

		snapshot := pool.Snapshot()
		if snapshot.Counters.Puts != 1 {
			t.Fatalf("rejected late Put count = %d, want 1", snapshot.Counters.Puts)
		}
		if snapshot.Counters.ReturnedBytes != 256 {
			t.Fatalf("rejected late returned bytes = %d, want 256", snapshot.Counters.ReturnedBytes)
		}
		if snapshot.Counters.Drops != 0 {
			t.Fatalf("rejected late drops = %d, want 0", snapshot.Counters.Drops)
		}
	})

	t.Run("drop returns mode ignores put after close", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSingleShardPolicy()
		policy.Admission.ZeroDroppedBuffers = true

		pool := MustNew(PoolConfig{
			Policy:           policy,
			ClosedOperations: PoolClosedOperationModeDropReturns,
		})
		closePoolForTest(t, pool)

		buffer := make([]byte, 512)
		buffer[0] = 1
		buffer[511] = 2

		if err := pool.Put(buffer[:1]); err != nil {
			t.Fatalf("Put() after close in drop-returns mode returned error: %v", err)
		}

		for index, value := range buffer {
			if value != 0 {
				t.Fatalf("late returned buffer byte %d = %d, want zeroed drop", index, value)
			}
		}

		snapshot := pool.Snapshot()
		if snapshot.Counters.Puts != 1 {
			t.Fatalf("late drop puts = %d, want 1", snapshot.Counters.Puts)
		}
		if snapshot.Counters.ReturnedBytes != 512 {
			t.Fatalf("late drop returned bytes = %d, want 512", snapshot.Counters.ReturnedBytes)
		}
		if snapshot.Counters.Drops != 1 {
			t.Fatalf("owner-side drops after late return = %d, want 1", snapshot.Counters.Drops)
		}
		if snapshot.Counters.DropReasons.ClosedPool != 1 {
			t.Fatalf("closed-pool drop reasons = %d, want 1", snapshot.Counters.DropReasons.ClosedPool)
		}

		buffer, err := pool.Get(128)
		if err == nil {
			t.Fatal("Get() after close returned nil error in drop-returns mode")
		}

		if buffer != nil {
			t.Fatalf("Get() after close returned buffer %#v", buffer)
		}

		if !errors.Is(err, ErrClosed) {
			t.Fatalf("Get() after close error does not match ErrClosed: %v", err)
		}
	})

	t.Run("drop returns mode does not mutate retained storage", func(t *testing.T) {
		t.Parallel()

		pool := MustNew(PoolConfig{
			Policy:           poolTestSingleShardPolicy(),
			CloseMode:        PoolCloseModeKeepRetained,
			ClosedOperations: PoolClosedOperationModeDropReturns,
		})

		if err := pool.Put(make([]byte, 0, KiB.Int())); err != nil {
			t.Fatalf("initial Put() returned error: %v", err)
		}

		closePoolForTest(t, pool)

		if err := pool.Put(make([]byte, 0, KiB.Int())); err != nil {
			t.Fatalf("late Put() returned error: %v", err)
		}

		usage := poolTestRetainedUsage(pool)
		if usage.buffers != 1 || usage.bytes != KiB.Bytes() {
			t.Fatalf("retained usage after late drop = %+v, want original retained buffer", usage)
		}
	})
}

// TestPoolNilAndUninitializedReceivers verifies fail-fast receiver validation.
func TestPoolNilAndUninitializedReceivers(t *testing.T) {
	t.Parallel()

	t.Run("nil receiver", func(t *testing.T) {
		t.Parallel()

		var pool *Pool

		assertPoolPanic(t, errNilPool, func() {
			_ = pool.Name()
		})
	})

	t.Run("zero value receiver", func(t *testing.T) {
		t.Parallel()

		var pool Pool

		assertPoolPanic(t, errUninitializedPool, func() {
			_ = pool.Name()
		})
	})
}

// TestPoolCloseWaitsForActiveOperations verifies the operation-drain barrier.
//
// The test uses internal lifecycle gate methods because public Get/Put operations
// are intentionally short. This directly verifies the Close coordination
// invariant: Close must not mark the Pool closed until admitted operations exit.
func TestPoolCloseWaitsForActiveOperations(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer func() {
		_ = pool.Close()
	}()

	if err := pool.beginAcquireOperation(); err != nil {
		t.Fatalf("beginAcquireOperation() returned error: %v", err)
	}

	closeReturned := make(chan struct{})
	go func() {
		_ = pool.Close()
		close(closeReturned)
	}()

	select {
	case <-closeReturned:
		t.Fatal("Close() returned before admitted operation ended")
	case <-time.After(20 * time.Millisecond):
	}

	pool.endOperation()

	select {
	case <-closeReturned:
	case <-time.After(time.Second):
		t.Fatal("Close() did not return after admitted operation ended")
	}

	if !pool.IsClosed() {
		t.Fatal("Pool is not closed after Close returned")
	}
}

// TestPoolConcurrentGetPutAndClose is a race-oriented lifecycle smoke test.
func TestPoolConcurrentGetPutAndClose(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})

	const workers = 8
	const iterations = 256

	var wg sync.WaitGroup
	start := make(chan struct{})

	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()

			<-start

			for iteration := 0; iteration < iterations; iteration++ {
				buffer, err := pool.Get(512)
				if err != nil {
					if errors.Is(err, ErrClosed) {
						return
					}

					t.Errorf("Get() returned unexpected error: %v", err)
					return
				}

				if len(buffer) != 512 {
					t.Errorf("len(Get(512)) = %d, want 512", len(buffer))
					return
				}

				if err := pool.Put(buffer); err != nil {
					if errors.Is(err, ErrClosed) {
						return
					}

					t.Errorf("Put() returned unexpected error: %v", err)
					return
				}
			}
		}()
	}

	close(start)
	time.Sleep(5 * time.Millisecond)

	if err := pool.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	wg.Wait()

	if pool.Lifecycle() != LifecycleClosed {
		t.Fatalf("Lifecycle() = %s, want %s", pool.Lifecycle(), LifecycleClosed)
	}
}
