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
	"testing"
)

// TestPoolPartitionSetPressureReportsClosedPool verifies partition pressure
// publication reports skipped children instead of silently partially mutating.
func TestPoolPartitionSetPressureReportsClosedPool(t *testing.T) {
	partition := testNewPoolPartition(t, "alpha", "beta")
	pool, ok := partition.registry.pool("beta")
	if !ok {
		t.Fatal("partition pool beta not found")
	}
	requirePartitionNoError(t, pool.Close())

	publication, err := partition.PublishPressure(PressureLevelHigh)
	if err != nil {
		t.Fatalf("PublishPressure() error = %v", err)
	}
	if publication.FullyApplied() {
		t.Fatal("FullyApplied() = true, want skipped closed pool")
	}
	if len(publication.SkippedPools) != 1 || publication.SkippedPools[0].PoolName != "beta" {
		t.Fatalf("SkippedPools = %+v, want closed beta", publication.SkippedPools)
	}
	if err := partition.SetPressure(PressureLevelHigh); !errors.Is(err, ErrClosed) {
		t.Fatalf("SetPressure() error = %v, want ErrClosed for partial publication", err)
	}
}

// TestPoolGroupSetPressureReportsClosedPartition verifies group pressure
// publication reports closed partitions before mutating group pressure state.
func TestPoolGroupSetPressureReportsClosedPartition(t *testing.T) {
	alphaConfig := testPartitionConfig("alpha-pool")
	alphaConfig.Name = "alpha"
	betaConfig := testPartitionConfig("beta-pool")
	betaConfig.Name = "beta"
	group := MustNewPoolGroup(PoolGroupConfig{
		Partitions: []GroupPartitionConfig{
			{Name: "alpha", Config: alphaConfig},
			{Name: "beta", Config: betaConfig},
		},
	})
	t.Cleanup(func() { requireGroupNoError(t, group.Close()) })

	partition, ok := group.registry.partition("beta")
	if !ok {
		t.Fatal("group partition beta not found")
	}
	requirePartitionNoError(t, partition.Close())

	publication, err := group.PublishPressure(PressureLevelHigh)
	if err != nil {
		t.Fatalf("PublishPressure() error = %v", err)
	}
	if publication.FullyApplied() {
		t.Fatal("FullyApplied() = true, want skipped closed partition")
	}
	if len(publication.SkippedPartitions) != 1 || publication.SkippedPartitions[0].PartitionName != "beta" {
		t.Fatalf("SkippedPartitions = %+v, want closed beta", publication.SkippedPartitions)
	}
	if err := group.SetPressure(PressureLevelHigh); !errors.Is(err, ErrClosed) {
		t.Fatalf("SetPressure() error = %v, want ErrClosed for partial publication", err)
	}
}
