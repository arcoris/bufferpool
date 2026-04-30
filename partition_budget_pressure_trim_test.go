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
	"reflect"
	"testing"
)

// TestPartitionBudgetPolicyValidate verifies partition budget relationships.
func TestPartitionBudgetPolicyValidate(t *testing.T) {
	tests := []struct {
		name   string
		policy PartitionBudgetPolicy
		wantOK bool
	}{
		{name: "zero unbounded", policy: PartitionBudgetPolicy{}, wantOK: true},
		{name: "retained under owned", policy: PartitionBudgetPolicy{MaxRetainedBytes: MiB, MaxOwnedBytes: 2 * MiB}, wantOK: true},
		{name: "active under owned", policy: PartitionBudgetPolicy{MaxActiveBytes: MiB, MaxOwnedBytes: 2 * MiB}, wantOK: true},
		{name: "retained exceeds owned", policy: PartitionBudgetPolicy{MaxRetainedBytes: 2 * MiB, MaxOwnedBytes: MiB}, wantOK: false},
		{name: "active exceeds owned", policy: PartitionBudgetPolicy{MaxActiveBytes: 2 * MiB, MaxOwnedBytes: MiB}, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if tt.wantOK {
				requirePartitionNoError(t, err)
				return
			}
			if !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("Validate() error = %v, want ErrInvalidPolicy", err)
			}
		})
	}
}

// TestPartitionBudgetSnapshotDetectsExceededLimits verifies budget projection flags.
func TestPartitionBudgetSnapshotDetectsExceededLimits(t *testing.T) {
	policy := PartitionBudgetPolicy{MaxRetainedBytes: 64, MaxActiveBytes: 64, MaxOwnedBytes: 100}
	sample := PoolPartitionSample{CurrentRetainedBytes: 70, CurrentActiveBytes: 40, CurrentOwnedBytes: 110}

	snapshot := newPartitionBudgetSnapshot(policy, sample)

	if !snapshot.RetainedOverBudget {
		t.Fatalf("RetainedOverBudget = false, want true")
	}
	if snapshot.ActiveOverBudget {
		t.Fatalf("ActiveOverBudget = true, want false")
	}
	if !snapshot.OwnedOverBudget {
		t.Fatalf("OwnedOverBudget = false, want true")
	}
	if !snapshot.IsOverBudget() {
		t.Fatalf("IsOverBudget() = false, want true")
	}
}

// TestPoolPartitionBudgetUsesCurrentSample verifies live active-byte budget sampling.
func TestPoolPartitionBudgetUsesCurrentSample(t *testing.T) {
	config := testPartitionConfig("primary")
	config.Policy.Budget = PartitionBudgetPolicy{MaxActiveBytes: 128}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	defer func() { requirePartitionNoError(t, partition.Release(lease, lease.Buffer())) }()

	budget := partition.Budget()
	if !budget.ActiveOverBudget {
		t.Fatalf("ActiveOverBudget = false, want true for active capacity %d over limit %d", budget.CurrentActiveBytes, budget.MaxActiveBytes)
	}
}

// TestPartitionPressurePolicyValidateAndSnapshot verifies threshold validation and mapping.
func TestPartitionPressurePolicyValidateAndSnapshot(t *testing.T) {
	if err := (PartitionPressurePolicy{Enabled: true}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("enabled pressure without thresholds error = %v, want ErrInvalidPolicy", err)
	}
	if err := (PartitionPressurePolicy{Enabled: true, MediumOwnedBytes: 2 * MiB, HighOwnedBytes: MiB}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("inverted medium/high pressure thresholds error = %v, want ErrInvalidPolicy", err)
	}
	if err := (PartitionPressurePolicy{Enabled: true, MediumOwnedBytes: 2 * MiB, CriticalOwnedBytes: MiB}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("inverted medium/critical pressure thresholds error = %v, want ErrInvalidPolicy", err)
	}
	if err := (PartitionPressurePolicy{Enabled: true, HighOwnedBytes: 2 * MiB, CriticalOwnedBytes: MiB}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("inverted pressure thresholds error = %v, want ErrInvalidPolicy", err)
	}

	policy := PartitionPressurePolicy{Enabled: true, MediumOwnedBytes: 50, HighOwnedBytes: 100, CriticalOwnedBytes: 200}
	if err := policy.Validate(); err != nil {
		t.Fatalf("valid pressure policy error = %v", err)
	}

	if got := newPartitionPressureSnapshot(policy, PoolPartitionSample{CurrentOwnedBytes: 49}).Level; got != PressureLevelNormal {
		t.Fatalf("pressure level at 49 = %s, want normal", got)
	}
	if got := newPartitionPressureSnapshot(policy, PoolPartitionSample{CurrentOwnedBytes: 50}).Level; got != PressureLevelMedium {
		t.Fatalf("pressure level at 50 = %s, want medium", got)
	}
	if got := newPartitionPressureSnapshot(policy, PoolPartitionSample{CurrentOwnedBytes: 100}).Level; got != PressureLevelHigh {
		t.Fatalf("pressure level at 100 = %s, want high", got)
	}
	if got := newPartitionPressureSnapshot(policy, PoolPartitionSample{CurrentOwnedBytes: 200}).Level; got != PressureLevelCritical {
		t.Fatalf("pressure level at 200 = %s, want critical", got)
	}
}

