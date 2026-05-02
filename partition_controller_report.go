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

// partition_controller_report.go owns report models and report assembly helpers
// for explicit PoolPartition controller cycles. The helpers may copy
// diagnostics into caller-owned reports, but controller state mutation, budget
// publication, trim execution, and status publication remain in TickInto.

// PartitionControllerReport describes one explicit controller tick.
type PartitionControllerReport struct {
	// Status is the lightweight retained controller-cycle status published for
	// this manual TickInto attempt. Full diagnostic data remains in the report
	// fields below; ControllerStatus keeps only generation and outcome counters.
	Status PoolPartitionControllerStatus

	// Generation is the partition event generation for this tick attempt. It
	// means the partition was sampled and a diagnostic report was produced; it
	// does not by itself mean Pool/class budgets were published.
	Generation Generation

	// PolicyGeneration is the partition runtime-policy generation used by the tick.
	PolicyGeneration Generation

	// Lifecycle is the observed partition lifecycle state.
	Lifecycle LifecycleState

	// Sample is the detailed aggregate sample captured by the tick.
	Sample PoolPartitionSample

	// Metrics is the lifetime-derived metrics projection from Sample.
	Metrics PoolPartitionMetrics

	// Window contains counter movement from the previous controller sample to
	// Sample.
	Window PoolPartitionWindow

	// Rates contains ratio and throughput projections derived from Window.
	Rates PoolPartitionWindowRates

	// EWMA contains the updated partition-level smoothed signals.
	EWMA PoolPartitionEWMAState

	// Scores contains partition-level score projections derived from window and
	// smoothed signals.
	Scores PoolPartitionScores

	// PoolScores are compact diagnostics for the typed Pool scores consumed by
	// budget allocation. The controller allocators use only Score.Value; the
	// components explain the value and do not cause additional decisions.
	PoolScores []PoolBudgetScoreReport

	// ClassScores are compact diagnostics for the typed class scores consumed by
	// class budget allocation. They are copied report values, not internal maps.
	ClassScores []PoolClassScoreReport

	// Budget is the partition budget projection from Sample.
	Budget PartitionBudgetSnapshot

	// Pressure is the pressure interpretation from Sample.
	Pressure PartitionPressureSnapshot

	// TrimPlan is the non-mutating trim decision for this tick.
	TrimPlan PartitionTrimPlan

	// TrimResult reports bounded physical trim executed during this applied tick.
	// It remains zero when budget publication is not published and controller
	// state is not committed.
	TrimResult PartitionTrimResult

	// PoolBudgetTargets are the Pool/class budget targets considered by this
	// tick. BudgetPublication.Published says whether they were applied.
	PoolBudgetTargets []PoolBudgetTarget

	// BudgetPublication reports partition-to-Pool and Pool-to-class feasibility
	// and publication status for this applied controller tick.
	BudgetPublication PoolPartitionBudgetPublicationReport
}

func partitionControllerClassScoreReports(report PoolPartitionBudgetPublicationReport) []PoolClassScoreReport {
	var total int
	for _, classReport := range report.ClassReports {
		total += len(classReport.ClassScores)
	}

	if total == 0 {
		return nil
	}

	scores := make([]PoolClassScoreReport, 0, total)
	for _, classReport := range report.ClassReports {
		scores = append(scores, copyPoolClassScoreReports(classReport.ClassScores)...)
	}

	return scores
}

func markControllerClassReportsPublished(report *PoolPartitionBudgetPublicationReport, applied PoolPartitionBudgetPublicationReport) {
	for index := range report.ClassReports {
		for _, appliedClassReport := range applied.ClassReports {
			if report.ClassReports[index].PoolName != appliedClassReport.PoolName {
				continue
			}

			report.ClassReports[index].Published = appliedClassReport.Published
			if report.ClassReports[index].FailureReason == "" {
				report.ClassReports[index].FailureReason = appliedClassReport.FailureReason
			}

			break
		}
	}
}
