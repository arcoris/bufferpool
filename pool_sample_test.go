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

// TestPoolSampleCountersMatchesSnapshot verifies the internal controller sample
// agrees with public Snapshot for aggregate counters in a stable scenario.
func TestPoolSampleCountersMatchesSnapshot(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	buffer, err := pool.Get(512)
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if err := pool.Put(buffer); err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}

	var sample poolCounterSample
	pool.sampleCounters(&sample)

	snapshot := pool.Snapshot()
	if sample.Generation != snapshot.Generation {
		t.Fatalf("sample generation = %s, snapshot generation = %s", sample.Generation, snapshot.Generation)
	}
	if sample.Lifecycle != snapshot.Lifecycle {
		t.Fatalf("sample lifecycle = %s, snapshot lifecycle = %s", sample.Lifecycle, snapshot.Lifecycle)
	}
	if sample.ClassCount != snapshot.ClassCount() {
		t.Fatalf("sample class count = %d, snapshot class count = %d", sample.ClassCount, snapshot.ClassCount())
	}
	if sample.ShardCount != snapshot.ShardCount() {
		t.Fatalf("sample shard count = %d, snapshot shard count = %d", sample.ShardCount, snapshot.ShardCount())
	}
	if sample.Counters != snapshot.Counters {
		t.Fatalf("sample counters = %#v, snapshot counters = %#v", sample.Counters, snapshot.Counters)
	}
}

// TestPoolSampleCountersAllowsNilDestination verifies nil samples are ignored.
func TestPoolSampleCountersAllowsNilDestination(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	pool.sampleCounters(nil)
}

// TestPoolSampleCountersDoesNotAllocate verifies the internal aggregate sample
// path avoids public snapshot slice allocation.
func TestPoolSampleCountersDoesNotAllocate(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	if err := pool.Put(make([]byte, 0, KiB.Int())); err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}

	var sample poolCounterSample
	allocations := testing.AllocsPerRun(100, func() {
		pool.sampleCounters(&sample)
	})

	if allocations != 0 {
		t.Fatalf("sampleCounters allocations = %f, want 0", allocations)
	}
}
