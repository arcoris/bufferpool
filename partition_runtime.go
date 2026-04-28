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

const (
	// errPartitionRuntimeSnapshotNil reports an internal nil partition publication.
	errPartitionRuntimeSnapshotNil = "bufferpool.PoolPartition: runtime snapshot must not be nil"
)

// partitionRuntimeSnapshot is the immutable partition-level runtime view.
type partitionRuntimeSnapshot struct {
	// Generation is the partition policy publication generation.
	Generation Generation

	// Policy is the immutable partition policy used by controller planning.
	Policy PartitionPolicy
}

// newPartitionRuntimeSnapshot returns a normalized immutable partition runtime view.
func newPartitionRuntimeSnapshot(generation Generation, policy PartitionPolicy) *partitionRuntimeSnapshot {
	return &partitionRuntimeSnapshot{Generation: generation, Policy: policy.Normalize()}
}

// publishRuntimeSnapshot atomically publishes a partition policy view.
func (p *PoolPartition) publishRuntimeSnapshot(snapshot *partitionRuntimeSnapshot) {
	if snapshot == nil {
		panic(errPartitionRuntimeSnapshotNil)
	}
	p.runtimeSnapshot.Store(newPartitionRuntimeSnapshot(snapshot.Generation, snapshot.Policy))
}

// currentRuntimeSnapshot returns the currently published partition policy view.
func (p *PoolPartition) currentRuntimeSnapshot() *partitionRuntimeSnapshot {
	snapshot := p.runtimeSnapshot.Load()
	if snapshot == nil {
		panic(errNilPoolPartition)
	}
	return snapshot
}

// publishPoolRuntimeSnapshot publishes an immutable runtime policy view into an
// owned Pool.
//
// Controller publication must fail closed with errors. It normalizes and
// validates the policy, checks standalone Pool support boundaries, checks
// compatibility with the already-built Pool topology, and only then calls the
// panic-on-invariant Pool publication hook.
func (p *PoolPartition) publishPoolRuntimeSnapshot(poolName string, generation Generation, policy Policy) error {
	p.mustBeInitialized()
	pool, ok := p.registry.pool(poolName)
	if !ok {
		return newError(ErrInvalidOptions, errPartitionPoolMissing+": "+poolName)
	}

	normalized := effectivePoolConfigPolicy(policy)
	if err := normalized.Validate(); err != nil {
		return wrapError(ErrInvalidOptions, err, errPoolConfigInvalidPolicy)
	}
	if err := validatePoolSupportedPolicy(normalized); err != nil {
		return wrapError(ErrInvalidOptions, err, errPoolConfigInvalidPolicy)
	}
	if err := pool.validateRuntimePolicyCompatible(normalized); err != nil {
		return err
	}

	pool.publishRuntimeSnapshot(newPoolRuntimeSnapshot(generation, normalized))
	p.generation.Advance()
	return nil
}
