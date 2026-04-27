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

import "time"

// Named profiles are complete policy presets for common static bounded-runtime
// postures.
//
// This file does not build runtime owners and does not publish live limits. It
// only constructs Policy values. The next layer can then validate the selected
// policy, merge caller overrides, and create the runtime objects that enforce
// the policy.
//
// A profile affects the current and planned runtime layers through the ordinary
// policy sections:
//
//   - Retention defines byte and buffer-count limits that later become owner
//     budgets, class budgets, and shard credits.
//   - Classes define the explicit size-class table used for request ceil lookup
//     and returned-capacity floor lookup.
//   - Shards define lock striping, bucket depth, and optional fallback probing.
//   - Admission defines how unsuitable returned buffers should be handled by the
//     future public Pool return path.
//   - Pressure defines static contraction rules for future pressure publishers;
//     it does not observe memory pressure by itself.
//   - Trim defines bounded cold-path cleanup work for future trim controllers.
//   - Ownership defines whether future ownership/lease layers should track and
//     validate checked-out buffers.
//
// Profiles are deliberately static. They are useful because they provide named,
// reviewable defaults before adaptive policy exists, but they are not a
// substitute for future Pool, pressure, ownership, or adaptive control-plane
// behavior.

const (
	// errUnknownPolicyProfile is used when a caller requests a named policy
	// profile that is not defined by this package.
	//
	// Profile lookup is intentionally strict for MustPolicyForProfile because an
	// unknown profile means a static construction path is using an invalid enum
	// value. User-facing configuration should translate this condition into a
	// classified options or policy error instead of exposing this internal panic
	// message directly.
	errUnknownPolicyProfile = "bufferpool.policyProfile: unknown policy profile"
)

// PolicyProfile identifies a named policy preset.
//
// A profile is not runtime state and is not a separate owner configuration. It
// is a convenient way to select a complete Policy tuned for a broad workload
// posture.
//
// Each profile answers the question "which static bounded retention behavior is
// a reasonable starting point for this workload?" It does not inspect live
// traffic, does not redistribute memory between classes, and does not replace
// future adaptive controllers. The selected Policy is still just data: owner
// construction code will later validate it, publish it, and project retention
// limits into class budgets and shard credits.
//
// Profiles deliberately return full Policy values instead of partial overrides.
// This keeps profile behavior explicit and prevents hidden coupling between
// profile code and DefaultPolicy internals. A caller can inspect a profile in
// isolation and see every retention, class, shard, admission, pressure, trim,
// and ownership choice that the profile makes.
type PolicyProfile uint8

const (
	// PolicyProfileUnset means no profile has been selected.
	//
	// It is useful in option structs that need to distinguish "caller did not
	// choose a profile" from "caller explicitly chose the default profile". It is
	// not accepted by PolicyForProfile because it is absence of configuration, not
	// a real preset.
	PolicyProfileUnset PolicyProfile = iota

	// PolicyProfileDefault selects the canonical balanced default policy.
	//
	// This is the safest general-purpose starting point: moderate retained-memory
	// limits, ordinary power-of-two class coverage, round-robin shard striping,
	// pressure handling, trim, and ownership tracking disabled by default.
	PolicyProfileDefault

	// PolicyProfileThroughput selects a policy that favors reuse hit rate and
	// higher retained capacity for allocation-heavy hot paths.
	//
	// It spends more retained memory and metadata on broader class coverage,
	// wider sharding, larger buckets, and limited fallback probing.
	PolicyProfileThroughput

	// PolicyProfileMemoryConstrained selects a policy that favors lower retained
	// memory and smaller class coverage.
	//
	// It keeps retained buffers tightly bounded and trims more frequently, which
	// helps services where resident memory is more important than the highest
	// possible reuse hit rate.
	PolicyProfileMemoryConstrained

	// PolicyProfileBursty selects a policy that tolerates short-lived bursts
	// while still bounding retained memory and trim work.
	//
	// It permits a higher hard retained-memory ceiling than the default, then
	// relies on pressure and trim settings to contract after burst traffic fades.
	PolicyProfileBursty

	// PolicyProfileStrictBounded selects a policy with tighter admission,
	// smaller physical buckets, and stronger bounds.
	//
	// It is meant for callers that want deterministic memory behavior and
	// ownership accounting even if that reduces reuse opportunities.
	PolicyProfileStrictBounded

	// PolicyProfileSecure selects a policy that favors data hygiene and strict
	// ownership accounting over raw throughput.
	//
	// It enables zeroing and strict ownership checks, so callers should expect
	// more hot-path work in exchange for stricter buffer handling.
	PolicyProfileSecure
)

