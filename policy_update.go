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

import "errors"

const (
	// policyUpdateFailureInvalid reports that the candidate policy is not valid
	// for the owner that would publish it.
	policyUpdateFailureInvalid = "policy_update_invalid"

	// policyUpdateFailureShapeChange reports that the candidate policy changes
	// construction-time Pool shape.
	policyUpdateFailureShapeChange = "policy_update_shape_change"

	// policyUpdateFailureOwnershipChange reports that the candidate policy
	// changes ownership/accounting semantics.
	policyUpdateFailureOwnershipChange = "policy_update_ownership_change"

	// policyUpdateFailureInfeasibleBudget is reserved for owner publication paths
	// that validate a candidate policy but cannot project it into feasible
	// runtime budgets.
	policyUpdateFailureInfeasibleBudget = "policy_update_infeasible_budget"

	// policyUpdateFailureClosed is reserved for owner publication paths that are
	// rejected because the publishing runtime owner is closing or closed.
	policyUpdateFailureClosed = "policy_update_closed"

	// policyUpdateFailureSkippedChild is reserved for hierarchy publication paths
	// that intentionally report a child owner skipped during policy propagation.
	policyUpdateFailureSkippedChild = "policy_update_skipped_child"

	// policyUpdateFailureTrimFailed is reserved for policy contraction paths that
	// successfully publish policy state but fail during bounded cleanup.
	policyUpdateFailureTrimFailed = "policy_update_trim_failed"

	// policyUpdateFailureSchedulerChange reports live policy attempts that would
	// start, stop, or retime a construction-time scheduler. Scheduler runtime is
	// owner lifecycle state in the current integration, so live policy publication
	// rejects those changes instead of silently changing goroutine ownership.
	policyUpdateFailureSchedulerChange = "policy_update_scheduler_change"
)

// policyUpdateFailureReasonForError maps an error class to stable publication
// report vocabulary.
//
// Publication reports are meant for machines as much as humans, so they must
// not expose formatted validation text as FailureReason. Callers still return
// the original error value, preserving errors.Is behavior and detailed error
// messages for callers that need them.
func policyUpdateFailureReasonForError(err error, fallback string) string {
	if fallback == "" {
		fallback = policyUpdateFailureInvalid
	}
	if err == nil {
		return fallback
	}
	if errors.Is(err, ErrClosed) {
		return policyUpdateFailureClosed
	}
	if errors.Is(err, ErrInvalidPolicy) || errors.Is(err, ErrInvalidOptions) {
		switch fallback {
		case policyUpdateFailureShapeChange,
			policyUpdateFailureOwnershipChange,
			policyUpdateFailureInfeasibleBudget,
			policyUpdateFailureSkippedChild,
			policyUpdateFailureSchedulerChange:
			return fallback
		default:
			return policyUpdateFailureInvalid
		}
	}
	return fallback
}

// PolicyUpdateKind identifies one policy section affected by a live-update diff.
//
// The kind is diagnostic vocabulary. It does not by itself authorize mutation.
// Live publication still belongs to runtime owners, which must validate the diff
// against lifecycle, topology, budget feasibility, and child publication state.
type PolicyUpdateKind uint8

const (
	// PolicyUpdateKindNone means no policy section is identified.
	PolicyUpdateKindNone PolicyUpdateKind = iota

	// PolicyUpdateKindShape means the diff touches construction-time Pool shape:
	// class table, shard count, selector mode, bucket shape, or fallback probing
	// shape that was fixed when the Pool was built.
	PolicyUpdateKindShape

	// PolicyUpdateKindRetention means the diff touches retention byte or buffer
	// limits.
	PolicyUpdateKindRetention

	// PolicyUpdateKindAdmission means the diff touches return-path admission
	// behavior.
	PolicyUpdateKindAdmission

	// PolicyUpdateKindPressure means the diff touches pressure contraction policy.
	PolicyUpdateKindPressure

	// PolicyUpdateKindTrim means the diff touches bounded trim policy.
	PolicyUpdateKindTrim

	// PolicyUpdateKindOwnership means the diff touches checked-out buffer
	// ownership, accounting, or release-validation semantics.
	PolicyUpdateKindOwnership
)

