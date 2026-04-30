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
)

// TestPolicyUpdateDiffDetectsRetentionContraction verifies that the value diff
// distinguishes stricter future-retention limits from ordinary section changes.
func TestPolicyUpdateDiffDetectsRetentionContraction(t *testing.T) {
	previous := policyUpdateTestPolicy()
	next := previous
	next.Retention.SoftRetainedBytes = KiB

	diff := classifyPolicyUpdate(previous, next)
	if !diff.Diagnostics.RetentionChanged {
		t.Fatal("RetentionChanged = false, want true")
	}
	if !diff.Diagnostics.RetentionContracted {
		t.Fatal("RetentionContracted = false, want true")
	}
	if diff.Diagnostics.RetentionExpanded {
		t.Fatal("RetentionExpanded = true, want false")
	}
	if !policyUpdateContractsRetention(previous, next) {
		t.Fatal("policyUpdateContractsRetention() = false, want true")
	}
}

// TestPolicyUpdateRejectsClassShapeChange verifies that class-table changes are
// construction-only until an owner implements a safe rebuild or transition.
func TestPolicyUpdateRejectsClassShapeChange(t *testing.T) {
	previous := policyUpdateTestPolicy()
	next := previous
	next.Classes.Sizes = next.Classes.SizesCopy()
	next.Classes.Sizes[0] = ClassSizeFromBytes(256)

	compatibility := checkLivePolicyUpdateCompatibility(previous, next, poolConstructionModeStandalone)
	if compatibility.Compatible {
		t.Fatal("Compatible = true, want false")
	}
	if compatibility.RejectedKind != PolicyUpdateKindShape {
		t.Fatalf("RejectedKind = %s, want %s", compatibility.RejectedKind, PolicyUpdateKindShape)
	}
	if compatibility.FailureReason != policyUpdateFailureShapeChange {
		t.Fatalf("FailureReason = %q, want %q", compatibility.FailureReason, policyUpdateFailureShapeChange)
	}
	if err := validateLivePolicyUpdate(previous, next, poolConstructionModeStandalone); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("validateLivePolicyUpdate() = %v, want ErrInvalidPolicy", err)
	}
}

// TestPolicyUpdateRejectsShardShapeChange verifies the live-update boundary for
// shard, selector, bucket, and fallback fields that are fixed by construction.
func TestPolicyUpdateRejectsShardShapeChange(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Policy)
	}{
		{
			name: "shard_count",
			mutate: func(policy *Policy) {
				policy.Shards.ShardsPerClass = 2
			},
		},
		{
			name: "bucket_segment",
			mutate: func(policy *Policy) {
				policy.Shards.BucketSegmentSlotsPerShard = 4
			},
		},
		{
			name: "selector",
			mutate: func(policy *Policy) {
				policy.Shards.Selection = ShardSelectionModeRoundRobin
			},
		},
		{
			name: "acquisition_fallback",
			mutate: func(policy *Policy) {
				policy.Shards.AcquisitionFallbackShards = 1
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := policyUpdateTestPolicy()
			next := previous
			tt.mutate(&next)

			compatibility := checkLivePolicyUpdateCompatibility(previous, next, poolConstructionModeStandalone)
			if compatibility.Compatible {
				t.Fatal("Compatible = true, want false")
			}
			if compatibility.RejectedKind != PolicyUpdateKindShape {
				t.Fatalf("RejectedKind = %s, want %s", compatibility.RejectedKind, PolicyUpdateKindShape)
			}
			if compatibility.FailureReason != policyUpdateFailureShapeChange {
				t.Fatalf("FailureReason = %q, want %q", compatibility.FailureReason, policyUpdateFailureShapeChange)
			}
		})
	}
}

// TestPolicyUpdateRejectsOwnershipModeChange verifies that ownership/accounting
// changes are rejected even when the target owner supports managed ownership.
func TestPolicyUpdateRejectsOwnershipModeChange(t *testing.T) {
	previous := policyUpdateTestPolicy()
	next := previous
	next.Ownership = StrictOwnershipPolicy()

	compatibility := checkLivePolicyUpdateCompatibility(previous, next, poolConstructionModePartitionOwned)
	if compatibility.Compatible {
		t.Fatal("Compatible = true, want false")
	}
	if compatibility.RejectedKind != PolicyUpdateKindOwnership {
		t.Fatalf("RejectedKind = %s, want %s", compatibility.RejectedKind, PolicyUpdateKindOwnership)
	}
	if compatibility.FailureReason != policyUpdateFailureOwnershipChange {
		t.Fatalf("FailureReason = %q, want %q", compatibility.FailureReason, policyUpdateFailureOwnershipChange)
	}
	if err := validateLivePolicyUpdate(previous, next, poolConstructionModePartitionOwned); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("validateLivePolicyUpdate() = %v, want ErrInvalidPolicy", err)
	}
}

