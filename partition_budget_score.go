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
	controlbudget "arcoris.dev/bufferpool/internal/control/budget"
	controlnumeric "arcoris.dev/bufferpool/internal/control/numeric"
)

// PoolPartitionBudgetScore is a normalized budget utilization projection.
type PoolPartitionBudgetScore struct {
	// Value is the highest configured budget utilization clamped to [0, 1].
	Value float64

	// RetainedUtilization is retained bytes divided by retained limit.
	RetainedUtilization float64

	// ActiveUtilization is active bytes divided by active limit.
	ActiveUtilization float64

	// OwnedUtilization is owned bytes divided by owned limit.
	OwnedUtilization float64

	// OverBudget reports whether any configured budget is exceeded.
	OverBudget bool
}

// newPoolPartitionBudgetScore maps a domain budget snapshot to a normalized score.
//
// The score uses the highest configured utilization because any retained,
// active, or owned budget can be the binding constraint. Unconfigured limits
// produce zero utilization and therefore do not force pressure.
func newPoolPartitionBudgetScore(snapshot PartitionBudgetSnapshot) PoolPartitionBudgetScore {
	retained := controlbudget.NewUsage(snapshot.CurrentRetainedBytes, snapshot.MaxRetainedBytes)
	active := controlbudget.NewUsage(snapshot.CurrentActiveBytes, snapshot.MaxActiveBytes)
	owned := controlbudget.NewUsage(snapshot.CurrentOwnedBytes, snapshot.MaxOwnedBytes)
	value := controlnumeric.Clamp01(maxFloat64(retained.Utilization, active.Utilization, owned.Utilization))
	return PoolPartitionBudgetScore{
		Value:               value,
		RetainedUtilization: retained.Utilization,
		ActiveUtilization:   active.Utilization,
		OwnedUtilization:    owned.Utilization,
		OverBudget:          snapshot.IsOverBudget(),
	}
}