// String returns a stable diagnostic label for p.
//
// These labels are intended for logs, snapshots, option diagnostics, and tests.
// Unknown values return "unknown" instead of panicking so corrupted or manually
// constructed enum values remain inspectable.
func (p PolicyProfile) String() string {
	switch p {
	case PolicyProfileUnset:
		return "unset"
	case PolicyProfileDefault:
		return "default"
	case PolicyProfileThroughput:
		return "throughput"
	case PolicyProfileMemoryConstrained:
		return "memory_constrained"
	case PolicyProfileBursty:
		return "bursty"
	case PolicyProfileStrictBounded:
		return "strict_bounded"
	case PolicyProfileSecure:
		return "secure"
	default:
		return "unknown"
	}
}

// Policy returns the policy associated with p.
//
// The returned bool is false when p is not a known selectable profile. The unset
// profile is not selectable because it represents absence of configuration.
//
// This method is a convenience wrapper around PolicyForProfile. It lets code
// that already carries a PolicyProfile resolve the preset without repeating the
// enum value at the call site.
func (p PolicyProfile) Policy() (Policy, bool) {
	return PolicyForProfile(p)
}

// PolicyForProfile returns a complete Policy for a named profile.
//
// Every successful call returns a fresh Policy value. In particular, class-size
// slices are independent and may be modified by the caller without corrupting
// later profile results.
//
// The returned value is intentionally concrete rather than layered as a set of
// overrides. This makes the resulting behavior easy to inspect: callers can see
// exactly which retention limits, class sizes, shard counts, admission actions,
// pressure levels, trim limits, and ownership settings will be used before any
// runtime owner is constructed.
//
// A profile result is suitable as a policy seed. Callers may use it directly,
// copy it and override selected fields, or pass it through future Pool/Group/
// Partition option processing. Validation remains a separate step; keeping
// lookup and validation separate makes profile selection usable in option
// builders, tests, examples, and static configuration paths.
func PolicyForProfile(profile PolicyProfile) (Policy, bool) {
	switch profile {
	case PolicyProfileDefault:
		return DefaultPolicy(), true
	case PolicyProfileThroughput:
		return ThroughputPolicy(), true
	case PolicyProfileMemoryConstrained:
		return MemoryConstrainedPolicy(), true
	case PolicyProfileBursty:
		return BurstyPolicy(), true
	case PolicyProfileStrictBounded:
		return StrictBoundedPolicy(), true
	case PolicyProfileSecure:
		return SecurePolicy(), true
	default:
		return Policy{}, false
	}
}

// MustPolicyForProfile returns a complete Policy for profile or panics when the
// profile is unknown or unset.
//
// This helper is intended for internal tests, examples, and static profile
// construction paths where an invalid profile is a programmer error. User-facing
// configuration should prefer PolicyForProfile and return a classified error.
//
// The panic does not mean policy validation failed. It means the requested enum
// value does not identify a selectable preset at all.
func MustPolicyForProfile(profile PolicyProfile) Policy {
	policy, ok := PolicyForProfile(profile)
	if !ok {
		panic(errUnknownPolicyProfile)
	}

	return policy
}

