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
	"runtime"
	"time"

	"arcoris.dev/bufferpool/internal/mathx"
)

const (
	// DefaultPolicySoftRetainedBytes is the preferred retained-memory target for
	// the default policy.
	//
	// The soft target is intentionally lower than the hard target. It gives
	// future controllers, pressure handling, and trim logic room to absorb short
	// bursts without immediately treating the runtime as over its safety limit.
	DefaultPolicySoftRetainedBytes Size = 32 * MiB

	// DefaultPolicyHardRetainedBytes is the hard retained-memory limit for the
	// default policy owner.
	//
	// This value is deliberately conservative. It is large enough to make reuse
	// useful for allocation-heavy workloads, but small enough to avoid turning
	// the default profile into an unbounded memory sink.
	DefaultPolicyHardRetainedBytes Size = 64 * MiB

	// DefaultPolicyMaxRetainedBuffers limits the total retained buffer count for
	// a default policy owner.
	//
	// Byte budgets are the primary safety mechanism. This count limit exists as a
	// second dimension so very small buffers cannot create unbounded object-count
	// retention even when retained bytes stay below the byte target.
	DefaultPolicyMaxRetainedBuffers uint64 = 4096

	// DefaultPolicyMaxRequestSize is the largest request size served by the
	// default class profile.
	//
	// Larger requests should be allocated directly by the future Pool layer
	// rather than routed through retained storage. This keeps default retention
	// focused on reusable common-size buffers instead of rare large allocations.
	DefaultPolicyMaxRequestSize Size = MiB

	// DefaultPolicyMaxRetainedBufferCapacity is the largest returned buffer
	// capacity admitted by the default policy.
	//
	// Returned buffers above this value should be dropped by admission. This
	// prevents accidental retention of unusually large backing arrays after
	// bursty or attacker-controlled inputs.
	DefaultPolicyMaxRetainedBufferCapacity Size = MiB

	// DefaultPolicyMaxClassRetainedBytes limits retained backing capacity assigned
	// to one size class under the default policy.
	//
	// This prevents one hot class from consuming the entire owner budget before
	// adaptive redistribution exists.
	DefaultPolicyMaxClassRetainedBytes Size = 16 * MiB

	// DefaultPolicyMaxShardRetainedBytes limits retained backing capacity assigned
	// to one class shard under the default policy.
	//
	// This keeps the largest default class from allowing one shard to retain all
	// of its physical bucket slots as 1 MiB buffers.
	DefaultPolicyMaxShardRetainedBytes Size = 4 * MiB

	// DefaultPolicyMaxShardRetainedBuffers limits retained buffer count assigned
	// to one class shard under the default policy.
	//
	// The value matches DefaultPolicyBucketSlotsPerShard. Shard credit may still
	// be lower than this physical storage limit after budget distribution.
	DefaultPolicyMaxShardRetainedBuffers uint64 = 32
)

const (
	// DefaultPolicyMinShardsPerClass is the lower bound for automatic default
	// class-local lock striping.
	DefaultPolicyMinShardsPerClass = 1

	// DefaultPolicyMaxShardsPerClass is the upper bound for automatic default
	// class-local lock striping.
	DefaultPolicyMaxShardsPerClass = 32

	// DefaultPolicyBucketSlotsPerShard defines the default physical retained
	// buffer slots per shard.
	//
	// This is a storage cap, not a budget. The runtime still relies on class
	// budget and shard credit to decide how much of this storage may actually be
	// used.
	DefaultPolicyBucketSlotsPerShard = 32

	// DefaultPolicyAcquisitionFallbackShards enables one bounded get-side
	// fallback probe after the primary selected shard misses.
	DefaultPolicyAcquisitionFallbackShards = 1

	// DefaultPolicyReturnFallbackShards disables default put-side fallback
	// probing.
	//
	// Returning a buffer should remain cheap and bounded by default. If the
	// selected shard cannot retain the buffer, the default behavior is to drop it
	// rather than search other shards.
	DefaultPolicyReturnFallbackShards = 0
)

// DefaultPolicyShardsPerClass returns the default lock-striping width for each
// size class.
//
// The value follows the current GOMAXPROCS, rounded up to the next power of two
// and bounded to keep metadata growth predictable. Shards remain data-plane
// contention units; this resolver does not imply any control-plane partitioning.
func DefaultPolicyShardsPerClass() int {
	return defaultPolicyShardsPerClassForGOMAXPROCS(runtime.GOMAXPROCS(0))
}

