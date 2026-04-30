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

// Policy describes memory-retention behavior for runtime owners.
//
// Policy is a value model, not construction options. It defines behavior that
// owner configuration can embed or compose before runtime state is built.
//
// Responsibility boundary:
//
//   - policy.go defines policy values and small diagnostic helpers;
//   - defaulting, validation, named profiles, and owner construction are separate
//     responsibilities;
//   - static runtime components enforce normalized class, shard, credit, and
//     bucket invariants.
//
// Policy does not own runtime state. It does not contain counters, snapshots,
// current pressure level, current budget assignment, retained bytes, in-use
// bytes, controller lag, trim backlog, or lifecycle state.
//
// Policy values are intended to be copied into immutable or atomically published
// runtime snapshots by owner components.
type Policy struct {
	// Retention defines byte and buffer-count limits for retained memory.
	Retention RetentionPolicy

	// Classes defines the size-class profile used to normalize requests and
	// classify returned capacities.
	Classes ClassPolicy

	// Shards defines class-local sharding and bucket-storage shape.
	Shards ShardPolicy

	// Admission defines return-path admission behavior.
	Admission AdmissionPolicy

	// Pressure defines how retention should contract under pressure.
	Pressure PressurePolicy

	// Trim defines bounded physical removal behavior for retained buffers.
	Trim TrimPolicy

	// Ownership defines whether checked-out buffers are tracked and validated.
	Ownership OwnershipPolicy
}

// PoolShapePolicy groups immutable construction shape for a Pool runtime.
//
// Shape fields decide which classes, shards, and bucket metadata exist after
// construction. They are not live budgets and should not be mutated to express
// current pressure, current trim backlog, or adaptive controller output.
type PoolShapePolicy struct {
	// Classes defines the size-class table constructed for the owner.
	Classes ClassPolicy

	// Shards defines class-local shard count, selector behavior, and bucket
	// storage shape.
	Shards ShardPolicy
}

// IsZero reports whether s contains no explicit shape policy.
func (s PoolShapePolicy) IsZero() bool {
	return s.Classes.IsZero() && s.Shards.IsZero()
}

// PoolRetentionPolicy groups runtime retention and correction behavior.
//
// These fields describe how retained storage is bounded, admitted, contracted
// under pressure, and physically trimmed. They are still policy data: current
// budgets, current pressure level, retained counters, and trim backlog remain
// runtime state owned by Pool, PoolPartition, or PoolGroup.
type PoolRetentionPolicy struct {
	// Retention defines byte and buffer-count limits.
	Retention RetentionPolicy

	// Admission defines return-path retention decisions.
	Admission AdmissionPolicy

	// Pressure defines pressure-level contraction rules.
	Pressure PressurePolicy

	// Trim defines bounded retained-buffer removal work.
	Trim TrimPolicy
}

// IsZero reports whether r contains no explicit retention policy.
func (r PoolRetentionPolicy) IsZero() bool {
	return r.Retention.IsZero() &&
		r.Admission.IsZero() &&
		r.Pressure.IsZero() &&
		r.Trim.IsZero()
}

// NewPolicyFromSections joins split policy sections into the legacy concrete
// Policy value used by owner configs.
//
// The class-size slice is copied so callers can freely mutate the input shape
// after construction without changing the returned Policy.
func NewPolicyFromSections(shape PoolShapePolicy, retention PoolRetentionPolicy, ownership OwnershipPolicy) Policy {
	return Policy{
		Retention: retention.Retention,
		Classes: ClassPolicy{
			Sizes: shape.Classes.SizesCopy(),
		},
		Shards:    shape.Shards,
		Admission: retention.Admission,
		Pressure:  retention.Pressure,
		Trim:      retention.Trim,
		Ownership: ownership,
	}
}

// ShapePolicy returns the construction-shape section of p.
//
// The returned class-size slice is caller-owned. Mutating it does not mutate p.
func (p Policy) ShapePolicy() PoolShapePolicy {
	return PoolShapePolicy{
		Classes: ClassPolicy{
			Sizes: p.Classes.SizesCopy(),
		},
		Shards: p.Shards,
	}
}