// TestPolicyUpdateAllowsRetentionAdmissionPressureTrimChanges verifies the
// initial live-compatible set for value-only policy publication.
func TestPolicyUpdateAllowsRetentionAdmissionPressureTrimChanges(t *testing.T) {
	previous := policyUpdateTestPolicy()
	next := previous
	next.Retention.SoftRetainedBytes = 3 * KiB
	next.Admission.ZeroDroppedBuffers = true
	next.Pressure = poolTestPressurePolicy()
	next.Trim = policyUpdateTestTrimPolicy()

	if err := validateLivePolicyUpdate(previous, next, poolConstructionModeStandalone); err != nil {
		t.Fatalf("validateLivePolicyUpdate() returned error: %v", err)
	}

	compatibility := checkLivePolicyUpdateCompatibility(previous, next, poolConstructionModeStandalone)
	if !compatibility.Compatible {
		t.Fatalf("Compatible = false, reason %q", compatibility.FailureReason)
	}
	if compatibility.Scope != PolicyUpdateScopeStandalonePool {
		t.Fatalf("Scope = %s, want %s", compatibility.Scope, PolicyUpdateScopeStandalonePool)
	}
	diagnostics := compatibility.Diff.Diagnostics
	if !diagnostics.RetentionChanged ||
		!diagnostics.AdmissionChanged ||
		!diagnostics.PressureChanged ||
		!diagnostics.TrimChanged {
		t.Fatalf("diagnostics = %+v, want retention/admission/pressure/trim changes", diagnostics)
	}
	if diagnostics.ShapeChanged || diagnostics.OwnershipChanged {
		t.Fatalf("diagnostics = %+v, want no shape or ownership change", diagnostics)
	}
}

// TestPolicyUpdateDiagnosticsSections verifies that diff diagnostics expose all
// section changes without requiring a publication attempt.
func TestPolicyUpdateDiagnosticsSections(t *testing.T) {
	previous := policyUpdateTestPolicy()
	next := previous
	next.Retention.SoftRetainedBytes = KiB
	next.Admission.ReturnedBuffers = ReturnedBufferPolicyDrop
	next.Pressure = poolTestPressurePolicy()
	next.Trim = policyUpdateTestTrimPolicy()
	next.Ownership = StrictOwnershipPolicy()

	diff := classifyPolicyUpdate(previous, next)
	diagnostics := diff.Diagnostics
	if diagnostics.ShapeChanged {
		t.Fatalf("ShapeChanged = true, want false")
	}
	if !diagnostics.RetentionChanged {
		t.Fatalf("RetentionChanged = false, want true")
	}
	if !diagnostics.RetentionContracted {
		t.Fatalf("RetentionContracted = false, want true")
	}
	if diagnostics.RetentionExpanded {
		t.Fatalf("RetentionExpanded = true, want false")
	}
	if !diagnostics.AdmissionChanged {
		t.Fatalf("AdmissionChanged = false, want true")
	}
	if !diagnostics.PressureChanged {
		t.Fatalf("PressureChanged = false, want true")
	}
	if !diagnostics.TrimChanged {
		t.Fatalf("TrimChanged = false, want true")
	}
	if !diagnostics.OwnershipChanged {
		t.Fatalf("OwnershipChanged = false, want true")
	}
	if !diagnostics.NeedsTrim {
		t.Fatalf("NeedsTrim = false, want true")
	}
	if !policyUpdateNeedsTrim(previous, next) {
		t.Fatalf("policyUpdateNeedsTrim() = false, want true")
	}
}

// TestPolicyUpdateZeroDiff verifies that equal policy values produce an empty
// diagnostic diff and remain live-compatible.
func TestPolicyUpdateZeroDiff(t *testing.T) {
	previous := policyUpdateTestPolicy()
	next := previous

	diff := classifyPolicyUpdate(previous, next)
	if !diff.IsZero() {
		t.Fatalf("diff.IsZero() = false, diagnostics %+v", diff.Diagnostics)
	}
	if err := validateLivePolicyUpdate(previous, next, poolConstructionModeStandalone); err != nil {
		t.Fatalf("validateLivePolicyUpdate() returned error: %v", err)
	}
}

// TestPolicyUpdateDiagnosticsDetectExpansion verifies that expansion is visible
// without being confused with contraction.
func TestPolicyUpdateDiagnosticsDetectExpansion(t *testing.T) {
	previous := policyUpdateTestPolicy()
	next := previous
	next.Retention.SoftRetainedBytes = 3 * KiB

	diagnostics := classifyPolicyUpdate(previous, next).Diagnostics
	if !diagnostics.RetentionExpanded {
		t.Fatalf("RetentionExpanded = false, want true")
	}
	if diagnostics.RetentionContracted {
		t.Fatalf("RetentionContracted = true, want false")
	}
	if policyUpdateContractsRetention(previous, next) {
		t.Fatalf("policyUpdateContractsRetention() = true, want false")
	}
	if !policyUpdateExpandsRetention(previous, next) {
		t.Fatalf("policyUpdateExpandsRetention() = false, want true")
	}
}

func policyUpdateTestPolicy() Policy {
	return poolTestSmallSingleShardPolicy()
}

func policyUpdateTestTrimPolicy() TrimPolicy {
	return TrimPolicy{
		Enabled:                   true,
		Interval:                  time.Second,
		FullScanInterval:          2 * time.Second,
		MaxBuffersPerCycle:        2,
		MaxBytesPerCycle:          KiB,
		MaxPoolsPerCycle:          1,
		MaxClassesPerPoolPerCycle: 1,
		MaxShardsPerClassPerCycle: 1,
		TrimOnPolicyShrink:        true,
		TrimOnPressure:            true,
		TrimOnClose:               true,
	}
}