// ThroughputPolicy returns a policy tuned for hot allocation-heavy workloads.
//
// This profile increases retained-memory budgets, class coverage, shard count,
// and bucket slots. It still preserves explicit bounds and pressure contraction,
// but it gives the runtime more retained capacity to trade memory for reuse hit
// rate.
//
// The main idea is to keep more reusable backing arrays close to hot request
// paths. More class sizes mean more returned capacities can be classified
// without being dropped. More shards and bucket slots reduce local contention and
// allow each class to keep a deeper LIFO history. Limited acquisition and return
// fallback probing improves hit rate when the primary selected shard is empty or
// full, at the cost of extra shard checks.
//
// These settings work together as a static upper envelope. The hard retained
// limit bounds total retained capacity. Class and shard limits prevent one hot
// size from consuming all retained memory. Bucket slots define the physical
// depth available in each shard, while shard credit decides how much of that
// physical storage may be used after budget projection.
//
// Pressure is still enabled. Medium and high pressure contract retained targets
// but do not immediately disable retention, because this profile assumes the
// application prefers throughput continuity. Critical pressure preserves small
// hot classes and heavily restricts retained buffer capacity rather than
// dropping all reuse immediately.
//
// Ownership tracking remains disabled. The profile is designed for lightweight
// public Pool paths where allocation reduction matters more than strict
// checked-out buffer accounting.
//
// Expected use cases:
//
//   - high-throughput gateways;
//   - serialization-heavy services;
//   - compression or encoding paths;
//   - workloads where allocation reduction matters more than minimum RSS.
//
// Avoid this profile when the process has a small memory budget, when retained
// buffers may contain sensitive data that must be cleared, or when deterministic
// retained-memory contraction is more important than reuse hit rate.
func ThroughputPolicy() Policy {
	return Policy{
		Retention: RetentionPolicy{
			SoftRetainedBytes:         128 * MiB,
			HardRetainedBytes:         256 * MiB,
			MaxRetainedBuffers:        16_384,
			MaxRequestSize:            4 * MiB,
			MaxRetainedBufferCapacity: 4 * MiB,
			MaxClassRetainedBytes:     64 * MiB,
			MaxClassRetainedBuffers:   1024,
			MaxShardRetainedBytes:     8 * MiB,
			MaxShardRetainedBuffers:   64,
		},
		Classes: ClassPolicy{
			Sizes: ThroughputClassSizes(),
		},
		Shards: ShardPolicy{
			Selection:                 ShardSelectionModeRoundRobin,
			ShardsPerClass:            16,
			BucketSlotsPerShard:       64,
			AcquisitionFallbackShards: 1,
			ReturnFallbackShards:      1,
		},
		Admission: AdmissionPolicy{
			ZeroSizeRequests:    ZeroSizeRequestEmptyBuffer,
			ReturnedBuffers:     ReturnedBufferPolicyAdmit,
			OversizedReturn:     AdmissionActionDrop,
			UnsupportedClass:    AdmissionActionDrop,
			ClassMismatch:       AdmissionActionDrop,
			CreditExhausted:     AdmissionActionDrop,
			BucketFull:          AdmissionActionDrop,
			ZeroRetainedBuffers: false,
			ZeroDroppedBuffers:  false,
		},
		Pressure: PressurePolicy{
			Enabled: true,
			Medium: PressureLevelPolicy{
				RetentionScale:            8000,
				TrimScale:                 PolicyRatioOne,
				MaxRetainedBufferCapacity: 0,
				DropReturnedCapacityAbove: 0,
				DisableRetention:          false,
				PreserveSmallHotClasses:   true,
			},
			High: PressureLevelPolicy{
				RetentionScale:            6000,
				TrimScale:                 2 * PolicyRatioOne,
				MaxRetainedBufferCapacity: 2 * MiB,
				DropReturnedCapacityAbove: 2 * MiB,
				DisableRetention:          false,
				PreserveSmallHotClasses:   true,
			},
			Critical: PressureLevelPolicy{
				RetentionScale:            2500,
				TrimScale:                 4 * PolicyRatioOne,
				MaxRetainedBufferCapacity: 256 * KiB,
				DropReturnedCapacityAbove: 256 * KiB,
				DisableRetention:          false,
				PreserveSmallHotClasses:   true,
			},
		},
		Trim: TrimPolicy{
			Enabled:                   true,
			Interval:                  45 * time.Second,
			FullScanInterval:          5 * time.Minute,
			MaxBuffersPerCycle:        2048,
			MaxBytesPerCycle:          16 * MiB,
			MaxPoolsPerCycle:          128,
			MaxClassesPerPoolPerCycle: 8,
			MaxShardsPerClassPerCycle: 4,
			TrimOnPolicyShrink:        true,
			TrimOnPressure:            true,
			TrimOnClose:               true,
		},
		Ownership: OwnershipPolicy{
			Mode:                      OwnershipModeNone,
			TrackInUseBytes:           false,
			TrackInUseBuffers:         false,
			DetectDoubleRelease:       false,
			MaxReturnedCapacityGrowth: 3 * PolicyRatioOne,
		},
	}
}