// String returns a stable diagnostic label for k.
func (k PolicyUpdateKind) String() string {
	switch k {
	case PolicyUpdateKindNone:
		return "none"
	case PolicyUpdateKindShape:
		return "shape"
	case PolicyUpdateKindRetention:
		return "retention"
	case PolicyUpdateKindAdmission:
		return "admission"
	case PolicyUpdateKindPressure:
		return "pressure"
	case PolicyUpdateKindTrim:
		return "trim"
	case PolicyUpdateKindOwnership:
		return "ownership"
	default:
		return "unknown"
	}
}

// PolicyUpdateScope identifies the owner shape a compatibility check targets.
//
// Scope deliberately mirrors construction responsibility rather than public API
// names. Pool is the data-plane owner; PoolPartition and PoolGroup publish
// policy values through explicit foreground control paths above it.
type PolicyUpdateScope uint8

const (
	// PolicyUpdateScopeUnset means no live-update owner scope was selected.
	PolicyUpdateScopeUnset PolicyUpdateScope = iota

	// PolicyUpdateScopeStandalonePool describes direct Pool use with raw Get/Put.
	PolicyUpdateScopeStandalonePool

	// PolicyUpdateScopePartitionOwnedPool describes a Pool whose acquisition and
	// release are mediated by PoolPartition and LeaseRegistry.
	PolicyUpdateScopePartitionOwnedPool
)

// String returns a stable diagnostic label for s.
func (s PolicyUpdateScope) String() string {
	switch s {
	case PolicyUpdateScopeUnset:
		return "unset"
	case PolicyUpdateScopeStandalonePool:
		return "standalone_pool"
	case PolicyUpdateScopePartitionOwnedPool:
		return "partition_owned_pool"
	default:
		return "unknown"
	}
}

// PolicyUpdateDiffDiagnostics summarizes which policy sections changed.
//
// This type is intentionally flat so publication reports can embed it without
// forcing callers to understand every individual field in Policy. A section flag
// means the value changed; it does not mean the change is live-compatible.
type PolicyUpdateDiffDiagnostics struct {
	// ShapeChanged reports construction-time class, shard, bucket, selector, or
	// fallback-probing changes.
	ShapeChanged bool

	// RetentionChanged reports byte or buffer retention-limit changes.
	RetentionChanged bool

	// RetentionContracted reports that at least one retention or closely related
	// request/capacity admission limit became more restrictive.
	//
	// The current policy structure keeps retained-memory limits,
	// MaxRequestSize, and MaxRetainedBufferCapacity in RetentionPolicy because
	// all three restrict future retained-buffer acceptance. Reports keep one
	// compatibility flag for that section instead of splitting the public result
	// vocabulary into request, capacity, and retained-memory subflags.
	RetentionContracted bool

	// RetentionExpanded reports that at least one retention limit became less
	// restrictive.
	RetentionExpanded bool

	// AdmissionChanged reports return-path admission policy changes.
	AdmissionChanged bool

	// PressureChanged reports pressure contraction policy changes.
	PressureChanged bool

	// TrimChanged reports bounded trim policy changes.
	TrimChanged bool

	// OwnershipChanged reports ownership/accounting policy changes.
	OwnershipChanged bool

	// NeedsTrim reports that the candidate contracts retention and enables trim
	// on policy shrink. It is a scheduling hint for future owner publication
	// paths, not an instruction to run trim from the policy layer.
	NeedsTrim bool
}

// IsZero reports whether d contains no policy section changes.
func (d PolicyUpdateDiffDiagnostics) IsZero() bool {
	return !d.ShapeChanged &&
		!d.RetentionChanged &&
		!d.RetentionContracted &&
		!d.RetentionExpanded &&
		!d.AdmissionChanged &&
		!d.PressureChanged &&
		!d.TrimChanged &&
		!d.OwnershipChanged &&
		!d.NeedsTrim
}

// PolicyUpdateDiff describes the value-level difference between two policies.
//
// Policy is not runtime state. A diff only says what changed between values; it
// does not publish those values, mutate budgets, execute trim, rebuild buckets,
// or alter checked-out lease accounting.
type PolicyUpdateDiff struct {
	// Diagnostics contains section-level change flags suitable for reports.
	Diagnostics PolicyUpdateDiffDiagnostics
}

// IsZero reports whether d describes no policy changes.
func (d PolicyUpdateDiff) IsZero() bool {
	return d.Diagnostics.IsZero()
}

