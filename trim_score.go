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
	controlpressure "arcoris.dev/bufferpool/internal/control/pressure"
	controlscore "arcoris.dev/bufferpool/internal/control/score"
)

const (
	trimVictimScoreComponentOverTarget           = "over_target"
	trimVictimScoreComponentRetained             = "retained_pressure"
	trimVictimScoreComponentColdness             = "coldness"
	trimVictimScoreComponentCapacityWaste        = "capacity_waste"
	trimVictimScoreComponentPressure             = "pressure_severity"
	trimVictimScoreComponentClassSize            = "class_size_penalty"
	trimVictimNeutralColdness                    = 0.5
	trimVictimHotActivityScale                   = 64
	trimVictimRetainedByteScale           uint64 = 4 * 1024
	trimVictimCapacityWasteGrowthRange           = 3
)

const (
	trimVictimScoreOverTargetWeight    = 0.30
	trimVictimScoreRetainedWeight      = 0.20
	trimVictimScoreColdnessWeight      = 0.20
	trimVictimScoreCapacityWasteWeight = 0.15
	trimVictimScorePressureWeight      = 0.10
	trimVictimScoreClassSizeWeight     = 0.05
)

// TrimVictimScoreComponent names one normalized trim-victim signal.
type TrimVictimScoreComponent = RuntimeScoreComponent

// TrimVictimScore is the normalized candidate-level score used to order trim
// victims.
//
// Trim scoring is control-plane diagnostics for retained storage. It ranks
// classes or Pools before bounded trim execution starts; it never scores
// individual buffers, publishes policy, calls Pool.Get/Pool.Put, or expands the
// configured trim bounds.
type TrimVictimScore struct {
	// Value is the normalized weighted victim score.
	Value float64

	// Components explain Value in finite normalized terms.
	Components []TrimVictimScoreComponent
}

// IsZero reports whether s has no effective victim score contribution.
func (s TrimVictimScore) IsZero() bool {
	return weightedScoreFromRuntimeComponents(s.Components).IsZero()
}

// Clamp returns a finite normalized copy of s.
func (s TrimVictimScore) Clamp() TrimVictimScore {
	score := weightedScoreFromRuntimeComponents(s.Components)
	return TrimVictimScore{
		Value:      score.Value,
		Components: runtimeScoreComponentsFromControl(score.Components),
	}
}

// ComponentsCopy returns a normalized copy of score components.
func (s TrimVictimScore) ComponentsCopy() []TrimVictimScoreComponent {
	return copyRuntimeScoreComponents(s.Clamp().Components)
}

// TrimVictimScoreInput contains candidate-level facts used by trim scoring.
type TrimVictimScoreInput struct {
	// OverTargetBytes is retained capacity above the applicable target.
	OverTargetBytes uint64

	// RetainedBytes is current retained capacity.
	RetainedBytes uint64

	// RetainedBuffers is current retained buffer count.
	RetainedBuffers uint64

	// ClassBytes is the nominal class size when scoring a class or shard.
	ClassBytes uint64

	// CapacityWasteBytes is retained capacity above nominal class capacity.
	CapacityWasteBytes uint64

	// RecentActivity is a caller-supplied activity estimate when available.
	RecentActivity float64

	// ActivityKnown reports whether RecentActivity is meaningful.
	ActivityKnown bool

	// Coldness is a caller-supplied coldness score when available.
	Coldness float64

	// ColdnessKnown reports whether Coldness is meaningful.
	ColdnessKnown bool

	// PressureLevel is the domain pressure level applied to this candidate.
	PressureLevel PressureLevel

	// PressureSeverity overrides PressureLevel when already projected.
	PressureSeverity float64
}