// MemoryConstrainedPolicy returns a policy tuned for lower retained-memory
// footprint.
//
// This profile narrows class coverage, reduces shard and bucket counts, and
// trims more frequently. It is suitable when the caller wants bounded reuse but
// cannot afford a large retained buffer footprint.
//
// The profile intentionally drops both very small and larger buffers from the
// class profile. Very small buffers often do not justify retained-storage
// metadata and synchronization cost. Larger buffers can dominate resident memory
// quickly, so this profile caps retained capacity at 512 KiB and uses lower
// class/shard limits.
//
// The smaller shard and bucket shape is part of the memory behavior. It reduces
// physical slots that can hold retained buffers and keeps the shard-local current
// retained ledger small. Class and shard byte limits then provide the credit
// side of the same bound, so retained storage is limited by both slot count and
// retained capacity.
//
// Fallback probing is disabled. This keeps get and return operations predictable:
// one selected shard is checked, and missed reuse is allowed instead of scanning
// other shards. Trim runs more often and with smaller per-cycle limits so memory
// correction happens steadily without large cleanup bursts.
//
// Pressure handling is conservative. Medium and high pressure reduce retained
// capacity thresholds, while critical pressure disables new retention. This makes
// the profile appropriate for multi-tenant processes, small containers, CLI
// tools, or services where heap footprint is monitored tightly.
func MemoryConstrainedPolicy() Policy {
	return Policy{
		Retention: RetentionPolicy{
			SoftRetainedBytes:         8 * MiB,
			HardRetainedBytes:         16 * MiB,
			MaxRetainedBuffers:        1024,
			MaxRequestSize:            512 * KiB,
			MaxRetainedBufferCapacity: 512 * KiB,
			MaxClassRetainedBytes:     4 * MiB,
			MaxClassRetainedBuffers:   64,
			MaxShardRetainedBytes:     MiB,
			MaxShardRetainedBuffers:   16,
		},
		Classes: ClassPolicy{
			Sizes: MemoryConstrainedClassSizes(),
		},
		Shards: ShardPolicy{
			Selection:                 ShardSelectionModeRoundRobin,
			ShardsPerClass:            4,
			BucketSlotsPerShard:       16,
			AcquisitionFallbackShards: 0,
			ReturnFallbackShards:      0,
		},
		Admission: AdmissionPolicy{
			ZeroSizeRequests:    ZeroSizeRequestEmptyBuffer,
			ReturnedBuffers:     ReturnedBufferPolicyAdmit,
			OversizedReturn:     AdmissionActionDrop,
			UnsupportedClass:    AdmissionActionDrop,
			ClassMismatch:       AdmissionActionDrop,
			CreditExhausted:     AdmissionActionDrop,
			BucketFull:          AdmissionActionDrop,
			ZeroRetainedBuffers: false,
			ZeroDroppedBuffers:  false,
		},
		Pressure: PressurePolicy{
			Enabled: true,
			Medium: PressureLevelPolicy{
				RetentionScale:            6000,
				TrimScale:                 PolicyRatioOne,
				MaxRetainedBufferCapacity: 256 * KiB,
				DropReturnedCapacityAbove: 256 * KiB,
				DisableRetention:          false,
				PreserveSmallHotClasses:   true,
			},
			High: PressureLevelPolicy{
				RetentionScale:            3000,
				TrimScale:                 2 * PolicyRatioOne,
				MaxRetainedBufferCapacity: 128 * KiB,
				DropReturnedCapacityAbove: 128 * KiB,
				DisableRetention:          false,
				PreserveSmallHotClasses:   true,
			},
			Critical: PressureLevelPolicy{
				RetentionScale:            0,
				TrimScale:                 4 * PolicyRatioOne,
				MaxRetainedBufferCapacity: 32 * KiB,
				DropReturnedCapacityAbove: 32 * KiB,
				DisableRetention:          true,
				PreserveSmallHotClasses:   false,
			},
		},
		Trim: TrimPolicy{
			Enabled:                   true,
			Interval:                  15 * time.Second,
			FullScanInterval:          2 * time.Minute,
			MaxBuffersPerCycle:        512,
			MaxBytesPerCycle:          4 * MiB,
			MaxPoolsPerCycle:          64,
			MaxClassesPerPoolPerCycle: 8,
			MaxShardsPerClassPerCycle: 4,
			TrimOnPolicyShrink:        true,
			TrimOnPressure:            true,
			TrimOnClose:               true,
		},
		Ownership: OwnershipPolicy{
			Mode:                      OwnershipModeNone,
			TrackInUseBytes:           false,
			TrackInUseBuffers:         false,
			DetectDoubleRelease:       false,
			MaxReturnedCapacityGrowth: 2 * PolicyRatioOne,
		},
	}
}

