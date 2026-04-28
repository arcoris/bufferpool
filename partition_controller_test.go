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

import "testing"

// TestPoolPartitionTickReportsCurrentStateAndAdvancesGeneration verifies tick report coherence.
func TestPoolPartitionTickReportsCurrentStateAndAdvancesGeneration(t *testing.T) {
	config := testPartitionConfig("primary")
	config.Policy.Controller = PartitionControllerPolicy{Enabled: true}
	config.Policy.Trim = PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 1, MaxBytesPerCycle: 4 * KiB}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	before := partition.Sample().Generation
	report, err := partition.Tick()
	requirePartitionNoError(t, err)

	if !report.Generation.After(before) {
		t.Fatalf("Tick generation = %s, want after %s", report.Generation, before)
	}
	if report.PolicyGeneration != InitialGeneration {
		t.Fatalf("PolicyGeneration = %s, want %s", report.PolicyGeneration, InitialGeneration)
	}
	if report.Sample.Generation != report.Generation {
		t.Fatalf("sample/report generation = %s/%s", report.Sample.Generation, report.Generation)
	}
	if report.Lifecycle != LifecycleActive {
		t.Fatalf("Tick lifecycle = %s, want active", report.Lifecycle)
	}
	if report.Sample.PoolCount != 1 || report.Metrics.PoolCount != 1 {
		t.Fatalf("Tick pool count mismatch: sample=%d metrics=%d", report.Sample.PoolCount, report.Metrics.PoolCount)
	}
	if !report.TrimPlan.Enabled {
		t.Fatalf("Tick trim plan should be enabled")
	}
	if report.TrimResult.Attempted || report.TrimResult.Executed {
		t.Fatalf("Tick should not execute physical trim in current implementation: %+v", report.TrimResult)
	}
}

// TestPoolPartitionTickRejectsClosedPartition verifies lifecycle gating for ticks.
func TestPoolPartitionTickRejectsClosedPartition(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Close())

	_, err = partition.Tick()
	requirePartitionErrorIs(t, err, ErrClosed)
}

// TestPoolPartitionManualTickAllowedWhenControllerDisabled verifies manual tick semantics.
func TestPoolPartitionManualTickAllowedWhenControllerDisabled(t *testing.T) {
	config := testPartitionConfig("primary")
	config.Policy.Controller = PartitionControllerPolicy{Enabled: false}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	report, err := partition.Tick()
	requirePartitionNoError(t, err)
	if report.Generation.IsZero() || report.Sample.Generation != report.Generation {
		t.Fatalf("manual Tick report has incoherent generation: %+v", report)
	}
}