// defaultPolicyShardsPerClassForGOMAXPROCS resolves a processor count into the
// default shard count used by DefaultShardPolicy.
//
// The helper is private so tests can cover boundary behavior without changing
// the process-wide GOMAXPROCS value. Inputs below one are treated as one because
// the public resolver receives runtime.GOMAXPROCS(0), which should already be
// positive, and negative test inputs should not reach mathx.NextPowerOfTwo.
func defaultPolicyShardsPerClassForGOMAXPROCS(procs int) int {
	if procs < DefaultPolicyMinShardsPerClass {
		procs = DefaultPolicyMinShardsPerClass
	}

	shards := int(mathx.NextPowerOfTwo(uint64(procs)))
	return mathx.Clamp(shards, DefaultPolicyMinShardsPerClass, DefaultPolicyMaxShardsPerClass)
}

// DefaultPolicyMaxClassRetainedBuffers returns the default retained-buffer
// count limit assigned to one size class.
//
// The value tracks the resolved default shard shape so DefaultPolicy remains
// valid even when GOMAXPROCS is small.
func DefaultPolicyMaxClassRetainedBuffers() uint64 {
	return uint64(DefaultPolicyShardsPerClass()) * DefaultPolicyMaxShardRetainedBuffers
}

const (
	// DefaultPolicyMediumPressureRetentionScale retains 75% of the normal target
	// under medium pressure.
	DefaultPolicyMediumPressureRetentionScale PolicyRatio = 7500

	// DefaultPolicyMediumPressureTrimScale keeps ordinary trim work unchanged
	// under medium pressure.
	DefaultPolicyMediumPressureTrimScale PolicyRatio = PolicyRatioOne

	// DefaultPolicyHighPressureRetentionScale retains 50% of the normal target
	// under high pressure.
	DefaultPolicyHighPressureRetentionScale PolicyRatio = 5000

	// DefaultPolicyHighPressureTrimScale doubles bounded trim work under high
	// pressure.
	DefaultPolicyHighPressureTrimScale PolicyRatio = 2 * PolicyRatioOne

	// DefaultPolicyCriticalPressureRetentionScale contracts retained memory to
	// zero under critical pressure.
	DefaultPolicyCriticalPressureRetentionScale PolicyRatio = 0

	// DefaultPolicyCriticalPressureTrimScale quadruples bounded trim work under
	// critical pressure.
	DefaultPolicyCriticalPressureTrimScale PolicyRatio = 4 * PolicyRatioOne

	// DefaultPolicyHighPressureMaxRetainedBufferCapacity caps retained buffer
	// capacity under high pressure.
	DefaultPolicyHighPressureMaxRetainedBufferCapacity Size = 512 * KiB

	// DefaultPolicyHighPressureDropReturnedCapacityAbove drops returned buffers
	// above this capacity under high pressure.
	DefaultPolicyHighPressureDropReturnedCapacityAbove Size = 512 * KiB

	// DefaultPolicyCriticalPressureMaxRetainedBufferCapacity caps retained buffer
	// capacity under critical pressure.
	//
	// Critical pressure also disables retention in the default level policy, so
	// this cap is mainly diagnostic and useful if a future profile preserves a
	// small hot class under critical pressure.
	DefaultPolicyCriticalPressureMaxRetainedBufferCapacity Size = 64 * KiB

	// DefaultPolicyCriticalPressureDropReturnedCapacityAbove drops returned
	// buffers above this capacity under critical pressure.
	DefaultPolicyCriticalPressureDropReturnedCapacityAbove Size = 64 * KiB
)

const (
	// DefaultPolicyTrimInterval is the ordinary corrective trim cadence.
	//
	// Trim is cold-path correction. The default interval is intentionally not
	// aggressive because hot-path admission and shard credit should perform most
	// retention limiting.
	DefaultPolicyTrimInterval time.Duration = 30 * time.Second

	// DefaultPolicyFullScanInterval is the broad diagnostic/control scan cadence.
	//
	// Full scans are expected to be less frequent than ordinary trim cycles.
	DefaultPolicyFullScanInterval time.Duration = 5 * time.Minute

	// DefaultPolicyMaxTrimBuffersPerCycle limits the number of buffers removed in
	// one default trim cycle.
	DefaultPolicyMaxTrimBuffersPerCycle uint64 = 1024

	// DefaultPolicyMaxTrimBytesPerCycle limits retained backing capacity removed
	// in one default trim cycle.
	DefaultPolicyMaxTrimBytesPerCycle Size = 8 * MiB

	// DefaultPolicyMaxTrimPoolsPerCycle limits how many pools a controller may
	// visit in one trim cycle.
	DefaultPolicyMaxTrimPoolsPerCycle = 64

	// DefaultPolicyMaxTrimClassesPerPoolPerCycle limits class fan-out during one
	// trim cycle for one pool.
	DefaultPolicyMaxTrimClassesPerPoolPerCycle = 8

	// DefaultPolicyMaxTrimShardsPerClassPerCycle limits shard fan-out during one
	// trim cycle for one class.
	DefaultPolicyMaxTrimShardsPerClassPerCycle = 4
)

