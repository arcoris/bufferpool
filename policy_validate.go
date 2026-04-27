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
	"strconv"

	"arcoris.dev/bufferpool/internal/multierr"
)

// ValidatePolicy validates a complete runtime memory-retention policy.
//
// Validation does not apply defaults, does not normalize values, does not build
// class tables, and does not create runtime state. Callers that accept partial
// user configuration should complete defaults before calling ValidatePolicy.
//
// A valid policy must define the required data-plane sections:
//
//   - retention;
//   - class profile;
//   - shard layout;
//   - admission behavior.
//
// Pressure, trim, and ownership sections may be disabled, but if they are
// configured they must be internally consistent.
func ValidatePolicy(policy Policy) error {
	return policy.Validate()
}

// Validate validates a complete runtime memory-retention policy.
//
// The method validates both section-local invariants and cross-section
// invariants. Section-local validation checks whether each policy component is
// coherent by itself. Cross-section validation checks whether class sizes,
// retention limits, shard physical limits, and pressure overrides can work
// together.
//
// The returned error is classified as ErrInvalidPolicy and may contain multiple
// validation failures. Callers should use errors.Is(err, ErrInvalidPolicy) for
// classification instead of matching error strings.
func (p Policy) Validate() error {
	var errs []error

	if p.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.Policy: policy must not be zero")
		return multierr.Combine(errs...)
	}

	retentionErr := p.Retention.Validate()
	classErr := p.Classes.Validate()
	shardErr := p.Shards.Validate()
	admissionErr := p.Admission.Validate()
	pressureErr := p.Pressure.Validate()
	trimErr := p.Trim.Validate()
	ownershipErr := p.Ownership.Validate()

	errs = appendPolicyError(errs, retentionErr)
	errs = appendPolicyError(errs, classErr)
	errs = appendPolicyError(errs, shardErr)
	errs = appendPolicyError(errs, admissionErr)
	errs = appendPolicyError(errs, pressureErr)
	errs = appendPolicyError(errs, trimErr)
	errs = appendPolicyError(errs, ownershipErr)

	if retentionErr == nil && classErr == nil {
		errs = appendPolicyError(errs, validatePolicyClassRetentionConsistency(p.Retention, p.Classes))
	}

	if retentionErr == nil && classErr == nil && shardErr == nil {
		errs = appendPolicyError(errs, validatePolicyShardRetentionConsistency(p.Retention, p.Classes, p.Shards))
	}

	if retentionErr == nil && pressureErr == nil {
		errs = appendPolicyError(errs, validatePolicyPressureRetentionConsistency(p.Retention, p.Pressure))
	}

	return multierr.Combine(errs...)
}

// Validate validates retained-memory limits.
//
// RetentionPolicy is required for a complete Policy. Zero retention policy means
// no retained-memory bounds, no request limit, and no class/shard targets, which
// is not enough to construct a bounded runtime owner.
func (r RetentionPolicy) Validate() error {
	var errs []error

	if r.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.RetentionPolicy: retention policy must not be zero")
		return multierr.Combine(errs...)
	}

	if r.SoftRetainedBytes.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.RetentionPolicy: soft retained bytes must be greater than zero")
	}

	if r.HardRetainedBytes.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.RetentionPolicy: hard retained bytes must be greater than zero")
	}

	if r.SoftRetainedBytes > r.HardRetainedBytes {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.RetentionPolicy: soft retained bytes "+r.SoftRetainedBytes.String()+
				" must be less than or equal to hard retained bytes "+r.HardRetainedBytes.String(),
		)
	}

	if r.MaxRetainedBuffers == 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.RetentionPolicy: max retained buffers must be greater than zero")
	}

	if r.MaxRequestSize.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.RetentionPolicy: max request size must be greater than zero")
	}

	if r.MaxRetainedBufferCapacity.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.RetentionPolicy: max retained buffer capacity must be greater than zero")
	}

	if r.MaxRetainedBufferCapacity > r.HardRetainedBytes {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.RetentionPolicy: max retained buffer capacity "+r.MaxRetainedBufferCapacity.String()+
				" must be less than or equal to hard retained bytes "+r.HardRetainedBytes.String(),
		)
	}

	if r.MaxClassRetainedBytes.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.RetentionPolicy: max class retained bytes must be greater than zero")
	}

	if r.MaxClassRetainedBytes > r.HardRetainedBytes {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.RetentionPolicy: max class retained bytes "+r.MaxClassRetainedBytes.String()+
				" must be less than or equal to hard retained bytes "+r.HardRetainedBytes.String(),
		)
	}

	if r.MaxClassRetainedBuffers == 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.RetentionPolicy: max class retained buffers must be greater than zero")
	}

	if r.MaxClassRetainedBuffers > r.MaxRetainedBuffers {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.RetentionPolicy: max class retained buffers "+policyUint64String(r.MaxClassRetainedBuffers)+
				" must be less than or equal to max retained buffers "+policyUint64String(r.MaxRetainedBuffers),
		)
	}

	if r.MaxShardRetainedBytes.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.RetentionPolicy: max shard retained bytes must be greater than zero")
	}

	if r.MaxShardRetainedBytes > r.MaxClassRetainedBytes {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.RetentionPolicy: max shard retained bytes "+r.MaxShardRetainedBytes.String()+
				" must be less than or equal to max class retained bytes "+r.MaxClassRetainedBytes.String(),
		)
	}

	if r.MaxShardRetainedBuffers == 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.RetentionPolicy: max shard retained buffers must be greater than zero")
	}

	if r.MaxShardRetainedBuffers > r.MaxClassRetainedBuffers {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.RetentionPolicy: max shard retained buffers "+policyUint64String(r.MaxShardRetainedBuffers)+
				" must be less than or equal to max class retained buffers "+policyUint64String(r.MaxClassRetainedBuffers),
		)
	}

	return multierr.Combine(errs...)
}

