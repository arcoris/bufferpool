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
	controlnumeric "arcoris.dev/bufferpool/internal/control/numeric"
	controlrate "arcoris.dev/bufferpool/internal/control/rate"
	controlscore "arcoris.dev/bufferpool/internal/control/score"
)

const (
	poolClassScoreComponentActivity            = "activity"
	poolClassScoreComponentHitRatio            = "hit_ratio"
	poolClassScoreComponentAllocationAvoidance = "allocation_avoidance"
	poolClassScoreComponentRetainRatio         = "retain_ratio"
	poolClassScoreComponentWasteAvoidance      = "waste_avoidance"
	poolClassScoreComponentRiskAvoidance       = "risk_avoidance"
	poolClassScoreComponentNeutralMissing      = "neutral_missing_score"

	poolClassScoreActivityWeight            = 0.25
	poolClassScoreHitRatioWeight            = 0.25
	poolClassScoreAllocationAvoidanceWeight = 0.20
	poolClassScoreRetainRatioWeight         = 0.10
	poolClassScoreWasteAvoidanceWeight      = 0.15
	poolClassScoreRiskAvoidanceWeight       = 0.05

	poolClassScoreActivityScale = 16
)

const (
	poolBudgetScoreComponentClass = "class"
)

// RuntimeScoreComponent is one named, finite, weighted score signal.
//
// Runtime scores are explainable control-plane projections. They summarize
// observed windows, snapshots, and EWMA state; they do not mutate policy, publish
// runtime state, execute trim, or enter Pool.Get/Pool.Put. Values are normalized
// to [0, 1] by constructors before they are used in score calculations.
type RuntimeScoreComponent struct {
	// Name identifies the signal for diagnostics.
	Name string

	// Value is the normalized signal value in [0, 1].
	Value float64

	// Weight is the non-negative contribution weight.
	Weight float64
}

// PoolClassScoreComponent names a class-level score component.
type PoolClassScoreComponent = RuntimeScoreComponent

// PoolBudgetScoreComponent names a pool-level score component.
type PoolBudgetScoreComponent = RuntimeScoreComponent

// PoolClassScore is the normalized budget-usefulness projection for one Pool
// class.
//
// Inputs are class samples, bounded window deltas, and optional EWMA state. The
// score is finite and clamped to [0, 1]. Component slices are owned by the score
// value returned by constructors and copied by Clamp/ComponentsCopy so callers
// can keep diagnostics without sharing mutable backing arrays.
type PoolClassScore struct {
	// Value is the normalized weighted class score.
	Value float64

	// Components are the normalized diagnostic inputs used to build Value.
	Components []PoolClassScoreComponent
}

// IsZero reports whether s has no effective score contribution.
func (s PoolClassScore) IsZero() bool {
	return s.Clamp().weightedScore().IsZero()
}

// Clamp returns a finite, normalized copy of s.
func (s PoolClassScore) Clamp() PoolClassScore {
	return poolClassScoreFromWeighted(weightedScoreFromRuntimeComponents(s.Components))
}

// ComponentsCopy returns a normalized copy of the class score components.
func (s PoolClassScore) ComponentsCopy() []PoolClassScoreComponent {
	return copyRuntimeScoreComponents(s.Clamp().Components)
}

func (s PoolClassScore) weightedScore() controlscore.WeightedScore {
	return weightedScoreFromRuntimeComponents(s.Components)
}

// PoolBudgetScore is the normalized budget-usefulness projection for one Pool.
//
// The pool score aggregates already-built class scores by weighted average
// rather than a raw sum. This keeps the score in [0, 1] and avoids making pools
// look more important merely because they have more configured classes.
type PoolBudgetScore struct {
	// PoolName identifies the partition-local Pool represented by this score.
	PoolName string

	// Value is the normalized weighted pool score.
	Value float64

	// Components are class-level aggregate contributions used to build Value.
	Components []PoolBudgetScoreComponent

	// ClassScores are copied class score diagnostics in deterministic order.
	ClassScores []PoolClassScoreEntry
}

// Clamp returns a finite, normalized copy of s.
func (s PoolBudgetScore) Clamp() PoolBudgetScore {
	components := copyRuntimeScoreComponents(s.Components)
	classScores := copyPoolClassScoreEntries(s.ClassScores)
	score := weightedScoreFromRuntimeComponents(components)
	return PoolBudgetScore{
		PoolName:    s.PoolName,
		Value:       score.Value,
		Components:  runtimeScoreComponentsFromControl(score.Components),
		ClassScores: classScores,
	}
}

