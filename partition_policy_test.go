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

// TestPartitionPolicyRejectsAutomaticController keeps reserved scheduler
// fields honest. PoolPartition has manual foreground Tick and TickInto, but it
// does not own a background controller runtime yet, so Enabled must not validate
// as an inert no-op configuration.
func TestPartitionPolicyRejectsAutomaticController(t *testing.T) {
	policy := PartitionPolicy{Controller: PartitionControllerPolicy{Enabled: true}}

	err := policy.Validate()
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)
}

// TestPartitionPolicyRejectsControllerTickIntervalWithoutScheduler rejects
// timer cadence values until a real scheduler owns goroutines, timers, and
// lifecycle integration. A non-zero interval would otherwise imply work that the
// current manual-only runtime never starts.
func TestPartitionPolicyRejectsControllerTickIntervalWithoutScheduler(t *testing.T) {
	policy := PartitionPolicy{Controller: PartitionControllerPolicy{TickInterval: time.Second}}

	err := policy.Validate()
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)
}

// TestPartitionPolicyDoesNotNormalizeUnsupportedControllerScheduler verifies
// Normalize does not complete scheduler defaults while scheduling is
// unsupported. Keeping the caller-provided value intact makes validation reject
// unsupported automation instead of turning it into a valid-looking policy.
func TestPartitionPolicyDoesNotNormalizeUnsupportedControllerScheduler(t *testing.T) {
	policy := PartitionPolicy{
		Controller: PartitionControllerPolicy{Enabled: true},
		Budget:     PartitionBudgetPolicy{MaxRetainedBytes: MiB},
	}

	normalized := policy.Normalize()
	if normalized != policy {
		t.Fatalf("Normalize() = %#v, want unchanged scheduler fields in %#v", normalized, policy)
	}
}

// TestDefaultPartitionPolicyIsManual pins the default partition policy as
// explicit manual orchestration. Manual Tick and TickInto remain supported, but
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
