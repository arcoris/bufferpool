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

// TestPoolPutRetainsBuffer verifies the ordinary successful return path.
func TestPoolPutRetainsBuffer(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	if err := pool.Put(make([]byte, 123, KiB.Int())); err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}

	usage := poolTestRetainedUsage(pool)
	if usage.buffers != 1 || usage.bytes != KiB.Bytes() {
		t.Fatalf("retained usage = %+v, want 1 buffer / 1 KiB", usage)
	}
}

// TestPoolPutRejectsInvalidBuffers verifies strict public input validation.
func TestPoolPutRejectsInvalidBuffers(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	t.Run("nil buffer", func(t *testing.T) {
		t.Parallel()

		err := pool.Put(nil)
		if err == nil {
			t.Fatal("Put(nil) returned nil error")
		}

		if !errors.Is(err, ErrNilBuffer) {
			t.Fatalf("Put(nil) error does not match ErrNilBuffer: %v", err)
		}
	})

	t.Run("zero capacity", func(t *testing.T) {
		t.Parallel()

		err := pool.Put(make([]byte, 0))
		if err == nil {
			t.Fatal("Put(zero capacity) returned nil error")
		}

		if !errors.Is(err, ErrZeroCapacity) {
			t.Fatalf("Put(zero capacity) error does not match ErrZeroCapacity: %v", err)
		}
	})
}

// TestPoolPutReturnedBufferPolicyDrop verifies policy-level no-retain mode.
func TestPoolPutReturnedBufferPolicyDrop(t *testing.T) {
	t.Parallel()

	policy := poolTestSingleShardPolicy()
	policy.Admission.ReturnedBuffers = ReturnedBufferPolicyDrop

	pool := MustNew(PoolConfig{Policy: policy})
	defer closePoolForTest(t, pool)

	if err := pool.Put(make([]byte, 0, KiB.Int())); err != nil {
		t.Fatalf("Put() returned error in drop mode: %v", err)
	}

	usage := poolTestRetainedUsage(pool)
	if usage.buffers != 0 || usage.bytes != 0 {
		t.Fatalf("retained usage in drop mode = %+v, want zero", usage)
	}

	snapshot := pool.Snapshot()
	if snapshot.Counters.Puts != 1 {
		t.Fatalf("snapshot puts = %d, want 1", snapshot.Counters.Puts)
	}
	if snapshot.Counters.ReturnedBytes != KiB.Bytes() {
		t.Fatalf("snapshot returned bytes = %d, want %d", snapshot.Counters.ReturnedBytes, KiB.Bytes())
	}
	if snapshot.Counters.Drops != 1 {
		t.Fatalf("snapshot drops = %d, want 1", snapshot.Counters.Drops)
	}
	if snapshot.Counters.DroppedBytes != KiB.Bytes() {
		t.Fatalf("snapshot dropped bytes = %d, want %d", snapshot.Counters.DroppedBytes, KiB.Bytes())
	}
	if snapshot.Counters.DropReasons.ReturnedBuffersDisabled != 1 {
		t.Fatalf("returned-buffer-disabled drops = %d, want 1", snapshot.Counters.DropReasons.ReturnedBuffersDisabled)
	}
}

