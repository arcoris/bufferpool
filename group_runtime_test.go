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

func TestGroupRuntimeSnapshotNormalize(t *testing.T) {
	snapshot := newGroupRuntimeSnapshot(Generation(7), PoolGroupPolicy{Budget: PartitionBudgetPolicy{MaxRetainedBytes: MiB}})
	if snapshot.Generation != Generation(7) {
		t.Fatalf("Generation = %v, want 7", snapshot.Generation)
	}
	if snapshot.Policy.Budget.MaxRetainedBytes != MiB {
		t.Fatalf("Budget not preserved")
	}
}

func TestPoolGroupPublishRuntimeSnapshot(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	snapshot := newGroupRuntimeSnapshot(Generation(9), PoolGroupPolicy{Budget: PartitionBudgetPolicy{MaxRetainedBytes: MiB}})
	group.publishRuntimeSnapshot(snapshot)
	current := group.currentRuntimeSnapshot()
	if current.Generation != Generation(9) {
		t.Fatalf("Generation = %v, want 9", current.Generation)
	}
	if current.Policy.Budget.MaxRetainedBytes != MiB {
		t.Fatalf("published policy was not preserved")
	}
}

func TestPoolGroupPublishRuntimeSnapshotRejectsNil(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	requireGroupPanic(t, func() { group.publishRuntimeSnapshot(nil) })
}
