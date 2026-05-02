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

// TestPoolGroupPolicyDoesNotNormalizeUnsupportedCoordinatorScheduler verifies
// Normalize keeps reserved scheduler fields exactly as the caller supplied them.
// Validation, not normalization, is responsible for rejecting unsupported
// automatic coordinator configuration.
func TestPoolGroupPolicyDoesNotNormalizeUnsupportedCoordinatorScheduler(t *testing.T) {
	policy := PoolGroupPolicy{
		Coordinator: PoolGroupCoordinatorPolicy{Enabled: true},
		Budget:      PartitionBudgetPolicy{MaxRetainedBytes: MiB},
	}
	normalized := policy.Normalize()
	if normalized != policy {
		t.Fatalf("Normalize() = %#v, want unchanged %#v", normalized, policy)
	}
}

// TestPoolGroupPolicyRejectsAutomaticCoordinator rejects accepted-but-inert
// automatic coordination until PoolGroup owns a real scheduler runtime.
func TestPoolGroupPolicyRejectsAutomaticCoordinator(t *testing.T) {
	policy := PoolGroupPolicy{Coordinator: PoolGroupCoordinatorPolicy{Enabled: true}}
	err := policy.Validate()
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
}

// TestPoolGroupPolicyRejectsCoordinatorTickIntervalWithoutScheduler rejects
// reserved cadence values because current group controller cycles are manual
// foreground Tick/TickInto calls, not timer-driven background work.
func TestPoolGroupPolicyRejectsCoordinatorTickIntervalWithoutScheduler(t *testing.T) {
	policy := PoolGroupPolicy{Coordinator: PoolGroupCoordinatorPolicy{TickInterval: time.Second}}
	err := policy.Validate()
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
}

// TestPoolGroupPolicyValidateRejectsNegativeCoordinatorInterval locks validation.
func TestPoolGroupPolicyValidateRejectsNegativeCoordinatorInterval(t *testing.T) {
	policy := PoolGroupPolicy{Coordinator: PoolGroupCoordinatorPolicy{TickInterval: -time.Second}}
	err := policy.Validate()
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
}

// TestDefaultPoolGroupPolicyIsManual keeps the zero group policy valid and
// explicitly scheduler-free. Manual foreground Tick and TickInto remain the
// supported orchestration surface for this policy.
func TestDefaultPoolGroupPolicyIsManual(t *testing.T) {
	policy := DefaultPoolGroupPolicy()
	if policy.Coordinator.Enabled || policy.Coordinator.TickInterval != 0 {
		t.Fatalf("default group coordinator policy = %+v, want manual scheduler-free policy", policy.Coordinator)
	}
	requireGroupNoError(t, policy.Validate())
}

// TestPoolGroupPolicyIsZero verifies that reserved scheduler fields still
// participate in value-model equality and zero-policy detection even though
// validation rejects them until automatic coordination exists.
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
