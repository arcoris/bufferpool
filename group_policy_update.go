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

// UpdatePolicy publishes a new group policy and pushes immediate retained
// budget contraction to partitions when the policy carries a retained target.
//
// UpdatePolicy is foreground and serialized with hard Close. It validates the
// policy, publishes a new group runtime snapshot, and computes deterministic
// partition targets from the new retained budget. It does not scan Pool shards
// and does not execute physical trim; partitions perform bounded trim locally.
func (g *PoolGroup) UpdatePolicy(policy PoolGroupPolicy) error {
	g.mustBeInitialized()
	normalized := policy.Normalize()
	if err := normalized.Validate(); err != nil {
		return err
	}

	g.runtimeMu.Lock()
	defer g.runtimeMu.Unlock()

	if !g.lifecycle.AllowsWork() {
		return newError(ErrClosed, errGroupClosed)
	}

	generation := g.generation.Load().Next()
	var allocation partitionBudgetAllocationReport
	if normalized.Budget.MaxRetainedBytes.IsZero() {
		current := g.currentRuntimeSnapshot()
		g.generation.Store(generation)
		g.publishRuntimeSnapshot(newGroupRuntimeSnapshotWithPressure(generation, normalized, current.Pressure))
		return nil
	}

	inputs := make([]partitionBudgetAllocationInput, 0, len(g.registry.entries))
	for _, entry := range g.registry.entries {
		inputs = append(inputs, partitionBudgetAllocationInput{PartitionName: entry.name})
	}
	allocation = g.computePartitionBudgetTargetsReport(generation, normalized.Budget.MaxRetainedBytes, inputs)
	if !allocation.Allocation.Feasible {
		return newError(ErrInvalidPolicy, allocation.Allocation.Reason)
	}

	current := g.currentRuntimeSnapshot()
	g.generation.Store(generation)
	g.publishRuntimeSnapshot(newGroupRuntimeSnapshotWithPressure(generation, normalized, current.Pressure))
	targets := allocation.Targets
	skipped, err := g.publishPartitionBudgetTargets(targets, nil)
	if err != nil {
		return err
	}
	if len(skipped) > 0 {
		return newError(ErrClosed, errGroupClosed)
	}
	return nil
}
