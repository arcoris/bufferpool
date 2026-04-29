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
	"errors"
	"math"
	"testing"
)

// testGroupPoolConfig returns a minimal Pool config for a group-owned partition.
func testGroupPoolConfig(name string) PartitionPoolConfig {
	return PartitionPoolConfig{
		Name: name,
		Config: PoolConfig{
			Name: name,
		},
	}
}

// testGroupPartitionConfig returns a minimal group partition config.
func testGroupPartitionConfig(name string, poolNames ...string) GroupPartitionConfig {
	if len(poolNames) == 0 {
		poolNames = []string{name + "-pool"}
	}
	partitionConfig := DefaultPoolPartitionConfig()
	partitionConfig.Name = name
	partitionConfig.Pools = make([]PartitionPoolConfig, len(poolNames))
	for index, poolName := range poolNames {
		partitionConfig.Pools[index] = testGroupPoolConfig(poolName)
	}
	return GroupPartitionConfig{Name: name, Config: partitionConfig}
}

// testGroupConfig returns a minimal valid PoolGroup config.
func testGroupConfig(partitionNames ...string) PoolGroupConfig {
	if len(partitionNames) == 0 {
		partitionNames = []string{"primary"}
	}
	config := DefaultPoolGroupConfig()
	config.Name = "test-group"
	config.Partitions = make([]GroupPartitionConfig, len(partitionNames))
	for index, name := range partitionNames {
		config.Partitions[index] = testGroupPartitionConfig(name)
	}
	return config
}

// testNewPoolGroup constructs a test group and registers cleanup.
func testNewPoolGroup(t *testing.T, partitionNames ...string) *PoolGroup {
	t.Helper()
	group, err := NewPoolGroup(testGroupConfig(partitionNames...))
	if err != nil {
		t.Fatalf("NewPoolGroup() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := group.Close(); closeErr != nil {
			t.Fatalf("PoolGroup.Close() error = %v", closeErr)
		}
	})
	return group
}

// requireGroupNoError fails the test when err is non-nil.
func requireGroupNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// requireGroupErrorIs fails the test unless err matches target.
func requireGroupErrorIs(t *testing.T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want errors.Is(..., %v)", err, target)
	}
}

// requireGroupPanic fails the test unless fn panics.
func requireGroupPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("expected panic")
		}
	}()
	fn()
}

// requireGroupRatioClose compares fixed-point ratios through their float form.
func requireGroupRatioClose(t *testing.T, got PolicyRatio, want float64) {
	t.Helper()
	if math.Abs(got.Float64()-want) > 0.0001 {
		t.Fatalf("ratio = %.4f, want %.4f", got.Float64(), want)
	}
}

// testGroupSampleWithCounters builds a hand-authored group sample for window,
// rate, metrics, and controller-evaluation tests.
func testGroupSampleWithCounters(generation Generation, counters PoolCountersSnapshot, leases LeaseCountersSnapshot) PoolGroupSample {
	aggregate := PoolPartitionSample{
		Generation:           generation,
		PolicyGeneration:     generation,
		Lifecycle:            LifecycleActive,
		Scope:                PoolPartitionSampleScopePartition,
		TotalPoolCount:       1,
		SampledPoolCount:     1,
		PoolCount:            1,
		PoolCounters:         counters,
		LeaseLifecycle:       LifecycleActive,
		LeaseGeneration:      generation,
		LeaseCounters:        leases,
		ActiveLeases:         int(leases.ActiveLeases),
		CurrentRetainedBytes: counters.CurrentRetainedBytes,
		CurrentActiveBytes:   leases.ActiveBytes,
		CurrentOwnedBytes:    poolSaturatingAdd(counters.CurrentRetainedBytes, leases.ActiveBytes),
	}
	return PoolGroupSample{
		Generation:           generation,
		PolicyGeneration:     generation,
		Lifecycle:            LifecycleActive,
		PartitionCount:       1,
		PoolCount:            1,
		Aggregate:            aggregate,
		CurrentRetainedBytes: aggregate.CurrentRetainedBytes,
		CurrentActiveBytes:   aggregate.CurrentActiveBytes,
		CurrentOwnedBytes:    aggregate.CurrentOwnedBytes,
	}
}