// BurstyPolicy returns a policy tuned for workloads with short-lived spikes.
//
// This profile allows a larger hard retained limit than the balanced default,
// keeps a lower soft target, and relies on pressure/trim to contract retained
// memory after bursts. It is useful when temporary allocation spikes are common
// but long-term retained footprint must still be bounded.
//
// The soft/hard split is the important part of this profile. The soft target
// represents the desired steady state. The much larger hard limit lets the
// runtime retain enough buffers to absorb bursts without immediately discarding
// every large allocation. After the burst, pressure and trim settings provide the
// correction path back toward the soft target.
//
// Class coverage extends to 8 MiB because burst traffic often appears as larger
// request bodies, batched serialization payloads, compression work buffers, or
// temporary fan-in aggregation. Return fallback is disabled so bursty return
// paths remain cheap even when shards are full; acquisition fallback is allowed
// to improve reuse during the next burst.
//
// The larger hard limit should be read with the trim policy. The hard limit
// defines what may be retained during a burst; trim and pressure define how much
// work future controllers may spend per cycle to remove retained buffers after
// the burst. This avoids making burst handling depend on one unbounded cleanup
// operation.
//
// This profile should not be read as an adaptive controller. It is still static
// policy data. Future owner, pressure, and trim layers decide when to publish
// pressure levels and when to run trim work.
func BurstyPolicy() Policy {
	return Policy{
		Retention: RetentionPolicy{
			SoftRetainedBytes:         64 * MiB,
			HardRetainedBytes:         512 * MiB,
			MaxRetainedBuffers:        16_384,
			MaxRequestSize:            8 * MiB,
			MaxRetainedBufferCapacity: 8 * MiB,
			MaxClassRetainedBytes:     128 * MiB,
			MaxClassRetainedBuffers:   1024,
			MaxShardRetainedBytes:     16 * MiB,
			MaxShardRetainedBuffers:   64,
		},
		Classes: ClassPolicy{
			Sizes: BurstyClassSizes(),
		},
		Shards: ShardPolicy{
			Selection:                 ShardSelectionModeRoundRobin,
			ShardsPerClass:            16,
			BucketSlotsPerShard:       64,
			AcquisitionFallbackShards: 1,
			ReturnFallbackShards:      0,
		},
		Admission: AdmissionPolicy{
			ZeroSizeRequests:    ZeroSizeRequestEmptyBuffer,
			ReturnedBuffers:     ReturnedBufferPolicyAdmit,
			OversizedReturn:     AdmissionActionDrop,
			UnsupportedClass:    AdmissionActionDrop,
			ClassMismatch:       AdmissionActionDrop,
			CreditExhausted:     AdmissionActionDrop,
			BucketFull:          AdmissionActionDrop,
			ZeroRetainedBuffers: false,
			ZeroDroppedBuffers:  false,
		},
		Pressure: PressurePolicy{
			Enabled: true,
			Medium: PressureLevelPolicy{
				RetentionScale:            7000,
				TrimScale:                 PolicyRatioOne,
				MaxRetainedBufferCapacity: 4 * MiB,
				DropReturnedCapacityAbove: 4 * MiB,
				DisableRetention:          false,
				PreserveSmallHotClasses:   true,
			},
			High: PressureLevelPolicy{
				RetentionScale:            4000,
				TrimScale:                 3 * PolicyRatioOne,
				MaxRetainedBufferCapacity: MiB,
				DropReturnedCapacityAbove: MiB,
				DisableRetention:          false,
				PreserveSmallHotClasses:   true,
			},
			Critical: PressureLevelPolicy{
				RetentionScale:            0,
				TrimScale:                 5 * PolicyRatioOne,
				MaxRetainedBufferCapacity: 256 * KiB,
				DropReturnedCapacityAbove: 256 * KiB,
				DisableRetention:          true,
				PreserveSmallHotClasses:   false,
			},
		},
		Trim: TrimPolicy{
			Enabled:                   true,
			Interval:                  20 * time.Second,
			FullScanInterval:          3 * time.Minute,
			MaxBuffersPerCycle:        4096,
			MaxBytesPerCycle:          32 * MiB,
			MaxPoolsPerCycle:          128,
			MaxClassesPerPoolPerCycle: 12,
			MaxShardsPerClassPerCycle: 8,
			TrimOnPolicyShrink:        true,
			TrimOnPressure:            true,
			TrimOnClose:               true,
		},
		Ownership: OwnershipPolicy{
			Mode:                      OwnershipModeNone,
			TrackInUseBytes:           false,
			TrackInUseBuffers:         false,
			DetectDoubleRelease:       false,
			MaxReturnedCapacityGrowth: 4 * PolicyRatioOne,
		},
	}
}