// NewTrimVictimScore returns a finite, explainable trim-victim score.
//
// Higher over-target usage, retained pressure, coldness, capacity waste, pressure
// severity, and class size increase the score. Recent activity lowers the
// coldness component when the caller has a bounded activity signal. When no
// activity is available, coldness is neutral so standalone Pool.Trim does not
// invent a hot or cold classification.
func NewTrimVictimScore(input TrimVictimScoreInput) TrimVictimScore {
	if input.RetainedBytes == 0 && input.OverTargetBytes == 0 && input.CapacityWasteBytes == 0 {
		return TrimVictimScore{}
	}

	components := []controlscore.Component{
		controlscore.NewComponent(
			trimVictimScoreComponentOverTarget,
			trimVictimOverTargetScore(input.OverTargetBytes, input.RetainedBytes),
			trimVictimScoreOverTargetWeight,
		),
		controlscore.NewComponent(
			trimVictimScoreComponentRetained,
			trimVictimRetainedScore(input.RetainedBytes),
			trimVictimScoreRetainedWeight,
		),
		controlscore.NewComponent(
			trimVictimScoreComponentColdness,
			trimVictimColdness(input),
			trimVictimScoreColdnessWeight,
		),
		controlscore.NewComponent(
			trimVictimScoreComponentCapacityWaste,
			trimVictimCapacityWasteScore(input),
			trimVictimScoreCapacityWasteWeight,
		),
		controlscore.NewComponent(
			trimVictimScoreComponentPressure,
			trimVictimPressureSeverity(input),
			trimVictimScorePressureWeight,
		),
		controlscore.NewComponent(
			trimVictimScoreComponentClassSize,
			trimVictimClassSizeScore(input.ClassBytes),
			trimVictimScoreClassSizeWeight,
		),
	}
	score := controlscore.NewWeightedScore(components)
	return TrimVictimScore{
		Value:      score.Value,
		Components: runtimeScoreComponentsFromControl(score.Components),
	}
}

func trimVictimOverTargetScore(overTargetBytes uint64, retainedBytes uint64) float64 {
	if retainedBytes > 0 {
		return controlnumeric.SafeRatio(overTargetBytes, retainedBytes)
	}
	return controlnumeric.NormalizeToLimit(overTargetBytes, KiB.Bytes())
}

func trimVictimRetainedScore(retainedBytes uint64) float64 {
	if retainedBytes == 0 {
		return 0
	}
	return controlnumeric.SafeFloatRatio(float64(retainedBytes), float64(poolSaturatingAdd(retainedBytes, trimVictimRetainedByteScale)))
}

func trimVictimColdness(input TrimVictimScoreInput) float64 {
	if input.ActivityKnown {
		activity := controlnumeric.NormalizeFloatToLimit(input.RecentActivity, trimVictimHotActivityScale)
		return controlnumeric.Invert01(activity)
	}
	if input.ColdnessKnown {
		return controlnumeric.Clamp01(input.Coldness)
	}
	return trimVictimNeutralColdness
}

func trimVictimCapacityWasteScore(input TrimVictimScoreInput) float64 {
	nominalBytes := trimVictimNominalBytes(input.RetainedBuffers, input.ClassBytes)
	if nominalBytes == 0 {
		if input.CapacityWasteBytes > 0 {
			return 1
		}
		return 0
	}
	retainedBytes := input.RetainedBytes
	if retainedBytes == 0 {
		retainedBytes = poolSaturatingAdd(nominalBytes, input.CapacityWasteBytes)
	}
	if retainedBytes <= nominalBytes {
		return 0
	}
	ratio := controlnumeric.SafeFloatRatio(float64(retainedBytes), float64(nominalBytes))
	return controlnumeric.Clamp01((ratio - 1) / trimVictimCapacityWasteGrowthRange)
}

func trimVictimPressureSeverity(input TrimVictimScoreInput) float64 {
	if input.PressureSeverity != 0 {
		return controlnumeric.Clamp01(input.PressureSeverity)
	}
	return pressureLevelSeverity(input.PressureLevel)
}

func pressureLevelSeverity(level PressureLevel) float64 {
	switch level {
	case PressureLevelMedium:
		return controlpressure.Severity(controlpressure.LevelMedium)
	case PressureLevelHigh:
		return controlpressure.Severity(controlpressure.LevelHigh)
	case PressureLevelCritical:
		return controlpressure.Severity(controlpressure.LevelCritical)
	default:
		return controlpressure.Severity(controlpressure.LevelNormal)
	}
}

func trimVictimClassSizeScore(classBytes uint64) float64 {
	if classBytes == 0 {
		return 0
	}
	return controlnumeric.SafeFloatRatio(float64(classBytes), float64(classBytes+KiB.Bytes()))
}

func trimVictimCapacityWasteBytes(retainedBytes uint64, retainedBuffers uint64, classBytes uint64) uint64 {
	nominalBytes := trimVictimNominalBytes(retainedBuffers, classBytes)
	if retainedBytes <= nominalBytes {
		return 0
	}
	return retainedBytes - nominalBytes
}

func trimVictimNominalBytes(retainedBuffers uint64, classBytes uint64) uint64 {
	if retainedBuffers == 0 || classBytes == 0 {
		return 0
	}
	if retainedBuffers > ^uint64(0)/classBytes {
		return ^uint64(0)
	}
	return retainedBuffers * classBytes
}

func trimVictimComponentValue(score TrimVictimScore, name string) float64 {
	for _, component := range score.Clamp().Components {
		if component.Name == name {
			return component.Value
		}
	}
	return 0
}
