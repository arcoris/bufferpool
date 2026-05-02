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

const (
	// defaultGroupCoordinatorTickInterval is the first opt-in scheduler cadence
	// for PoolGroup. It is used only when Coordinator.Enabled is true and the
	// caller leaves TickInterval unset. Manual Tick and TickInto do not depend on
	// this value.
	defaultGroupCoordinatorTickInterval = time.Second
)

// PoolGroupPolicy defines group-level manual control behavior.
//
// PoolGroupPolicy describes how a group interprets aggregate partition state and
// how TickInto distributes retained-budget targets. PoolGroup is manual by
// default; automatic coordinator ticks run only when Coordinator.Enabled is
// explicitly true. Manual Tick and TickInto remain available regardless of
// scheduler policy.
type PoolGroupPolicy struct {
	// Coordinator configures the group-level coordinator scheduler. Disabled
	// policy is manual-only. Enabled policy starts an opt-in owner-local
	// scheduler that calls the same TickInto path used by manual callers.
	Coordinator PoolGroupCoordinatorPolicy

	// Budget configures group aggregate retained, active, and owned limits.
	// MaxRetainedBytes is also the parent retained target for partition budget
	// redistribution during manual TickInto cycles.
	Budget PartitionBudgetPolicy

	// Pressure maps partition-shaped aggregate owned bytes into pressure levels.
	Pressure PartitionPressurePolicy

	// Score configures group-level score projection.
	Score PoolGroupScoreEvaluatorConfig
}

// PoolGroupCoordinatorPolicy configures opt-in automatic group coordinator ticks.
//
// The scheduler is disabled by default. When enabled, PoolGroup starts an
// owner-local scheduler after construction is fully initialized; each scheduled
// event calls TickInto and discards the full report after TickInto publishes its
// lightweight ControllerStatus. The group scheduler does not tick partitions
// automatically, does not scan Pool shard/class internals, does not execute Pool
// trim directly, and does not replace manual foreground Tick/TickInto calls.
// Live PublishPolicy updates reject coordinator scheduler mode or interval
// changes in this stage so construction remains the only activation point.
type PoolGroupCoordinatorPolicy struct {
	// Enabled starts the opt-in group-level coordinator scheduler.
	Enabled bool

	// TickInterval is the scheduler cadence. When Enabled is true and
	// TickInterval is zero, Normalize applies defaultGroupCoordinatorTickInterval.
	// When Enabled is false, TickInterval must stay zero so a disabled scheduler
	// cannot carry a misleading dormant cadence.
	TickInterval time.Duration
}

// DefaultPoolGroupPolicy returns the default manual group policy.
func DefaultPoolGroupPolicy() PoolGroupPolicy { return PoolGroupPolicy{} }

// Normalize returns p with supported group defaults applied.
//
// Scheduler defaults are completed only for explicit opt-in scheduling. Disabled
// policy remains manual-only and does not receive a dormant interval.
func (p PoolGroupPolicy) Normalize() PoolGroupPolicy {
	if p.Coordinator.Enabled && p.Coordinator.TickInterval == 0 {
		p.Coordinator.TickInterval = defaultGroupCoordinatorTickInterval
	}
	return p
}

// Validate validates group policy values.
func (p PoolGroupPolicy) Validate() error {
	p = p.Normalize()

	if p.Coordinator.Enabled {
		if p.Coordinator.TickInterval <= 0 {
			return newError(ErrInvalidPolicy, "bufferpool.PoolGroupPolicy: coordinator tick interval must be positive")
		}
	} else if p.Coordinator.TickInterval != 0 {
		return newError(ErrInvalidPolicy, "bufferpool.PoolGroupPolicy: coordinator tick interval requires enabled coordinator scheduler")
	}

	if err := p.Budget.Validate(); err != nil {
		return err
	}
	if err := p.Pressure.Validate(); err != nil {
		return err
	}

	return nil
}

// IsZero reports whether p contains no explicit group policy values.
func (p PoolGroupPolicy) IsZero() bool {
	return !p.Coordinator.Enabled &&
		p.Coordinator.TickInterval == 0 &&
		p.Budget.IsZero() &&
		p.Pressure.IsZero() &&
		p.Score.IsZero()
}
