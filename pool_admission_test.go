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

// TestPoolApplyReturnOutcome verifies action-to-error mapping for already
// decided no-retain outcomes.
func TestPoolApplyReturnOutcome(t *testing.T) {
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

			buffer := []byte{1, 2, 3}
			input := poolReturnInput{
				Buffer:   buffer[:1],
				Capacity: uint64(cap(buffer)),
			}
			outcome := poolReturnOutcomeDrop(tt.action, ErrInvalidBuffer, "test admission", PoolDropReasonNone, input.Capacity)

			err := pool.applyReturnOutcome(pool.Policy(), input, outcome)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("applyReturnOutcome() returned error: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatal("applyReturnOutcome() returned nil error")
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("applyReturnOutcome() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestPoolApplyReturnOutcomeZeroesDroppedBuffers verifies that
// ZeroDroppedBuffers is applied to every no-retain return outcome.
func TestPoolApplyReturnOutcomeZeroesDroppedBuffers(t *testing.T) {
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
			input := poolReturnInput{Buffer: buffer, Capacity: uint64(cap(buffer))}
			outcome := poolReturnOutcomeDrop(tt.action, ErrInvalidBuffer, "test admission", PoolDropReasonOversized, input.Capacity)

			_ = pool.applyReturnOutcome(pool.Policy(), input, outcome)

			full := buffer[:cap(buffer)]
			for index, value := range full {
				if value != 0 {
					t.Fatalf("buffer[%d] = %d, want zero after admission no-retain outcome", index, value)
				}
			}
		})
	}
}

// TestPoolAdmissionActionKnown verifies the narrow action set accepted by the
// return-outcome application stage.
func TestPoolAdmissionActionKnown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action AdmissionAction
		want   bool
	}{
		{action: AdmissionActionDrop, want: true},
		{action: AdmissionActionIgnore, want: true},
		{action: AdmissionActionError, want: true},
		{action: AdmissionActionUnset, want: false},
		{action: AdmissionAction(255), want: false},
	}

	for _, tt := range tests {
		if got := poolAdmissionActionKnown(tt.action); got != tt.want {
			t.Fatalf("poolAdmissionActionKnown(%s) = %v, want %v", tt.action, got, tt.want)
		}
	}
}

// TestZeroBufferCapacity verifies the shared full-capacity clearing primitive.
func TestZeroBufferCapacity(t *testing.T) {
	t.Parallel()

	buffer := []byte{1, 2, 3}
	zeroBufferCapacity(buffer[:1])

	for index, value := range buffer {
		if value != 0 {
			t.Fatalf("buffer[%d] = %d, want zero", index, value)
		}
	}
}