const (
	// DefaultPolicyMaxReturnedCapacityGrowth is the default ownership-aware
	// returned-capacity growth limit.
	//
	// A value of 2.0 means a buffer may grow up to twice its origin class size
	// before strict ownership-aware admission should reject or drop it. The
	// default ownership mode is disabled, so this value becomes relevant only
	// when a future config/profile enables ownership accounting.
	DefaultPolicyMaxReturnedCapacityGrowth PolicyRatio = 2 * PolicyRatioOne
)

// DefaultPolicy returns the canonical default runtime policy.
//
// The default policy is intentionally production-oriented but conservative:
//
//   - bounded retained memory;
//   - explicit size classes;
//   - shard-local hot-path synchronization;
//   - drop-on-return admission for unsuitable buffers;
//   - pressure-aware contraction behavior;
//   - bounded trim work;
//   - ownership tracking disabled by default.
//
// The returned Policy owns its class-size slice. Callers may modify the returned
// value without mutating package-level defaults.
func DefaultPolicy() Policy {
	return Policy{
		Retention: DefaultRetentionPolicy(),
		Classes:   DefaultClassPolicy(),
		Shards:    DefaultShardPolicy(),
		Admission: DefaultAdmissionPolicy(),
		Pressure:  DefaultPressurePolicy(),
		Trim:      DefaultTrimPolicy(),
		Ownership: DefaultOwnershipPolicy(),
	}
}

// DefaultRetentionPolicy returns retained-memory limits for the canonical
// default policy.
//
// These values define policy-level bounds. They do not directly allocate memory
// and do not imply that every class or shard receives its maximum. Runtime owners
// still need to distribute retained budget into class budgets and shard credits.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		SoftRetainedBytes:         DefaultPolicySoftRetainedBytes,
		HardRetainedBytes:         DefaultPolicyHardRetainedBytes,
		MaxRetainedBuffers:        DefaultPolicyMaxRetainedBuffers,
		MaxRequestSize:            DefaultPolicyMaxRequestSize,
		MaxRetainedBufferCapacity: DefaultPolicyMaxRetainedBufferCapacity,
		MaxClassRetainedBytes:     DefaultPolicyMaxClassRetainedBytes,
		MaxClassRetainedBuffers:   DefaultPolicyMaxClassRetainedBuffers(),
		MaxShardRetainedBytes:     DefaultPolicyMaxShardRetainedBytes,
		MaxShardRetainedBuffers:   DefaultPolicyMaxShardRetainedBuffers,
	}
}

// DefaultClassPolicy returns the default size-class profile.
//
// The profile is explicit and power-of-two aligned. It starts at 256 B to cover
// small serialization and protocol buffers, and it ends at 1 MiB to avoid
// retaining rare large buffers by default.
func DefaultClassPolicy() ClassPolicy {
	return ClassPolicy{
		Sizes: DefaultClassSizes(),
	}
}

// DefaultClassSizes returns a new slice containing the default size-class
// profile.
//
// The function returns a fresh slice on every call. This prevents caller-side
// mutation from corrupting defaults used by other policy values.
func DefaultClassSizes() []ClassSize {
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
	}
}

// DefaultShardPolicy returns the default class-local sharding and bucket-storage
// policy.
//
// Processor-inspired selection is the default because it avoids a single
// class-wide round-robin sequence while staying deterministic and independent of
// private runtime-local APIs. Acquisition fallback is bounded to one extra
// shard; return fallback remains disabled to keep Put predictable.
func DefaultShardPolicy() ShardPolicy {
	return ShardPolicy{
		Selection:                 ShardSelectionModeProcessorInspired,
		ShardsPerClass:            DefaultPolicyShardsPerClass(),
		BucketSlotsPerShard:       DefaultPolicyBucketSlotsPerShard,
		AcquisitionFallbackShards: DefaultPolicyAcquisitionFallbackShards,
		ReturnFallbackShards:      DefaultPolicyReturnFallbackShards,
	}
}

// DefaultAdmissionPolicy returns the default return-path admission behavior.
//
// The default policy admits ordinary returned buffers only when all runtime
// checks allow retention. Buffers that fail admission are dropped rather than
// reported as errors because the future convenient public Put path should be
// safe to call for best-effort reuse.
func DefaultAdmissionPolicy() AdmissionPolicy {
	return AdmissionPolicy{
		ZeroSizeRequests:    ZeroSizeRequestEmptyBuffer,
		ReturnedBuffers:     ReturnedBufferPolicyAdmit,
		OversizedReturn:     AdmissionActionDrop,
		UnsupportedClass:    AdmissionActionDrop,
		ClassMismatch:       AdmissionActionDrop,
		CreditExhausted:     AdmissionActionDrop,
		BucketFull:          AdmissionActionDrop,
		ZeroRetainedBuffers: false,
		ZeroDroppedBuffers:  false,
	}
}

