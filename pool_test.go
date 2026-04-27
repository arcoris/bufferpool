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
	"reflect"
	"testing"
)

// TestNewPoolWithZeroConfig verifies the canonical standalone Pool construction
// path.
//
// A zero PoolConfig is valid after normalization. Construction must produce an
// active Pool with normalized config, effective default policy, immutable class
// table, class states, shard selector, and initialized close coordination.
func TestNewPoolWithZeroConfig(t *testing.T) {
	t.Parallel()

	pool, err := New(PoolConfig{})
	if err != nil {
		t.Fatalf("New(PoolConfig{}) returned error: %v", err)
	}
	defer closePoolForTest(t, pool)

	if pool.Name() != DefaultConfigPoolName {
		t.Fatalf("Name() = %q, want %q", pool.Name(), DefaultConfigPoolName)
	}

	if pool.Lifecycle() != LifecycleActive {
		t.Fatalf("Lifecycle() = %s, want %s", pool.Lifecycle(), LifecycleActive)
	}

	if !pool.IsActive() {
		t.Fatal("IsActive() = false, want true")
	}

	if pool.IsClosing() {
		t.Fatal("IsClosing() = true, want false")
	}

	if pool.IsClosed() {
		t.Fatal("IsClosed() = true, want false")
	}

	if pool.ClassCount() != len(DefaultClassSizes()) {
		t.Fatalf("ClassCount() = %d, want %d", pool.ClassCount(), len(DefaultClassSizes()))
	}

	if !reflect.DeepEqual(pool.ClassSizes(), DefaultClassSizes()) {
		t.Fatalf("ClassSizes() = %#v, want DefaultClassSizes()", pool.ClassSizes())
	}

	if pool.Config().Name != DefaultConfigPoolName {
		t.Fatalf("Config().Name = %q, want %q", pool.Config().Name, DefaultConfigPoolName)
	}

	if !reflect.DeepEqual(pool.Policy(), DefaultPolicy()) {
		t.Fatal("Policy() does not match DefaultPolicy()")
	}
}

// TestNewPoolRejectsInvalidConfig verifies that Pool construction delegates to
// PoolConfig validation and preserves public error classification.
func TestNewPoolRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	policy := DefaultPolicy()
	policy.Retention.SoftRetainedBytes = policy.Retention.HardRetainedBytes + Byte

	pool, err := New(PoolConfig{Policy: policy})
	if err == nil {
		t.Fatal("New() returned nil error for invalid config")
	}

	if pool != nil {
		t.Fatalf("New() returned non-nil pool for invalid config: %#v", pool)
	}

	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("New() error does not match ErrInvalidOptions: %v", err)
	}

	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("New() error does not unwrap ErrInvalidPolicy: %v", err)
	}
}

// TestMustNewPanicsForInvalidConfig verifies the programmer-error constructor.
func TestMustNewPanicsForInvalidConfig(t *testing.T) {
	t.Parallel()

	policy := DefaultPolicy()
	policy.Retention.SoftRetainedBytes = policy.Retention.HardRetainedBytes + Byte

	assertPoolPanicMatches(t, func(recovered any) bool {
		err, ok := recovered.(error)
		return ok && errors.Is(err, ErrInvalidOptions) && errors.Is(err, ErrInvalidPolicy)
	}, func() {
		_ = MustNew(PoolConfig{Policy: policy})
	})
}

// TestPoolMetadataUsesDefensiveCopies verifies that Pool does not expose mutable
// policy/config/class-table slices.
//
// Pool stores policy and class table after construction. A caller must not be
// able to mutate live Pool behavior by modifying values returned from Config,
// Policy, or ClassSizes.
func TestPoolMetadataUsesDefensiveCopies(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	config := pool.Config()
	policy := pool.Policy()
	classSizes := pool.ClassSizes()

	if len(config.Policy.Classes.Sizes) == 0 {
		t.Fatal("Config().Policy.Classes.Sizes is empty")
	}

	if len(policy.Classes.Sizes) == 0 {
		t.Fatal("Policy().Classes.Sizes is empty")
	}

	if len(classSizes) == 0 {
		t.Fatal("ClassSizes() is empty")
	}

	originalConfigFirst := pool.Config().Policy.Classes.Sizes[0]
	originalPolicyFirst := pool.Policy().Classes.Sizes[0]
	originalClassFirst := pool.ClassSizes()[0]

	config.Policy.Classes.Sizes[0] = ClassSizeFromSize(8 * MiB)
	policy.Classes.Sizes[0] = ClassSizeFromSize(4 * MiB)
	classSizes[0] = ClassSizeFromSize(2 * MiB)

	if got := pool.Config().Policy.Classes.Sizes[0]; got != originalConfigFirst {
		t.Fatalf("Config() exposed live policy slice: got %s, want %s", got, originalConfigFirst)
	}

	if got := pool.Policy().Classes.Sizes[0]; got != originalPolicyFirst {
		t.Fatalf("Policy() exposed live policy slice: got %s, want %s", got, originalPolicyFirst)
	}

	if got := pool.ClassSizes()[0]; got != originalClassFirst {
		t.Fatalf("ClassSizes() exposed live class table slice: got %s, want %s", got, originalClassFirst)
	}
}