// RetentionPolicy returns the retention/admission/correction section of p.
func (p Policy) RetentionPolicy() PoolRetentionPolicy {
	return PoolRetentionPolicy{
		Retention: p.Retention,
		Admission: p.Admission,
		Pressure:  p.Pressure,
		Trim:      p.Trim,
	}
}

// IsZero reports whether p contains no explicit policy values.
//
// A zero Policy is not necessarily valid for constructing a runtime object. It
// means "unset" and should normally be completed by defaults before validation.
func (p Policy) IsZero() bool {
	return p.Retention.IsZero() &&
		p.Classes.IsZero() &&
		p.Shards.IsZero() &&
		p.Admission.IsZero() &&
		p.Pressure.IsZero() &&
		p.Trim.IsZero() &&
		p.Ownership.IsZero()
}

// RetentionPolicy defines memory-retention limits.
//
// These limits are policy-level constraints. Runtime owners decide how to
// interpret them at their own scope and how to project them to classes, shards,
// credits, and buckets.
//
// Retention is projected toward the static runtime as:
//
//	owner retained budget
//	-> class target
//	-> shard credit
//	-> bucket admission
//
// HardRetainedBytes is the upper safety limit for retained backing capacity in
// the owner scope using this policy. SoftRetainedBytes is the preferred target
// before pressure or contraction. A runtime may run above the soft limit
// temporarily, but must use admission and trim to converge back toward target.
//
// Zero values mean "unset" before defaulting and validation. Owner-specific
// validation decides which zero fields are acceptable.
type RetentionPolicy struct {
	// SoftRetainedBytes is the preferred retained-byte target.
	SoftRetainedBytes Size

	// HardRetainedBytes is the hard retained-byte limit for the policy owner.
	HardRetainedBytes Size

	// MaxRetainedBuffers limits the number of retained buffers in the policy
	// owner scope.
	MaxRetainedBuffers uint64

	// MaxRequestSize is the largest requested size that may be served through
	// this runtime policy.
	MaxRequestSize Size

	// MaxRetainedBufferCapacity is the largest returned buffer capacity that may
	// be admitted into retained storage.
	MaxRetainedBufferCapacity Size

	// MaxClassRetainedBytes limits retained bytes assigned to one size class.
	MaxClassRetainedBytes Size

	// MaxClassRetainedBuffers limits retained buffer count assigned to one size
	// class.
	MaxClassRetainedBuffers uint64

	// MaxShardRetainedBytes limits retained bytes assigned to one class shard.
	MaxShardRetainedBytes Size

	// MaxShardRetainedBuffers limits retained buffer count assigned to one class
	// shard.
	MaxShardRetainedBuffers uint64
}

// IsZero reports whether r contains no explicit retention limits.
func (r RetentionPolicy) IsZero() bool {
	return r.SoftRetainedBytes.IsZero() &&
		r.HardRetainedBytes.IsZero() &&
		r.MaxRetainedBuffers == 0 &&
		r.MaxRequestSize.IsZero() &&
		r.MaxRetainedBufferCapacity.IsZero() &&
		r.MaxClassRetainedBytes.IsZero() &&
		r.MaxClassRetainedBuffers == 0 &&
		r.MaxShardRetainedBytes.IsZero() &&
		r.MaxShardRetainedBuffers == 0
}

// ClassPolicy defines the configured size-class profile.
//
// Class sizes are explicit rather than inferred from only min/max fields. This
// keeps class-table construction deterministic and allows profiles to use
// power-of-two, geometric, hand-tuned, or workload-specific class layouts.
//
// The slice is intentionally stored as []ClassSize because policy.go models
// runtime behavior after generic user configuration has been normalized. Public
// config code may accept []Size and convert it before building Policy.
type ClassPolicy struct {
	// Sizes is the ordered class-size profile used by class-table construction.
	//
	// Validation must require this slice to be non-empty, positive, unique, and
	// strictly increasing before constructing a class table.
	Sizes []ClassSize
}

