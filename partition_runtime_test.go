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

// TestPartitionRuntimeSnapshotPublication verifies partition policy publication.
func TestPartitionRuntimeSnapshotPublication(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	requirePartitionPanic(t, func() { partition.publishRuntimeSnapshot(nil) })

	policy := PartitionPolicy{Controller: PartitionControllerPolicy{Enabled: true}}
	partition.publishRuntimeSnapshot(newPartitionRuntimeSnapshot(Generation(10), policy))

	snapshot := partition.currentRuntimeSnapshot()
	if snapshot.Generation != Generation(10) {
		t.Fatalf("runtime generation = %s, want 10", snapshot.Generation)
	}
	if !snapshot.Policy.Controller.Enabled {
		t.Fatalf("runtime policy controller should be enabled")
	}
	if snapshot.Policy.Controller.TickInterval != defaultPartitionControllerTickInterval {
		t.Fatalf("runtime policy tick interval = %s, want default", snapshot.Policy.Controller.TickInterval)
	}
}

// TestPoolPartitionRuntimePublicationGenerationStreams verifies generation separation.
func TestPoolPartitionRuntimePublicationGenerationStreams(t *testing.T) {
	partition := testNewPoolPartition(t, "primary", "secondary")
	partition.activeRegistry.resetDirty()
	beforeGeneration := partition.Sample().Generation
	beforeActivityGeneration := partition.activeRegistry.generationSnapshot()

	policy := PartitionPolicy{Budget: PartitionBudgetPolicy{MaxOwnedBytes: 64 * KiB}}
	partition.publishRuntimeSnapshot(newPartitionRuntimeSnapshot(Generation(21), policy))

	afterPartitionPublish := partition.Sample()
	if afterPartitionPublish.PolicyGeneration != Generation(21) {
		t.Fatalf("partition policy generation = %s, want 21", afterPartitionPublish.PolicyGeneration)
	}
	if afterPartitionPublish.Generation != beforeGeneration {
		t.Fatalf("partition state generation advanced on policy publish: got %s want %s", afterPartitionPublish.Generation, beforeGeneration)
	}
	if !partition.activeRegistry.generationSnapshot().After(beforeActivityGeneration) {
		t.Fatalf("active registry generation did not advance after dirty publication")
	}
	dirty := partition.activeRegistry.dirtyIndexes(nil)
	if len(dirty) != 2 || dirty[0] != 0 || dirty[1] != 1 {
		t.Fatalf("dirty after partition policy publish = %v, want [0 1]", dirty)
	}

	partition.activeRegistry.resetDirty()
	beforePoolPublishGeneration := partition.Sample().Generation
	beforeActivityGeneration = partition.activeRegistry.generationSnapshot()
	poolPolicy := partition.config.Pools[0].Config.Policy
	requirePartitionNoError(t, partition.publishPoolRuntimeSnapshot("primary", Generation(31), poolPolicy))

	afterPoolPublish := partition.Sample()
	if afterPoolPublish.Generation != beforePoolPublishGeneration {
		t.Fatalf("partition state generation advanced on pool policy publish: got %s want %s", afterPoolPublish.Generation, beforePoolPublishGeneration)
	}
	if !partition.activeRegistry.generationSnapshot().After(beforeActivityGeneration) {
		t.Fatalf("active registry generation did not advance after pool dirty publication")
	}
	dirty = partition.activeRegistry.dirtyIndexes(dirty)
	if len(dirty) != 1 || dirty[0] != 0 {
		t.Fatalf("dirty after pool policy publish = %v, want [0]", dirty)
	}
	metrics, ok := partition.PoolMetrics("primary")
	if !ok {
		t.Fatalf("missing primary pool metrics")
	}
	if metrics.Generation != Generation(31) {
		t.Fatalf("pool policy generation = %s, want 31", metrics.Generation)
	}
}

// TestPoolPartitionPolicyReturnsPublishedPolicy verifies policy accessor behavior.
func TestPoolPartitionPolicyReturnsPublishedPolicy(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	policy := PartitionPolicy{Budget: PartitionBudgetPolicy{MaxOwnedBytes: 64 * KiB}}
	partition.publishRuntimeSnapshot(newPartitionRuntimeSnapshot(Generation(3), policy))

	got := partition.Policy()
	if got.Budget.MaxOwnedBytes != 64*KiB {
		t.Fatalf("Policy().Budget.MaxOwnedBytes = %s, want 64 KiB", got.Budget.MaxOwnedBytes)
	}
}

// TestPoolPartitionPublishPoolRuntimeSnapshot verifies safe Pool runtime publication.
func TestPoolPartitionPublishPoolRuntimeSnapshot(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	beforeMetrics, ok := partition.PoolMetrics("primary")
	if !ok {
		t.Fatalf("missing primary pool metrics")
	}

	policy := partition.Config().Pools[0].Config.Policy
	policy.Admission.ReturnedBuffers = ReturnedBufferPolicyDrop
	requirePartitionNoError(t, partition.publishPoolRuntimeSnapshot("primary", Generation(11), policy))

	metrics, ok := partition.PoolMetrics("primary")
	if !ok {
		t.Fatalf("missing primary pool metrics after publish")
	}
	if metrics.Generation != Generation(11) {
		t.Fatalf("pool runtime generation = %s, want 11", metrics.Generation)
	}
	if metrics.Generation == beforeMetrics.Generation {
		t.Fatalf("pool runtime generation did not change")
	}
	snapshot, ok := partition.PoolSnapshot("primary")
	if !ok {
		t.Fatalf("missing primary pool snapshot")
	}
	if snapshot.Policy.Admission.ReturnedBuffers != ReturnedBufferPolicyDrop {
		t.Fatalf("pool policy did not update returned-buffer policy")
	}

	err := partition.publishPoolRuntimeSnapshot("missing", Generation(12), policy)
	requirePartitionErrorIs(t, err, ErrInvalidOptions)

	unsupported := policy
	unsupported.Ownership = StrictOwnershipPolicy()
	err = partition.publishPoolRuntimeSnapshot("primary", Generation(13), unsupported)
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)

	invalid := policy
	invalid.Retention.MaxRequestSize = 0
	err = partition.publishPoolRuntimeSnapshot("primary", Generation(14), invalid)
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)

	incompatible := policy
	incompatible.Shards.ShardsPerClass++
	err = partition.publishPoolRuntimeSnapshot("primary", Generation(15), incompatible)
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)
}