// ComponentsCopy returns a normalized copy of the pool score components.
func (s PoolBudgetScore) ComponentsCopy() []PoolBudgetScoreComponent {
	return copyRuntimeScoreComponents(s.Clamp().Components)
}

// ClassScoresCopy returns a normalized copy of the class score entries.
func (s PoolBudgetScore) ClassScoresCopy() []PoolClassScoreEntry {
	return copyPoolClassScoreEntries(s.ClassScores)
}

// PoolClassScoreEntry binds a class identifier to its typed class score.
type PoolClassScoreEntry struct {
	// ClassID identifies the Pool class represented by Score.
	ClassID ClassID

	// Score is the normalized score for ClassID.
	Score PoolClassScore
}

// PoolClassScoreReport is the compact diagnostic form of one class score used
// by controller reports.
//
// Controller reports expose this value so callers can see why a numeric
// allocation input was chosen. The report is still observational: score
// diagnostics do not publish policy, execute trim, or enter Pool.Get/Pool.Put.
type PoolClassScoreReport struct {
	// PoolName identifies the partition-local Pool that owns ClassID.
	PoolName string

	// ClassID identifies the Pool class represented by Score.
	ClassID ClassID

	// Score is the normalized score value consumed by budget allocators.
	Score float64

	// Components explain Score in finite normalized terms.
	Components []PoolClassScoreComponent
}

// PoolBudgetScoreReport is the compact diagnostic form of one Pool budget
// score used by controller reports.
type PoolBudgetScoreReport struct {
	// PoolName identifies the partition-local Pool represented by Score.
	PoolName string

	// Score is the normalized pool score value consumed by budget allocators.
	Score float64

	// Components explain the Pool-level aggregation.
	Components []PoolBudgetScoreComponent

	// ClassScores contains the class scores used to build this Pool score.
	ClassScores []PoolClassScoreReport
}

// PoolClassScoreInput contains the immutable inputs for one class score.
//
// Current supplies retained usage and budget context. Window and Activity must
// describe bounded deltas from the same controller window. EWMA, when
// initialized, supplies the smoothed activity and drop pressure for the same
// pool/class key. Missing future signals such as capacity-growth waste should be
// added to this input instead of being read from Pool hot paths.
type PoolClassScoreInput struct {
	// Current is the current class sample.
	Current PoolPartitionClassSample

	// Window is the bounded class window used for delta-derived ratios.
	Window PoolPartitionClassWindow

	// Activity is the class activity projection for Window.
	Activity PoolPartitionClassActivity

	// EWMA is the optional smoothed class activity state.
	EWMA PoolClassEWMAState
}

// PoolBudgetScoreInput contains the immutable inputs for one pool score.
type PoolBudgetScoreInput struct {
	// PoolName identifies the scored Pool.
	PoolName string

	// Window supplies class retained/budget context for weighting.
	Window PoolPartitionPoolWindow

	// ClassScores are the class scores to aggregate in deterministic order.
	ClassScores []PoolClassScoreEntry
}

// NewPoolClassScore returns a typed, explainable class budget score.
//
// The formula intentionally stays conservative and finite-protected:
// useful activity, hits, allocation avoidance, and retained returns raise the
// score; drops, retained-over-target pressure, and low activity lower it. This
// helper is pure control-plane code. It never publishes runtime state, executes
// trim, or calls Pool.Get/Pool.Put.
func NewPoolClassScore(input PoolClassScoreInput) PoolClassScore {
	activityValue := input.Activity.Activity
	dropRatio := input.Activity.DropRatio
	if input.EWMA.Initialized {
		activityValue = input.EWMA.Activity
		dropRatio = input.EWMA.DropRatio
	}

	retainAttempts := input.Window.Delta.PutOutcomes()
	retainRatio := controlrate.RetainRatio(input.Window.Delta.Retains, retainAttempts)
	waste := poolClassScoreWaste(input, dropRatio)
	risk := poolClassScoreRisk(dropRatio)

	components := []controlscore.Component{
		controlscore.NewComponent(
			poolClassScoreComponentActivity,
			poolClassScoreNormalizeActivity(activityValue),
			poolClassScoreActivityWeight,
		),
		controlscore.NewComponent(
			poolClassScoreComponentHitRatio,
			input.Activity.HitRatio,
			poolClassScoreHitRatioWeight,
		),
		controlscore.NewComponent(
			poolClassScoreComponentAllocationAvoidance,
			controlnumeric.Invert01(input.Activity.AllocationRatio),
			poolClassScoreAllocationAvoidanceWeight,
		),
		controlscore.NewComponent(
			poolClassScoreComponentRetainRatio,
			retainRatio,
			poolClassScoreRetainRatioWeight,
		),
		controlscore.NewComponent(
			poolClassScoreComponentWasteAvoidance,
			controlnumeric.Invert01(waste),
			poolClassScoreWasteAvoidanceWeight,
		),
		controlscore.NewComponent(
			poolClassScoreComponentRiskAvoidance,
			controlnumeric.Invert01(risk),
			poolClassScoreRiskAvoidanceWeight,
		),
	}

	return poolClassScoreFromWeighted(controlscore.NewWeightedScore(components))
}

