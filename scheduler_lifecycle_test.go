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

// TestManualTickStillWorksWithoutScheduler verifies the opt-in scheduler
// integration does not change the manual foreground control contract. Default
// owners keep their scheduler runtimes stopped, while TickInto still executes
// through the retained controller/coordinator status path.
func TestManualTickStillWorksWithoutScheduler(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	var partitionReport PartitionControllerReport

	requirePartitionNoError(t, partition.TickInto(&partitionReport))
	assertPoolPartitionSchedulerStopped(t, partition, "manual partition")
	if partitionReport.Status.Status == ControllerCycleStatusUnset {
		t.Fatalf("partition TickInto status = %+v, want a published manual cycle status", partitionReport.Status)
	}

	group := testNewPoolGroup(t, "alpha")
	var groupReport PoolGroupCoordinatorReport

	requireGroupNoError(t, group.TickInto(&groupReport))
	assertPoolGroupSchedulerStopped(t, group, "manual group")
	if groupReport.Status.Status == ControllerCycleStatusUnset {
		t.Fatalf("group TickInto status = %+v, want a published manual cycle status", groupReport.Status)
	}
}

// TestSchedulerRejectsIntervalWithoutEnabled keeps both reserved scheduler
// policy surfaces honest: an interval without Enabled would imply background
// work that neither owner starts in manual mode.
func TestSchedulerRejectsIntervalWithoutEnabled(t *testing.T) {
	partitionPolicy := DefaultPartitionPolicy()
	partitionPolicy.Controller.TickInterval = time.Second
	requirePartitionErrorIs(t, partitionPolicy.Validate(), ErrInvalidPolicy)

	groupPolicy := DefaultPoolGroupPolicy()
	groupPolicy.Coordinator.TickInterval = time.Second
	requireGroupErrorIs(t, groupPolicy.Validate(), ErrInvalidPolicy)
}

// TestSchedulerRejectsNegativeInterval verifies both opt-in scheduler policies
// reject negative cadences instead of passing invalid intervals to the internal
// ticker runtime.
func TestSchedulerRejectsNegativeInterval(t *testing.T) {
	partitionPolicy := DefaultPartitionPolicy()
	partitionPolicy.Controller.Enabled = true
	partitionPolicy.Controller.TickInterval = -time.Second
	requirePartitionErrorIs(t, partitionPolicy.Validate(), ErrInvalidPolicy)

	groupPolicy := DefaultPoolGroupPolicy()
	groupPolicy.Coordinator.Enabled = true
	groupPolicy.Coordinator.TickInterval = -time.Second
	requireGroupErrorIs(t, groupPolicy.Validate(), ErrInvalidPolicy)
}
