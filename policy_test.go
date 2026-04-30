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
	"math"
	"testing"
	"time"
)

// TestPolicyIsZero verifies aggregate zero-state semantics for the top-level
// runtime policy.
//
// Policy is a behavioral model, not a concrete Pool/Group/Partition config.
// Its zero value means "no explicit policy has been configured yet". A single
// non-zero nested policy section must make the whole Policy non-zero so default
// completion and validation code can distinguish an empty policy from a partially
// configured policy.
func TestPolicyIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy Policy
		want   bool
	}{
		{
			name:   "zero",
			policy: Policy{},
			want:   true,
		},
		{
			name: "retention",
			policy: Policy{
				Retention: RetentionPolicy{
					HardRetainedBytes: 64 * MiB,
				},
			},
			want: false,
		},
		{
			name: "classes",
			policy: Policy{
				Classes: ClassPolicy{
					Sizes: []ClassSize{ClassSizeFromSize(KiB)},
				},
			},
			want: false,
		},
		{
			name: "shards",
			policy: Policy{
				Shards: ShardPolicy{
					ShardsPerClass: 4,
				},
			},
			want: false,
		},
		{
			name: "admission",
			policy: Policy{
				Admission: AdmissionPolicy{
					ReturnedBuffers: ReturnedBufferPolicyAdmit,
				},
			},
			want: false,
		},
		{
			name: "pressure",
			policy: Policy{
				Pressure: PressurePolicy{
					Enabled: true,
				},
			},
			want: false,
		},
		{
			name: "trim",
			policy: Policy{
				Trim: TrimPolicy{
					Enabled: true,
				},
			},
			want: false,
		},
		{
			name: "ownership",
			policy: Policy{
				Ownership: OwnershipPolicy{
					Mode: OwnershipModeStrict,
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.policy.IsZero(); got != tt.want {
				t.Fatalf("Policy.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRetentionPolicyIsZero verifies zero-state detection for retained-memory
// limits.
//
// RetentionPolicy is intentionally field-rich because it covers multiple scopes:
// request limits, retained capacity limits, class limits, and shard limits. Each
// field must independently mark the retention policy as explicit so validation
// can later reject contradictory combinations or complete missing defaults.
func TestRetentionPolicyIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		retention RetentionPolicy
		want      bool
	}{
		{
			name:      "zero",
			retention: RetentionPolicy{},
			want:      true,
		},
		{
			name: "soft retained bytes",
			retention: RetentionPolicy{
				SoftRetainedBytes: 32 * MiB,
			},
			want: false,
		},
		{
			name: "hard retained bytes",
			retention: RetentionPolicy{
				HardRetainedBytes: 64 * MiB,
			},
			want: false,
		},
		{
			name: "max retained buffers",
			retention: RetentionPolicy{
				MaxRetainedBuffers: 1024,
			},
			want: false,
		},
		{
			name: "max request size",
			retention: RetentionPolicy{
				MaxRequestSize: 4 * MiB,
			},
			want: false,
		},
		{
			name: "max retained buffer capacity",
			retention: RetentionPolicy{
				MaxRetainedBufferCapacity: 8 * MiB,
			},
			want: false,
		},
		{
			name: "max class retained bytes",
			retention: RetentionPolicy{
				MaxClassRetainedBytes: 16 * MiB,
			},
			want: false,
		},
		{
			name: "max class retained buffers",
			retention: RetentionPolicy{
				MaxClassRetainedBuffers: 256,
			},
			want: false,
		},
		{
			name: "max shard retained bytes",
			retention: RetentionPolicy{
				MaxShardRetainedBytes: 2 * MiB,
			},
			want: false,
		},
		{
			name: "max shard retained buffers",
			retention: RetentionPolicy{
				MaxShardRetainedBuffers: 64,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.retention.IsZero(); got != tt.want {
				t.Fatalf("RetentionPolicy.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestClassPolicyIsZero verifies class-size profile zero-state semantics.
//
// A class policy with no sizes is unset. A policy with at least one normalized
// ClassSize is explicit. Validation of strict ordering belongs to policy
// validation or class-table construction, not to ClassPolicy.IsZero.
func TestClassPolicyIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		classes ClassPolicy
		want    bool
	}{
		{
			name:    "zero",
			classes: ClassPolicy{},
			want:    true,
		},
		{
			name: "empty slice",
			classes: ClassPolicy{
				Sizes: []ClassSize{},
			},
			want: true,
		},
		{
			name: "one class",
			classes: ClassPolicy{
				Sizes: []ClassSize{ClassSizeFromSize(KiB)},
			},
			want: false,
		},
		{
			name: "multiple classes",
			classes: ClassPolicy{
				Sizes: []ClassSize{
					ClassSizeFromBytes(512),
					ClassSizeFromSize(KiB),
					ClassSizeFromSize(2 * KiB),
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.classes.IsZero(); got != tt.want {
				t.Fatalf("ClassPolicy.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestClassPolicySizesCopy verifies defensive-copy behavior for configured class
// sizes.
//
// Policy objects may be completed, validated, or published into runtime
// snapshots. A helper that exposes the configured class sizes must not leak the
// internal slice, otherwise caller-side mutations could change a policy after it
// has been validated or published.
func TestClassPolicySizesCopy(t *testing.T) {
	t.Parallel()

	t.Run("nil for zero policy", func(t *testing.T) {
		t.Parallel()

		var classes ClassPolicy

		if got := classes.SizesCopy(); got != nil {
			t.Fatalf("SizesCopy() = %#v, want nil", got)
		}
	})

	t.Run("copy is independent from source", func(t *testing.T) {
		t.Parallel()

		classes := ClassPolicy{
			Sizes: []ClassSize{
				ClassSizeFromBytes(512),
				ClassSizeFromSize(KiB),
				ClassSizeFromSize(2 * KiB),
			},
		}

		copied := classes.SizesCopy()

		if len(copied) != len(classes.Sizes) {
			t.Fatalf("len(SizesCopy()) = %d, want %d", len(copied), len(classes.Sizes))
		}

		for index := range classes.Sizes {
			if copied[index] != classes.Sizes[index] {
				t.Fatalf("SizesCopy()[%d] = %s, want %s", index, copied[index], classes.Sizes[index])
			}
		}

		copied[0] = ClassSizeFromSize(4 * KiB)

		if classes.Sizes[0] != ClassSizeFromBytes(512) {
			t.Fatalf("mutating copied slice changed source: source[0] = %s, want 512 B", classes.Sizes[0])
		}

		classes.Sizes[1] = ClassSizeFromSize(8 * KiB)

		if copied[1] != ClassSizeFromSize(KiB) {
			t.Fatalf("mutating source slice changed copy: copied[1] = %s, want 1 KiB", copied[1])
		}
	})
}

// TestShardPolicyIsZero verifies zero-state detection for sharding behavior.
//
// ShardPolicy combines hot-path lock striping, physical bucket capacity, and
// optional fallback probing. Any non-zero field represents an explicit policy
// decision and must make the policy non-zero.
func TestShardPolicyIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		shards ShardPolicy
		want   bool
	}{
		{
			name:   "zero",
			shards: ShardPolicy{},
			want:   true,
		},
		{
			name: "selection",
			shards: ShardPolicy{
				Selection: ShardSelectionModeRoundRobin,
			},
			want: false,
		},
		{
			name: "shards per class",
			shards: ShardPolicy{
				ShardsPerClass: 16,
			},
			want: false,
		},
		{
			name: "bucket slots per shard",
			shards: ShardPolicy{
				BucketSlotsPerShard: 64,
			},
			want: false,
		},
		{
			name: "bucket segment slots per shard",
			shards: ShardPolicy{
				BucketSegmentSlotsPerShard: 8,
			},
			want: false,
		},
		{
			name: "acquisition fallback shards",
			shards: ShardPolicy{
				AcquisitionFallbackShards: 1,
			},
			want: false,
		},
		{
			name: "return fallback shards",
			shards: ShardPolicy{
				ReturnFallbackShards: 1,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.shards.IsZero(); got != tt.want {
				t.Fatalf("ShardPolicy.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestShardSelectionModeString verifies stable diagnostic labels for shard
// selection modes.
//
// These labels may appear in snapshots, debug output, benchmark reports, and
// validation errors. Unknown values should not panic because they can appear when
// corrupted, manually constructed, or outside the constants known to this code.
func TestShardSelectionModeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode ShardSelectionMode
		want string
	}{
		{
			name: "unset",
			mode: ShardSelectionModeUnset,
			want: "unset",
		},
		{
			name: "single",
			mode: ShardSelectionModeSingle,
			want: "single",
		},
		{
			name: "round robin",
			mode: ShardSelectionModeRoundRobin,
			want: "round_robin",
		},
		{
			name: "random",
			mode: ShardSelectionModeRandom,
			want: "random",
		},
		{
			name: "processor inspired",
			mode: ShardSelectionModeProcessorInspired,
			want: "processor_inspired",
		},
		{
			name: "affinity",
			mode: ShardSelectionModeAffinity,
			want: "affinity",
		},
		{
			name: "unknown",
			mode: ShardSelectionMode(255),
			want: "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.mode.String(); got != tt.want {
				t.Fatalf("ShardSelectionMode.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDefaultShardPolicyUsesRuntimeDerivedShards verifies the default shard
// shape follows the GOMAXPROCS-derived resolver and keeps bounded acquisition
// fallback enabled.
func TestDefaultShardPolicyUsesRuntimeDerivedShards(t *testing.T) {
	t.Parallel()

	shards := DefaultPolicyShardsPerClass()
	if shards < DefaultPolicyMinShardsPerClass || shards > DefaultPolicyMaxShardsPerClass {
		t.Fatalf("DefaultPolicyShardsPerClass() = %d, want in [%d, %d]",
			shards,
			DefaultPolicyMinShardsPerClass,
			DefaultPolicyMaxShardsPerClass,
		)
	}
	if shards&(shards-1) != 0 {
		t.Fatalf("DefaultPolicyShardsPerClass() = %d, want power of two", shards)
	}

	policy := DefaultShardPolicy()
	if policy.Selection != ShardSelectionModeProcessorInspired {
		t.Fatalf("default selection = %s, want processor-inspired", policy.Selection)
	}
	if policy.ShardsPerClass != shards {
		t.Fatalf("default shards per class = %d, want %d", policy.ShardsPerClass, shards)
	}
	if policy.BucketSegmentSlotsPerShard != DefaultPolicyBucketSegmentSlotsPerShard {
		t.Fatalf("default bucket segment slots = %d, want %d",
			policy.BucketSegmentSlotsPerShard,
			DefaultPolicyBucketSegmentSlotsPerShard,
		)
	}
	if policy.AcquisitionFallbackShards != DefaultPolicyAcquisitionFallbackShards {
		t.Fatalf("default acquisition fallback = %d, want %d",
			policy.AcquisitionFallbackShards,
			DefaultPolicyAcquisitionFallbackShards,
		)
	}
	if policy.ReturnFallbackShards != 0 {
		t.Fatalf("default return fallback = %d, want zero", policy.ReturnFallbackShards)
	}
}

// TestDefaultPolicyShardsPerClassForGOMAXPROCS verifies resolver boundaries
// without mutating process-wide GOMAXPROCS in a parallel test.
func TestDefaultPolicyShardsPerClassForGOMAXPROCS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		procs int
		want  int
	}{
		{name: "zero", procs: 0, want: 1},
		{name: "one", procs: 1, want: 1},
		{name: "two", procs: 2, want: 2},
		{name: "round up", procs: 3, want: 4},
		{name: "cap boundary", procs: 17, want: 32},
		{name: "above cap", procs: 64, want: 32},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := defaultPolicyShardsPerClassForGOMAXPROCS(tt.procs); got != tt.want {
				t.Fatalf("defaultPolicyShardsPerClassForGOMAXPROCS(%d) = %d, want %d", tt.procs, got, tt.want)
			}
		})
	}
}

// TestAdmissionPolicyIsZero verifies zero-state detection for return-path
// admission behavior.
//
// AdmissionPolicy intentionally separates ordinary returned-buffer posture from
// specific failure-condition actions. A policy that configures any one of those
// decisions is explicit and must not be considered zero.
func TestAdmissionPolicyIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		admission AdmissionPolicy
		want      bool
	}{
		{
			name:      "zero",
			admission: AdmissionPolicy{},
			want:      true,
		},
		{
			name: "zero size requests",
			admission: AdmissionPolicy{
				ZeroSizeRequests: ZeroSizeRequestReject,
			},
			want: false,
		},
		{
			name: "returned buffers",
			admission: AdmissionPolicy{
				ReturnedBuffers: ReturnedBufferPolicyAdmit,
			},
			want: false,
		},
		{
			name: "oversized return",
			admission: AdmissionPolicy{
				OversizedReturn: AdmissionActionDrop,
			},
			want: false,
		},
		{
			name: "unsupported class",
			admission: AdmissionPolicy{
				UnsupportedClass: AdmissionActionError,
			},
			want: false,
		},
		{
			name: "class mismatch",
			admission: AdmissionPolicy{
				ClassMismatch: AdmissionActionDrop,
			},
			want: false,
		},
		{
			name: "credit exhausted",
			admission: AdmissionPolicy{
				CreditExhausted: AdmissionActionDrop,
			},
			want: false,
		},
		{
			name: "bucket full",
			admission: AdmissionPolicy{
				BucketFull: AdmissionActionDrop,
			},
			want: false,
		},
		{
			name: "zero retained buffers",
			admission: AdmissionPolicy{
				ZeroRetainedBuffers: true,
			},
			want: false,
		},
		{
			name: "zero dropped buffers",
			admission: AdmissionPolicy{
				ZeroDroppedBuffers: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.admission.IsZero(); got != tt.want {
				t.Fatalf("AdmissionPolicy.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestZeroSizeRequestPolicyString verifies stable diagnostic labels for
// zero-size acquisition behavior.
func TestZeroSizeRequestPolicyString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy ZeroSizeRequestPolicy
		want   string
	}{
		{
			name:   "unset",
			policy: ZeroSizeRequestUnset,
			want:   "unset",
		},
		{
			name:   "smallest class",
			policy: ZeroSizeRequestSmallestClass,
			want:   "smallest_class",
		},
		{
			name:   "empty buffer",
			policy: ZeroSizeRequestEmptyBuffer,
			want:   "empty_buffer",
		},
		{
			name:   "reject",
			policy: ZeroSizeRequestReject,
			want:   "reject",
		},
		{
			name:   "unknown",
			policy: ZeroSizeRequestPolicy(255),
			want:   "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.policy.String(); got != tt.want {
				t.Fatalf("ZeroSizeRequestPolicy.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestReturnedBufferPolicyString verifies stable diagnostic labels for ordinary
// returned-buffer posture.
func TestReturnedBufferPolicyString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy ReturnedBufferPolicy
		want   string
	}{
		{
			name:   "unset",
			policy: ReturnedBufferPolicyUnset,
			want:   "unset",
		},
		{
			name:   "admit",
			policy: ReturnedBufferPolicyAdmit,
			want:   "admit",
		},
		{
			name:   "drop",
			policy: ReturnedBufferPolicyDrop,
			want:   "drop",
		},
		{
			name:   "unknown",
			policy: ReturnedBufferPolicy(255),
			want:   "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.policy.String(); got != tt.want {
				t.Fatalf("ReturnedBufferPolicy.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAdmissionActionString verifies stable diagnostic labels for admission
// actions.
func TestAdmissionActionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action AdmissionAction
		want   string
	}{
		{
			name:   "unset",
			action: AdmissionActionUnset,
			want:   "unset",
		},
		{
			name:   "drop",
			action: AdmissionActionDrop,
			want:   "drop",
		},
		{
			name:   "ignore",
			action: AdmissionActionIgnore,
			want:   "ignore",
		},
		{
			name:   "error",
			action: AdmissionActionError,
			want:   "error",
		},
		{
			name:   "unknown",
			action: AdmissionAction(255),
			want:   "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.action.String(); got != tt.want {
				t.Fatalf("AdmissionAction.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPressurePolicyIsZero verifies zero-state detection for pressure behavior.
//
// PressurePolicy has both an enable flag and per-level behavior. Either one is
// enough to make the policy explicit. This supports validation code that can
// reject level configuration when pressure handling is disabled, or complete
// defaults when pressure is enabled without explicit level policies.
func TestPressurePolicyIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pressure PressurePolicy
		want     bool
	}{
		{
			name:     "zero",
			pressure: PressurePolicy{},
			want:     true,
		},
		{
			name: "enabled",
			pressure: PressurePolicy{
				Enabled: true,
			},
			want: false,
		},
		{
			name: "medium",
			pressure: PressurePolicy{
				Medium: PressureLevelPolicy{
					RetentionScale: PolicyRatioOne / 2,
				},
			},
			want: false,
		},
		{
			name: "high",
			pressure: PressurePolicy{
				High: PressureLevelPolicy{
					DisableRetention: true,
				},
			},
			want: false,
		},
		{
			name: "critical",
			pressure: PressurePolicy{
				Critical: PressureLevelPolicy{
					TrimScale: 2 * PolicyRatioOne,
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.pressure.IsZero(); got != tt.want {
				t.Fatalf("PressurePolicy.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPressureLevelString verifies stable diagnostic labels for runtime pressure
// levels.
func TestPressureLevelString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level PressureLevel
		want  string
	}{
		{
			name:  "normal",
			level: PressureLevelNormal,
			want:  "normal",
		},
		{
			name:  "medium",
			level: PressureLevelMedium,
			want:  "medium",
		},
		{
			name:  "high",
			level: PressureLevelHigh,
			want:  "high",
		},
		{
			name:  "critical",
			level: PressureLevelCritical,
			want:  "critical",
		},
		{
			name:  "unknown",
			level: PressureLevel(255),
			want:  "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.level.String(); got != tt.want {
				t.Fatalf("PressureLevel.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPressureLevelPolicyIsZero verifies zero-state detection for one pressure
// level policy.
//
// The policy uses fixed-point ratios for retention and trim scaling, but a ratio
// of zero still means "unset" at this layer. Field-specific validation and
// defaulting decide whether zero is acceptable for a given pressure level.
func TestPressureLevelPolicyIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy PressureLevelPolicy
		want   bool
	}{
		{
			name:   "zero",
			policy: PressureLevelPolicy{},
			want:   true,
		},
		{
			name: "retention scale",
			policy: PressureLevelPolicy{
				RetentionScale: PolicyRatioOne / 2,
			},
			want: false,
		},
		{
			name: "trim scale",
			policy: PressureLevelPolicy{
				TrimScale: 2 * PolicyRatioOne,
			},
			want: false,
		},
		{
			name: "max retained buffer capacity",
			policy: PressureLevelPolicy{
				MaxRetainedBufferCapacity: 2 * MiB,
			},
			want: false,
		},
		{
			name: "drop returned capacity above",
			policy: PressureLevelPolicy{
				DropReturnedCapacityAbove: 512 * KiB,
			},
			want: false,
		},
		{
			name: "disable retention",
			policy: PressureLevelPolicy{
				DisableRetention: true,
			},
			want: false,
		},
		{
			name: "preserve small hot classes",
			policy: PressureLevelPolicy{
				PreserveSmallHotClasses: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.policy.IsZero(); got != tt.want {
				t.Fatalf("PressureLevelPolicy.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTrimPolicyIsZero verifies zero-state detection for bounded physical
// removal behavior.
//
// TrimPolicy is intentionally separated from AdmissionPolicy: admission decides
// whether new returned buffers may enter retained storage, while trim physically
// removes buffers already retained by buckets.
func TestTrimPolicyIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		trim TrimPolicy
		want bool
	}{
		{
			name: "zero",
			trim: TrimPolicy{},
			want: true,
		},
		{
			name: "enabled",
			trim: TrimPolicy{
				Enabled: true,
			},
			want: false,
		},
		{
			name: "interval",
			trim: TrimPolicy{
				Interval: time.Second,
			},
			want: false,
		},
		{
			name: "full scan interval",
			trim: TrimPolicy{
				FullScanInterval: 30 * time.Second,
			},
			want: false,
		},
		{
			name: "max buffers per cycle",
			trim: TrimPolicy{
				MaxBuffersPerCycle: 128,
			},
			want: false,
		},
		{
			name: "max bytes per cycle",
			trim: TrimPolicy{
				MaxBytesPerCycle: 16 * MiB,
			},
			want: false,
		},
		{
			name: "max pools per cycle",
			trim: TrimPolicy{
				MaxPoolsPerCycle: 8,
			},
			want: false,
		},
		{
			name: "max classes per pool per cycle",
			trim: TrimPolicy{
				MaxClassesPerPoolPerCycle: 4,
			},
			want: false,
		},
		{
			name: "max shards per class per cycle",
			trim: TrimPolicy{
				MaxShardsPerClassPerCycle: 2,
			},
			want: false,
		},
		{
			name: "trim on policy shrink",
			trim: TrimPolicy{
				TrimOnPolicyShrink: true,
			},
			want: false,
		},
		{
			name: "trim on pressure",
			trim: TrimPolicy{
				TrimOnPressure: true,
			},
			want: false,
		},
		{
			name: "trim on close",
			trim: TrimPolicy{
				TrimOnClose: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.trim.IsZero(); got != tt.want {
				t.Fatalf("TrimPolicy.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOwnershipPolicyIsZero verifies zero-state detection for ownership-aware
// accounting behavior.
//
// OwnershipPolicy affects checked-out buffer accounting and release validation.
// Any explicit mode or tracking flag must make the policy non-zero.
func TestOwnershipPolicyIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ownership OwnershipPolicy
		want      bool
	}{
		{
			name:      "zero",
			ownership: OwnershipPolicy{},
			want:      true,
		},
		{
			name: "mode",
			ownership: OwnershipPolicy{
				Mode: OwnershipModeAccounting,
			},
			want: false,
		},
		{
			name: "track in-use bytes",
			ownership: OwnershipPolicy{
				TrackInUseBytes: true,
			},
			want: false,
		},
		{
			name: "track in-use buffers",
			ownership: OwnershipPolicy{
				TrackInUseBuffers: true,
			},
			want: false,
		},
		{
			name: "detect double release",
			ownership: OwnershipPolicy{
				DetectDoubleRelease: true,
			},
			want: false,
		},
		{
			name: "max returned capacity growth",
			ownership: OwnershipPolicy{
				MaxReturnedCapacityGrowth: 2 * PolicyRatioOne,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ownership.IsZero(); got != tt.want {
				t.Fatalf("OwnershipPolicy.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOwnershipModeString verifies stable diagnostic labels for ownership modes.
func TestOwnershipModeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode OwnershipMode
		want string
	}{
		{
			name: "unset",
			mode: OwnershipModeUnset,
			want: "unset",
		},
		{
			name: "none",
			mode: OwnershipModeNone,
			want: "none",
		},
		{
			name: "accounting",
			mode: OwnershipModeAccounting,
			want: "accounting",
		},
		{
			name: "strict",
			mode: OwnershipModeStrict,
			want: "strict",
		},
		{
			name: "unknown",
			mode: OwnershipMode(255),
			want: "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.mode.String(); got != tt.want {
				t.Fatalf("OwnershipMode.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPolicyRatioConstants verifies fixed-point ratio constants.
//
// PolicyRatio uses basis-point style fixed-point values so policy fields can be
// represented without floating-point drift. PolicyRatioOne must remain equal to
// PolicyRatioScale because many defaults and validation rules will use it as the
// canonical 100% value.
func TestPolicyRatioConstants(t *testing.T) {
	t.Parallel()

	if PolicyRatioScale != 10_000 {
		t.Fatalf("PolicyRatioScale = %d, want 10000", PolicyRatioScale)
	}

	if PolicyRatioOne != PolicyRatio(PolicyRatioScale) {
		t.Fatalf("PolicyRatioOne = %d, want %d", PolicyRatioOne, PolicyRatio(PolicyRatioScale))
	}
}

// TestPolicyRatioFromBasisPoints verifies that basis-point construction is a
// simple domain conversion, not validation.
//
// Field-specific validation belongs with policy validation because some policy
// fields are unit ratios while others, such as trim scaling or returned-capacity
// growth, may validly exceed one.
func TestPolicyRatioFromBasisPoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		basisPoints uint32
		want        PolicyRatio
	}{
		{
			name:        "zero",
			basisPoints: 0,
			want:        0,
		},
		{
			name:        "half",
			basisPoints: PolicyRatioScale / 2,
			want:        PolicyRatioOne / 2,
		},
		{
			name:        "one",
			basisPoints: PolicyRatioScale,
			want:        PolicyRatioOne,
		},
		{
			name:        "above one is preserved",
			basisPoints: 2 * PolicyRatioScale,
			want:        2 * PolicyRatioOne,
		},
		{
			name:        "max uint32 is preserved",
			basisPoints: math.MaxUint32,
			want:        PolicyRatio(math.MaxUint32),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := PolicyRatioFromBasisPoints(tt.basisPoints); got != tt.want {
				t.Fatalf("PolicyRatioFromBasisPoints(%d) = %d, want %d", tt.basisPoints, got, tt.want)
			}
		})
	}
}

// TestPolicyRatioAccessors verifies the small helper methods on PolicyRatio.
//
// These helpers intentionally do not perform validation or clamping. They expose
// the fixed-point value in forms useful for diagnostics, validation, and control
// calculations.
func TestPolicyRatioAccessors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ratio       PolicyRatio
		basisPoints uint32
		float       float64
		isZero      bool
		isOne       bool
	}{
		{
			name:        "zero",
			ratio:       0,
			basisPoints: 0,
			float:       0,
			isZero:      true,
			isOne:       false,
		},
		{
			name:        "half",
			ratio:       PolicyRatioOne / 2,
			basisPoints: PolicyRatioScale / 2,
			float:       0.5,
			isZero:      false,
			isOne:       false,
		},
		{
			name:        "one",
			ratio:       PolicyRatioOne,
			basisPoints: PolicyRatioScale,
			float:       1.0,
			isZero:      false,
			isOne:       true,
		},
		{
			name:        "above one",
			ratio:       2 * PolicyRatioOne,
			basisPoints: 2 * PolicyRatioScale,
			float:       2.0,
			isZero:      false,
			isOne:       false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ratio.BasisPoints(); got != tt.basisPoints {
				t.Fatalf("BasisPoints() = %d, want %d", got, tt.basisPoints)
			}

			if got := tt.ratio.Float64(); got != tt.float {
				t.Fatalf("Float64() = %f, want %f", got, tt.float)
			}

			if got := tt.ratio.IsZero(); got != tt.isZero {
				t.Fatalf("IsZero() = %v, want %v", got, tt.isZero)
			}

			if got := tt.ratio.IsOne(); got != tt.isOne {
				t.Fatalf("IsOne() = %v, want %v", got, tt.isOne)
			}
		})
	}
}