// TestPoolPutAdmissionErrors verifies public error classes for return-path
// admission failures configured as AdmissionActionError.
func TestPoolPutAdmissionErrors(t *testing.T) {
	t.Parallel()

	t.Run("oversized return", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSmallSingleShardPolicy()
		policy.Admission.OversizedReturn = AdmissionActionError

		pool := MustNew(PoolConfig{Policy: policy})
		defer closePoolForTest(t, pool)

		err := pool.Put(make([]byte, 0, 2*KiB.Int()))
		if err == nil {
			t.Fatal("Put(oversized) returned nil error")
		}

		if !errors.Is(err, ErrBufferTooLarge) {
			t.Fatalf("Put(oversized) error does not match ErrBufferTooLarge: %v", err)
		}
	})

	t.Run("unsupported class", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSmallSingleShardPolicy()
		policy.Admission.UnsupportedClass = AdmissionActionError

		pool := MustNew(PoolConfig{Policy: policy})
		defer closePoolForTest(t, pool)

		err := pool.Put(make([]byte, 0, 1))
		if err == nil {
			t.Fatal("Put(unsupported class) returned nil error")
		}

		if !errors.Is(err, ErrUnsupportedClass) {
			t.Fatalf("Put(unsupported class) error does not match ErrUnsupportedClass: %v", err)
		}
	})

	t.Run("credit exhausted", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSmallSingleShardPolicy()
		policy.Admission.CreditExhausted = AdmissionActionError
		policy.Shards.BucketSlotsPerShard = 2
		policy.Retention.MaxClassRetainedBuffers = 1
		policy.Retention.MaxShardRetainedBuffers = 1

		pool := MustNew(PoolConfig{Policy: policy})
		defer closePoolForTest(t, pool)

		if err := pool.Put(make([]byte, 0, 512)); err != nil {
			t.Fatalf("first Put() returned error: %v", err)
		}

		err := pool.Put(make([]byte, 0, 512))
		if err == nil {
			t.Fatal("second Put() returned nil error despite exhausted credit")
		}

		if !errors.Is(err, ErrRetentionCreditExhausted) {
			t.Fatalf("second Put() error does not match ErrRetentionCreditExhausted: %v", err)
		}
	})

	t.Run("bucket full with validation disabled", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSmallSingleShardPolicy()
		policy.Admission.BucketFull = AdmissionActionError
		policy.Shards.BucketSlotsPerShard = 1
		policy.Retention.MaxClassRetainedBuffers = 2
		policy.Retention.MaxShardRetainedBuffers = 2

		pool := MustNew(PoolConfig{
			Policy:           policy,
			PolicyValidation: PoolPolicyValidationModeDisabled,
		})
		defer closePoolForTest(t, pool)

		if err := pool.Put(make([]byte, 0, 512)); err != nil {
			t.Fatalf("first Put() returned error: %v", err)
		}

		err := pool.Put(make([]byte, 0, 512))
		if err == nil {
			t.Fatal("second Put() returned nil error despite full bucket")
		}

		if !errors.Is(err, ErrRetentionStorageFull) {
			t.Fatalf("second Put() error does not match ErrRetentionStorageFull: %v", err)
		}
	})
}

// TestPoolPutZeroingPolicies verifies retained and dropped buffer hygiene.
func TestPoolPutZeroingPolicies(t *testing.T) {
	t.Parallel()

	t.Run("zero retained buffers", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSingleShardPolicy()
		policy.Admission.ZeroRetainedBuffers = true

		pool := MustNew(PoolConfig{Policy: policy})
		defer closePoolForTest(t, pool)

		buffer := make([]byte, 512)
		buffer[0] = 1
		buffer[511] = 3

		if err := pool.Put(buffer[:1]); err != nil {
			t.Fatalf("Put() returned error: %v", err)
		}

		reused, err := pool.Get(300)
		if err != nil {
			t.Fatalf("Get() returned error: %v", err)
		}

		if reused[0] != 0 {
			t.Fatalf("reused[0] = %d, want zeroed retained buffer", reused[0])
		}
		full := reused[:cap(reused)]
		if full[511] != 0 {
			t.Fatalf("reused full-capacity byte = %d, want zeroed retained buffer", full[511])
		}
	})

	t.Run("zero dropped buffers", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSmallSingleShardPolicy()
		policy.Admission.ZeroDroppedBuffers = true

		pool := MustNew(PoolConfig{Policy: policy})
		defer closePoolForTest(t, pool)

		buffer := []byte{1, 2, 3}
		err := pool.Put(buffer[:1])
		if err != nil {
			t.Fatalf("Put() returned error: %v", err)
		}

		for index, value := range buffer {
			if value != 0 {
				t.Fatalf("buffer[%d] = %d, want zeroed dropped buffer", index, value)
			}
		}
	})

	t.Run("zero retained does not clear dropped buffer", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSmallSingleShardPolicy()
		policy.Admission.ZeroRetainedBuffers = true
		policy.Admission.ZeroDroppedBuffers = false

		pool := MustNew(PoolConfig{Policy: policy})
		defer closePoolForTest(t, pool)

		buffer := []byte{1, 2, 3}
		if err := pool.Put(buffer[:1]); err != nil {
			t.Fatalf("Put() returned error: %v", err)
		}

		if buffer[0] != 1 || buffer[1] != 2 || buffer[2] != 3 {
			t.Fatalf("dropped buffer was cleared by retained-zeroing policy: %#v", buffer)
		}
	})

	t.Run("zero dropped does not require retained zeroing", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSmallSingleShardPolicy()
		policy.Admission.ZeroRetainedBuffers = false
		policy.Admission.ZeroDroppedBuffers = true

		pool := MustNew(PoolConfig{Policy: policy})
		defer closePoolForTest(t, pool)

		buffer := []byte{1, 2, 3}
		if err := pool.Put(buffer[:1]); err != nil {
			t.Fatalf("Put() returned error: %v", err)
		}

		for index, value := range buffer {
			if value != 0 {
				t.Fatalf("buffer[%d] = %d, want zeroed dropped buffer", index, value)
			}
		}
	})
}