// PolicyUpdateCompatibility reports whether a candidate policy can be published
// live in a selected owner scope.
//
// Compatibility is deliberately stricter than low-level Pool topology
// compatibility. A field can technically fit inside an existing Pool runtime
// snapshot and still be rejected here until a runtime owner has a deliberate,
// report-first publication flow for that transition.
type PolicyUpdateCompatibility struct {
	// Scope is the owner mode used for the compatibility decision.
	Scope PolicyUpdateScope

	// Diff describes the value sections changed by the candidate.
	Diff PolicyUpdateDiff

	// Compatible is true only when the candidate is valid for the selected owner
	// and changes only live-updatable policy sections.
	Compatible bool

	// RejectedKind identifies the policy section that made the update
	// incompatible.
	RejectedKind PolicyUpdateKind

	// FailureReason is a stable machine-readable reason for rejection.
	FailureReason string
}

// PolicyPublicationStatus identifies the coarse outcome of a policy publication
// attempt.
//
// The status is shared report vocabulary. Owner-specific publication code
// populates it after validation, lifecycle admission, runtime publication, and
// any child publication state are known.
type PolicyPublicationStatus uint8

const (
	// PolicyPublicationStatusUnset means no publication was attempted.
	PolicyPublicationStatusUnset PolicyPublicationStatus = iota

	// PolicyPublicationStatusRejected means compatibility, validation, lifecycle,
	// or feasibility checks rejected the update before runtime publication.
	PolicyPublicationStatusRejected

	// PolicyPublicationStatusPublished means the owner accepted and published the
	// policy value into runtime state.
	PolicyPublicationStatusPublished

	// PolicyPublicationStatusPartial means some child runtime owners accepted the
	// publication and some did not; reports must describe the partial state.
	PolicyPublicationStatusPartial
)

// String returns a stable diagnostic label for s.
func (s PolicyPublicationStatus) String() string {
	switch s {
	case PolicyPublicationStatusUnset:
		return "unset"
	case PolicyPublicationStatusRejected:
		return "rejected"
	case PolicyPublicationStatusPublished:
		return "published"
	case PolicyPublicationStatusPartial:
		return "partial"
	default:
		return "unknown"
	}
}

// PolicyPublicationDiagnostics carries reusable fields for policy publication
// reports.
//
// The type keeps owner-specific publication APIs from hiding policy contraction,
// runtime publication, trim intent, or failure reason behind a bare error
// return.
type PolicyPublicationDiagnostics struct {
	// PreviousGeneration is the runtime policy generation observed before the
	// attempted publication.
	PreviousGeneration Generation

	// Generation is the generation assigned to the attempted or completed
	// publication.
	Generation Generation

	// Status is the coarse publication outcome.
	Status PolicyPublicationStatus

	// Published reports whether the owner accepted the candidate as the effective
	// policy value.
	Published bool

	// RuntimePublished reports whether an immutable runtime snapshot was actually
	// published.
	RuntimePublished bool

	// Contracted reports whether the candidate reduced future retention limits.
	Contracted bool

	// TrimAttempted reports whether bounded cleanup was attempted as part of the
	// publication flow.
	TrimAttempted bool

	// FailureReason is a stable machine-readable rejection or partial-publication
	// reason. It is empty when publication fully succeeds.
	FailureReason string
}

// validateLivePolicyUpdate validates that next can replace previous through a
// live owner-published policy update.
//
// Shape policy is construction-time by default. Class sizes, shard count,
// selector mode, bucket shape, and fallback probing change the layout or
// operation state built by Pool construction, so they are rejected here until a
// later owner explicitly supports rebuilding or transitioning that shape.
//
// Ownership policy is also construction-time by default. It affects active lease
// records, in-use counters, double-release detection, and capacity-growth
// validation. Changing it while leases may exist would make old and new records
// ambiguous, so this foundation rejects it rather than silently switching modes.
//
// Retention contraction only changes future admission and budget targets. It
// does not force reclaim checked-out buffers; owner publication paths must pair
// contraction with bounded trim if physical retained storage needs correction.
func validateLivePolicyUpdate(previous Policy, next Policy, mode poolConstructionMode) error {
	compatibility := checkLivePolicyUpdateCompatibility(previous, next, mode)
	if compatibility.Compatible {
		return nil
	}

	reason := compatibility.FailureReason
	if reason == "" {
		reason = policyUpdateFailureInvalid
	}
	return newError(ErrInvalidPolicy, reason)
}

