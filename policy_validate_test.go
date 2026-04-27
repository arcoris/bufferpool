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
	"time"

	"arcoris.dev/bufferpool/internal/multierr"
)

// TestValidatePolicyAcceptsCompleteStaticPolicy verifies the smallest complete
// policy shape needed by the current static bounded runtime.
func TestValidatePolicyAcceptsCompleteStaticPolicy(t *testing.T) {
	t.Parallel()

	if err := ValidatePolicy(newValidTestPolicy()); err != nil {
		t.Fatalf("ValidatePolicy(valid policy) = %v, want nil", err)
	}
}

// TestValidatePolicyRejectsZeroPolicy verifies top-level classification for an
// unset policy.
func TestValidatePolicyRejectsZeroPolicy(t *testing.T) {
	t.Parallel()

	err := Policy{}.Validate()
	if err == nil {
		t.Fatal("Policy{}.Validate() = nil, want error")
	}

	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("errors.Is(err, ErrInvalidPolicy) = false for %v", err)
	}
}

// TestValidatePolicyAggregatesMultipleFailures verifies that section errors are
// preserved as independent classified failures.
func TestValidatePolicyAggregatesMultipleFailures(t *testing.T) {
	t.Parallel()

	policy := newValidTestPolicy()
	policy.Retention.SoftRetainedBytes = 0
	policy.Classes.Sizes = nil

	err := policy.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want aggregate error")
	}

	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("errors.Is(err, ErrInvalidPolicy) = false for %v", err)
	}

	failures := multierr.Errors(err)
	if len(failures) < 2 {
		t.Fatalf("len(multierr.Errors(err)) = %d, want at least 2: %v", len(failures), err)
	}

	for index, failure := range failures {
		if !errors.Is(failure, ErrInvalidPolicy) {
			t.Fatalf("failure %d is not classified as ErrInvalidPolicy: %v", index, failure)
		}
	}
}

// TestPolicySectionsPreserveInvalidPolicyClassification verifies classification
// for each section-level validator.
func TestPolicySectionsPreserveInvalidPolicyClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "retention",
			err:  RetentionPolicy{}.Validate(),
		},
		{
			name: "classes",
			err:  ClassPolicy{}.Validate(),
		},
		{
			name: "shards",
			err:  ShardPolicy{}.Validate(),
		},
		{
			name: "admission",
			err:  AdmissionPolicy{}.Validate(),
		},
		{
			name: "pressure",
			err: PressurePolicy{
				Medium: PressureLevelPolicy{
					TrimScale: PolicyRatioOne,
				},
			}.Validate(),
		},
		{
			name: "trim",
			err: TrimPolicy{
				Interval: time.Second,
			}.Validate(),
		},
		{
			name: "ownership",
			err: OwnershipPolicy{
				TrackInUseBytes: true,
			}.Validate(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.err == nil {
				t.Fatal("Validate() = nil, want error")
			}

			if !errors.Is(tt.err, ErrInvalidPolicy) {
				t.Fatalf("errors.Is(err, ErrInvalidPolicy) = false for %v", tt.err)
			}
		})
	}
}

// newValidTestPolicy returns a compact complete policy that exercises the
// current static bounded runtime constraints without depending on default
// policy files.
func newValidTestPolicy() Policy {
	return Policy{
		Retention: RetentionPolicy{
			SoftRetainedBytes:         8 * KiB,
			HardRetainedBytes:         16 * KiB,
			MaxRetainedBuffers:        8,
			MaxRequestSize:            KiB,
			MaxRetainedBufferCapacity: 2 * KiB,
			MaxClassRetainedBytes:     4 * KiB,
			MaxClassRetainedBuffers:   4,
			MaxShardRetainedBytes:     2 * KiB,
			MaxShardRetainedBuffers:   2,
		},
		Classes: ClassPolicy{
			Sizes: []ClassSize{
				ClassSizeFromBytes(512),
				ClassSizeFromSize(KiB),
			},
		},
		Shards: ShardPolicy{
			Selection:                 ShardSelectionModeRoundRobin,
			ShardsPerClass:            2,
			BucketSlotsPerShard:       2,
			AcquisitionFallbackShards: 0,
			ReturnFallbackShards:      0,
		},
		Admission: AdmissionPolicy{
			ZeroSizeRequests: ZeroSizeRequestEmptyBuffer,
			ReturnedBuffers:  ReturnedBufferPolicyAdmit,
			OversizedReturn:  AdmissionActionDrop,
			UnsupportedClass: AdmissionActionDrop,
			ClassMismatch:    AdmissionActionDrop,
			CreditExhausted:  AdmissionActionDrop,
			BucketFull:       AdmissionActionDrop,
		},
	}
}