// IsZero reports whether c contains no class-size profile.
func (c ClassPolicy) IsZero() bool {
	return len(c.Sizes) == 0
}

// SizesCopy returns a copy of configured class sizes.
func (c ClassPolicy) SizesCopy() []ClassSize {
	return append([]ClassSize(nil), c.Sizes...)
}

// ShardPolicy defines class-local sharding and bucket storage shape.
type ShardPolicy struct {
	// Selection defines how classState chooses a shard for ordinary operations.
	Selection ShardSelectionMode

	// ShardsPerClass is the number of shards owned by each size class.
	//
	// This value controls lock striping in the hot path. Validation should
	// require it to be greater than zero.
	ShardsPerClass int

	// BucketSlotsPerShard is the maximum number of buffers physically retained
	// by one shard bucket.
	//
	// This is a physical storage cap. Shard credit may be lower than the bucket
	// slot limit.
	BucketSlotsPerShard int

	// BucketSegmentSlotsPerShard is the number of bucket slots allocated in one
	// lazy segment.
	//
	// The value controls metadata allocation granularity only. BucketSlotsPerShard
	// remains the total physical storage cap for the shard. Segment slots must be
	// positive and no larger than BucketSlotsPerShard.
	BucketSegmentSlotsPerShard int

	// AcquisitionFallbackShards is the maximum number of additional shards that
	// an acquisition path may probe after the primary selected shard misses.
	//
	// Zero means no fallback probing. Positive values are bounded at runtime by
	// the available shard count, so a shared policy can be valid for both
	// single-shard and multi-shard owners. A small value keeps Get local while
	// allowing one nearby reuse opportunity before allocation.
	AcquisitionFallbackShards int

	// ReturnFallbackShards is the maximum number of additional shards that a
	// return path may try after the primary selected shard rejects because its
	// bucket is full.
	//
	// Zero means returned buffers are either retained by the selected shard or
	// dropped. Positive values are reserved until return fallback is implemented;
	// validation rejects them so callers do not configure a silent no-op.
	ReturnFallbackShards int
}

// IsZero reports whether s contains no shard policy values.
func (s ShardPolicy) IsZero() bool {
	return s.Selection == ShardSelectionModeUnset &&
		s.ShardsPerClass == 0 &&
		s.BucketSlotsPerShard == 0 &&
		s.BucketSegmentSlotsPerShard == 0 &&
		s.AcquisitionFallbackShards == 0 &&
		s.ReturnFallbackShards == 0
}

// ShardSelectionMode identifies shard selection behavior.
type ShardSelectionMode uint8

const (
	// ShardSelectionModeUnset means no shard-selection mode has been chosen.
	ShardSelectionModeUnset ShardSelectionMode = iota

	// ShardSelectionModeSingle always selects shard zero.
	//
	// This is useful for deterministic tests and very small pools.
	ShardSelectionModeSingle

	// ShardSelectionModeRoundRobin stripes operations across class-owned shards
	// using an atomic counter.
	ShardSelectionModeRoundRobin

	// ShardSelectionModeRandom stripes operations using runtime-local pseudo
	// random selection.
	//
	// The implementation must not use private Go runtime P-local APIs.
	ShardSelectionModeRandom

	// ShardSelectionModeProcessorInspired uses owner-local striped entropy to
	// map operations to class shards without private runtime P-local APIs.
	//
	// The selector is processor-inspired in shape, not runtime-coupled: it uses
	// GOMAXPROCS-derived shard defaults, per-class sequence state, and bounded
	// mixed indexing, but it does not inspect or pin to Go scheduler P state.
	ShardSelectionModeProcessorInspired

	// ShardSelectionModeAffinity is reserved for a future routing path that
	// supplies an explicit affinity key with the operation.
	//
	// Current Pool and managed acquisition APIs do not pass such a key to the
	// data plane. Construction rejects this mode instead of silently degrading it
	// to processor-inspired entropy, because doing so would make the policy name
	// claim affinity behavior the runtime cannot prove.
	ShardSelectionModeAffinity
)

