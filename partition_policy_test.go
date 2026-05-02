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

// TestPartitionPolicyAcceptsAutomaticController keeps the policy contract honest
// now that PoolPartition owns an opt-in scheduler runtime. Enabled is no longer
// accepted-but-inert: Normalize assigns a real cadence and construction starts
// the scheduler after the partition is fully initialized.
func TestPartitionPolicyAcceptsAutomaticController(t *testing.T) {
	policy := PartitionPolicy{Controller: PartitionControllerPolicy{Enabled: true}}

	requirePartitionNoError(t, policy.Validate())
}

// TestPartitionPolicyRejectsControllerTickIntervalWithoutScheduler rejects a
// dormant interval when the scheduler is disabled. A cadence without Enabled
// would imply background work that the partition will not start.
func TestPartitionPolicyRejectsControllerTickIntervalWithoutScheduler(t *testing.T) {
	policy := PartitionPolicy{Controller: PartitionControllerPolicy{TickInterval: time.Second}}

	err := policy.Validate()
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)
}

// TestPartitionPolicyNormalizesEnabledControllerScheduler verifies that the
// opt-in scheduler receives a deterministic default cadence when the caller sets
// Enabled without a TickInterval.
func TestPartitionPolicyNormalizesEnabledControllerScheduler(t *testing.T) {
	policy := PartitionPolicy{
		Controller: PartitionControllerPolicy{Enabled: true},
		Budget:     PartitionBudgetPolicy{MaxRetainedBytes: MiB},
	}

	normalized := policy.Normalize()
	if !normalized.Controller.Enabled || normalized.Controller.TickInterval != defaultPartitionControllerTickInterval {
		t.Fatalf("Normalize() controller = %+v, want enabled default interval %s",
			normalized.Controller, defaultPartitionControllerTickInterval)
	}
}

// TestDefaultPartitionPolicyIsManual pins the default partition policy as
// explicit manual orchestration. Manual Tick and TickInto remain supported, and
// no scheduler fields are enabled or filled by default.
func TestDefaultPartitionPolicyIsManual(t *testing.T) {
	policy := DefaultPartitionPolicy()
	if policy.Controller.Enabled || policy.Controller.TickInterval != 0 {
		t.Fatalf("default partition controller policy = %+v, want manual scheduler-free policy", policy.Controller)
	}
	requirePartitionNoError(t, policy.Validate())
}

// TestManualTickPolicyStillValid proves rejecting automatic scheduler policy
// does not weaken the existing manual orchestration API. Both owners can still
// run foreground Tick calls when their scheduler-reserved policy fields are
// disabled.
func TestManualTickPolicyStillValid(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	partitionReport, err := partition.Tick()
	requirePartitionNoError(t, err)
	if partitionReport.Generation.IsZero() {
		t.Fatalf("partition Tick() generation = %s, want manual cycle generation", partitionReport.Generation)
	}

	group := testNewPoolGroup(t, "alpha")
	groupReport, err := group.Tick()
	requireGroupNoError(t, err)
	if groupReport.Generation.IsZero() {
		t.Fatalf("group Tick() generation = %s, want manual cycle generation", groupReport.Generation)
	}
}