// StrictBoundedPolicy returns a policy tuned for deterministic retained-memory
// bounds.
//
// This profile keeps class coverage narrower, rejects zero-size requests, uses
// smaller buckets, and enables ownership accounting. It is appropriate for
// systems where predictable memory behavior is more important than maximum reuse
// hit rate.
//
// Strict bounded behavior means two things here. First, physical storage is kept
// shallow: fewer retained slots per shard and no fallback probing. Second,
// admission is less tolerant: unsupported classes, oversized returns, and class
// mismatches are configured as errors rather than silent best-effort drops.
//
// The profile still uses ordinary shard-local storage and credit. "Strict" here
// means the configured envelope is narrower and invalid inputs are surfaced more
// aggressively; it does not mean classState adds a global mutex or that clear
// becomes a transactional class-wide barrier.
//
// Ownership mode is Accounting rather than Strict. The profile records in-use
// byte and buffer facts but does not require every public return path to perform
// strict lease validation. That makes it a middle ground between the lightweight
// default profile and the stricter SecurePolicy.
//
// Use this profile when predictable retained-memory behavior, operational
// diagnostics, and explicit invalid-return reporting are more valuable than
// squeezing out every possible allocation reuse.
func StrictBoundedPolicy() Policy {
	return Policy{
		Retention: RetentionPolicy{
			SoftRetainedBytes:         16 * MiB,
			HardRetainedBytes:         32 * MiB,
			MaxRetainedBuffers:        2048,
			MaxRequestSize:            512 * KiB,
			MaxRetainedBufferCapacity: 512 * KiB,
			MaxClassRetainedBytes:     8 * MiB,
			MaxClassRetainedBuffers:   128,
			MaxShardRetainedBytes:     MiB,
			MaxShardRetainedBuffers:   16,
		},
		Classes: ClassPolicy{
			Sizes: StrictBoundedClassSizes(),
		},
		Shards: ShardPolicy{
			Selection:                 ShardSelectionModeRoundRobin,
			ShardsPerClass:            8,
			BucketSlotsPerShard:       16,
			AcquisitionFallbackShards: 0,
			ReturnFallbackShards:      0,
		},
		Admission: AdmissionPolicy{
			ZeroSizeRequests:    ZeroSizeRequestReject,
			ReturnedBuffers:     ReturnedBufferPolicyAdmit,
			OversizedReturn:     AdmissionActionError,
			UnsupportedClass:    AdmissionActionError,
			ClassMismatch:       AdmissionActionError,
			CreditExhausted:     AdmissionActionDrop,
			BucketFull:          AdmissionActionDrop,
			ZeroRetainedBuffers: false,
			ZeroDroppedBuffers:  false,
		},
		Pressure: PressurePolicy{
			Enabled: true,
			Medium: PressureLevelPolicy{
				RetentionScale:            5000,
				TrimScale:                 PolicyRatioOne,
				MaxRetainedBufferCapacity: 256 * KiB,
				DropReturnedCapacityAbove: 256 * KiB,
				DisableRetention:          false,
				PreserveSmallHotClasses:   true,
			},
			High: PressureLevelPolicy{
				RetentionScale:            2500,
				TrimScale:                 2 * PolicyRatioOne,
				MaxRetainedBufferCapacity: 128 * KiB,
				DropReturnedCapacityAbove: 128 * KiB,
				DisableRetention:          false,
				PreserveSmallHotClasses:   true,
			},
			Critical: PressureLevelPolicy{
				RetentionScale:            0,
				TrimScale:                 4 * PolicyRatioOne,
				MaxRetainedBufferCapacity: 32 * KiB,
				DropReturnedCapacityAbove: 32 * KiB,
				DisableRetention:          true,
				PreserveSmallHotClasses:   false,
			},
		},
		Trim: TrimPolicy{
			Enabled:                   true,
			Interval:                  10 * time.Second,
			FullScanInterval:          time.Minute,
			MaxBuffersPerCycle:        1024,
			MaxBytesPerCycle:          8 * MiB,
			MaxPoolsPerCycle:          64,
			MaxClassesPerPoolPerCycle: 8,
			MaxShardsPerClassPerCycle: 8,
			TrimOnPolicyShrink:        true,
			TrimOnPressure:            true,
			TrimOnClose:               true,
		},
		Ownership: OwnershipPolicy{
			Mode:                      OwnershipModeAccounting,
			TrackInUseBytes:           true,
			TrackInUseBuffers:         true,
			DetectDoubleRelease:       false,
			MaxReturnedCapacityGrowth: PolicyRatioOne,
		},
	}
}

