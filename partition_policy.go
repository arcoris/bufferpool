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
	// defaultPartitionControllerTickInterval is the first opt-in scheduler
	// cadence for PoolPartition. It is used only when Controller.Enabled is true
	// and the caller leaves TickInterval unset. Manual Tick and TickInto do not
	// depend on this value.
	defaultPartitionControllerTickInterval = time.Second
)

// PartitionPolicy defines partition-level control-plane behavior.
//
// Pool Policy describes local retained-storage behavior. PartitionPolicy
// describes how a partition samples owned Pools, interprets aggregate retained
// plus active memory, and plans future control-plane work. PoolPartition is
// manual by default; automatic controller ticks run only when Controller.Enabled
// is explicitly true. Manual Tick and TickInto remain available regardless of
// scheduler policy.
type PartitionPolicy struct {
	// Controller configures the partition-local controller scheduler. Disabled
	// policy is manual-only. Enabled policy starts an opt-in owner-local
	// scheduler that calls the same TickInto path used by manual callers.
	Controller PartitionControllerPolicy

	// Budget configures partition-local retained, active, and owned limits.
	Budget PartitionBudgetPolicy

	// Pressure maps aggregate owned bytes into pressure levels.
	Pressure PartitionPressurePolicy

	// Trim configures bounded partition-local retained-storage trim work.
	Trim PartitionTrimPolicy
}

// PartitionControllerPolicy configures opt-in automatic partition controller ticks.
//
// The scheduler is disabled by default. When enabled, PoolPartition starts an
// owner-local scheduler after construction is fully initialized; each scheduled
// event calls TickInto and discards the full report after TickInto publishes its
// lightweight ControllerStatus. The scheduler never enters Pool.Get or Pool.Put
// and does not replace manual foreground Tick/TickInto calls.
type PartitionControllerPolicy struct {
	// Enabled starts the opt-in partition-local controller scheduler.
	Enabled bool

	// TickInterval is the scheduler cadence. When Enabled is true and
	// TickInterval is zero, Normalize applies defaultPartitionControllerTickInterval.
	// When Enabled is false, TickInterval must stay zero so a disabled scheduler
	// cannot carry a misleading dormant cadence.
	TickInterval time.Duration
}

// PartitionBudgetPolicy defines partition-level memory limits. Zero means
// unbounded at partition scope.
type PartitionBudgetPolicy struct {
	// MaxRetainedBytes limits bytes retained by owned Pools.
	MaxRetainedBytes Size

	// MaxActiveBytes limits bytes currently checked out through leases.
	MaxActiveBytes Size

	// MaxOwnedBytes limits retained plus active bytes.
	MaxOwnedBytes Size
}

// DefaultPartitionPolicy returns the default explicit-control policy.
func DefaultPartitionPolicy() PartitionPolicy { return PartitionPolicy{} }

// Normalize returns p with supported partition defaults applied.
//
// Scheduler defaults are completed only for explicit opt-in scheduling. Disabled
// policy remains manual-only and does not receive a dormant interval.
func (p PartitionPolicy) Normalize() PartitionPolicy {
	if p.Controller.Enabled && p.Controller.TickInterval == 0 {
		p.Controller.TickInterval = defaultPartitionControllerTickInterval
	}
	p.Trim = p.Trim.Normalize()
	return p
}

// Validate validates partition policy values.
func (p PartitionPolicy) Validate() error {
	p = p.Normalize()
	if p.Controller.Enabled {
		if p.Controller.TickInterval <= 0 {
			return newError(ErrInvalidPolicy, "bufferpool.PartitionPolicy: controller tick interval must be positive")
		}
	} else if p.Controller.TickInterval != 0 {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionPolicy: controller tick interval requires enabled controller scheduler")
	}

	if err := p.Budget.Validate(); err != nil {
		return err
	}
	if err := p.Pressure.Validate(); err != nil {
		return err
	}
	if err := p.Trim.Validate(); err != nil {
		return err
	}
	return nil
}

// IsZero reports whether p contains no explicit partition policy values.
func (p PartitionPolicy) IsZero() bool {
	return !p.Controller.Enabled && p.Controller.TickInterval == 0 && p.Budget.IsZero() && p.Pressure.IsZero() && p.Trim.IsZero()
}

// IsZero reports whether b contains no partition budget limits.
func (b PartitionBudgetPolicy) IsZero() bool {
	return b.MaxRetainedBytes.IsZero() && b.MaxActiveBytes.IsZero() && b.MaxOwnedBytes.IsZero()
}

// Validate validates budget limit relationships.
func (b PartitionBudgetPolicy) Validate() error {
	if !b.MaxOwnedBytes.IsZero() {
		if !b.MaxRetainedBytes.IsZero() && b.MaxRetainedBytes > b.MaxOwnedBytes {
			return newError(ErrInvalidPolicy, "bufferpool.PartitionBudgetPolicy: retained limit must not exceed owned limit")
		}
		if !b.MaxActiveBytes.IsZero() && b.MaxActiveBytes > b.MaxOwnedBytes {
			return newError(ErrInvalidPolicy, "bufferpool.PartitionBudgetPolicy: active limit must not exceed owned limit")
		}
	}
	return nil
}
