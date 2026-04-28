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

// PartitionBudgetSnapshot describes aggregate partition budget usage.
type PartitionBudgetSnapshot struct {
	// MaxRetainedBytes is the configured retained-byte limit, or zero when
	// unbounded.
	MaxRetainedBytes uint64

	// MaxActiveBytes is the configured checked-out active-byte limit, or zero
	// when unbounded.
	MaxActiveBytes uint64

	// MaxOwnedBytes is the configured retained-plus-active limit, or zero when
	// unbounded.
	MaxOwnedBytes uint64

	// CurrentRetainedBytes is retained storage currently held by owned Pools.
	CurrentRetainedBytes uint64

	// CurrentActiveBytes is checked-out capacity currently tracked by leases.
	CurrentActiveBytes uint64

	// CurrentOwnedBytes is retained plus active bytes.
	CurrentOwnedBytes uint64

	// RetainedOverBudget reports whether retained bytes exceed their configured
	// limit.
	RetainedOverBudget bool

	// ActiveOverBudget reports whether active bytes exceed their configured
	// limit.
	ActiveOverBudget bool

	// OwnedOverBudget reports whether retained plus active bytes exceed their
	// configured limit.
	OwnedOverBudget bool
}

// Budget returns an aggregate partition budget snapshot.
func (p *PoolPartition) Budget() PartitionBudgetSnapshot {
	p.mustBeInitialized()
	runtime := p.currentRuntimeSnapshot()
	var sample PoolPartitionSample
	p.sampleWithRuntimeAndGeneration(&sample, runtime, p.generation.Load(), false)
	return newPartitionBudgetSnapshot(runtime.Policy.Budget, sample)
}

// newPartitionBudgetSnapshot projects sample usage against partition limits.
func newPartitionBudgetSnapshot(policy PartitionBudgetPolicy, sample PoolPartitionSample) PartitionBudgetSnapshot {
	s := PartitionBudgetSnapshot{
		MaxRetainedBytes:     policy.MaxRetainedBytes.Bytes(),
		MaxActiveBytes:       policy.MaxActiveBytes.Bytes(),
		MaxOwnedBytes:        policy.MaxOwnedBytes.Bytes(),
		CurrentRetainedBytes: sample.CurrentRetainedBytes,
		CurrentActiveBytes:   sample.CurrentActiveBytes,
		CurrentOwnedBytes:    partitionOwnedBytesFromSample(sample),
	}
	if s.MaxRetainedBytes > 0 && s.CurrentRetainedBytes > s.MaxRetainedBytes {
		s.RetainedOverBudget = true
	}
	if s.MaxActiveBytes > 0 && s.CurrentActiveBytes > s.MaxActiveBytes {
		s.ActiveOverBudget = true
	}
	if s.MaxOwnedBytes > 0 && s.CurrentOwnedBytes > s.MaxOwnedBytes {
		s.OwnedOverBudget = true
	}
	return s
}

// IsOverBudget reports whether any configured partition budget is exceeded.
func (s PartitionBudgetSnapshot) IsOverBudget() bool {
	return s.RetainedOverBudget || s.ActiveOverBudget || s.OwnedOverBudget
}