// Validate validates the configured class-size profile.
//
// ClassPolicy validation checks only profile shape. It does not build a
// classTable. Table construction remains owned by class_table.go, while policy
// validation ensures that the configured profile is suitable before an owner
// attempts to build runtime state from it.
func (c ClassPolicy) Validate() error {
	var errs []error

	if c.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.ClassPolicy: at least one class size is required")
		return multierr.Combine(errs...)
	}

	if len(c.Sizes) > maxClassTableClasses {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.ClassPolicy: class count "+strconv.Itoa(len(c.Sizes))+" exceeds ClassID range",
		)
	}

	previous := ClassSize(0)
	for index, size := range c.Sizes {
		if size.IsZero() {
			errs = appendPolicyValidationError(errs, "bufferpool.ClassPolicy: class size at index "+strconv.Itoa(index)+" must be greater than zero")
			continue
		}

		if index > 0 && !previous.Less(size) {
			errs = appendPolicyValidationError(
				errs,
				"bufferpool.ClassPolicy: class sizes must be strictly increasing at index "+strconv.Itoa(index)+
					": "+size.String()+" must be greater than "+previous.String(),
			)
		}

		previous = size
	}

	return multierr.Combine(errs...)
}

// Validate validates class-local shard and bucket-storage policy.
//
// ShardPolicy controls hot-path lock striping and physical retained-storage
// shape. It does not decide byte budgets; byte and buffer credit are projected
// later from RetentionPolicy through class budgets into shard credits.
func (s ShardPolicy) Validate() error {
	var errs []error

	if s.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.ShardPolicy: shard policy must not be zero")
		return multierr.Combine(errs...)
	}

	if !isKnownShardSelectionMode(s.Selection) {
		errs = appendPolicyValidationError(errs, "bufferpool.ShardPolicy: unknown shard selection mode "+policyUint8String(uint8(s.Selection)))
	} else if s.Selection == ShardSelectionModeUnset {
		errs = appendPolicyValidationError(errs, "bufferpool.ShardPolicy: shard selection mode must be set")
	}

	if s.ShardsPerClass <= 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.ShardPolicy: shards per class must be greater than zero")
	}

	if s.BucketSlotsPerShard <= 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.ShardPolicy: bucket slots per shard must be greater than zero")
	}

	if s.AcquisitionFallbackShards < 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.ShardPolicy: acquisition fallback shards must not be negative")
	}

	if s.ReturnFallbackShards < 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.ShardPolicy: return fallback shards must not be negative")
	}

	if s.ShardsPerClass > 0 {
		maxFallback := s.ShardsPerClass - 1

		if s.AcquisitionFallbackShards > maxFallback {
			errs = appendPolicyValidationError(
				errs,
				"bufferpool.ShardPolicy: acquisition fallback shards "+strconv.Itoa(s.AcquisitionFallbackShards)+
					" must be less than shards per class "+strconv.Itoa(s.ShardsPerClass),
			)
		}

		if s.ReturnFallbackShards > maxFallback {
			errs = appendPolicyValidationError(
				errs,
				"bufferpool.ShardPolicy: return fallback shards "+strconv.Itoa(s.ReturnFallbackShards)+
					" must be less than shards per class "+strconv.Itoa(s.ShardsPerClass),
			)
		}
	}

	return multierr.Combine(errs...)
}