// TestPoolRuntimeSnapshotPublicationUsesDefensiveCopies verifies the internal
// policy publication boundary used by future partition-managed wiring.
func TestPoolRuntimeSnapshotPublicationUsesDefensiveCopies(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	policy := pool.Policy()
	policy.Classes.Sizes[0] = ClassSizeFromBytes(384)

	pool.publishRuntimeSnapshot(newPoolRuntimeSnapshot(InitialGeneration.Next(), policy))
	policy.Classes.Sizes[0] = ClassSizeFromSize(8 * MiB)

	current := pool.Policy()
	if current.Classes.Sizes[0] != ClassSizeFromBytes(384) {
		t.Fatalf("published policy was mutated through caller slice: got %s", current.Classes.Sizes[0])
	}

	snapshot := pool.Snapshot()
	if snapshot.Generation != InitialGeneration.Next() {
		t.Fatalf("snapshot generation = %s, want %s", snapshot.Generation, InitialGeneration.Next())
	}
}

// TestPoolRuntimeSnapshotPublicationPanicsForNil verifies fail-fast protection
// for the internal publication hook.
func TestPoolRuntimeSnapshotPublicationPanicsForNil(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	assertPoolPanic(t, errPoolRuntimeSnapshotNil, func() {
		pool.publishRuntimeSnapshot(nil)
	})
}

// closePoolForTest closes pool and fails the test if close returns an error.
func closePoolForTest(t *testing.T, pool *Pool) {
	t.Helper()

	if err := pool.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
}

// poolTestSingleShardPolicy returns a valid default-derived policy with
// deterministic single-shard routing.
//
// Single-shard routing is useful for tests that need a Put followed by Get to
// touch the same shard without depending on round-robin sequencing.
func poolTestSingleShardPolicy() Policy {
	policy := DefaultPolicy()
	policy.Shards.Selection = ShardSelectionModeSingle

	return policy
}

// poolTestSmallSingleShardPolicy returns a compact valid policy for focused
// admission, snapshot, and metrics tests.
func poolTestSmallSingleShardPolicy() Policy {
	return Policy{
		Retention: RetentionPolicy{
			SoftRetainedBytes:         2 * KiB,
			HardRetainedBytes:         4 * KiB,
			MaxRetainedBuffers:        4,
			MaxRequestSize:            KiB,
			MaxRetainedBufferCapacity: KiB,
			MaxClassRetainedBytes:     2 * KiB,
			MaxClassRetainedBuffers:   4,
			MaxShardRetainedBytes:     2 * KiB,
			MaxShardRetainedBuffers:   4,
		},
		Classes: ClassPolicy{
			Sizes: []ClassSize{
				ClassSizeFromBytes(512),
				ClassSizeFromSize(KiB),
			},
		},
		Shards: ShardPolicy{
			Selection:                 ShardSelectionModeSingle,
			ShardsPerClass:            1,
			BucketSlotsPerShard:       4,
			AcquisitionFallbackShards: 0,
			ReturnFallbackShards:      0,
		},
		Admission: AdmissionPolicy{
			ZeroSizeRequests:    ZeroSizeRequestEmptyBuffer,
			ReturnedBuffers:     ReturnedBufferPolicyAdmit,
			OversizedReturn:     AdmissionActionDrop,
			UnsupportedClass:    AdmissionActionDrop,
			ClassMismatch:       AdmissionActionDrop,
			CreditExhausted:     AdmissionActionDrop,
			BucketFull:          AdmissionActionDrop,
			ZeroRetainedBuffers: false,
			ZeroDroppedBuffers:  false,
		},
		Pressure: PressurePolicy{},
		Trim:     TrimPolicy{},
		Ownership: OwnershipPolicy{
			Mode:                      OwnershipModeNone,
			TrackInUseBytes:           false,
			TrackInUseBuffers:         false,
			DetectDoubleRelease:       false,
			MaxReturnedCapacityGrowth: 2 * PolicyRatioOne,
		},
	}
}

// poolTestRetainedUsage sums current retained usage across Pool-owned classes.
func poolTestRetainedUsage(pool *Pool) poolTestUsage {
	var usage poolTestUsage

	for index := range pool.classes {
		state := pool.classes[index].state()
		usage.buffers += state.CurrentRetainedBuffers
		usage.bytes += state.CurrentRetainedBytes
	}

	return usage
}

type poolTestUsage struct {
	buffers uint64
	bytes   uint64
}

func assertPoolPanic(t *testing.T, want any, fn func()) {
	t.Helper()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}

		if recovered != want {
			t.Fatalf("panic = %#v, want %#v", recovered, want)
		}
	}()

	fn()
}

func assertPoolPanicMatches(t *testing.T, match func(any) bool, fn func()) {
	t.Helper()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}

		if !match(recovered) {
			t.Fatalf("panic = %#v did not match predicate", recovered)
		}
	}()

	fn()
}
