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

// PoolGroupPolicy defines group-level manual control behavior.
//
// PoolGroupPolicy describes how a group interprets aggregate partition state and
// how manual TickInto distributes retained-budget targets. It does not authorize
// automatic scheduling, background goroutines, trim execution, or pressure
// propagation.
type PoolGroupPolicy struct {
	// Coordinator reserves automatic scheduling policy. Current PoolGroup only
	// supports manual foreground coordinator cycles through Tick and TickInto.
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

// PoolGroupCoordinatorPolicy reserves future automatic coordination settings.
//
// The current PoolGroup implementation does not start goroutines, timers, or
// tickers. Validate rejects enabled or scheduled coordinator policy until a
// real automatic coordinator exists.
type PoolGroupCoordinatorPolicy struct {
	// Enabled is reserved for future automatic coordination and is rejected today.
	Enabled bool

	// TickInterval is reserved for a future automatic scheduling cadence.
	TickInterval time.Duration
}

// DefaultPoolGroupPolicy returns the default manual group policy.
func DefaultPoolGroupPolicy() PoolGroupPolicy { return PoolGroupPolicy{} }

// Normalize returns p unchanged.
func (p PoolGroupPolicy) Normalize() PoolGroupPolicy {
	return p
}

// Validate validates group policy values.
func (p PoolGroupPolicy) Validate() error {
	p = p.Normalize()
	if p.Coordinator.TickInterval < 0 {
		return newError(ErrInvalidPolicy, "bufferpool.PoolGroupPolicy: coordinator tick interval must not be negative")
	}
	if p.Coordinator.Enabled {
		return newError(ErrInvalidPolicy, "bufferpool.PoolGroupPolicy: automatic coordinator is not supported")
	}
	if p.Coordinator.TickInterval != 0 {
		return newError(ErrInvalidPolicy, "bufferpool.PoolGroupPolicy: coordinator tick interval is not supported without automatic coordination")
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
