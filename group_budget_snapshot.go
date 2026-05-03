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

// group_budget_snapshot.go owns group budget projections. Snapshot helpers are
// read-only report inputs; they do not allocate targets, publish runtime state,
// or touch Pool hot-path operations.

// PoolGroupBudgetSnapshot describes aggregate group budget usage.
//
// The group currently evaluates budget pressure by projecting its aggregate
// partition sample through partition-shaped retained-byte policy fields. This
// type gives report-facing group APIs group vocabulary while keeping the shared
// internal math reusable and explicit.
type PoolGroupBudgetSnapshot PartitionBudgetSnapshot

// IsOverBudget reports whether the aggregate group sample exceeds any enabled
// retained-byte budget limit.
func (s PoolGroupBudgetSnapshot) IsOverBudget() bool {
	return PartitionBudgetSnapshot(s).IsOverBudget()
}

// newGroupBudgetSnapshot projects group aggregate sample usage against group limits.
func newGroupBudgetSnapshot(policy PartitionBudgetPolicy, sample PoolGroupSample) PoolGroupBudgetSnapshot {
	return PoolGroupBudgetSnapshot(newPartitionBudgetSnapshot(policy, sample.Aggregate))
}
