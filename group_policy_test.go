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
	policy := PoolGroupPolicy{Coordinator: PoolGroupCoordinatorPolicy{Enabled: true}}
	normalized := policy.Normalize()
	if normalized.Coordinator.TickInterval != defaultGroupCoordinatorTickInterval {
		t.Fatalf("TickInterval = %v, want %v", normalized.Coordinator.TickInterval, defaultGroupCoordinatorTickInterval)
	}
}

func TestPoolGroupPolicyValidateRejectsNegativeCoordinatorInterval(t *testing.T) {
	policy := PoolGroupPolicy{Coordinator: PoolGroupCoordinatorPolicy{Enabled: true, TickInterval: -time.Second}}
	err := policy.Validate()
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
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
