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

import "testing"

// TestEvaluateClassRetain verifies class-local retain eligibility decisions.
func TestEvaluateClassRetain(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))

	tests := []struct {
		name string

		buffer []byte
		want   classRetainDecision
	}{
		{
			name:   "nil buffer",
			buffer: nil,
			want:   classRetainRejectNilBuffer,
		},
		{
			name:   "zero capacity",
			buffer: make([]byte, 0),
			want:   classRetainRejectZeroCapacity,
		},
		{
			name:   "below class size",
			buffer: make([]byte, 0, 512),
			want:   classRetainRejectCapacityBelowClassSize,
		},
		{
			name:   "equal class size",
			buffer: make([]byte, 0, 1024),
			want:   classRetainAccept,
		},
		{
			name:   "above class size",
			buffer: make([]byte, 0, 2048),
			want:   classRetainAccept,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := evaluateClassRetain(class, tt.buffer)
			if got != tt.want {
				t.Fatalf("evaluateClassRetain() = %s, want %s", got, tt.want)
			}

			if got.AllowsShardRetain() != (tt.want == classRetainAccept) {
				t.Fatalf("AllowsShardRetain() = %t, want %t", got.AllowsShardRetain(), tt.want == classRetainAccept)
			}
		})
	}
}

// TestClassRetainDecisionString verifies stable diagnostic decision names.
func TestClassRetainDecisionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		decision classRetainDecision

		wantString string
		wantAllows bool
	}{
		{
			name:       "accept",
			decision:   classRetainAccept,
			wantString: "accept",
			wantAllows: true,
		},
		{
			name:       "nil buffer",
			decision:   classRetainRejectNilBuffer,
			wantString: "reject_nil_buffer",
			wantAllows: false,
		},
		{
			name:       "zero capacity",
			decision:   classRetainRejectZeroCapacity,
			wantString: "reject_zero_capacity",
			wantAllows: false,
		},
		{
			name:       "below class size",
			decision:   classRetainRejectCapacityBelowClassSize,
			wantString: "reject_capacity_below_class_size",
			wantAllows: false,
		},
		{
			name:       "unknown",
			decision:   classRetainDecision(255),
			wantString: "unknown",
			wantAllows: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.decision.String(); got != tt.wantString {
				t.Fatalf("String() = %q, want %q", got, tt.wantString)
			}

			if got := tt.decision.AllowsShardRetain(); got != tt.wantAllows {
				t.Fatalf("AllowsShardRetain() = %t, want %t", got, tt.wantAllows)
			}
		})
	}
}
