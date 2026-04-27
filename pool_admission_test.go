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

// TestPoolHandleAdmissionAction verifies action-to-error mapping for
// non-buffer-specific admission conditions.
func TestPoolHandleAdmissionAction(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	tests := []struct {
		name    string
		action  AdmissionAction
		wantErr error
	}{
		{
			name:    "drop",
			action:  AdmissionActionDrop,
			wantErr: nil,
		},
		{
			name:    "ignore",
			action:  AdmissionActionIgnore,
			wantErr: nil,
		},
		{
			name:    "error",
			action:  AdmissionActionError,
			wantErr: ErrInvalidBuffer,
		},
		{
			name:    "unset",
			action:  AdmissionActionUnset,
			wantErr: ErrInvalidPolicy,
		},
		{
			name:    "unknown",
			action:  AdmissionAction(255),
			wantErr: ErrInvalidPolicy,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := pool.handleAdmissionAction(tt.action, ErrInvalidBuffer, "test admission")
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("handleAdmissionAction() returned error: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatal("handleAdmissionAction() returned nil error")
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("handleAdmissionAction() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestPoolHandleBufferAdmissionActionZeroesDroppedBuffers verifies that
// ZeroDroppedBuffers is applied to every no-retain outcome.
func TestPoolHandleBufferAdmissionActionZeroesDroppedBuffers(t *testing.T) {
	t.Parallel()

	policy := poolTestSingleShardPolicy()
	policy.Admission.ZeroDroppedBuffers = true

	pool := MustNew(PoolConfig{Policy: policy})
	defer closePoolForTest(t, pool)

	tests := []struct {
		name   string
		action AdmissionAction
	}{
		{name: "drop", action: AdmissionActionDrop},
		{name: "ignore", action: AdmissionActionIgnore},
		{name: "error", action: AdmissionActionError},
		{name: "invalid", action: AdmissionAction(255)},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			buffer := []byte{1, 2, 3}
			buffer = buffer[:1]

			_ = pool.handleBufferAdmissionAction(
				pool.Policy(),
				tt.action,
				ErrInvalidBuffer,
				"test admission",
				buffer,
				PoolDropReasonOversized,
			)

			full := buffer[:cap(buffer)]
			for index, value := range full {
				if value != 0 {
					t.Fatalf("buffer[%d] = %d, want zero after admission no-retain outcome", index, value)
				}
			}
		})
	}
}

// TestPoolZeroRetainedBuffer verifies retained-buffer hygiene.
func TestPoolZeroRetainedBuffer(t *testing.T) {
	t.Parallel()

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()

		pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
		defer closePoolForTest(t, pool)

		buffer := []byte{1, 2, 3}
		pool.zeroRetainedBuffer(buffer[:1], pool.Policy())

		if buffer[0] != 1 || buffer[1] != 2 || buffer[2] != 3 {
			t.Fatalf("zeroRetainedBuffer changed buffer with zeroing disabled: %#v", buffer)
		}
	})

	t.Run("enabled clears full capacity", func(t *testing.T) {
		t.Parallel()

		policy := poolTestSingleShardPolicy()
		policy.Admission.ZeroRetainedBuffers = true

		pool := MustNew(PoolConfig{Policy: policy})
		defer closePoolForTest(t, pool)

		buffer := []byte{1, 2, 3}
		pool.zeroRetainedBuffer(buffer[:1], pool.Policy())

		for index, value := range buffer {
			if value != 0 {
				t.Fatalf("buffer[%d] = %d, want zero", index, value)
			}
		}
	})
}
