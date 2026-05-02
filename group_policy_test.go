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
	"testing"
	"time"
)

// TestPoolGroupPolicyNormalizesEnabledCoordinatorScheduler verifies that the
// opt-in scheduler receives a deterministic default cadence when the caller sets
// Enabled without a TickInterval.
func TestPoolGroupPolicyNormalizesEnabledCoordinatorScheduler(t *testing.T) {
	policy := PoolGroupPolicy{
		Coordinator: PoolGroupCoordinatorPolicy{Enabled: true},
		Budget:      PartitionBudgetPolicy{MaxRetainedBytes: MiB},
	}

	normalized := policy.Normalize()
	if !normalized.Coordinator.Enabled || normalized.Coordinator.TickInterval != defaultGroupCoordinatorTickInterval {
		t.Fatalf("Normalize() coordinator = %+v, want enabled default interval %s",
			normalized.Coordinator, defaultGroupCoordinatorTickInterval)
	}
}

// TestPoolGroupPolicyAcceptsAutomaticCoordinator keeps the policy contract
// honest now that PoolGroup owns an opt-in scheduler runtime. Enabled is no
// longer accepted-but-inert: Normalize assigns a real cadence and construction
// starts the scheduler after the group is fully initialized.
func TestPoolGroupPolicyAcceptsAutomaticCoordinator(t *testing.T) {
	policy := PoolGroupPolicy{Coordinator: PoolGroupCoordinatorPolicy{Enabled: true}}

	requireGroupNoError(t, policy.Validate())
}

// TestPoolGroupPolicyRejectsCoordinatorTickIntervalWithoutScheduler rejects
// a dormant interval when the scheduler is disabled. A cadence without Enabled
// would imply background work that the group will not start.
func TestPoolGroupPolicyRejectsCoordinatorTickIntervalWithoutScheduler(t *testing.T) {
	policy := PoolGroupPolicy{Coordinator: PoolGroupCoordinatorPolicy{TickInterval: time.Second}}
	err := policy.Validate()
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
}

// TestPoolGroupPolicyValidateRejectsNegativeCoordinatorInterval locks validation.
func TestPoolGroupPolicyValidateRejectsNegativeCoordinatorInterval(t *testing.T) {
	policy := PoolGroupPolicy{Coordinator: PoolGroupCoordinatorPolicy{Enabled: true, TickInterval: -time.Second}}
	err := policy.Validate()
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
}

// TestDefaultPoolGroupPolicyIsManual keeps the zero group policy valid and
// explicitly scheduler-free. Manual foreground Tick and TickInto remain
// supported, and no scheduler fields are enabled or filled by default.
func TestDefaultPoolGroupPolicyIsManual(t *testing.T) {
	policy := DefaultPoolGroupPolicy()
	if policy.Coordinator.Enabled || policy.Coordinator.TickInterval != 0 {
		t.Fatalf("default group coordinator policy = %+v, want manual scheduler-free policy", policy.Coordinator)
	}
	requireGroupNoError(t, policy.Validate())
}

// TestPoolGroupPolicyIsZero verifies that scheduler fields participate in
// value-model equality and zero-policy detection.
func TestPoolGroupPolicyIsZero(t *testing.T) {
	if !DefaultPoolGroupPolicy().IsZero() {
		t.Fatalf("default policy should be zero")
	}
	policy := DefaultPoolGroupPolicy()
	policy.Coordinator.Enabled = true
	if policy.IsZero() {
		t.Fatalf("enabled coordinator should make policy non-zero")
	}
}
