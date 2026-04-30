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

func TestPoolGroupComputeDefaultPartitionCountFormula(t *testing.T) {
	policy := PoolGroupPartitioningPolicy{
		Mode:                          PoolGroupPartitioningModeAuto,
		MinPartitions:                 1,
		MaxPartitions:                 8,
		TargetActiveEntries:           100,
		EstimatedActiveClassesPerPool: 10,
		EstimatedActiveShardsPerClass: 5,
		Placement:                     PoolPlacementPolicyHash,
	}
	if got := computeDefaultPartitionCount(policy, 5); got != 3 {
		t.Fatalf("partition count = %d, want 3", got)
	}
}

func TestPoolGroupComputeDefaultPartitionCountRespectsMin(t *testing.T) {
	policy := PoolGroupPartitioningPolicy{
		Mode:                          PoolGroupPartitioningModeAuto,
		MinPartitions:                 4,
		MaxPartitions:                 8,
		TargetActiveEntries:           1000,
		EstimatedActiveClassesPerPool: 1,
		EstimatedActiveShardsPerClass: 1,
		Placement:                     PoolPlacementPolicyHash,
	}
	if got := computeDefaultPartitionCount(policy, 1); got != 4 {
		t.Fatalf("partition count = %d, want 4", got)
	}
}

func TestPoolGroupComputeDefaultPartitionCountRespectsMax(t *testing.T) {
	policy := PoolGroupPartitioningPolicy{
		Mode:                          PoolGroupPartitioningModeAuto,
		MinPartitions:                 1,
		MaxPartitions:                 3,
		TargetActiveEntries:           1,
		EstimatedActiveClassesPerPool: 10,
		EstimatedActiveShardsPerClass: 10,
		Placement:                     PoolPlacementPolicyHash,
	}
	if got := computeDefaultPartitionCount(policy, 10); got != 3 {
		t.Fatalf("partition count = %d, want 3", got)
	}
}

func TestPoolGroupPartitionAssignmentIsDeterministic(t *testing.T) {
	config := testManagedGroupConfig("alpha", "beta", "gamma", "delta")
	config.Partitioning.MinPartitions = 4
	config.Partitioning.MaxPartitions = 4

	_, first, err := newGroupPartitionAssignments(config)
	requireGroupNoError(t, err)
	_, second, err := newGroupPartitionAssignments(config)
	requireGroupNoError(t, err)

	for _, name := range testManagedGroupPoolNames(config) {
		firstLocation, ok := first.location(name)
		if !ok {
			t.Fatalf("first location missing for %q", name)
		}
		secondLocation, ok := second.location(name)
		if !ok {
			t.Fatalf("second location missing for %q", name)
		}
		if firstLocation != secondLocation {
			t.Fatalf("location for %q changed: first=%+v second=%+v", name, firstLocation, secondLocation)
		}
	}
}

func TestPoolGroupPartitionAssignmentIgnoresInputOrderForPlacement(t *testing.T) {
	firstConfig := testManagedGroupConfig("alpha", "beta", "gamma")
	secondConfig := testManagedGroupConfig("gamma", "alpha", "beta")
	firstConfig.Partitioning.MinPartitions = 4
	firstConfig.Partitioning.MaxPartitions = 4
	secondConfig.Partitioning = firstConfig.Partitioning

	_, first, err := newGroupPartitionAssignments(firstConfig)
	requireGroupNoError(t, err)
	_, second, err := newGroupPartitionAssignments(secondConfig)
	requireGroupNoError(t, err)

	for _, name := range []string{"alpha", "beta", "gamma"} {
		firstLocation, _ := first.location(name)
		secondLocation, _ := second.location(name)
		if firstLocation.PartitionName != secondLocation.PartitionName {
			t.Fatalf("partition for %q = %q, want %q", name, secondLocation.PartitionName, firstLocation.PartitionName)
		}
	}
}

func TestPoolGroupExplicitPoolAssignmentUsesNamedPartitions(t *testing.T) {
	config := testManagedGroupConfig("api", "worker")
	config.Partitioning.Mode = PoolGroupPartitioningModeExplicit
	config.Partitions = []GroupPartitionConfig{
		testEmptyGroupPartitionConfig("alpha"),
		testEmptyGroupPartitionConfig("beta"),
	}
	config.Pools[0].Partition = "alpha"
	config.Pools[1].Partition = "beta"

	partitions, directory, err := newGroupPartitionAssignments(config)
	requireGroupNoError(t, err)
	if len(partitions) != 2 {
		t.Fatalf("partition count = %d, want 2", len(partitions))
	}
	api, ok := directory.location("api")
	if !ok || api.PartitionName != "alpha" {
		t.Fatalf("api location = %+v ok=%v, want alpha", api, ok)
	}
	worker, ok := directory.location("worker")
	if !ok || worker.PartitionName != "beta" {
		t.Fatalf("worker location = %+v ok=%v, want beta", worker, ok)
	}
}

func TestPoolGroupExplicitPoolAssignmentRejectsMissingPartition(t *testing.T) {
	config := testManagedGroupConfig("api")
	config.Partitioning.Mode = PoolGroupPartitioningModeExplicit
	config.Partitions = []GroupPartitionConfig{testEmptyGroupPartitionConfig("alpha")}
	config.Pools[0].Partition = "missing"

	_, _, err := newGroupPartitionAssignments(config)
	requireGroupErrorIs(t, err, ErrInvalidOptions)
}

func TestPoolGroupAutomaticPoolAssignmentRejectsPerPoolPartition(t *testing.T) {
	config := testManagedGroupConfig("api")
	config.Pools[0].Partition = "alpha"

	_, _, err := newGroupPartitionAssignments(config)
	requireGroupErrorIs(t, err, ErrInvalidOptions)
}

func TestPoolGroupPartitioningPolicyValidateRejectsInvalidValues(t *testing.T) {
	tests := []PoolGroupPartitioningPolicy{
		{Mode: PoolGroupPartitioningMode(99)},
		{MinPartitions: -1},
		{MaxPartitions: -1},
		{MinPartitions: 4, MaxPartitions: 2},
		{TargetActiveEntries: -1},
		{EstimatedActiveClassesPerPool: -1},
		{EstimatedActiveShardsPerClass: -1},
		{Placement: PoolPlacementPolicy(99)},
	}
	for _, policy := range tests {
		if err := policy.Validate(); err == nil {
			t.Fatalf("Validate(%+v) error = nil, want error", policy)
		}
	}
}