// newNeutralMissingPoolClassScore returns the conservative score used when a
// sampled class is missing a score input.
//
// The stable component name makes a missing diagnostic visible in reports while
// keeping the scalar score at zero. Budget allocators still consume only
// Score.Value, so the missing-input path cannot inflate retained targets.
func newNeutralMissingPoolClassScore() PoolClassScore {
	return PoolClassScore{
		Value: 0,
		Components: []PoolClassScoreComponent{
			{Name: poolClassScoreComponentNeutralMissing, Value: 0, Weight: 1},
		},
	}
}

// NewPoolBudgetScore aggregates class scores into one typed Pool score.
//
// Empty class input returns a documented zero score. Non-empty input is weighted
// by class retained/budget context so a pool with many tiny or idle classes does
// not win allocation solely by raw score summation.
func NewPoolBudgetScore(input PoolBudgetScoreInput) PoolBudgetScore {
	if len(input.ClassScores) == 0 {
		return PoolBudgetScore{
			PoolName: input.PoolName,
		}
	}

	classScores := copyPoolClassScoreEntries(input.ClassScores)
	components := make([]controlscore.Component, 0, len(classScores))
	for _, entry := range classScores {
		weight := poolBudgetClassScoreWeight(input.Window, entry.ClassID)
		components = append(components, controlscore.NewComponent(
			poolBudgetScoreComponentClass+":"+entry.ClassID.String(),
			entry.Score.Value,
			weight,
		))
	}

	weighted := controlscore.NewWeightedScore(components)
	return PoolBudgetScore{
		PoolName:    input.PoolName,
		Value:       weighted.Value,
		Components:  runtimeScoreComponentsFromControl(weighted.Components),
		ClassScores: classScores,
	}
}

func poolClassScoreFromWeighted(score controlscore.WeightedScore) PoolClassScore {
	return PoolClassScore{
		Value:      score.Value,
		Components: runtimeScoreComponentsFromControl(score.Components),
	}
}

func poolClassScoreNormalizeActivity(activity float64) float64 {
	activity = controlnumeric.FiniteOrZero(activity)
	if activity <= 0 {
		return 0
	}
	return controlnumeric.SafeFloatRatio(activity, activity+poolClassScoreActivityScale)
}

func poolClassScoreWaste(input PoolClassScoreInput, dropRatio float64) float64 {
	lowHitWaste := controlnumeric.Invert01(input.Activity.HitRatio)
	retainedPressure := poolClassScoreRetainedPressure(input.Current)
	lowActivityWaste := controlnumeric.Invert01(poolClassScoreNormalizeActivity(input.Activity.Activity))
	return controlscore.WeightedScoreValue([]controlscore.Component{
		controlscore.NewComponent("low_hit_ratio", lowHitWaste, 0.35),
		controlscore.NewComponent("retained_pressure", retainedPressure, 0.30),
		controlscore.NewComponent("drop_ratio", dropRatio, 0.25),
		controlscore.NewComponent("low_activity", lowActivityWaste, 0.10),
	})
}

func poolClassScoreRetainedPressure(class PoolPartitionClassSample) float64 {
	if class.Budget.TargetBytes > 0 {
		return controlnumeric.SafeRatio(class.CurrentRetainedBytes, class.Budget.TargetBytes)
	}
	if class.Budget.AssignedBytes > 0 {
		return controlnumeric.SafeRatio(class.CurrentRetainedBytes, class.Budget.AssignedBytes)
	}
	if class.CurrentRetainedBytes > 0 || class.CurrentRetainedBuffers > 0 {
		return 1
	}
	return 0
}

func poolClassScoreRisk(dropRatio float64) float64 {
	return controlnumeric.Clamp01(dropRatio)
}

func poolBudgetClassScoreWeight(window PoolPartitionPoolWindow, classID ClassID) float64 {
	class, ok := findPoolWindowClass(window, classID)
	if !ok {
		return 1
	}
	retainedWeight := 1 + controlnumeric.SafeRatio(class.Current.CurrentRetainedBytes, poolBudgetWindowRetainedBytes(window))
	sizeWeight := 1 + poolBudgetClassSizeWeight(class.Current.Class.Size())
	return controlnumeric.FiniteOrZero(retainedWeight * sizeWeight)
}

