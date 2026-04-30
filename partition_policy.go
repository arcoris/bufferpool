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
	// defaultPartitionControllerTickInterval is used when automatic scheduling
	// is enabled in policy, even though current PoolPartition exposes only
	// manual ticks.
	defaultPartitionControllerTickInterval = time.Second
)

// PartitionPolicy defines partition-level control-plane behavior.
//
// Pool Policy describes local retained-storage behavior. PartitionPolicy
// describes how a partition samples owned Pools, interprets aggregate retained
// plus active memory, and plans future control-plane work. Current
// PoolPartition does not start background goroutines.
type PartitionPolicy struct {
	// Controller configures explicit tick behavior and future scheduling policy.
	Controller PartitionControllerPolicy

	// Budget configures partition-local retained, active, and owned limits.
	Budget PartitionBudgetPolicy

	// Pressure maps aggregate owned bytes into pressure levels.
	Pressure PartitionPressurePolicy

	// Trim configures bounded partition-local retained-storage trim work.
	Trim PartitionTrimPolicy
}

// PartitionControllerPolicy defines explicit controller tick behavior.
//
// Enabled reserves policy space for future automatic/background scheduling.
// Current PoolPartition has no background goroutine, and manual Tick remains
// available whenever the partition lifecycle is active.
type PartitionControllerPolicy struct {
	// Enabled reserves policy space for future automatic scheduling.
	Enabled bool

	// TickInterval is the future automatic scheduling cadence.
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

// Normalize returns p with unset controller defaults completed.
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
	if p.Controller.Enabled && p.Controller.TickInterval <= 0 {
		return newError(ErrInvalidPolicy, "bufferpool.PartitionPolicy: controller tick interval must be positive")
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