// SecurePolicy returns a policy tuned for stricter data hygiene and ownership
// validation.
//
// This profile zeros retained and dropped buffers, rejects zero-size requests,
// enables strict ownership checks, and keeps capacity growth tightly bounded.
// It intentionally trades hot-path cost for stronger safety properties.
//
// Zeroing retained buffers reduces the risk of reusing backing arrays that still
// contain prior contents. Zeroing dropped buffers extends that hygiene to
// rejected returns, which may matter when a caller returns buffers carrying
// sensitive payloads. Both operations add CPU cost proportional to buffer
// capacity, so this profile keeps class coverage and shard storage conservative.
//
// The retained set is intentionally modest. Zeroing cost scales with capacity,
// not with logical length, because retained-buffer accounting and admission are
// capacity based. Keeping the upper class size at 1 MiB avoids making ordinary
// secure returns pay large scrub costs by default.
//
// Strict ownership means future ownership/lease layers should validate origin,
// capacity growth, double release, and in-use accounting before accepting
// returned buffers. Admission actions are configured as errors for invalid
// classes and mismatches so unsafe returns can be surfaced instead of silently
// ignored.
//
// This profile is a policy preset, not a complete security boundary by itself.
// It prepares the runtime policy for stricter handling; callers still need the
// future public Pool and ownership layers to enforce end-to-end ownership
// semantics.
func SecurePolicy() Policy {
	return Policy{
		Retention: RetentionPolicy{
			SoftRetainedBytes:         16 * MiB,
			HardRetainedBytes:         64 * MiB,
			MaxRetainedBuffers:        2048,
			MaxRequestSize:            MiB,
			MaxRetainedBufferCapacity: MiB,
			MaxClassRetainedBytes:     8 * MiB,
			MaxClassRetainedBuffers:   128,
			MaxShardRetainedBytes:     2 * MiB,
			MaxShardRetainedBuffers:   16,
		},
		Classes: ClassPolicy{
			Sizes: SecureClassSizes(),
		},
		Shards: ShardPolicy{
			Selection:                 ShardSelectionModeRoundRobin,
			ShardsPerClass:            8,
			BucketSlotsPerShard:       16,
			AcquisitionFallbackShards: 0,
			ReturnFallbackShards:      0,
		},
		Admission: AdmissionPolicy{
			ZeroSizeRequests:    ZeroSizeRequestReject,
			ReturnedBuffers:     ReturnedBufferPolicyAdmit,
			OversizedReturn:     AdmissionActionError,
			UnsupportedClass:    AdmissionActionError,
			ClassMismatch:       AdmissionActionError,
			CreditExhausted:     AdmissionActionDrop,
			BucketFull:          AdmissionActionDrop,
			ZeroRetainedBuffers: true,
			ZeroDroppedBuffers:  true,
		},
		Pressure: PressurePolicy{
			Enabled: true,
			Medium: PressureLevelPolicy{
				RetentionScale:            5000,
				TrimScale:                 PolicyRatioOne,
				MaxRetainedBufferCapacity: 512 * KiB,
				DropReturnedCapacityAbove: 512 * KiB,
				DisableRetention:          false,
				PreserveSmallHotClasses:   true,
			},
			High: PressureLevelPolicy{
				RetentionScale:            2500,
				TrimScale:                 3 * PolicyRatioOne,
				MaxRetainedBufferCapacity: 128 * KiB,
				DropReturnedCapacityAbove: 128 * KiB,
				DisableRetention:          false,
				PreserveSmallHotClasses:   true,
			},
			Critical: PressureLevelPolicy{
				RetentionScale:            0,
				TrimScale:                 5 * PolicyRatioOne,
				MaxRetainedBufferCapacity: 32 * KiB,
				DropReturnedCapacityAbove: 32 * KiB,
				DisableRetention:          true,
				PreserveSmallHotClasses:   false,
			},
		},
		Trim: TrimPolicy{
			Enabled:                   true,
			Interval:                  10 * time.Second,
			FullScanInterval:          time.Minute,
			MaxBuffersPerCycle:        2048,
			MaxBytesPerCycle:          16 * MiB,
			MaxPoolsPerCycle:          64,
			MaxClassesPerPoolPerCycle: 8,
			MaxShardsPerClassPerCycle: 8,
			TrimOnPolicyShrink:        true,
			TrimOnPressure:            true,
			TrimOnClose:               true,
		},
		Ownership: OwnershipPolicy{
			Mode:                      OwnershipModeStrict,
			TrackInUseBytes:           true,
			TrackInUseBuffers:         true,
			DetectDoubleRelease:       true,
			MaxReturnedCapacityGrowth: PolicyRatioOne,
		},
	}
}

