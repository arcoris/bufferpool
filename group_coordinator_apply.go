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
	"time"
)

// groupPartitionScores computes deterministic partition score projections for
// one group window.
//
// The group coordinator scores partition windows only. It does not inspect Pool
// shards, update class EWMA, or derive Pool/class budgets. Those remain
// PoolPartition responsibilities.
func (g *PoolGroup) groupPartitionScores(
	window PoolGroupWindow,
	elapsed time.Duration,
	scoreEvaluator PoolGroupScoreEvaluator,
) []PoolGroupPartitionScore {
	scores := make([]PoolGroupPartitionScore, 0, len(window.Current.Partitions))
	for _, current := range window.Current.Partitions {
		previous, _ := findPoolGroupPartitionSample(window.Previous, current.Name)
		partitionWindow := NewPoolPartitionWindow(previous.Sample, current.Sample)
		rates := NewPoolPartitionTimedWindowRates(partitionWindow, elapsed)
		partition, ok := g.registry.partition(current.Name)
		if !ok {
			continue
		}
		runtime := partition.currentRuntimeSnapshot()
		budget := newPartitionBudgetSnapshot(runtime.Policy.Budget, current.Sample)
		pressure := newPartitionPressureSnapshot(runtime.Policy.Pressure, current.Sample)
		values := scoreEvaluator.PartitionScoreValues(rates, PoolPartitionEWMAState{}, budget, pressure)
		scores = append(scores, PoolGroupPartitionScore{
			PartitionName: current.Name,
			Scores:        values,
			Score:         groupPartitionBudgetScore(partitionWindow, rates),
		})
	}
	return scores
}

// coordinatorPartitionBudgetTargets computes retained-budget targets for owned
// partitions.
//
// A zero group retained budget means the group is unbounded at retained scope, so
// no partition targets are published. When a hard group retained budget is
// configured, the full target is distributed across current partitions by score
// or equally when all partition scores are zero.
func (g *PoolGroup) coordinatorPartitionBudgetReport(
	generation Generation,
	runtime *groupRuntimeSnapshot,
	window PoolGroupWindow,
	partitionScores []PoolGroupPartitionScore,
) partitionBudgetAllocationReport {
	retainedBytes := runtime.Policy.Budget.MaxRetainedBytes
	if retainedBytes.IsZero() || len(window.Current.Partitions) == 0 {
		return partitionBudgetAllocationReport{}
	}

	inputs := make([]partitionBudgetAllocationInput, 0, len(window.Current.Partitions))
	for _, partition := range window.Current.Partitions {
		inputs = append(inputs, partitionBudgetAllocationInput{
			PartitionName: partition.Name,
			Score:         findPoolGroupPartitionBudgetScore(partitionScores, partition.Name),
		})
	}

	return g.computePartitionBudgetTargetsReport(generation, retainedBytes, inputs)
}

// groupPartitionBudgetScore maps bounded-window movement to one positive
// allocation weight.
//
// The full partition score report can contain useful static pressure or
// usefulness baselines. Redistribution should not give those baselines memory by
// themselves, so the allocation weight is derived only from movement inside this
// group coordinator window.
func groupPartitionBudgetScore(window PoolPartitionWindow, rates PoolPartitionWindowRates) float64 {
	delta := window.Delta
	events := poolSaturatingAdd(
		delta.Gets,
		poolSaturatingAdd(
			delta.Hits,
			poolSaturatingAdd(
				poolSaturatingAdd(delta.Misses, delta.Misses),
				poolSaturatingAdd(
					poolSaturatingAdd(delta.Allocations, delta.Allocations),
					poolSaturatingAdd(delta.Puts, poolSaturatingAdd(delta.Retains, delta.Drops)),
				),
			),
		),
	)
	leaseOps := poolSaturatingAdd(delta.LeaseAcquisitions, delta.LeaseReleases)
	score := float64(poolSaturatingAdd(events, leaseOps)) +
		rates.GetsPerSecond +
		rates.PutsPerSecond +
		rates.AllocationsPerSecond +
		rates.LeaseOpsPerSecond
	if score <= 0 || math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}
	return score
}

// findPoolGroupPartitionBudgetScore returns the allocation score for name.
func findPoolGroupPartitionBudgetScore(scores []PoolGroupPartitionScore, name string) float64 {
	for _, score := range scores {
		if score.PartitionName == name {
			return score.Score
		}
	}
	return 0
}

// findPoolGroupPartitionSample returns one partition sample by group-local name.
func findPoolGroupPartitionSample(sample PoolGroupSample, name string) (PoolGroupPartitionSample, bool) {
	for _, partition := range sample.Partitions {
		if partition.Name == name {
			return partition, true
		}
	}
	return PoolGroupPartitionSample{}, false
}
