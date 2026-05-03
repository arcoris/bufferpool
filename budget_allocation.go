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
	"math/bits"
)

// budget_allocation.go owns the generic retained-byte allocation algorithm.
// Callers provide owner-specific inputs and receive deterministic assignments
// plus feasibility diagnostics. This file is pure planning: it does not publish
// runtime snapshots, enter lifecycle gates, or mutate Pool/Partition/Group state.

const (
	// budgetAllocationScoreScale converts normalized floating-point score values
	// into deterministic integer weights.
	//
	// Score evaluators currently return advisory float64 values. Allocation keeps
	// integer byte arithmetic by scaling positive scores into weights and using
	// overflow-safe integer division for byte shares.
	budgetAllocationScoreScale = 1_000_000
)

const (
	// budgetAllocationReasonFeasible reports a target set that fits the parent
	// retained-byte budget.
	budgetAllocationReasonFeasible = "feasible"

	// budgetAllocationReasonMinimumsExceedParent reports a malformed target set
	// whose child minimums alone exceed the parent retained-byte budget.
	budgetAllocationReasonMinimumsExceedParent = "minimums_exceed_parent_budget"
)

// budgetAllocationInput describes one child target in a retained-byte budget
// distribution.
//
// BaseBytes is applied first. MinBytes and MaxBytes clamp the child assignment.
// A zero MaxBytes means "unbounded" for allocation math. Score receives a share
// of adaptive remainder after base assignments have been fitted under the parent
// target.
type budgetAllocationInput struct {
	BaseBytes uint64
	MinBytes  uint64
	MaxBytes  uint64
	Score     float64
}

// budgetAllocationResult is one computed child assignment.
type budgetAllocationResult struct {
	AssignedBytes uint64
	MinBytes      uint64
	MaxBytes      uint64
}

// budgetAllocationReport describes one allocation attempt.
//
// Feasible is false only when the child minimums force assignments above the
// parent budget. In that case Results still contains deterministic assignments,
// but callers in hard-budget contexts must surface the overcommit instead of
// treating the targets as an ordinary successful publication.
type budgetAllocationReport struct {
	Results            []budgetAllocationResult
	Feasible           bool
	RequestedBytes     uint64
	AssignedBytes      uint64
	OvercommittedBytes uint64
	Reason             string
}

// BudgetAllocationDiagnostics is the report-facing summary of one retained-byte
// allocation attempt.
//
// Applied controller and coordinator paths use this value instead of exposing
// the internal allocator result slice. Feasible is the publication guard for
// hard retained budgets: when it is false, child minimums alone exceed the
// parent target and the caller must either reject publication or explicitly
// report soft overcommit. Pool, PoolPartition, and PoolGroup do not call this
// allocator from Get or Put.
type BudgetAllocationDiagnostics struct {
	// Feasible reports whether child minimum assignments fit inside the parent
	// retained-byte target.
	Feasible bool

	// RequestedBytes is the parent retained-byte target provided to the allocator.
	RequestedBytes uint64

	// AssignedBytes is the total bytes assigned across all child targets.
	AssignedBytes uint64

	// OvercommittedBytes is AssignedBytes - RequestedBytes when Feasible is false.
	OvercommittedBytes uint64

	// Reason is a stable diagnostic reason for feasibility.
	Reason string

	// TargetCount is the number of child targets produced by the allocation.
	TargetCount int
}

// newBudgetAllocationDiagnostics converts an internal allocator report into a
// stable report-facing summary.
func newBudgetAllocationDiagnostics(report budgetAllocationReport) BudgetAllocationDiagnostics {
	return BudgetAllocationDiagnostics{
		Feasible:           report.Feasible,
		RequestedBytes:     report.RequestedBytes,
		AssignedBytes:      report.AssignedBytes,
		OvercommittedBytes: report.OvercommittedBytes,
		Reason:             report.Reason,
		TargetCount:        len(report.Results),
	}
}