// ThroughputClassSizes returns the class-size profile used by ThroughputPolicy.
//
// The profile extends the default upper bound to 4 MiB. This improves reuse for
// larger transient buffers while still excluding very large allocations from
// retained storage.
//
// Class sizes are powers of two so request routing remains predictable: requests
// use ceil lookup and returned capacities use floor lookup. The returned slice is
// fresh and caller-owned; mutating it does not affect later ThroughputPolicy or
// ThroughputClassSizes calls.
//
// The class table built from this slice assigns stable ClassID values in slice
// order. Keeping the sizes explicit makes review easier and avoids hiding a
// min/max generator behind the profile.
func ThroughputClassSizes() []ClassSize {
	return []ClassSize{
		ClassSizeFromBytes(256),
		ClassSizeFromBytes(512),
		ClassSizeFromSize(KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
		ClassSizeFromSize(8 * KiB),
		ClassSizeFromSize(16 * KiB),
		ClassSizeFromSize(32 * KiB),
		ClassSizeFromSize(64 * KiB),
		ClassSizeFromSize(128 * KiB),
		ClassSizeFromSize(256 * KiB),
		ClassSizeFromSize(512 * KiB),
		ClassSizeFromSize(MiB),
		ClassSizeFromSize(2 * MiB),
		ClassSizeFromSize(4 * MiB),
	}
}

// MemoryConstrainedClassSizes returns the class-size profile used by
// MemoryConstrainedPolicy.
//
// The profile starts at 512 B and ends at 512 KiB to avoid retaining very small
// or very large buffers by default.
//
// Starting at 512 B avoids spending retained-storage slots on tiny buffers where
// allocation cost is often low relative to pool bookkeeping. Ending at 512 KiB
// prevents a small number of large returned buffers from dominating retained
// memory. The returned slice is fresh and caller-owned.
//
// Requests below the first class are still routable to that first class through
// ceil lookup if the request is otherwise supported by policy. Returned
// capacities below the first class are not retained by this class profile
// because returned-capacity routing uses floor lookup.
func MemoryConstrainedClassSizes() []ClassSize {
	return []ClassSize{
		ClassSizeFromBytes(512),
		ClassSizeFromSize(KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
		ClassSizeFromSize(8 * KiB),
		ClassSizeFromSize(16 * KiB),
		ClassSizeFromSize(32 * KiB),
		ClassSizeFromSize(64 * KiB),
		ClassSizeFromSize(128 * KiB),
		ClassSizeFromSize(256 * KiB),
		ClassSizeFromSize(512 * KiB),
	}
}

// BurstyClassSizes returns the class-size profile used by BurstyPolicy.
//
// The profile covers larger buffers than the default because bursts often appear
// as temporary growth in request bodies, serialization payloads, or compression
// buffers. Retention remains bounded by BurstyPolicy retention limits.
//
// The profile starts at 512 B because burst workloads usually care more about
// medium and large transient allocations than tiny objects. It ends at 8 MiB so
// the policy can classify larger burst buffers without requiring a caller to
// build a custom class table. The returned slice is fresh and caller-owned.
//
// Larger classes are still subject to BurstyPolicy's class and shard retained
// limits. Defining a class size makes a buffer eligible for routing; it does not
// guarantee storage if budget, credit, pressure, or admission rejects it.
func BurstyClassSizes() []ClassSize {
	return []ClassSize{
		ClassSizeFromBytes(512),
		ClassSizeFromSize(KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
		ClassSizeFromSize(8 * KiB),
		ClassSizeFromSize(16 * KiB),
		ClassSizeFromSize(32 * KiB),
		ClassSizeFromSize(64 * KiB),
		ClassSizeFromSize(128 * KiB),
		ClassSizeFromSize(256 * KiB),
		ClassSizeFromSize(512 * KiB),
		ClassSizeFromSize(MiB),
		ClassSizeFromSize(2 * MiB),
		ClassSizeFromSize(4 * MiB),
		ClassSizeFromSize(8 * MiB),
	}
}

// StrictBoundedClassSizes returns the class-size profile used by
// StrictBoundedPolicy.
//
// The profile starts at 1 KiB and ends at 512 KiB. This keeps retained storage
// focused on common medium-size buffers and avoids retaining very small objects
// where pool metadata and synchronization overhead can outweigh reuse benefits.
//
// The narrower range is part of the strictness: unsupported smaller or larger
// capacities are forced through admission decisions instead of being retained by
// default. The returned slice is fresh and caller-owned.
//
// The explicit range also makes strict policy behavior easier to audit: every
// retained capacity must map to one of these classes and then pass class-level
// admission, shard credit, and bucket storage checks.
func StrictBoundedClassSizes() []ClassSize {
	return []ClassSize{
		ClassSizeFromSize(KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
		ClassSizeFromSize(8 * KiB),
		ClassSizeFromSize(16 * KiB),
		ClassSizeFromSize(32 * KiB),
		ClassSizeFromSize(64 * KiB),
		ClassSizeFromSize(128 * KiB),
		ClassSizeFromSize(256 * KiB),
		ClassSizeFromSize(512 * KiB),
	}
}

// SecureClassSizes returns the class-size profile used by SecurePolicy.
//
// The profile starts at 512 B and ends at 1 MiB. It keeps broad enough coverage
// for practical reuse while avoiding very large retained buffers that would be
// expensive to zero and risky to keep around.
//
// The upper bound reflects the secure profile's zeroing cost. Larger buffers are
// more expensive to scrub on return or drop, so they should be allocated
// directly or handled by caller-specific policy instead of being part of the
// default secure retained set. The returned slice is fresh and caller-owned.
//
// As with the other profile class lists, this slice is only the classification
// shape. Future ownership and admission layers still decide whether a returned
// buffer may be accepted, whether it must be zeroed, and whether invalid returns
// become drops or errors.
func SecureClassSizes() []ClassSize {
	return []ClassSize{
		ClassSizeFromBytes(512),
		ClassSizeFromSize(KiB),
		ClassSizeFromSize(2 * KiB),
		ClassSizeFromSize(4 * KiB),
		ClassSizeFromSize(8 * KiB),
		ClassSizeFromSize(16 * KiB),
		ClassSizeFromSize(32 * KiB),
		ClassSizeFromSize(64 * KiB),
		ClassSizeFromSize(128 * KiB),
		ClassSizeFromSize(256 * KiB),
		ClassSizeFromSize(512 * KiB),
		ClassSizeFromSize(MiB),
	}
}
