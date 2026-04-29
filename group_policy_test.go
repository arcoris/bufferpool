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

func TestPoolGroupPolicyNormalize(t *testing.T) {
	policy := PoolGroupPolicy{Budget: PartitionBudgetPolicy{MaxRetainedBytes: MiB}}
	normalized := policy.Normalize()
	if normalized != policy {
		t.Fatalf("Normalize() = %#v, want unchanged %#v", normalized, policy)
	}
}

// TestPoolGroupPolicyValidateRejectsEnabledCoordinator rejects no-op automation.
func TestPoolGroupPolicyValidateRejectsEnabledCoordinator(t *testing.T) {
	policy := PoolGroupPolicy{Coordinator: PoolGroupCoordinatorPolicy{Enabled: true}}
	err := policy.Validate()
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
}

// TestPoolGroupPolicyValidateRejectsCoordinatorInterval rejects inert cadence.
func TestPoolGroupPolicyValidateRejectsCoordinatorInterval(t *testing.T) {
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

// TestPoolGroupPolicyValidateAcceptsDefault keeps the zero policy usable.
func TestPoolGroupPolicyValidateAcceptsDefault(t *testing.T) {
	requireGroupNoError(t, DefaultPoolGroupPolicy().Validate())
}

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