// String returns a stable diagnostic label for m.
func (m ShardSelectionMode) String() string {
	switch m {
	case ShardSelectionModeUnset:
		return "unset"
	case ShardSelectionModeSingle:
		return "single"
	case ShardSelectionModeRoundRobin:
		return "round_robin"
	case ShardSelectionModeRandom:
		return "random"
	case ShardSelectionModeProcessorInspired:
		return "processor_inspired"
	case ShardSelectionModeAffinity:
		return "affinity"
	default:
		return "unknown"
	}
}

// AdmissionPolicy defines return-path admission behavior.
//
// Admission is hot-path enforcement. It decides whether returned capacity may
// enter retained storage. Trim is cold-path correction and must not be used as a
// substitute for rejecting obviously unsuitable returned buffers.
type AdmissionPolicy struct {
	// ZeroSizeRequests defines how zero-length acquisition requests are handled.
	ZeroSizeRequests ZeroSizeRequestPolicy

	// ReturnedBuffers defines the default return-path behavior for ordinary
	// returned buffers.
	ReturnedBuffers ReturnedBufferPolicy

	// OversizedReturn defines the action for returned buffers whose capacity
	// exceeds Retention.MaxRetainedBufferCapacity.
	OversizedReturn AdmissionAction

	// UnsupportedClass defines the action for returned capacities that cannot be
	// mapped to a configured size class.
	UnsupportedClass AdmissionAction

	// ClassMismatch defines the action when ownership-aware accounting detects
	// that a returned buffer does not belong to the expected origin class.
	ClassMismatch AdmissionAction

	// CreditExhausted defines the action when the selected shard has no remaining
	// byte or buffer credit.
	CreditExhausted AdmissionAction

	// BucketFull defines the action when shard credit accepts the returned buffer
	// but physical bucket storage is full.
	BucketFull AdmissionAction

	// ZeroRetainedBuffers controls whether buffers are zeroed before entering
	// retained storage.
	//
	// This improves data hygiene but adds hot-path cost.
	ZeroRetainedBuffers bool

	// ZeroDroppedBuffers controls whether buffers are zeroed before being
	// discarded by admission.
	//
	// This may be useful for strict security profiles, but it can make rejected
	// returns more expensive.
	ZeroDroppedBuffers bool
}

// IsZero reports whether a contains no explicit admission policy.
func (a AdmissionPolicy) IsZero() bool {
	return a.ZeroSizeRequests == ZeroSizeRequestUnset &&
		a.ReturnedBuffers == ReturnedBufferPolicyUnset &&
		a.OversizedReturn == AdmissionActionUnset &&
		a.UnsupportedClass == AdmissionActionUnset &&
		a.ClassMismatch == AdmissionActionUnset &&
		a.CreditExhausted == AdmissionActionUnset &&
		a.BucketFull == AdmissionActionUnset &&
		!a.ZeroRetainedBuffers &&
		!a.ZeroDroppedBuffers
}

// ZeroSizeRequestPolicy identifies zero-size acquisition behavior.
type ZeroSizeRequestPolicy uint8

const (
	// ZeroSizeRequestUnset means zero-size request behavior is not configured.
	ZeroSizeRequestUnset ZeroSizeRequestPolicy = iota

	// ZeroSizeRequestSmallestClass serves zero-size requests from the smallest
	// configured class.
	ZeroSizeRequestSmallestClass

	// ZeroSizeRequestEmptyBuffer returns an empty non-retained buffer and does
	// not touch class/shard retained storage.
	ZeroSizeRequestEmptyBuffer

	// ZeroSizeRequestReject rejects zero-size acquisition requests.
	ZeroSizeRequestReject
)

