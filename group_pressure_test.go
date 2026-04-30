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

// TestPoolGroupSetPressurePropagatesToPartitions verifies group pressure
// reaches partition-owned Pools without group shard scanning.
func TestPoolGroupSetPressurePropagatesToPartitions(t *testing.T) {
	config := testManagedGroupConfig("api")
	policy := poolTestSmallSingleShardPolicy()
	policy.Pressure = poolTestPressurePolicy()
	config.Pools[0].Config.Policy = policy
	group, err := NewPoolGroup(config)
	requireGroupNoError(t, err)
	t.Cleanup(func() { requireGroupNoError(t, group.Close()) })

	requireGroupNoError(t, group.SetPressure(PressureLevelCritical))
	lease, err := group.Acquire("api", 300)
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.Release(lease, lease.Buffer()))

	metrics := group.Metrics()
	if metrics.CurrentRetainedBytes != 0 || metrics.Retains != 0 || metrics.Drops != 1 {
		t.Fatalf("group metrics after critical pressure release = %+v, want dropped return and no retention", metrics)
	}
	if location, ok := group.poolDirectory.location("api"); ok {
		partition, _ := group.registry.partition(location.PartitionName)
		if pressure := partition.Pressure(); pressure.Level != PressureLevelCritical {
			t.Fatalf("partition pressure = %+v, want critical", pressure)
		}
	} else {
		t.Fatal("group directory missing api pool")
	}
}

// TestPoolGroupUpdatePolicyContractsPartitionBudget verifies policy contraction
// lowers partition and class targets without reclaiming active leases.
func TestPoolGroupUpdatePolicyContractsPartitionBudget(t *testing.T) {
	group, err := NewPoolGroup(testManagedGroupConfig("api"))
	requireGroupNoError(t, err)
	t.Cleanup(func() { requireGroupNoError(t, group.Close()) })

	lease, err := group.Acquire("api", 300)
	requireGroupNoError(t, err)

	policy := DefaultPoolGroupPolicy()
	policy.Budget.MaxRetainedBytes = SizeFromBytes(512)
	requireGroupNoError(t, group.UpdatePolicy(policy))

	location, ok := group.poolDirectory.location("api")
	if !ok {
		t.Fatal("group directory missing api pool")
	}
	partition, ok := group.registry.partition(location.PartitionName)
	if !ok {
		t.Fatalf("partition %q missing", location.PartitionName)
	}
	if budget := partition.Budget(); budget.MaxRetainedBytes != 512 {
		t.Fatalf("partition budget after UpdatePolicy = %+v, want retained max 512", budget)
	}

	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))
	if len(report.PoolBudgetTargets) != 1 || report.PoolBudgetTargets[0].RetainedBytes != SizeFromBytes(512) {
		t.Fatalf("pool budget targets = %+v, want retained target 512", report.PoolBudgetTargets)
	}
	if metrics := group.Metrics(); metrics.ActiveLeases != 1 || metrics.CurrentActiveBytes == 0 {
		t.Fatalf("metrics after contraction = %+v, want active lease preserved", metrics)
	}

	requireGroupNoError(t, group.Release(lease, lease.Buffer()))
	if final := group.Metrics(); final.ActiveLeases != 0 || final.CurrentRetainedBytes > 512 {
		t.Fatalf("metrics after release under contracted policy = %+v, want no active leases and retained bytes <= 512", final)
	}
}
