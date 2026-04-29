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

func TestGroupRegistryLookup(t *testing.T) {
	registry, err := newGroupRegistry([]GroupPartitionConfig{
		testGroupPartitionConfig("alpha"),
		testGroupPartitionConfig("beta"),
	})
	requireGroupNoError(t, err)
	t.Cleanup(func() { requireGroupNoError(t, registry.closeAll()) })

	partition, ok := registry.partition("beta")
	if !ok || partition == nil {
		t.Fatalf("partition(beta) = %v, %v", partition, ok)
	}
	if index, ok := registry.partitionIndex("beta"); !ok || index != 1 {
		t.Fatalf("partitionIndex(beta) = %d, %v", index, ok)
	}
	if index, ok := registry.partitionIndexForPartition(partition); !ok || index != 1 {
		t.Fatalf("partitionIndexForPartition(beta) = %d, %v", index, ok)
	}
}

func TestGroupRegistryNamesCopy(t *testing.T) {
	registry, err := newGroupRegistry([]GroupPartitionConfig{testGroupPartitionConfig("alpha")})
	requireGroupNoError(t, err)
	t.Cleanup(func() { requireGroupNoError(t, registry.closeAll()) })

	names := registry.namesCopy()
	names[0] = "changed"
	if got := registry.namesCopy()[0]; got != "alpha" {
		t.Fatalf("namesCopy mutated registry: got %q", got)
	}
}

func TestGroupRegistryRejectsInvalidInputs(t *testing.T) {
	if _, err := newGroupRegistry(nil); err == nil {
		t.Fatalf("newGroupRegistry(nil) error = nil")
	}
	if _, err := newGroupRegistry([]GroupPartitionConfig{{Name: ""}}); err == nil {
		t.Fatalf("newGroupRegistry(empty name) error = nil")
	}
	if _, err := newGroupRegistry([]GroupPartitionConfig{testGroupPartitionConfig("alpha"), testGroupPartitionConfig("alpha")}); err == nil {
		t.Fatalf("newGroupRegistry(duplicate) error = nil")
	}
	mismatched := testGroupPartitionConfig("alpha")
	mismatched.Config.Name = "backend"
	if _, err := newGroupRegistry([]GroupPartitionConfig{mismatched}); err == nil {
		t.Fatalf("newGroupRegistry(mismatched partition name) error = nil")
	}
}
