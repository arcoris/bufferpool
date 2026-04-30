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

// allocateShardCreditsForClassBudget converts one class budget target into
// shard-local credit limits.
//
// This helper is pure and allocation-only. Applying the returned credit limits
// belongs to classState.updateBudgetAtLeast, which owns the live shardCredit
// publication path.
func allocateShardCreditsForClassBudget(classSize ClassSize, shardCount int, target ClassBudgetTarget) []shardCreditLimit {
	limit := newClassBudgetLimit(classSize, target.TargetBytes)
	plan := newClassShardCreditPlan(limit, shardCount)
	return plan.credits()
}