// String returns a stable diagnostic label for p.
func (p ZeroSizeRequestPolicy) String() string {
	switch p {
	case ZeroSizeRequestUnset:
		return "unset"
	case ZeroSizeRequestSmallestClass:
		return "smallest_class"
	case ZeroSizeRequestEmptyBuffer:
		return "empty_buffer"
	case ZeroSizeRequestReject:
		return "reject"
	default:
		return "unknown"
	}
}

// ReturnedBufferPolicy identifies the default return-path posture.
type ReturnedBufferPolicy uint8

const (
	// ReturnedBufferPolicyUnset means return behavior is not configured.
	ReturnedBufferPolicyUnset ReturnedBufferPolicy = iota

	// ReturnedBufferPolicyAdmit retains returned buffers when class admission,
	// pressure policy, shard credit, and bucket storage allow retention.
	ReturnedBufferPolicyAdmit

	// ReturnedBufferPolicyDrop drops all returned buffers.
	//
	// This can be useful during shutdown, critical pressure, or diagnostic
	// profiles.
	ReturnedBufferPolicyDrop
)

// String returns a stable diagnostic label for p.
func (p ReturnedBufferPolicy) String() string {
	switch p {
	case ReturnedBufferPolicyUnset:
		return "unset"
	case ReturnedBufferPolicyAdmit:
		return "admit"
	case ReturnedBufferPolicyDrop:
		return "drop"
	default:
		return "unknown"
	}
}

// AdmissionAction identifies the action taken when an admission condition fails.
type AdmissionAction uint8

const (
	// AdmissionActionUnset means the action is not configured.
	AdmissionActionUnset AdmissionAction = iota

	// AdmissionActionDrop drops the returned buffer and records the reason.
	AdmissionActionDrop

	// AdmissionActionIgnore ignores the condition without reporting an error.
	//
	// This should be used sparingly. It is primarily useful for tolerant public
	// return APIs where invalid buffers are treated as no-op returns.
	AdmissionActionIgnore

	// AdmissionActionError reports the condition to the caller when the public
	// API supports returning errors.
	AdmissionActionError
)

// String returns a stable diagnostic label for a.
func (a AdmissionAction) String() string {
	switch a {
	case AdmissionActionUnset:
		return "unset"
	case AdmissionActionDrop:
		return "drop"
	case AdmissionActionIgnore:
		return "ignore"
	case AdmissionActionError:
		return "error"
	default:
		return "unknown"
	}
}

// PressurePolicy defines pressure-based retention contraction behavior.
//
// The current pressure level is runtime state and does not belong in Policy.
// Policy defines how the runtime should respond when a level is published by an
// owner-side pressure signal.
type PressurePolicy struct {
	// Enabled reports whether pressure-based contraction is enabled.
	Enabled bool

	// Medium defines behavior under medium pressure.
	Medium PressureLevelPolicy

	// High defines behavior under high pressure.
	High PressureLevelPolicy

	// Critical defines behavior under critical pressure.
	Critical PressureLevelPolicy
}

// IsZero reports whether p contains no pressure policy.
func (p PressurePolicy) IsZero() bool {
	return !p.Enabled &&
		p.Medium.IsZero() &&
		p.High.IsZero() &&
		p.Critical.IsZero()
}

// PressureLevel identifies a runtime pressure level.
type PressureLevel uint8

const (
	// PressureLevelNormal is the default pressure level.
	PressureLevelNormal PressureLevel = iota

	// PressureLevelMedium indicates moderate pressure where cold or wasteful
	// retention should contract.
	PressureLevelMedium

	// PressureLevelHigh indicates high pressure where retention should become
	// significantly more conservative.
	PressureLevelHigh

	// PressureLevelCritical indicates critical pressure where most returned
	// buffers should be discarded unless policy explicitly preserves essential
	// hot classes.
	PressureLevelCritical
)

