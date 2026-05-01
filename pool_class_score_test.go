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
	"reflect"
	"testing"
	"time"
)

func TestPoolClassScoreUsesWindowDeltas(t *testing.T) {
	class := testPoolClassScoreSample(ClassID(0), KiB.Bytes(), 0, 32*KiB.Bytes())
	low := testPoolClassScoreForDelta(class, classCountersDelta{
		Gets: 1,
		Hits: 1,
	})
	high := testPoolClassScoreForDelta(class, classCountersDelta{
		Gets:        20,
		Hits:        18,
		Misses:      2,
		Allocations: 1,
		Puts:        16,
		Retains:     16,
	})

	if high.Value <= low.Value {
		t.Fatalf("high activity score = %v, want greater than low activity %v", high.Value, low.Value)
	}
}

func TestPoolClassScoreUsesEWMAWhenInitialized(t *testing.T) {
	class := testPoolClassScoreSample(ClassID(0), KiB.Bytes(), 0, 32*KiB.Bytes())
	window := testPoolClassScoreWindow(class, classCountersDelta{
		Gets: 1,
		Hits: 1,
	})
	activity := newPoolPartitionClassActivity(window, time.Second)

	raw := NewPoolClassScore(PoolClassScoreInput{
		Current:  class,
		Window:   window,
		Activity: activity,
	})
	smoothed := NewPoolClassScore(PoolClassScoreInput{
		Current:  class,
		Window:   window,
		Activity: activity,
		EWMA: PoolClassEWMAState{
			Initialized: true,
			Activity:    128,
			DropRatio:   0,
		},
	})

	if smoothed.Value <= raw.Value {
		t.Fatalf("EWMA score = %v, want greater than raw score %v", smoothed.Value, raw.Value)
	}
	if got := testPoolClassScoreComponent(t, smoothed, poolClassScoreComponentActivity); got <= testPoolClassScoreComponent(t, raw, poolClassScoreComponentActivity) {
		t.Fatalf("EWMA activity component = %v, want greater than raw component", got)
	}
}

func TestPoolClassScoreFallsBackToWindowRatesWithoutEWMA(t *testing.T) {
	class := testPoolClassScoreSample(ClassID(0), KiB.Bytes(), 0, 32*KiB.Bytes())
	window := testPoolClassScoreWindow(class, classCountersDelta{
		Gets:        8,
		Hits:        4,
		Misses:      4,
		Allocations: 2,
	})
	activity := newPoolPartitionClassActivity(window, time.Second)

	score := NewPoolClassScore(PoolClassScoreInput{
		Current:  class,
		Window:   window,
		Activity: activity,
	})
	got := testPoolClassScoreComponent(t, score, poolClassScoreComponentActivity)
	want := poolClassScoreNormalizeActivity(activity.Activity)
	if got != want {
		t.Fatalf("activity component = %v, want window-derived %v", got, want)
	}
}

func TestPoolClassScoreRewardsHitsAndAllocationAvoidance(t *testing.T) {
	class := testPoolClassScoreSample(ClassID(0), KiB.Bytes(), 0, 32*KiB.Bytes())
	useful := testPoolClassScoreForDelta(class, classCountersDelta{
		Gets:    20,
		Hits:    20,
		Puts:    20,
		Retains: 20,
	})
	allocationHeavy := testPoolClassScoreForDelta(class, classCountersDelta{
		Gets:        20,
		Misses:      20,
		Allocations: 20,
		Puts:        20,
		Drops:       20,
	})

	if useful.Value <= allocationHeavy.Value {
		t.Fatalf("useful score = %v, want greater than allocation-heavy %v", useful.Value, allocationHeavy.Value)
	}
}

func TestPoolClassScorePenalizesDrops(t *testing.T) {
	class := testPoolClassScoreSample(ClassID(0), KiB.Bytes(), 0, 32*KiB.Bytes())
	retained := testPoolClassScoreForDelta(class, classCountersDelta{
		Gets:    10,
		Hits:    10,
		Puts:    10,
		Retains: 10,
	})
	dropped := testPoolClassScoreForDelta(class, classCountersDelta{
		Gets:  10,
		Hits:  10,
		Puts:  10,
		Drops: 10,
	})

	if dropped.Value >= retained.Value {
		t.Fatalf("drop-heavy score = %v, want less than retained score %v", dropped.Value, retained.Value)
	}
}

func TestPoolClassScorePenalizesWaste(t *testing.T) {
	efficient := testPoolClassScoreSample(ClassID(0), KiB.Bytes(), 4*KiB.Bytes(), 32*KiB.Bytes())
	wasteful := testPoolClassScoreSample(ClassID(0), KiB.Bytes(), 64*KiB.Bytes(), 32*KiB.Bytes())
	delta := classCountersDelta{
		Gets:    16,
		Hits:    16,
		Puts:    16,
		Retains: 16,
	}

	efficientScore := testPoolClassScoreForDelta(efficient, delta)
	wastefulScore := testPoolClassScoreForDelta(wasteful, delta)
	if wastefulScore.Value >= efficientScore.Value {
		t.Fatalf("wasteful score = %v, want less than efficient score %v", wastefulScore.Value, efficientScore.Value)
	}
}