func poolBudgetWindowRetainedBytes(window PoolPartitionPoolWindow) uint64 {
	var retained uint64
	for _, class := range window.Classes {
		retained = poolSaturatingAdd(retained, class.Current.CurrentRetainedBytes)
	}
	if retained == 0 {
		return 1
	}
	return retained
}

func poolBudgetClassSizeWeight(size ClassSize) float64 {
	bytes := size.Bytes()
	if bytes == 0 {
		return 0
	}
	return controlnumeric.SafeFloatRatio(float64(bytes), float64(bytes)+float64(KiB.Bytes()))
}

func findPoolWindowClass(window PoolPartitionPoolWindow, classID ClassID) (PoolPartitionClassWindow, bool) {
	for _, class := range window.Classes {
		if class.ClassID == classID {
			return class, true
		}
	}
	return PoolPartitionClassWindow{}, false
}

func weightedScoreFromRuntimeComponents(components []RuntimeScoreComponent) controlscore.WeightedScore {
	return controlscore.NewWeightedScore(controlScoreComponentsFromRuntime(components))
}

func controlScoreComponentsFromRuntime(components []RuntimeScoreComponent) []controlscore.Component {
	if len(components) == 0 {
		return nil
	}
	copied := make([]controlscore.Component, len(components))
	for i, component := range components {
		copied[i] = controlscore.NewComponent(component.Name, component.Value, component.Weight)
	}
	return copied
}

func runtimeScoreComponentsFromControl(components []controlscore.Component) []RuntimeScoreComponent {
	if len(components) == 0 {
		return nil
	}
	copied := make([]RuntimeScoreComponent, len(components))
	for i, component := range components {
		normalized := controlscore.NewComponent(component.Name, component.Value, component.Weight)
		copied[i] = RuntimeScoreComponent{
			Name:   normalized.Name,
			Value:  normalized.Value,
			Weight: normalized.Weight,
		}
	}
	return copied
}

func copyRuntimeScoreComponents(components []RuntimeScoreComponent) []RuntimeScoreComponent {
	return runtimeScoreComponentsFromControl(controlScoreComponentsFromRuntime(components))
}

func copyPoolClassScoreEntries(entries []PoolClassScoreEntry) []PoolClassScoreEntry {
	if len(entries) == 0 {
		return nil
	}
	copied := make([]PoolClassScoreEntry, len(entries))
	for i, entry := range entries {
		copied[i] = PoolClassScoreEntry{
			ClassID: entry.ClassID,
			Score:   entry.Score.Clamp(),
		}
	}
	return copied
}

func copyPoolClassScoreReports(reports []PoolClassScoreReport) []PoolClassScoreReport {
	if len(reports) == 0 {
		return nil
	}
	copied := make([]PoolClassScoreReport, len(reports))
	for i, report := range reports {
		copied[i] = PoolClassScoreReport{
			PoolName:   report.PoolName,
			ClassID:    report.ClassID,
			Score:      controlnumeric.Clamp01(report.Score),
			Components: copyRuntimeScoreComponents(report.Components),
		}
	}
	return copied
}

func copyPoolBudgetScoreReports(reports []PoolBudgetScoreReport) []PoolBudgetScoreReport {
	if len(reports) == 0 {
		return nil
	}
	copied := make([]PoolBudgetScoreReport, len(reports))
	for i, report := range reports {
		copied[i] = PoolBudgetScoreReport{
			PoolName:    report.PoolName,
			Score:       controlnumeric.Clamp01(report.Score),
			Components:  copyRuntimeScoreComponents(report.Components),
			ClassScores: copyPoolClassScoreReports(report.ClassScores),
		}
	}
	return copied
}

func newPoolClassScoreReport(poolName string, classID ClassID, score PoolClassScore) PoolClassScoreReport {
	clamped := score.Clamp()
	return PoolClassScoreReport{
		PoolName:   poolName,
		ClassID:    classID,
		Score:      clamped.Value,
		Components: clamped.ComponentsCopy(),
	}
}

func newPoolBudgetScoreReport(score PoolBudgetScore) PoolBudgetScoreReport {
	clamped := score.Clamp()
	classReports := make([]PoolClassScoreReport, 0, len(clamped.ClassScores))
	for _, entry := range clamped.ClassScores {
		classReports = append(classReports, newPoolClassScoreReport(clamped.PoolName, entry.ClassID, entry.Score))
	}
	return PoolBudgetScoreReport{
		PoolName:    clamped.PoolName,
		Score:       clamped.Value,
		Components:  clamped.ComponentsCopy(),
		ClassScores: classReports,
	}
}