// TestPartitionPressurePolicyPartialThresholds verifies optional pressure levels.
func TestPartitionPressurePolicyPartialThresholds(t *testing.T) {
	tests := []struct {
		name   string
		policy PartitionPressurePolicy
		owned  uint64
		want   PressureLevel
	}{
		{name: "only medium below", policy: PartitionPressurePolicy{Enabled: true, MediumOwnedBytes: 50}, owned: 49, want: PressureLevelNormal},
		{name: "only medium reached", policy: PartitionPressurePolicy{Enabled: true, MediumOwnedBytes: 50}, owned: 50, want: PressureLevelMedium},
		{name: "only high below", policy: PartitionPressurePolicy{Enabled: true, HighOwnedBytes: 100}, owned: 99, want: PressureLevelNormal},
		{name: "only high reached", policy: PartitionPressurePolicy{Enabled: true, HighOwnedBytes: 100}, owned: 100, want: PressureLevelHigh},
		{name: "only critical below", policy: PartitionPressurePolicy{Enabled: true, CriticalOwnedBytes: 200}, owned: 199, want: PressureLevelNormal},
		{name: "only critical reached", policy: PartitionPressurePolicy{Enabled: true, CriticalOwnedBytes: 200}, owned: 200, want: PressureLevelCritical},
		{name: "medium and critical middle", policy: PartitionPressurePolicy{Enabled: true, MediumOwnedBytes: 50, CriticalOwnedBytes: 200}, owned: 100, want: PressureLevelMedium},
		{name: "medium and critical critical", policy: PartitionPressurePolicy{Enabled: true, MediumOwnedBytes: 50, CriticalOwnedBytes: 200}, owned: 200, want: PressureLevelCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requirePartitionNoError(t, tt.policy.Validate())

			got := newPartitionPressureSnapshot(tt.policy, PoolPartitionSample{CurrentOwnedBytes: tt.owned}).Level
			if got != tt.want {
				t.Fatalf("pressure level = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestPartitionTrimPolicyValidateNormalizeAndPlan verifies trim policy semantics.
func TestPartitionTrimPolicyValidateNormalizeAndPlan(t *testing.T) {
	policy := PartitionTrimPolicy{Enabled: true, MaxBytesPerCycle: 4 * KiB}
	normalized := policy.Normalize()
	if normalized.MaxPoolsPerCycle != defaultPartitionTrimMaxPoolsPerCycle {
		t.Fatalf("MaxPoolsPerCycle = %d, want %d", normalized.MaxPoolsPerCycle, defaultPartitionTrimMaxPoolsPerCycle)
	}
	requirePartitionNoError(t, normalized.Validate())

	if err := (PartitionTrimPolicy{Enabled: true}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("enabled trim without max bytes error = %v, want ErrInvalidPolicy", err)
	}
	if err := (PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 1, MaxBytesPerCycle: 4 * KiB}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("enabled trim without max buffers error = %v, want ErrInvalidPolicy", err)
	}

	disabled := newPartitionTrimPlan(PartitionTrimPolicy{}, PartitionPressureSnapshot{}, PoolPartitionSample{})
	if disabled.Enabled || disabled.Reason != "trim_disabled" {
		t.Fatalf("disabled trim plan = %+v, want trim_disabled", disabled)
	}

	pressure := PartitionPressureSnapshot{Enabled: true, Level: PressureLevelHigh}
	plan := newPartitionTrimPlan(PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 2, MaxBytesPerCycle: 8 * KiB, TrimOnPressure: true}, pressure, PoolPartitionSample{PoolCount: 3})
	if !plan.Enabled || plan.Reason != "pressure" || plan.CandidatePools != 3 {
		t.Fatalf("pressure trim plan = %+v", plan)
	}
	if plan.MaxPoolsPerCycle != 2 || plan.MaxBytesPerCycle != (8*KiB).Bytes() {
		t.Fatalf("trim plan limits = %+v", plan)
	}

	noPressure := newPartitionTrimPlan(PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 2, MaxBytesPerCycle: 8 * KiB, TrimOnPressure: true}, PartitionPressureSnapshot{Enabled: true, Level: PressureLevelNormal}, PoolPartitionSample{PoolCount: 3})
	if noPressure.Enabled || noPressure.Reason != "no_pressure" {
		t.Fatalf("normal-pressure trim plan = %+v, want disabled no_pressure", noPressure)
	}

	mediumPressure := newPartitionTrimPlan(PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 2, MaxBytesPerCycle: 8 * KiB, TrimOnPressure: true}, PartitionPressureSnapshot{Enabled: true, Level: PressureLevelMedium}, PoolPartitionSample{PoolCount: 3})
	if !mediumPressure.Enabled || mediumPressure.Reason != "pressure" {
		t.Fatalf("medium-pressure trim plan = %+v, want pressure", mediumPressure)
	}
}

// TestPoolPartitionPlanAndExecuteTrim verifies bounded physical trim execution.
func TestPoolPartitionPlanAndExecuteTrim(t *testing.T) {
	config := testPartitionConfig("primary")
	config.Policy.Trim = PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 1, MaxBytesPerCycle: 4 * KiB}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
	retainedBefore := partition.Metrics().CurrentRetainedBytes
	if retainedBefore == 0 {
		t.Fatalf("test requires retained bytes before ExecuteTrim")
	}
	partition.activeRegistry.resetDirty()

	plan := partition.PlanTrim()
	if !plan.Enabled || plan.Reason != "policy" {
		t.Fatalf("PlanTrim() = %+v, want enabled policy plan", plan)
	}

	result := partition.ExecuteTrim()
	if !result.Attempted {
		t.Fatalf("ExecuteTrim().Attempted = false, want true")
	}
	if !result.Executed || result.Reason != "policy" || result.VisitedPools != 1 || result.TrimmedBuffers == 0 || result.TrimmedBytes == 0 {
		t.Fatalf("ExecuteTrim() = %+v, want bounded physical trim", result)
	}
	retainedAfter := partition.Metrics().CurrentRetainedBytes
	if retainedAfter >= retainedBefore {
		t.Fatalf("retained bytes after ExecuteTrim = %d, want below %d", retainedAfter, retainedBefore)
	}
	if dirty := partition.activeRegistry.dirtyIndexes(nil); len(dirty) != 1 || dirty[0] != 0 {
		t.Fatalf("ExecuteTrim dirty indexes = %v, want trimmed pool marked dirty", dirty)
	}

	disabled := testNewPoolPartition(t, "secondary")
	disabledResult := disabled.ExecuteTrim()
	if disabledResult.Attempted || disabledResult.Executed || disabledResult.Reason != "trim_disabled" {
		t.Fatalf("disabled ExecuteTrim() = %+v, want trim_disabled without execution", disabledResult)
	}
}