func TestPoolClassScoreClampsNaNAndInf(t *testing.T) {
	class := testPoolClassScoreSample(ClassID(0), KiB.Bytes(), math.MaxUint64, KiB.Bytes())
	score := NewPoolClassScore(PoolClassScoreInput{
		Current: class,
		Activity: PoolPartitionClassActivity{
			Activity:        math.Inf(1),
			HitRatio:        math.NaN(),
			AllocationRatio: math.Inf(1),
			DropRatio:       math.NaN(),
		},
		EWMA: PoolClassEWMAState{
			Initialized: true,
			Activity:    math.Inf(1),
			DropRatio:   math.NaN(),
		},
	})

	if !isFiniteUnit(score.Value) {
		t.Fatalf("score = %v, want finite unit value", score.Value)
	}
	for _, component := range score.Components {
		if !isFiniteUnit(component.Value) || component.Weight < 0 || math.IsNaN(component.Weight) || math.IsInf(component.Weight, 0) {
			t.Fatalf("component = %+v, want finite normalized value and weight", component)
		}
	}
}

func TestPoolClassScoreComponentsAreCopied(t *testing.T) {
	score := testPoolClassScoreForDelta(
		testPoolClassScoreSample(ClassID(0), KiB.Bytes(), 0, 32*KiB.Bytes()),
		classCountersDelta{Gets: 8, Hits: 8},
	)
	copied := score.ComponentsCopy()
	if len(copied) == 0 {
		t.Fatal("ComponentsCopy returned no components")
	}

	original := copied[0]
	score.Components[0].Value = 0
	if copied[0] != original {
		t.Fatalf("component copy changed after source mutation: got %+v, want %+v", copied[0], original)
	}

	clamped := score.Clamp()
	score.Components[0].Weight = 0
	if clamped.Components[0].Weight == score.Components[0].Weight {
		t.Fatalf("Clamp shared component backing storage")
	}
}

func TestPoolBudgetScoreAggregatesClassScoresDeterministically(t *testing.T) {
	input := PoolBudgetScoreInput{
		PoolName: "pool",
		ClassScores: []PoolClassScoreEntry{
			testPoolClassScoreEntry(ClassID(0), 0.2),
			testPoolClassScoreEntry(ClassID(1), 0.8),
		},
	}

	first := NewPoolBudgetScore(input)
	second := NewPoolBudgetScore(input)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("pool score is not deterministic:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if first.Value <= 0 || first.Value >= 1 {
		t.Fatalf("pool score = %v, want finite weighted aggregate inside (0,1)", first.Value)
	}
}

func TestPoolBudgetScoreDoesNotUseRawSumOnlyBehavior(t *testing.T) {
	input := PoolBudgetScoreInput{
		PoolName: "pool",
		ClassScores: []PoolClassScoreEntry{
			testPoolClassScoreEntry(ClassID(0), 0.2),
			testPoolClassScoreEntry(ClassID(1), 0.4),
		},
	}

	score := NewPoolBudgetScore(input)
	rawSum := input.ClassScores[0].Score.Value + input.ClassScores[1].Score.Value
	if score.Value == rawSum {
		t.Fatalf("pool score = %v, unexpectedly matched raw class-score sum", score.Value)
	}
	if math.Abs(score.Value-0.3) > 1e-12 {
		t.Fatalf("pool score = %v, want equal-weight average 0.3 without class context", score.Value)
	}
}

func TestPoolBudgetScoreHandlesEmptyClasses(t *testing.T) {
	score := NewPoolBudgetScore(PoolBudgetScoreInput{PoolName: "pool"})
	if score.PoolName != "pool" {
		t.Fatalf("PoolName = %q, want pool", score.PoolName)
	}
	if score.Value != 0 || len(score.Components) != 0 || len(score.ClassScores) != 0 {
		t.Fatalf("empty score = %+v, want zero score with no diagnostics", score)
	}
}

func testPoolClassScoreForDelta(class PoolPartitionClassSample, delta classCountersDelta) PoolClassScore {
	window := testPoolClassScoreWindow(class, delta)
	return NewPoolClassScore(PoolClassScoreInput{
		Current:  class,
		Window:   window,
		Activity: newPoolPartitionClassActivity(window, time.Second),
	})
}

func testPoolClassScoreWindow(class PoolPartitionClassSample, delta classCountersDelta) PoolPartitionClassWindow {
	return PoolPartitionClassWindow{
		Class:   class.Class,
		ClassID: class.ClassID,
		Current: class,
		Delta:   delta,
	}
}

func testPoolClassScoreSample(classID ClassID, classBytes uint64, retainedBytes uint64, budgetBytes uint64) PoolPartitionClassSample {
	classSize := ClassSizeFromBytes(classBytes)
	targetBuffers := uint64(0)
	retainedBuffers := uint64(0)
	if classBytes > 0 {
		targetBuffers = budgetBytes / classBytes
		retainedBuffers = retainedBytes / classBytes
	}
	return PoolPartitionClassSample{
		Class:   NewSizeClass(classID, classSize),
		ClassID: classID,
		Budget: PoolClassBudgetSnapshot{
			ClassSize:     classSize,
			AssignedBytes: budgetBytes,
			TargetBuffers: targetBuffers,
			TargetBytes:   targetBuffers * classBytes,
		},
		CurrentRetainedBuffers: retainedBuffers,
		CurrentRetainedBytes:   retainedBytes,
	}
}

func testPoolClassScoreEntry(classID ClassID, value float64) PoolClassScoreEntry {
	return PoolClassScoreEntry{
		ClassID: classID,
		Score: PoolClassScore{
			Value: value,
			Components: []PoolClassScoreComponent{
				{Name: "test", Value: value, Weight: 1},
			},
		},
	}
}

func testPoolClassScoreComponent(t *testing.T, score PoolClassScore, name string) float64 {
	t.Helper()
	for _, component := range score.Components {
		if component.Name == name {
			return component.Value
		}
	}
	t.Fatalf("score missing component %q: %+v", name, score.Components)
	return 0
}