// Validate validates return-path admission behavior.
//
// AdmissionPolicy is required for a complete Policy because it defines how the
// runtime handles returned buffers that cannot or should not be retained. The
// policy layer validates configured actions only; concrete drop counters and
// public error reporting belong to later runtime/admission code.
func (a AdmissionPolicy) Validate() error {
	var errs []error

	if a.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.AdmissionPolicy: admission policy must not be zero")
		return multierr.Combine(errs...)
	}

	if !isKnownZeroSizeRequestPolicy(a.ZeroSizeRequests) {
		errs = appendPolicyValidationError(errs, "bufferpool.AdmissionPolicy: unknown zero-size request policy "+policyUint8String(uint8(a.ZeroSizeRequests)))
	} else if a.ZeroSizeRequests == ZeroSizeRequestUnset {
		errs = appendPolicyValidationError(errs, "bufferpool.AdmissionPolicy: zero-size request policy must be set")
	}

	if !isKnownReturnedBufferPolicy(a.ReturnedBuffers) {
		errs = appendPolicyValidationError(errs, "bufferpool.AdmissionPolicy: unknown returned-buffer policy "+policyUint8String(uint8(a.ReturnedBuffers)))
	} else if a.ReturnedBuffers == ReturnedBufferPolicyUnset {
		errs = appendPolicyValidationError(errs, "bufferpool.AdmissionPolicy: returned-buffer policy must be set")
	}

	errs = appendPolicyError(errs, validateAdmissionAction("oversized return", a.OversizedReturn))
	errs = appendPolicyError(errs, validateAdmissionAction("unsupported class", a.UnsupportedClass))
	errs = appendPolicyError(errs, validateAdmissionAction("class mismatch", a.ClassMismatch))
	errs = appendPolicyError(errs, validateAdmissionAction("credit exhausted", a.CreditExhausted))
	errs = appendPolicyError(errs, validateAdmissionAction("bucket full", a.BucketFull))

	return multierr.Combine(errs...)
}

// Validate validates pressure-contraction behavior.
//
// A zero PressurePolicy is valid and means pressure handling is not configured.
// If pressure behavior is configured, it must be enabled and all standard
// pressure levels must have explicit behavior so runtime pressure publication
// cannot reach an undefined level.
func (p PressurePolicy) Validate() error {
	var errs []error

	if p.IsZero() {
		return nil
	}

	if !p.Enabled {
		errs = appendPolicyValidationError(errs, "bufferpool.PressurePolicy: pressure policy must be enabled when pressure levels are configured")
		return multierr.Combine(errs...)
	}

	if p.Medium.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.PressurePolicy: medium pressure policy must be configured")
	}

	if p.High.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.PressurePolicy: high pressure policy must be configured")
	}

	if p.Critical.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.PressurePolicy: critical pressure policy must be configured")
	}

	mediumErr := p.Medium.Validate()
	highErr := p.High.Validate()
	criticalErr := p.Critical.Validate()

	errs = appendPolicyError(errs, prefixPolicyValidationError("bufferpool.PressurePolicy: medium pressure policy is invalid", mediumErr))
	errs = appendPolicyError(errs, prefixPolicyValidationError("bufferpool.PressurePolicy: high pressure policy is invalid", highErr))
	errs = appendPolicyError(errs, prefixPolicyValidationError("bufferpool.PressurePolicy: critical pressure policy is invalid", criticalErr))

	if mediumErr == nil && highErr == nil && criticalErr == nil &&
		!p.Medium.IsZero() && !p.High.IsZero() && !p.Critical.IsZero() {
		errs = appendPolicyError(errs, validatePressureLevelOrdering(p))
	}

	return multierr.Combine(errs...)
}

