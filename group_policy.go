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
	// defaultGroupCoordinatorTickInterval is used only when future automatic
	// group coordination is enabled by policy. Current PoolGroup exposes only
	// manual foreground ticks.
	defaultGroupCoordinatorTickInterval = time.Second
)

// PoolGroupPolicy defines group-level observational control behavior.
//
// PoolGroupPolicy describes how a group interprets aggregate partition state.
// It does not authorize automatic policy application in the current
// implementation. Budget and pressure are group-level projections only.
type PoolGroupPolicy struct {
	// Coordinator configures explicit tick behavior and future scheduling policy.
	Coordinator PoolGroupCoordinatorPolicy

	// Budget configures group-level aggregate retained, active, and owned limits.
	Budget PartitionBudgetPolicy

	// Pressure maps aggregate group-owned bytes into pressure levels.
	Pressure PartitionPressurePolicy

	// Score configures group-level score projection.
	Score PoolGroupScoreEvaluatorConfig
}

// PoolGroupCoordinatorPolicy defines explicit group coordinator tick behavior.
//
// Enabled reserves policy space for future automatic/background scheduling. The
// current PoolGroup implementation does not start goroutines; manual Tick
// remains foreground-only.
type PoolGroupCoordinatorPolicy struct {
	// Enabled reserves policy space for future automatic coordination.
	Enabled bool

	// TickInterval is the future automatic scheduling cadence.
	TickInterval time.Duration
}

// DefaultPoolGroupPolicy returns the default observational group policy.
func DefaultPoolGroupPolicy() PoolGroupPolicy { return PoolGroupPolicy{} }

// Normalize returns p with unset coordinator defaults completed.
func (p PoolGroupPolicy) Normalize() PoolGroupPolicy {
	if p.Coordinator.Enabled && p.Coordinator.TickInterval == 0 {
		p.Coordinator.TickInterval = defaultGroupCoordinatorTickInterval
	}
	return p
}

// Validate validates group policy values.
func (p PoolGroupPolicy) Validate() error {
	p = p.Normalize()
	if p.Coordinator.Enabled && p.Coordinator.TickInterval <= 0 {
		return newError(ErrInvalidPolicy, "bufferpool.PoolGroupPolicy: coordinator tick interval must be positive")
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
