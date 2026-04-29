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

// PoolGroupScoreEvaluatorConfig configures group score projection.
//
// The first group scorer intentionally reuses partition score semantics over a
// partition-shaped aggregate group sample. A zero config selects the default
// partition score evaluator config.
type PoolGroupScoreEvaluatorConfig struct {
	// Partition configures the partition-shaped aggregate scorer used by group.
	Partition PoolPartitionScoreEvaluatorConfig
}

// IsZero reports whether c contains no explicit group score settings.
func (c PoolGroupScoreEvaluatorConfig) IsZero() bool {
	return c.Partition == (PoolPartitionScoreEvaluatorConfig{})
}

// PoolGroupScoreEvaluator is the root-domain adapter over aggregate group rates.
//
// It owns no runtime state and is safe to reuse across foreground coordinator
// ticks. It computes advisory scalar values only and never mutates group or
// partition policy.
type PoolGroupScoreEvaluator struct {
	partition   PoolPartitionScoreEvaluator
	initialized bool
}

// NewPoolGroupScoreEvaluator returns an immutable group score evaluator.
func NewPoolGroupScoreEvaluator(config PoolGroupScoreEvaluatorConfig) PoolGroupScoreEvaluator {
	return PoolGroupScoreEvaluator{partition: NewPoolPartitionScoreEvaluator(config.Partition), initialized: true}
}

// ScoreValues returns scalar aggregate group score values.
func (e PoolGroupScoreEvaluator) ScoreValues(
	rates PoolGroupWindowRates,
	budget PartitionBudgetSnapshot,
	pressure PartitionPressureSnapshot,
) PoolGroupScoreValues {
	if !e.initialized {
		return PoolGroupScoreValues{}
	}
	partitionValues := e.partition.ScoreValues(rates.Aggregate, PoolPartitionEWMAState{}, budget, pressure)
	return PoolGroupScoreValues{
		Usefulness: partitionValues.Usefulness,
		Waste:      partitionValues.Waste,
		Budget:     partitionValues.Budget,
		Pressure:   partitionValues.Pressure,
		Activity:   partitionValues.Activity,
		Risk:       partitionValues.Risk,
	}
}

// PartitionScoreValues returns scalar partition score values using the group
// evaluator's partition scorer.
func (e PoolGroupScoreEvaluator) PartitionScoreValues(
	rates PoolPartitionWindowRates,
	ewma PoolPartitionEWMAState,
	budget PartitionBudgetSnapshot,
	pressure PartitionPressureSnapshot,
) PoolPartitionScoreValues {
	if !e.initialized {
		return PoolPartitionScoreValues{}
	}
	return e.partition.ScoreValues(rates, ewma, budget, pressure)
}

// isZero reports whether e is the zero disabled evaluator.
func (e PoolGroupScoreEvaluator) isZero() bool {
	return !e.initialized
}