// checkLivePolicyUpdateCompatibility returns the report form used by tests and
// later owner-specific publication code.
//
// The function performs no runtime mutation. It validates only policy values and
// live-update boundaries, keeping publication responsibility with Pool,
// PoolPartition, and PoolGroup.
func checkLivePolicyUpdateCompatibility(previous Policy, next Policy, mode poolConstructionMode) PolicyUpdateCompatibility {
	scope, ok := policyUpdateScopeForPoolConstructionMode(mode)
	compatibility := PolicyUpdateCompatibility{
		Scope: scope,
		Diff:  classifyPolicyUpdate(previous, next),
	}
	if !ok {
		compatibility.RejectedKind = PolicyUpdateKindNone
		compatibility.FailureReason = policyUpdateFailureInvalid
		return compatibility
	}

	if err := previous.Validate(); err != nil {
		compatibility.RejectedKind = PolicyUpdateKindNone
		compatibility.FailureReason = policyUpdateFailureInvalid
		return compatibility
	}
	if err := next.Validate(); err != nil {
		compatibility.RejectedKind = PolicyUpdateKindNone
		compatibility.FailureReason = policyUpdateFailureInvalid
		return compatibility
	}

	if compatibility.Diff.Diagnostics.ShapeChanged {
		compatibility.RejectedKind = PolicyUpdateKindShape
		compatibility.FailureReason = policyUpdateFailureShapeChange
		return compatibility
	}
	if compatibility.Diff.Diagnostics.OwnershipChanged {
		compatibility.RejectedKind = PolicyUpdateKindOwnership
		compatibility.FailureReason = policyUpdateFailureOwnershipChange
		return compatibility
	}

	if err := validatePoolSupportedPolicy(previous, mode); err != nil {
		compatibility.RejectedKind = PolicyUpdateKindNone
		compatibility.FailureReason = policyUpdateFailureInvalid
		return compatibility
	}
	if err := validatePoolSupportedPolicy(next, mode); err != nil {
		compatibility.RejectedKind = PolicyUpdateKindNone
		compatibility.FailureReason = policyUpdateFailureInvalid
		return compatibility
	}

	compatibility.Compatible = true
	return compatibility
}

// classifyPolicyUpdate returns a deterministic section-level diff.
//
// The helper compares policy values only. It deliberately does not call
// Validate because diagnostics should still identify which section changed even
// when a later validation step rejects the candidate.
func classifyPolicyUpdate(previous Policy, next Policy) PolicyUpdateDiff {
	diagnostics := PolicyUpdateDiffDiagnostics{
		ShapeChanged:        policyUpdateChangesShape(previous, next),
		RetentionChanged:    previous.Retention != next.Retention,
		RetentionContracted: policyUpdateContractsRetention(previous, next),
		RetentionExpanded:   policyUpdateExpandsRetention(previous, next),
		AdmissionChanged:    previous.Admission != next.Admission,
		PressureChanged:     previous.Pressure != next.Pressure,
		TrimChanged:         previous.Trim != next.Trim,
		OwnershipChanged:    policyUpdateChangesOwnership(previous, next),
	}
	diagnostics.NeedsTrim = diagnostics.RetentionContracted &&
		next.Trim.Enabled &&
		next.Trim.TrimOnPolicyShrink

	return PolicyUpdateDiff{Diagnostics: diagnostics}
}

// policyUpdateChangesShape reports whether two policies differ in fields that
// are construction-time for the current runtime model.
func policyUpdateChangesShape(previous Policy, next Policy) bool {
	return !classPolicyEqual(previous.Classes, next.Classes) ||
		previous.Shards != next.Shards
}

// policyUpdateChangesOwnership reports whether two policies differ in lease or
// checked-out buffer accounting semantics.
func policyUpdateChangesOwnership(previous Policy, next Policy) bool {
	return previous.Ownership != next.Ownership
}