// allocateBudgetTargets distributes parentBytes across inputs.
//
// The algorithm is base-first:
//
//   - every input starts at clamp(BaseBytes, MinBytes, MaxBytes);
//   - if base assignments exceed parentBytes, assignments are reduced toward
//     MinBytes until the parent target is respected or only minimums remain;
//   - any remaining parent budget is distributed by positive score weights;
//   - if all scores are zero, adaptive remainder is distributed equally;
//   - MaxBytes clamps child targets, with zero meaning no explicit max.
//
// The result never exceeds parentBytes when the sum of minimums is feasible. If
// minimums alone exceed parentBytes, the allocator returns minimums because
// violating the hard minimum is the least surprising deterministic behavior for
// a malformed target set.
func allocateBudgetTargets(parentBytes uint64, inputs []budgetAllocationInput) []budgetAllocationResult {
	return allocateBudgetTargetsReport(parentBytes, inputs).Results
}

// allocateBudgetTargetsReport distributes parentBytes and reports feasibility.
func allocateBudgetTargetsReport(parentBytes uint64, inputs []budgetAllocationInput) budgetAllocationReport {
	results := make([]budgetAllocationResult, len(inputs))
	report := budgetAllocationReport{
		Results:        results,
		Feasible:       true,
		RequestedBytes: parentBytes,
		Reason:         budgetAllocationReasonFeasible,
	}
	if len(inputs) == 0 {
		return report
	}

	// Phase 1: choose the initial assignment for each child from base, min, and
	// max bounds. This step is deterministic and does not look at scores.
	for index, input := range inputs {
		minBytes := input.MinBytes
		maxBytes := budgetAllocationMaxBytes(input.MaxBytes)
		if maxBytes < minBytes {
			maxBytes = minBytes
		}

		results[index] = budgetAllocationResult{
			AssignedBytes: budgetClampUint64(input.BaseBytes, minBytes, maxBytes),
			MinBytes:      minBytes,
			MaxBytes:      maxBytes,
		}
	}

	// Phase 2: shrink base assignments toward child minimums if the parent
	// budget cannot fund the initial assignment set.
	total := budgetAllocationTotal(results)
	if total > parentBytes {
		total = reduceBudgetAssignmentsToLimit(results, parentBytes)
	}
	if total >= parentBytes {
		report.AssignedBytes = total
		if total > parentBytes {
			report.Feasible = false
			report.OvercommittedBytes = total - parentBytes
			report.Reason = budgetAllocationReasonMinimumsExceedParent
		}
		return report
	}

	// Phase 3: distribute any remaining parent budget by positive score weights,
	// or equally when no child has a positive score.
	distributeBudgetRemainder(results, inputs, parentBytes-total)
	report.AssignedBytes = budgetAllocationTotal(results)
	return report
}

// reduceBudgetAssignmentsToLimit reduces assignments toward minimums.
func reduceBudgetAssignmentsToLimit(results []budgetAllocationResult, limit uint64) uint64 {
	total := budgetAllocationTotal(results)
	for total > limit {
		eligible := budgetAllocationReducibleCount(results)
		if eligible == 0 {
			return total
		}

		over := total - limit
		share := over / uint64(eligible)
		if share == 0 {
			share = 1
		}

		for index := range results {
			if results[index].AssignedBytes <= results[index].MinBytes {
				continue
			}

			room := results[index].AssignedBytes - results[index].MinBytes
			reduction := poolMinUint64(share, room)
			results[index].AssignedBytes -= reduction
			total -= reduction
			if total <= limit {
				return total
			}
		}
	}

	return total
}

// distributeBudgetRemainder adds adaptive remainder to assignments.
func distributeBudgetRemainder(results []budgetAllocationResult, inputs []budgetAllocationInput, remainder uint64) {
	for remainder > 0 {
		weights := budgetAllocationWeights(results, inputs)
		totalWeight := budgetAllocationWeightTotal(weights)
		if totalWeight == 0 {
			return
		}

		passRemainder := remainder
		changed := false
		for index := range results {
			if remainder == 0 {
				return
			}
			if results[index].AssignedBytes >= results[index].MaxBytes || weights[index] == 0 {
				continue
			}

			room := results[index].MaxBytes - results[index].AssignedBytes
			share := budgetWeightedShare(passRemainder, weights[index], totalWeight)
			if share == 0 {
				share = 1
			}
			added := poolMinUint64(share, poolMinUint64(room, remainder))
			results[index].AssignedBytes += added
			remainder -= added
			changed = true
		}

		if !changed {
			return
		}
	}
}