// DefaultPressurePolicy returns the default pressure-contraction policy.
//
// Pressure handling is enabled in the default policy, but the current pressure
// level remains runtime state owned by future Pool/Partition/Group components.
// The normal level is intentionally not represented here because no contraction
// is needed under normal pressure.
func DefaultPressurePolicy() PressurePolicy {
	return PressurePolicy{
		Enabled:  true,
		Medium:   DefaultMediumPressurePolicy(),
		High:     DefaultHighPressurePolicy(),
		Critical: DefaultCriticalPressurePolicy(),
	}
}

// DefaultMediumPressurePolicy returns the default medium-pressure behavior.
//
// Medium pressure contracts retained targets moderately without disabling
// retention or changing retained buffer capacity limits.
func DefaultMediumPressurePolicy() PressureLevelPolicy {
	return PressureLevelPolicy{
		RetentionScale:            DefaultPolicyMediumPressureRetentionScale,
		TrimScale:                 DefaultPolicyMediumPressureTrimScale,
		MaxRetainedBufferCapacity: 0,
		DropReturnedCapacityAbove: 0,
		DisableRetention:          false,
		PreserveSmallHotClasses:   true,
	}
}

// DefaultHighPressurePolicy returns the default high-pressure behavior.
//
// High pressure halves the retained target, increases trim work, and stops
// retaining returned buffers above 512 KiB.
func DefaultHighPressurePolicy() PressureLevelPolicy {
	return PressureLevelPolicy{
		RetentionScale:            DefaultPolicyHighPressureRetentionScale,
		TrimScale:                 DefaultPolicyHighPressureTrimScale,
		MaxRetainedBufferCapacity: DefaultPolicyHighPressureMaxRetainedBufferCapacity,
		DropReturnedCapacityAbove: DefaultPolicyHighPressureDropReturnedCapacityAbove,
		DisableRetention:          false,
		PreserveSmallHotClasses:   true,
	}
}

// DefaultCriticalPressurePolicy returns the default critical-pressure behavior.
//
// Critical pressure disables new retention by default and allows substantially
// more bounded trim work per cycle. If a future profile enables preservation of
// small hot classes under critical pressure, the capacity cap still prevents
// large returned buffers from being retained.
func DefaultCriticalPressurePolicy() PressureLevelPolicy {
	return PressureLevelPolicy{
		RetentionScale:            DefaultPolicyCriticalPressureRetentionScale,
		TrimScale:                 DefaultPolicyCriticalPressureTrimScale,
		MaxRetainedBufferCapacity: DefaultPolicyCriticalPressureMaxRetainedBufferCapacity,
		DropReturnedCapacityAbove: DefaultPolicyCriticalPressureDropReturnedCapacityAbove,
		DisableRetention:          true,
		PreserveSmallHotClasses:   false,
	}
}

// DefaultTrimPolicy returns the default bounded trim policy.
//
// Trim is enabled by default because retained-memory contraction must have a
// cold-path correction mechanism. Admission prevents new unsuitable retention;
// trim removes retained buffers that are already present after budget shrink,
// pressure, close, or workload changes.
func DefaultTrimPolicy() TrimPolicy {
	return TrimPolicy{
		Enabled:                   true,
		Interval:                  DefaultPolicyTrimInterval,
		FullScanInterval:          DefaultPolicyFullScanInterval,
		MaxBuffersPerCycle:        DefaultPolicyMaxTrimBuffersPerCycle,
		MaxBytesPerCycle:          DefaultPolicyMaxTrimBytesPerCycle,
		MaxPoolsPerCycle:          DefaultPolicyMaxTrimPoolsPerCycle,
		MaxClassesPerPoolPerCycle: DefaultPolicyMaxTrimClassesPerPoolPerCycle,
		MaxShardsPerClassPerCycle: DefaultPolicyMaxTrimShardsPerClassPerCycle,
		TrimOnPolicyShrink:        true,
		TrimOnPressure:            true,
		TrimOnClose:               true,
	}
}

// DefaultOwnershipPolicy returns the default ownership/accounting policy.
//
// Explicit ownership tracking is disabled by default because it changes public
// API shape, memory accounting cost, and release validation semantics. Future
// strict profiles can enable ownership without changing the default lightweight
// data path.
func DefaultOwnershipPolicy() OwnershipPolicy {
	return OwnershipPolicy{
		Mode:                      OwnershipModeNone,
		TrackInUseBytes:           false,
		TrackInUseBuffers:         false,
		DetectDoubleRelease:       false,
		MaxReturnedCapacityGrowth: DefaultPolicyMaxReturnedCapacityGrowth,
	}
}