func TestPartitionTrimPrefersOverTargetPool(t *testing.T) {
	t.Parallel()

	config := testPartitionConfig("alpha", "beta")
	config.Policy.Trim = PartitionTrimPolicy{
		Enabled:                   true,
		MaxPoolsPerCycle:          1,
		MaxBuffersPerCycle:        1,
		MaxBytesPerCycle:          KiB,
		MaxClassesPerPoolPerCycle: 1,
		MaxShardsPerClassPerCycle: 1,
	}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	alpha, _ := partition.pool("alpha")
	beta, _ := partition.pool("beta")
	seedPoolRetainedBuffers(t, alpha, 1, 512)
	seedPoolRetainedBuffers(t, beta, 1, 512)
	retainedClassID := firstRetainedPoolClassID(t, beta)
	if _, err := alpha.applyClassBudgets([]ClassBudgetTarget{{Generation: Generation(30), ClassID: retainedClassID, TargetBytes: 4 * KiB}}); err != nil {
		t.Fatalf("alpha applyClassBudgets() error = %v", err)
	}
	if _, err := beta.applyClassBudgets([]ClassBudgetTarget{{Generation: Generation(30), ClassID: retainedClassID, TargetBytes: 0}}); err != nil {
		t.Fatalf("beta applyClassBudgets() error = %v", err)
	}

	result := partition.ExecuteTrim()
	if !result.Executed || len(result.CandidatePools) == 0 || result.CandidatePools[0].PoolName != "beta" {
		t.Fatalf("ExecuteTrim() = %+v, want beta selected first", result)
	}
	if beta.Metrics().CurrentRetainedBuffers != 0 || alpha.Metrics().CurrentRetainedBuffers != 1 {
		t.Fatalf("retained after partition trim = alpha %d beta %d, want beta trimmed",
			alpha.Metrics().CurrentRetainedBuffers,
			beta.Metrics().CurrentRetainedBuffers,
		)
	}
}