// budgetAllocationWeights returns one positive integer weight per child that has
// remaining room.
func budgetAllocationWeights(results []budgetAllocationResult, inputs []budgetAllocationInput) []uint64 {
	weights := make([]uint64, len(results))
	var positiveWeight bool
	for index := range results {
		if results[index].AssignedBytes >= results[index].MaxBytes {
			continue
		}

		weight := budgetScoreWeight(inputs[index].Score)
		if weight > 0 {
			positiveWeight = true
		}
		weights[index] = weight
	}

	if positiveWeight {
		return weights
	}

	for index := range weights {
		if results[index].AssignedBytes < results[index].MaxBytes {
			weights[index] = 1
		}
	}

	return weights
}

// budgetScoreWeight converts a positive score into a deterministic integer
// allocation weight.
func budgetScoreWeight(score float64) uint64 {
	if score <= 0 || math.IsNaN(score) {
		return 0
	}
	if math.IsInf(score, 1) {
		return ^uint64(0)
	}
	if math.IsInf(score, -1) {
		return 0
	}

	scaled := score * budgetAllocationScoreScale
	if scaled < 1 {
		return 1
	}
	if scaled >= float64(^uint64(0)) {
		return ^uint64(0)
	}

	return uint64(scaled)
}

// budgetWeightedShare returns floor(total * weight / totalWeight) without
// overflowing uint64 multiplication.
func budgetWeightedShare(total uint64, weight uint64, totalWeight uint64) uint64 {
	if total == 0 || weight == 0 || totalWeight == 0 {
		return 0
	}
	if weight >= totalWeight {
		return total
	}

	// Split total first. A direct 128-bit product divided by totalWeight can
	// violate bits.Div64's quotient-size precondition when all values are large.
	quotient := total / totalWeight
	remainder := total % totalWeight

	share := quotient * weight
	hi, lo := bits.Mul64(remainder, weight)
	remainderShare, _ := bits.Div64(hi, lo, totalWeight)
	share = poolSaturatingAdd(share, remainderShare)
	if share > total {
		return total
	}

	return share
}

// budgetAllocationMaxBytes maps zero max to an unbounded allocation cap.
func budgetAllocationMaxBytes(maxBytes uint64) uint64 {
	if maxBytes == 0 {
		return ^uint64(0)
	}

	return maxBytes
}

// budgetClampUint64 clamps value to [minValue, maxValue].
func budgetClampUint64(value uint64, minValue uint64, maxValue uint64) uint64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}

	return value
}

// budgetAllocationTotal sums assigned bytes with saturation.
func budgetAllocationTotal(results []budgetAllocationResult) uint64 {
	var total uint64
	for _, result := range results {
		total = poolSaturatingAdd(total, result.AssignedBytes)
	}

	return total
}

// budgetAllocationReducibleCount counts assignments above their minimums.
func budgetAllocationReducibleCount(results []budgetAllocationResult) int {
	var count int
	for _, result := range results {
		if result.AssignedBytes > result.MinBytes {
			count++
		}
	}

	return count
}

// budgetAllocationWeightTotal sums allocation weights with saturation.
func budgetAllocationWeightTotal(weights []uint64) uint64 {
	var total uint64
	for _, weight := range weights {
		total = poolSaturatingAdd(total, weight)
	}

	return total
}

// budgetPublicationGeneration chooses a monotonic publication generation for a
// runtime target application.
func budgetPublicationGeneration(current Generation, target Generation) Generation {
	if target.IsZero() || !current.Before(target) {
		return current.Next()
	}

	return target
}