// Validate validates one pressure-level behavior.
func (p PressureLevelPolicy) Validate() error {
	var errs []error

	if p.IsZero() {
		return nil
	}

	if p.RetentionScale > PolicyRatioOne {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.PressureLevelPolicy: retention scale "+policyRatioString(p.RetentionScale)+
				" must be less than or equal to "+policyRatioString(PolicyRatioOne),
		)
	}

	if p.TrimScale.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.PressureLevelPolicy: trim scale must be greater than zero")
	}

	if !p.MaxRetainedBufferCapacity.IsZero() &&
		!p.DropReturnedCapacityAbove.IsZero() &&
		p.DropReturnedCapacityAbove > p.MaxRetainedBufferCapacity {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.PressureLevelPolicy: drop returned capacity threshold "+p.DropReturnedCapacityAbove.String()+
				" must be less than or equal to max retained buffer capacity "+p.MaxRetainedBufferCapacity.String(),
		)
	}

	return multierr.Combine(errs...)
}

// Validate validates bounded trim behavior.
//
// A zero TrimPolicy is valid and means trim is not configured. If any trim field
// is configured, trim must be explicitly enabled and cycle bounds must be
// positive.
func (t TrimPolicy) Validate() error {
	var errs []error

	if t.IsZero() {
		return nil
	}

	if !t.Enabled {
		errs = appendPolicyValidationError(errs, "bufferpool.TrimPolicy: trim policy must be enabled when trim settings are configured")
		return multierr.Combine(errs...)
	}

	if t.Interval <= 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.TrimPolicy: interval must be greater than zero")
	}

	if t.FullScanInterval <= 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.TrimPolicy: full scan interval must be greater than zero")
	}

	if t.Interval > 0 && t.FullScanInterval > 0 && t.FullScanInterval < t.Interval {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.TrimPolicy: full scan interval "+t.FullScanInterval.String()+
				" must be greater than or equal to interval "+t.Interval.String(),
		)
	}

	if t.MaxBuffersPerCycle == 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.TrimPolicy: max buffers per cycle must be greater than zero")
	}

	if t.MaxBytesPerCycle.IsZero() {
		errs = appendPolicyValidationError(errs, "bufferpool.TrimPolicy: max bytes per cycle must be greater than zero")
	}

	if t.MaxPoolsPerCycle <= 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.TrimPolicy: max pools per cycle must be greater than zero")
	}

	if t.MaxClassesPerPoolPerCycle <= 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.TrimPolicy: max classes per pool per cycle must be greater than zero")
	}

	if t.MaxShardsPerClassPerCycle <= 0 {
		errs = appendPolicyValidationError(errs, "bufferpool.TrimPolicy: max shards per class per cycle must be greater than zero")
	}

	return multierr.Combine(errs...)
}

// Validate validates ownership/accounting policy.
//
// A zero OwnershipPolicy is valid and means ownership behavior is not configured.
// OwnershipModeNone is also valid and explicitly disables ownership tracking.
func (o OwnershipPolicy) Validate() error {
	var errs []error

	if o.IsZero() {
		return nil
	}

	if !isKnownOwnershipMode(o.Mode) {
		errs = appendPolicyValidationError(errs, "bufferpool.OwnershipPolicy: unknown ownership mode "+policyUint8String(uint8(o.Mode)))
		return multierr.Combine(errs...)
	}

	if o.Mode == OwnershipModeUnset {
		errs = appendPolicyValidationError(errs, "bufferpool.OwnershipPolicy: ownership mode must be set when ownership fields are configured")
		return multierr.Combine(errs...)
	}

	if o.Mode == OwnershipModeNone {
		if o.TrackInUseBytes {
			errs = appendPolicyValidationError(errs, "bufferpool.OwnershipPolicy: track in-use bytes requires ownership mode other than none")
		}

		if o.TrackInUseBuffers {
			errs = appendPolicyValidationError(errs, "bufferpool.OwnershipPolicy: track in-use buffers requires ownership mode other than none")
		}

		if o.DetectDoubleRelease {
			errs = appendPolicyValidationError(errs, "bufferpool.OwnershipPolicy: double-release detection requires ownership mode other than none")
		}
	}

	if o.Mode == OwnershipModeAccounting || o.Mode == OwnershipModeStrict {
		if o.MaxReturnedCapacityGrowth.IsZero() {
			errs = appendPolicyValidationError(errs, "bufferpool.OwnershipPolicy: max returned capacity growth must be greater than zero when ownership tracking is enabled")
		}

		if !o.MaxReturnedCapacityGrowth.IsZero() && o.MaxReturnedCapacityGrowth < PolicyRatioOne {
			errs = appendPolicyValidationError(
				errs,
				"bufferpool.OwnershipPolicy: max returned capacity growth "+policyRatioString(o.MaxReturnedCapacityGrowth)+
					" must be greater than or equal to "+policyRatioString(PolicyRatioOne),
			)
		}
	}

	return multierr.Combine(errs...)
}