func TestPartitionTrimCandidateOrderingDeterministic(t *testing.T) {
	t.Parallel()

	config := testPartitionConfig("alpha", "beta")
	config.Policy.Trim = PartitionTrimPolicy{
		Enabled:                   true,
		MaxPoolsPerCycle:          1,
		MaxBuffersPerCycle:        1,
		MaxBytesPerCycle:          KiB,
		MaxClassesPerPoolPerCycle: 1,
		MaxShardsPerClassPerCycle: 1,
	}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	alpha, _ := partition.pool("alpha")
	beta, _ := partition.pool("beta")
	seedPoolRetainedBuffers(t, alpha, 1, 512)
	seedPoolRetainedBuffers(t, beta, 1, 512)
	retainedClassID := firstRetainedPoolClassID(t, beta)
	if _, err := alpha.applyClassBudgets([]ClassBudgetTarget{{Generation: Generation(31), ClassID: retainedClassID, TargetBytes: 4 * KiB}}); err != nil {
		t.Fatalf("alpha applyClassBudgets() error = %v", err)
	}
	if _, err := beta.applyClassBudgets([]ClassBudgetTarget{{Generation: Generation(31), ClassID: retainedClassID, TargetBytes: 0}}); err != nil {
		t.Fatalf("beta applyClassBudgets() error = %v", err)
	}

	first := partitionTrimCandidateReports(partition.partitionTrimCandidates())
	second := partitionTrimCandidateReports(partition.partitionTrimCandidates())
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("candidate order is not deterministic: first=%+v second=%+v", first, second)
	}
	if len(first) == 0 || first[0].PoolName != "beta" {
		t.Fatalf("candidate order = %+v, want over-target beta first", first)
	}
}

func firstRetainedPoolClassID(t *testing.T, pool *Pool) ClassID {
	t.Helper()
	for _, class := range pool.Snapshot().Classes {
		if class.CurrentRetainedBytes > 0 {
			return class.Class.ID()
		}
	}
	t.Fatal("pool has no retained class")
	return ClassID(0)
}

func TestPoolPartitionExecuteTrimNeverTouchesActiveLeases(t *testing.T) {
	config := testPartitionConfig("primary")
	config.Policy.Trim = PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 1, MaxBytesPerCycle: 4 * KiB}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	active, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	retained, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Release(retained, retained.Buffer()))

	before := partition.Metrics()
	if before.ActiveLeases != 1 || before.CurrentRetainedBytes == 0 {
		t.Fatalf("setup metrics = %+v, want one active lease and retained bytes", before)
	}

	result := partition.ExecuteTrim()
	if !result.Executed || result.TrimmedBytes == 0 {
		t.Fatalf("ExecuteTrim() = %+v, want retained trim", result)
	}
	after := partition.Metrics()
	if after.ActiveLeases != 1 || after.CurrentActiveBytes != before.CurrentActiveBytes {
		t.Fatalf("active metrics after trim = %+v, before %+v", after, before)
	}

	requirePartitionNoError(t, partition.Release(active, active.Buffer()))
	if final := partition.Metrics(); final.ActiveLeases != 0 {
		t.Fatalf("final ActiveLeases = %d, want 0", final.ActiveLeases)
	}
}

func TestPoolPartitionPressureSignalSchedulesTrim(t *testing.T) {
	config := testPartitionConfig("primary")
	config.Policy.Trim = PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 1, MaxBytesPerCycle: 4 * KiB, TrimOnPressure: true}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
	requirePartitionNoError(t, partition.SetPressure(PressureLevelHigh))

	plan := partition.PlanTrim()
	if !plan.Enabled || plan.Reason != "pressure" || plan.PressureLevel != PressureLevelHigh {
		t.Fatalf("PlanTrim() = %+v, want pressure trim from runtime signal", plan)
	}
	result := partition.ExecuteTrim()
	if !result.Executed || result.TrimmedBytes == 0 {
		t.Fatalf("ExecuteTrim() = %+v, want pressure-triggered physical trim", result)
	}
}

