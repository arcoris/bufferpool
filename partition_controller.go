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

import "time"

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

// Tick performs one explicit partition controller cycle.
//
// The current implementation samples active Pools, computes window deltas,
// updates partition-owned EWMA state, scores active Pool classes, publishes class
// budgets into owned Pools, and executes bounded physical trim when the trim
// plan is enabled. It does not start background scheduling. Controller.Enabled
// controls future automatic scheduling; manual Tick remains available when the
// partition is active.
//
// Tick returns a detailed diagnostic/report value and may allocate per-Pool
// sample/report storage. Controller sampling uses the active/dirty registry and
// periodic full scans, so ordinary ticks can narrow work after clean idle Pools
// age out while still revalidating inactive Pools deterministically.
func (p *PoolPartition) Tick() (PartitionControllerReport, error) {
	p.mustBeInitialized()
	var report PartitionControllerReport
	err := p.TickInto(&report)
	return report, err
}

// TickInto writes one explicit partition controller cycle into dst.
//
// TickInto has the same control semantics as Tick but reuses dst.Sample.Pools
// capacity. A nil dst is a no-op after receiver and lifecycle validation. Like
// Tick, this is a manual foreground call: it does not start background
// goroutines, coordinate with PoolGroup, or redistribute group-level budgets.
//
// If budget publication returns an error, TickInto reports Failed, returns the
// original error, and commits no controller state. If publication is a non-error
// diagnostic non-publication, TickInto reports Unpublished and also commits no
// controller state. That keeps observed-but-unapplied ticks from half-advancing
// EWMA, previous sample/time, dirty-marker processing, trim execution, or
// controller generation.
//
// Applied is owner-specific: a partition tick can be Applied because it
// committed observation, EWMA, active/dirty, and trim controller state even when
// no Pool budget targets were needed. Group ticks use Skipped for their own
// no-target case because the group has no lower-level observation state to
// apply in that path.
func (p *PoolPartition) TickInto(dst *PartitionControllerReport) error {
	p.mustBeInitialized()
	if err := p.beginForegroundOperation(); err != nil {
		status := p.controller.status.publish(ControllerCycleStatusClosed, NoGeneration, NoGeneration, controllerCycleReasonClosed)
		if dst != nil {
			*dst = PartitionControllerReport{Status: status, Lifecycle: p.lifecycle.Load()}
		}
		return err
	}
	defer p.endForegroundOperation()

	if dst == nil {
		return nil
	}
	if !p.controller.cycleGate.begin() {
		status := p.controller.status.publish(
			ControllerCycleStatusAlreadyRunning,
			NoGeneration,
			NoGeneration,
			controllerCycleReasonAlreadyRunning,
		)
		*dst = PartitionControllerReport{Status: status, Lifecycle: p.lifecycle.Load()}
		return nil
	}
	defer p.controller.cycleGate.end()

	p.controller.mu.Lock()
	defer p.controller.mu.Unlock()

	generation := p.generation.Advance()
	runtime := p.currentRuntimeSnapshot()
	now := clockNow(p.controller.clock)
	indexes := p.controllerSampleIndexes(nil)
	sample := dst.Sample
	p.sampleIndexesWithRuntimeAndGeneration(&sample, runtime, generation, indexes, true)

	previous := sample
	elapsed := time.Duration(0)
	if p.controller.hasPreviousSample {
		previous = p.controller.previousSample
		elapsed = clockElapsed(p.controller.previousSampleTime, now)
	}
	window := dst.Window
	window.Reset(previous, sample)
	rates := NewPoolPartitionTimedWindowRates(window, elapsed)
	ewma := p.controller.ewma.WithUpdate(PoolPartitionEWMAConfig{}, elapsed, rates)
	nextClassEWMA := p.controllerUpdatedClassEWMA(window, elapsed)

	metrics := newPoolPartitionMetrics(p.name, sample)
	budget := newPartitionBudgetSnapshot(runtime.Policy.Budget, sample)
	pressure := newEffectivePartitionPressureSnapshot(runtime.Policy.Pressure, runtime.Pressure, sample)
	scores := NewPoolPartitionScores(rates, ewma, budget, pressure)
	trimPlan := newPartitionTrimPlan(runtime.Policy.Trim, pressure, sample)
	dirtyIndexes := p.activeRegistry.dirtyIndexes(nil)
	activeDeltas := controllerPoolActivityDeltas(indexes, window, dirtyIndexes)
	budgetPublication := p.controllerPoolBudgetReport(generation, runtime, window, elapsed, nextClassEWMA)
	poolBudgetTargets := budgetPublication.Targets
	var budgetPublicationErr error
	if len(poolBudgetTargets) > 0 && budgetPublication.CanPublish() {
		appliedPublication, err := p.applyPoolBudgetTargetsLocked(poolBudgetTargets)
		budgetPublicationErr = err
		budgetPublication.Published = appliedPublication.Published
		markControllerClassReportsPublished(&budgetPublication, appliedPublication)
		if err != nil {
			budgetPublication.FailureReason = controllerCycleBudgetPublicationFailureReasonForError(err, controllerCycleReasonFailed)
		}
		if !appliedPublication.Published {
			budgetPublication.FailureReason = appliedPublication.FailureReason
		}
	}
	if budgetPublicationErr != nil {
		statusReason := controllerCycleBudgetPublicationFailureReasonForError(budgetPublicationErr, controllerCycleReasonFailed)
		budgetPublication.FailureReason = statusReason
		status := p.controller.status.publish(ControllerCycleStatusFailed, generation, NoGeneration, statusReason)

		poolScoreReports := copyPoolBudgetScoreReports(budgetPublication.PoolScores)
		classScoreReports := partitionControllerClassScoreReports(budgetPublication)
		*dst = PartitionControllerReport{
			Status:            status,
			Generation:        generation,
			PolicyGeneration:  sample.PolicyGeneration,
			Lifecycle:         p.lifecycle.Load(),
			Sample:            sample,
			Metrics:           metrics,
			Window:            window,
			Rates:             rates,
			EWMA:              ewma,
			Scores:            scores,
			PoolScores:        poolScoreReports,
			ClassScores:       classScoreReports,
			Budget:            budget,
			Pressure:          pressure,
			TrimPlan:          trimPlan,
			PoolBudgetTargets: poolBudgetTargets,
			BudgetPublication: budgetPublication,
		}
		return budgetPublicationErr
	}
	if len(poolBudgetTargets) > 0 && !budgetPublication.Published {
		statusReason := controllerCycleUnpublishedFailureReason(budgetPublication.FailureReason)
		status := p.controller.status.publish(ControllerCycleStatusUnpublished, generation, NoGeneration, statusReason)

		poolScoreReports := copyPoolBudgetScoreReports(budgetPublication.PoolScores)
		classScoreReports := partitionControllerClassScoreReports(budgetPublication)
		*dst = PartitionControllerReport{
			Status:            status,
			Generation:        generation,
			PolicyGeneration:  sample.PolicyGeneration,
			Lifecycle:         p.lifecycle.Load(),
			Sample:            sample,
			Metrics:           metrics,
			Window:            window,
			Rates:             rates,
			EWMA:              ewma,
			Scores:            scores,
			PoolScores:        poolScoreReports,
			ClassScores:       classScoreReports,
			Budget:            budget,
			Pressure:          pressure,
			TrimPlan:          trimPlan,
			PoolBudgetTargets: poolBudgetTargets,
			BudgetPublication: budgetPublication,
		}
		return nil
	}
	// Activity observation is the last normally fallible bookkeeping step before
	// physical trim. The sampled indexes are generated by the partition registry,
	// so an error here indicates an internal index invariant failure; running it
	// before trim prevents a Failed status from hiding already-executed trim work.
	if err := p.activeRegistry.observeControllerActivityWithDirtyIndexes(indexes, activeDeltas, dirtyIndexes); err != nil {
		status := p.controller.status.publish(
			ControllerCycleStatusFailed,
			generation,
			NoGeneration,
			controllerCycleFailureReasonForError(err, controllerCycleReasonFailed),
		)
		*dst = PartitionControllerReport{Status: status, Generation: generation, Lifecycle: p.lifecycle.Load()}
		return err
	}
	trimResult := p.executeTrimPlanWithScoring(trimPlan, newPartitionTrimScoringContext(window))
	p.markDirtyProcessed()
	p.controller.previousSample = copyPoolPartitionSampleInto(p.controller.previousSample, sample)
	p.controller.previousSampleTime = now
	p.controller.hasPreviousSample = true
	p.controller.ewma = ewma
	p.controller.ewmaByPoolClass = nextClassEWMA
	p.controller.cycles++
	_ = p.controller.generation.Advance()

	status := p.controller.status.publish(ControllerCycleStatusApplied, generation, generation, "")
	poolScoreReports := copyPoolBudgetScoreReports(budgetPublication.PoolScores)
	classScoreReports := partitionControllerClassScoreReports(budgetPublication)
	*dst = PartitionControllerReport{
		Status:            status,
		Generation:        generation,
		PolicyGeneration:  sample.PolicyGeneration,
		Lifecycle:         p.lifecycle.Load(),
		Sample:            sample,
		Metrics:           metrics,
		Window:            window,
		Rates:             rates,
		EWMA:              ewma,
		Scores:            scores,
		PoolScores:        poolScoreReports,
		ClassScores:       classScoreReports,
		Budget:            budget,
		Pressure:          pressure,
		TrimPlan:          trimPlan,
		TrimResult:        trimResult,
		PoolBudgetTargets: poolBudgetTargets,
		BudgetPublication: budgetPublication,
	}
	return nil
}

// ControllerStatus returns the lightweight status for the last manual
// partition controller cycle.
//
// The accessor is safe after Close and returns a copy. It does not sample Pools,
// publish budgets, execute trim, or retain the heavy report returned by TickInto.
func (p *PoolPartition) ControllerStatus() PoolPartitionControllerStatus {
	p.mustBeInitialized()
	return p.controller.status.load()
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