func validatePolicyClassRetentionConsistency(retention RetentionPolicy, classes ClassPolicy) error {
	var errs []error

	if len(classes.Sizes) == 0 {
		return nil
	}

	largestClass := classes.Sizes[len(classes.Sizes)-1].Size()
	smallestClass := classes.Sizes[0].Size()

	if retention.MaxRequestSize > largestClass {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.Policy: max request size "+retention.MaxRequestSize.String()+
				" must be less than or equal to largest class size "+largestClass.String(),
		)
	}

	if retention.MaxRetainedBufferCapacity < smallestClass {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.Policy: max retained buffer capacity "+retention.MaxRetainedBufferCapacity.String()+
				" must be greater than or equal to smallest class size "+smallestClass.String(),
		)
	}

	if largestClass > retention.MaxRetainedBufferCapacity {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.Policy: largest class size "+largestClass.String()+
				" must be less than or equal to max retained buffer capacity "+retention.MaxRetainedBufferCapacity.String(),
		)
	}

	if largestClass > retention.MaxClassRetainedBytes {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.Policy: largest class size "+largestClass.String()+
				" must be less than or equal to max class retained bytes "+retention.MaxClassRetainedBytes.String(),
		)
	}

	if largestClass > retention.MaxShardRetainedBytes {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.Policy: largest class size "+largestClass.String()+
				" must be less than or equal to max shard retained bytes "+retention.MaxShardRetainedBytes.String(),
		)
	}

	return multierr.Combine(errs...)
}

func validatePolicyShardRetentionConsistency(retention RetentionPolicy, classes ClassPolicy, shards ShardPolicy) error {
	var errs []error

	if retention.MaxShardRetainedBuffers > uint64(shards.BucketSlotsPerShard) {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.Policy: max shard retained buffers "+policyUint64String(retention.MaxShardRetainedBuffers)+
				" must be less than or equal to bucket slots per shard "+strconv.Itoa(shards.BucketSlotsPerShard),
		)
	}

	totalClassSlots, ok := policyUint64Product(shards.ShardsPerClass, shards.BucketSlotsPerShard)
	if !ok {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.Policy: shards per class "+strconv.Itoa(shards.ShardsPerClass)+
				" and bucket slots per shard "+strconv.Itoa(shards.BucketSlotsPerShard)+
				" overflow class physical slot count",
		)

		return multierr.Combine(errs...)
	}

	if retention.MaxClassRetainedBuffers > totalClassSlots {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.Policy: max class retained buffers "+policyUint64String(retention.MaxClassRetainedBuffers)+
				" must be less than or equal to physical class slots "+policyUint64String(totalClassSlots),
		)
	}

	_ = classes

	return multierr.Combine(errs...)
}

func validatePolicyPressureRetentionConsistency(retention RetentionPolicy, pressure PressurePolicy) error {
	var errs []error

	if pressure.IsZero() {
		return nil
	}

	errs = appendPolicyError(
		errs,
		validatePressureLevelRetentionConsistency("medium", retention, pressure.Medium),
	)

	errs = appendPolicyError(
		errs,
		validatePressureLevelRetentionConsistency("high", retention, pressure.High),
	)

	errs = appendPolicyError(
		errs,
		validatePressureLevelRetentionConsistency("critical", retention, pressure.Critical),
	)

	return multierr.Combine(errs...)
}

func validatePressureLevelRetentionConsistency(name string, retention RetentionPolicy, level PressureLevelPolicy) error {
	var errs []error

	if level.IsZero() {
		return nil
	}

	if !level.MaxRetainedBufferCapacity.IsZero() &&
		level.MaxRetainedBufferCapacity > retention.MaxRetainedBufferCapacity {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.Policy: "+name+" pressure max retained buffer capacity "+level.MaxRetainedBufferCapacity.String()+
				" must be less than or equal to normal max retained buffer capacity "+retention.MaxRetainedBufferCapacity.String(),
		)
	}

	if !level.DropReturnedCapacityAbove.IsZero() &&
		level.DropReturnedCapacityAbove > retention.MaxRetainedBufferCapacity {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.Policy: "+name+" pressure drop returned capacity threshold "+level.DropReturnedCapacityAbove.String()+
				" must be less than or equal to normal max retained buffer capacity "+retention.MaxRetainedBufferCapacity.String(),
		)
	}

	return multierr.Combine(errs...)
}