// TestPartitionTrimRespectsMaxBuffersPerCycle verifies partition trim cannot
// remove more retained buffers than the configured cycle bound.
func TestPartitionTrimRespectsMaxBuffersPerCycle(t *testing.T) {
	config := testPartitionConfig("primary")
	config.Policy.Trim = PartitionTrimPolicy{
		Enabled:                   true,
		MaxPoolsPerCycle:          1,
		MaxBuffersPerCycle:        1,
		MaxBytesPerCycle:          16 * KiB,
		MaxClassesPerPoolPerCycle: 64,
		MaxShardsPerClassPerCycle: 32,
	}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	seedPartitionRetainedBuffers(t, partition, "primary", 3, 300)

	result := partition.ExecuteTrim()
	if result.TrimmedBuffers > 1 {
		t.Fatalf("TrimmedBuffers = %d, want <= 1", result.TrimmedBuffers)
	}
}

// TestPartitionTrimRespectsMaxBytesPerCycle verifies byte bounds stop physical
// trim work in the same cycle.
func TestPartitionTrimRespectsMaxBytesPerCycle(t *testing.T) {
	config := testPartitionConfig("primary")
	config.Policy.Trim = PartitionTrimPolicy{
		Enabled:                   true,
		MaxPoolsPerCycle:          1,
		MaxBuffersPerCycle:        64,
		MaxBytesPerCycle:          SizeFromBytes(512),
		MaxClassesPerPoolPerCycle: 64,
		MaxShardsPerClassPerCycle: 32,
	}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	seedPartitionRetainedBuffers(t, partition, "primary", 3, 300)

	result := partition.ExecuteTrim()
	if result.TrimmedBytes > 512 {
		t.Fatalf("TrimmedBytes = %d, want <= 512", result.TrimmedBytes)
	}
}

// TestPartitionTrimRespectsMaxPoolsPerCycle verifies partition trim does not
// visit more Pools than the configured bound.
func TestPartitionTrimRespectsMaxPoolsPerCycle(t *testing.T) {
	config := testPartitionConfig("alpha", "beta")
	config.Policy.Trim = PartitionTrimPolicy{
		Enabled:                   true,
		MaxPoolsPerCycle:          1,
		MaxBuffersPerCycle:        64,
		MaxBytesPerCycle:          16 * KiB,
		MaxClassesPerPoolPerCycle: 64,
		MaxShardsPerClassPerCycle: 32,
	}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	seedPartitionRetainedBuffers(t, partition, "alpha", 1, 300)
	seedPartitionRetainedBuffers(t, partition, "beta", 1, 300)

	result := partition.ExecuteTrim()
	if result.VisitedPools > 1 {
		t.Fatalf("VisitedPools = %d, want <= 1", result.VisitedPools)
	}
}

// TestPartitionTrimPassesClassAndShardBoundsToPoolTrim verifies class and shard
// visit bounds are propagated into Pool.Trim instead of being advisory only.
func TestPartitionTrimPassesClassAndShardBoundsToPoolTrim(t *testing.T) {
	config := testPartitionConfig("primary")
	config.Policy.Trim = PartitionTrimPolicy{
		Enabled:                   true,
		MaxPoolsPerCycle:          1,
		MaxBuffersPerCycle:        64,
		MaxBytesPerCycle:          16 * KiB,
		MaxClassesPerPoolPerCycle: 1,
		MaxShardsPerClassPerCycle: 1,
	}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	seedPartitionRetainedBuffers(t, partition, "primary", 2, 300)

	result := partition.ExecuteTrim()
	if result.VisitedClasses > 1 {
		t.Fatalf("VisitedClasses = %d, want <= 1", result.VisitedClasses)
	}
	if result.VisitedShards > 1 {
		t.Fatalf("VisitedShards = %d, want <= 1", result.VisitedShards)
	}
}

// seedPartitionRetainedBuffers creates retained storage through managed
// Acquire/Release so trim tests exercise LeaseRegistry-compatible state.
func seedPartitionRetainedBuffers(t *testing.T, partition *PoolPartition, poolName string, count int, size int) {
	t.Helper()
	for index := 0; index < count; index++ {
		lease, err := partition.Acquire(poolName, size)
		requirePartitionNoError(t, err)
		requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
	}
}