// TestPoolPutOwnerSideDropAccounting verifies drops decided before class/shard
// storage are visible in aggregate snapshots without being mixed into class
// counters.
func TestPoolPutOwnerSideDropAccounting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(*Policy)
		buffer     []byte
		wantReason func(PoolDropReasonCounters) uint64
		wantBytes  uint64
	}{
		{
			name: "returned buffers disabled",
			mutate: func(policy *Policy) {
				policy.Admission.ReturnedBuffers = ReturnedBufferPolicyDrop
			},
			buffer:     make([]byte, 0, 512),
			wantReason: func(c PoolDropReasonCounters) uint64 { return c.ReturnedBuffersDisabled },
			wantBytes:  512,
		},
		{
			name: "oversized return",
			mutate: func(policy *Policy) {
				policy.Admission.OversizedReturn = AdmissionActionDrop
			},
			buffer:     make([]byte, 0, 2*KiB.Int()),
			wantReason: func(c PoolDropReasonCounters) uint64 { return c.Oversized },
			wantBytes:  uint64(2 * KiB),
		},
		{
			name: "unsupported class",
			mutate: func(policy *Policy) {
				policy.Admission.UnsupportedClass = AdmissionActionDrop
			},
			buffer:     make([]byte, 0, 1),
			wantReason: func(c PoolDropReasonCounters) uint64 { return c.UnsupportedClass },
			wantBytes:  1,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy := poolTestSmallSingleShardPolicy()
			tt.mutate(&policy)

			pool := MustNew(PoolConfig{Policy: policy})
			defer closePoolForTest(t, pool)

			if err := pool.Put(tt.buffer); err != nil {
				t.Fatalf("Put() returned error: %v", err)
			}

			snapshot := pool.Snapshot()
			if snapshot.Counters.Drops != 1 {
				t.Fatalf("snapshot drops = %d, want 1", snapshot.Counters.Drops)
			}
			if snapshot.Counters.DroppedBytes != tt.wantBytes {
				t.Fatalf("snapshot dropped bytes = %d, want %d", snapshot.Counters.DroppedBytes, tt.wantBytes)
			}
			if got := tt.wantReason(snapshot.Counters.DropReasons); got != 1 {
				t.Fatalf("drop reason count = %d, want 1", got)
			}
			if snapshot.Counters.DropReasons.Total() != 1 {
				t.Fatalf("drop reason total = %d, want 1", snapshot.Counters.DropReasons.Total())
			}

			for _, class := range snapshot.Classes {
				if class.Counters.Puts != 0 || class.Counters.Drops != 0 {
					t.Fatalf("owner-side drop reached class counters: %#v", class.Counters)
				}
			}
		})
	}
}

// TestPoolPutClassShardDropsAreNotOwnerSideDropReasons verifies class/shard
// retain failures are counted once by class/shard counters.
func TestPoolPutClassShardDropsAreNotOwnerSideDropReasons(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	policy.Shards.BucketSlotsPerShard = 2
	policy.Retention.MaxClassRetainedBuffers = 1
	policy.Retention.MaxShardRetainedBuffers = 1

	pool := MustNew(PoolConfig{Policy: policy})
	defer closePoolForTest(t, pool)

	if err := pool.Put(make([]byte, 0, 512)); err != nil {
		t.Fatalf("first Put() returned error: %v", err)
	}
	if err := pool.Put(make([]byte, 0, 512)); err != nil {
		t.Fatalf("credit drop Put() returned error: %v", err)
	}

	snapshot := pool.Snapshot()
	if snapshot.Counters.Drops != 1 {
		t.Fatalf("aggregate drops = %d, want one class/shard drop", snapshot.Counters.Drops)
	}
	if snapshot.Counters.DropReasons.Total() != 0 {
		t.Fatalf("owner-side drop reasons = %d, want zero for class/shard drop", snapshot.Counters.DropReasons.Total())
	}
}