func validatePressureLevelOrdering(policy PressurePolicy) error {
	var errs []error

	if policy.Medium.RetentionScale < policy.High.RetentionScale {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.PressurePolicy: medium retention scale "+policyRatioString(policy.Medium.RetentionScale)+
				" must be greater than or equal to high retention scale "+policyRatioString(policy.High.RetentionScale),
		)
	}

	if policy.High.RetentionScale < policy.Critical.RetentionScale {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.PressurePolicy: high retention scale "+policyRatioString(policy.High.RetentionScale)+
				" must be greater than or equal to critical retention scale "+policyRatioString(policy.Critical.RetentionScale),
		)
	}

	if policy.Medium.TrimScale > policy.High.TrimScale {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.PressurePolicy: medium trim scale "+policyRatioString(policy.Medium.TrimScale)+
				" must be less than or equal to high trim scale "+policyRatioString(policy.High.TrimScale),
		)
	}

	if policy.High.TrimScale > policy.Critical.TrimScale {
		errs = appendPolicyValidationError(
			errs,
			"bufferpool.PressurePolicy: high trim scale "+policyRatioString(policy.High.TrimScale)+
				" must be less than or equal to critical trim scale "+policyRatioString(policy.Critical.TrimScale),
		)
	}

	return multierr.Combine(errs...)
}

func validateAdmissionAction(name string, action AdmissionAction) error {
	if !isKnownAdmissionAction(action) {
		return policyValidationError("bufferpool.AdmissionPolicy: unknown " + name + " action " + policyUint8String(uint8(action)))
	}

	if action == AdmissionActionUnset {
		return policyValidationError("bufferpool.AdmissionPolicy: " + name + " action must be set")
	}

	return nil
}

func isKnownShardSelectionMode(mode ShardSelectionMode) bool {
	switch mode {
	case ShardSelectionModeUnset,
		ShardSelectionModeSingle,
		ShardSelectionModeRoundRobin,
		ShardSelectionModeRandom:
		return true
	default:
		return false
	}
}

func isKnownZeroSizeRequestPolicy(policy ZeroSizeRequestPolicy) bool {
	switch policy {
	case ZeroSizeRequestUnset,
		ZeroSizeRequestSmallestClass,
		ZeroSizeRequestEmptyBuffer,
		ZeroSizeRequestReject:
		return true
	default:
		return false
	}
}

func isKnownReturnedBufferPolicy(policy ReturnedBufferPolicy) bool {
	switch policy {
	case ReturnedBufferPolicyUnset,
		ReturnedBufferPolicyAdmit,
		ReturnedBufferPolicyDrop:
		return true
	default:
		return false
	}
}

func isKnownAdmissionAction(action AdmissionAction) bool {
	switch action {
	case AdmissionActionUnset,
		AdmissionActionDrop,
		AdmissionActionIgnore,
		AdmissionActionError:
		return true
	default:
		return false
	}
}

func isKnownOwnershipMode(mode OwnershipMode) bool {
	switch mode {
	case OwnershipModeUnset,
		OwnershipModeNone,
		OwnershipModeAccounting,
		OwnershipModeStrict:
		return true
	default:
		return false
	}
}

func policyUint64Product(left, right int) (uint64, bool) {
	if left < 0 || right < 0 {
		return 0, false
	}

	unsignedLeft := uint64(left)
	unsignedRight := uint64(right)

	if unsignedRight != 0 && unsignedLeft > ^uint64(0)/unsignedRight {
		return 0, false
	}

	return unsignedLeft * unsignedRight, true
}

func appendPolicyError(errs []error, err error) []error {
	if err == nil {
		return errs
	}

	return append(errs, err)
}

func appendPolicyValidationError(errs []error, message string) []error {
	return append(errs, policyValidationError(message))
}

func policyValidationError(message string) error {
	return newError(ErrInvalidPolicy, message)
}

func prefixPolicyValidationError(prefix string, err error) error {
	if err == nil {
		return nil
	}

	return wrapError(ErrInvalidPolicy, err, prefix+": "+err.Error())
}

func policyUint8String(value uint8) string {
	return strconv.FormatUint(uint64(value), 10)
}

func policyUint64String(value uint64) string {
	return strconv.FormatUint(value, 10)
}

func policyRatioString(value PolicyRatio) string {
	return strconv.FormatUint(uint64(value), 10)
}
