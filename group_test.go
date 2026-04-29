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

func TestNewPoolGroup(t *testing.T) {
	group := testNewPoolGroup(t, "alpha", "beta")
	if group.Name() != "test-group" {
		t.Fatalf("Name() = %q, want test-group", group.Name())
	}
	if !group.Lifecycle().IsActive() {
		t.Fatalf("Lifecycle() = %v, want active", group.Lifecycle())
	}
	names := group.PartitionNames()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("PartitionNames() = %#v", names)
	}
	if _, ok := group.PartitionSnapshot("alpha"); !ok {
		t.Fatalf("PartitionSnapshot(alpha) not found")
	}
	if _, ok := group.PartitionMetrics("missing"); ok {
		t.Fatalf("PartitionMetrics(missing) found")
	}
}

func TestMustNewPoolGroupPanicsOnInvalidConfig(t *testing.T) {
	requireGroupPanic(t, func() {
		_ = MustNewPoolGroup(PoolGroupConfig{})
	})
}

func TestPoolGroupAccessorsDefensiveCopies(t *testing.T) {
	group := testNewPoolGroup(t, "alpha", "beta")
	config := group.Config()
	config.Name = "changed"
	config.Partitions[0].Name = "changed"

	if group.Name() != "test-group" {
		t.Fatalf("group name changed through Config copy")
	}
	names := group.PartitionNames()
	names[0] = "changed"
	if got := group.PartitionNames()[0]; got != "alpha" {
		t.Fatalf("PartitionNames defensive copy failed: got %q", got)
	}
}

func TestPoolGroupZeroValuePanics(t *testing.T) {
	var group PoolGroup
	requireGroupPanic(t, func() { _ = group.Name() })
}
