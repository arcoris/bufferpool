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

// testPartitionPoolConfig returns a minimal partition Pool config for tests.
func testPartitionPoolConfig(name string) PartitionPoolConfig {
	return PartitionPoolConfig{
		Name: name,
		Config: PoolConfig{
			Name: name,
		},
	}
}

// testPartitionConfig returns a minimal test partition config.
func testPartitionConfig(poolNames ...string) PoolPartitionConfig {
	if len(poolNames) == 0 {
		poolNames = []string{"primary"}
	}

	config := DefaultPoolPartitionConfig()
	config.Name = "test-partition"
	config.Pools = make([]PartitionPoolConfig, len(poolNames))
	for index, name := range poolNames {
		config.Pools[index] = testPartitionPoolConfig(name)
	}

	return config
}

// testNewPoolPartition constructs a test partition and registers cleanup.
func testNewPoolPartition(t *testing.T, poolNames ...string) *PoolPartition {
	t.Helper()

	partition, err := NewPoolPartition(testPartitionConfig(poolNames...))
	if err != nil {
		t.Fatalf("NewPoolPartition() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := partition.Close(); closeErr != nil {
			t.Fatalf("PoolPartition.Close() error = %v", closeErr)
		}
	})

	return partition
}

// requirePartitionErrorIs fails the test unless err matches target.
func requirePartitionErrorIs(t *testing.T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want errors.Is(..., %v)", err, target)
	}
}

// requirePartitionNoError fails the test when err is non-nil.
func requirePartitionNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// requirePartitionPanic fails the test unless fn panics.
func requirePartitionPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("expected panic")
		}
	}()
	fn()
}