// policyUpdateContractsRetention reports whether next reduces any retention
// limit compared with previous.
//
// A contraction restricts future retention and may require bounded trim to make
// already-retained storage converge. It never implies forced reclamation of
// active leases or checked-out buffers.
func policyUpdateContractsRetention(previous Policy, next Policy) bool {
	return sizeDecreased(previous.Retention.SoftRetainedBytes, next.Retention.SoftRetainedBytes) ||
		sizeDecreased(previous.Retention.HardRetainedBytes, next.Retention.HardRetainedBytes) ||
		uint64Decreased(previous.Retention.MaxRetainedBuffers, next.Retention.MaxRetainedBuffers) ||
		sizeDecreased(previous.Retention.MaxRequestSize, next.Retention.MaxRequestSize) ||
		sizeDecreased(previous.Retention.MaxRetainedBufferCapacity, next.Retention.MaxRetainedBufferCapacity) ||
		sizeDecreased(previous.Retention.MaxClassRetainedBytes, next.Retention.MaxClassRetainedBytes) ||
		uint64Decreased(previous.Retention.MaxClassRetainedBuffers, next.Retention.MaxClassRetainedBuffers) ||
		sizeDecreased(previous.Retention.MaxShardRetainedBytes, next.Retention.MaxShardRetainedBytes) ||
		uint64Decreased(previous.Retention.MaxShardRetainedBuffers, next.Retention.MaxShardRetainedBuffers)
}

// policyUpdateExpandsRetention reports whether next raises any retention limit
// compared with previous.
func policyUpdateExpandsRetention(previous Policy, next Policy) bool {
	return sizeIncreased(previous.Retention.SoftRetainedBytes, next.Retention.SoftRetainedBytes) ||
		sizeIncreased(previous.Retention.HardRetainedBytes, next.Retention.HardRetainedBytes) ||
		uint64Increased(previous.Retention.MaxRetainedBuffers, next.Retention.MaxRetainedBuffers) ||
		sizeIncreased(previous.Retention.MaxRequestSize, next.Retention.MaxRequestSize) ||
		sizeIncreased(previous.Retention.MaxRetainedBufferCapacity, next.Retention.MaxRetainedBufferCapacity) ||
		sizeIncreased(previous.Retention.MaxClassRetainedBytes, next.Retention.MaxClassRetainedBytes) ||
		uint64Increased(previous.Retention.MaxClassRetainedBuffers, next.Retention.MaxClassRetainedBuffers) ||
		sizeIncreased(previous.Retention.MaxShardRetainedBytes, next.Retention.MaxShardRetainedBytes) ||
		uint64Increased(previous.Retention.MaxShardRetainedBuffers, next.Retention.MaxShardRetainedBuffers)
}

// policyUpdateNeedsTrim reports whether the candidate asks owner publication to
// schedule bounded cleanup after retention contraction.
func policyUpdateNeedsTrim(previous Policy, next Policy) bool {
	return classifyPolicyUpdate(previous, next).Diagnostics.NeedsTrim
}

// policyUpdateScopeForPoolConstructionMode maps Pool construction mode into
// policy-update report vocabulary.
func policyUpdateScopeForPoolConstructionMode(mode poolConstructionMode) (PolicyUpdateScope, bool) {
	switch mode {
	case poolConstructionModeStandalone:
		return PolicyUpdateScopeStandalonePool, true
	case poolConstructionModePartitionOwned:
		return PolicyUpdateScopePartitionOwnedPool, true
	default:
		return PolicyUpdateScopeUnset, false
	}
}

// classPolicyEqual compares class policy without treating nil and empty slices
// as different shapes. Both mean "no class profile" before validation.
func classPolicyEqual(left ClassPolicy, right ClassPolicy) bool {
	if len(left.Sizes) != len(right.Sizes) {
		return false
	}
	for index := range left.Sizes {
		if left.Sizes[index] != right.Sizes[index] {
			return false
		}
	}
	return true
}

// sizeDecreased reports whether next is lower than previous.
func sizeDecreased(previous Size, next Size) bool {
	return next < previous
}

// sizeIncreased reports whether next is higher than previous.
func sizeIncreased(previous Size, next Size) bool {
	return next > previous
}

// uint64Decreased reports whether next is lower than previous.
func uint64Decreased(previous uint64, next uint64) bool {
	return next < previous
}

// uint64Increased reports whether next is higher than previous.
func uint64Increased(previous uint64, next uint64) bool {
	return next > previous
}