// String returns a stable diagnostic label for l.
func (l PressureLevel) String() string {
	switch l {
	case PressureLevelNormal:
		return "normal"
	case PressureLevelMedium:
		return "medium"
	case PressureLevelHigh:
		return "high"
	case PressureLevelCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// PressureLevelPolicy defines behavior for one pressure level.
type PressureLevelPolicy struct {
	// RetentionScale scales the normal retained-byte target.
	//
	// PolicyRatioOne means 100% of the normal target. Policy validation may
	// require pressure scales to be <= PolicyRatioOne.
	RetentionScale PolicyRatio

	// TrimScale scales bounded trim work for this pressure level.
	//
	// Values above PolicyRatioOne are meaningful here: high pressure may allow
	// more trim work per cycle than normal operation.
	TrimScale PolicyRatio

	// MaxRetainedBufferCapacity overrides the maximum retained buffer capacity
	// under this pressure level when non-zero.
	MaxRetainedBufferCapacity Size

	// DropReturnedCapacityAbove drops returned buffers above this capacity under
	// this pressure level when non-zero.
	DropReturnedCapacityAbove Size

	// DisableRetention disables all new retention under this pressure level.
	DisableRetention bool

	// PreserveSmallHotClasses allows pressure handling to keep small hot classes
	// eligible for retention even when larger or colder classes are contracted.
	PreserveSmallHotClasses bool
}

// IsZero reports whether p contains no pressure-level behavior.
func (p PressureLevelPolicy) IsZero() bool {
	return p.RetentionScale == 0 &&
		p.TrimScale == 0 &&
		p.MaxRetainedBufferCapacity.IsZero() &&
		p.DropReturnedCapacityAbove.IsZero() &&
		!p.DisableRetention &&
		!p.PreserveSmallHotClasses
}

// TrimPolicy defines bounded physical removal behavior for retained buffers.
type TrimPolicy struct {
	// Enabled reports whether corrective trimming is enabled.
	Enabled bool

	// Interval is the target cadence for ordinary trim cycles.
	Interval time.Duration

	// FullScanInterval is the target cadence for broader diagnostic/control
	// scans. It should be slower than ordinary trim cycles.
	FullScanInterval time.Duration

	// MaxBuffersPerCycle limits the number of retained buffers removed by one
	// trim cycle.
	MaxBuffersPerCycle uint64

	// MaxBytesPerCycle limits retained backing capacity removed by one trim
	// cycle.
	MaxBytesPerCycle Size

	// MaxPoolsPerCycle limits how many pools one trim cycle may visit.
	MaxPoolsPerCycle int

	// MaxClassesPerPoolPerCycle limits how many classes may be trimmed for one
	// pool in one cycle.
	MaxClassesPerPoolPerCycle int

	// MaxShardsPerClassPerCycle limits how many shards may be trimmed for one
	// class in one cycle.
	MaxShardsPerClassPerCycle int

	// TrimOnPolicyShrink enables contraction after policy target shrink.
	TrimOnPolicyShrink bool

	// TrimOnPressure enables pressure-triggered trim work.
	TrimOnPressure bool

	// TrimOnClose enables physical retained-storage removal during lifecycle
	// close after the owner has stopped new operations.
	TrimOnClose bool
}

// IsZero reports whether t contains no trim policy.
func (t TrimPolicy) IsZero() bool {
	return !t.Enabled &&
		t.Interval == 0 &&
		t.FullScanInterval == 0 &&
		t.MaxBuffersPerCycle == 0 &&
		t.MaxBytesPerCycle.IsZero() &&
		t.MaxPoolsPerCycle == 0 &&
		t.MaxClassesPerPoolPerCycle == 0 &&
		t.MaxShardsPerClassPerCycle == 0 &&
		!t.TrimOnPolicyShrink &&
		!t.TrimOnPressure &&
		!t.TrimOnClose
}

// OwnershipPolicy defines ownership/accounting behavior for checked-out buffers.
//
// Ownership policy is part of runtime policy because it changes admission,
// release validation, observability, and cost. Concrete ownership records and
// checked-out buffer state are runtime state and do not belong in Policy.
type OwnershipPolicy struct {
	// Mode selects ownership/accounting strictness.
	Mode OwnershipMode

	// TrackInUseBytes enables accounting for checked-out backing capacity.
	TrackInUseBytes bool

	// TrackInUseBuffers enables accounting for checked-out buffer count.
	TrackInUseBuffers bool

	// DetectDoubleRelease enables double-release detection when ownership state
	// is available.
	DetectDoubleRelease bool

	// MaxReturnedCapacityGrowth limits returned capacity relative to the origin
	// class size when ownership-aware accounting is enabled.
	//
	// PolicyRatioOne means returned capacity must be no larger than the origin
	// class size. A value of 2*PolicyRatioOne allows up to 2x growth. Zero means
	// unset before defaults/validation.
	MaxReturnedCapacityGrowth PolicyRatio
}

// IsZero reports whether o contains no ownership policy.
func (o OwnershipPolicy) IsZero() bool {
	return o.Mode == OwnershipModeUnset &&
		!o.TrackInUseBytes &&
		!o.TrackInUseBuffers &&
		!o.DetectDoubleRelease &&
		o.MaxReturnedCapacityGrowth == 0
}

// OwnershipMode identifies checked-out buffer ownership behavior.
type OwnershipMode uint8

const (
	// OwnershipModeUnset means ownership behavior is not configured.
	OwnershipModeUnset OwnershipMode = iota

	// OwnershipModeNone disables explicit ownership tracking.
	OwnershipModeNone

	// OwnershipModeAccounting records owner-side accounting without requiring
	// strict ownership validation on every public return path.
	OwnershipModeAccounting

	// OwnershipModeStrict requires explicit ownership validation and may report
	// ownership violations as errors.
	OwnershipModeStrict
)

// String returns a stable diagnostic label for m.
func (m OwnershipMode) String() string {
	switch m {
	case OwnershipModeUnset:
		return "unset"
	case OwnershipModeNone:
		return "none"
	case OwnershipModeAccounting:
		return "accounting"
	case OwnershipModeStrict:
		return "strict"
	default:
		return "unknown"
	}
}

const (
	// PolicyRatioScale is the fixed-point denominator used by PolicyRatio.
	//
	// A value of 10_000 represents 1.0 or 100%.
	PolicyRatioScale uint32 = 10_000

	// PolicyRatioOne is the fixed-point value for 1.0.
	PolicyRatioOne PolicyRatio = PolicyRatio(PolicyRatioScale)
)

// PolicyRatio is a small fixed-point ratio used by policy fields.
//
// PolicyRatio intentionally does not enforce a universal upper bound. Some
// fields are unit ratios and should be validated in [0, PolicyRatioOne]. Other
// fields, such as trim scaling or returned-capacity growth, may validly exceed
// PolicyRatioOne.
//
// Examples:
//
//	0                         -> 0.0
//	PolicyRatioOne / 2        -> 0.5
//	PolicyRatioOne            -> 1.0
//	2 * PolicyRatioOne        -> 2.0
type PolicyRatio uint32

// PolicyRatioFromBasisPoints returns a PolicyRatio from basis points.
//
// The helper does not clamp. Field-specific validation belongs with policy
// validation because some policy fields permit values greater than one.
func PolicyRatioFromBasisPoints(basisPoints uint32) PolicyRatio {
	return PolicyRatio(basisPoints)
}

// BasisPoints returns r in fixed-point basis points.
func (r PolicyRatio) BasisPoints() uint32 {
	return uint32(r)
}

// Float64 returns r as a floating-point ratio.
func (r PolicyRatio) Float64() float64 {
	return float64(r) / float64(PolicyRatioScale)
}

// IsZero reports whether r is zero.
func (r PolicyRatio) IsZero() bool {
	return r == 0
}

// IsOne reports whether r equals PolicyRatioOne.
func (r PolicyRatio) IsOne() bool {
	return r == PolicyRatioOne
}
