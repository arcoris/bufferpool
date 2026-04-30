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

import "time"

// controllerSampleIndexes returns deterministic Pool indexes for one controller
// cycle.
//
// The normal path samples active and dirty Pools. Periodic full scans are still
// manual foreground work and only revalidate inactive Pools; they do not start a
// background controller or add Pool hot-path dependencies.
func (p *PoolPartition) controllerSampleIndexes(dst []int) []int {
	if p.controller.cycles%partitionControllerFullScanInterval == 0 {
		return p.allPartitionPoolIndexes(dst)
	}
	active := p.activeRegistry.activeIndexes(dst)
	dirty := p.activeRegistry.dirtyIndexes(nil)
	return unionPartitionPoolIndexes(active, dirty, p.registry.len())
}

// allPartitionPoolIndexes appends every registry index in deterministic order.
func (p *PoolPartition) allPartitionPoolIndexes(dst []int) []int {
	dst = dst[:0]
	for index := range p.registry.entries {
		dst = append(dst, index)
	}
	return dst
}

// unionPartitionPoolIndexes merges active and dirty indexes in deterministic
// registry order.
func unionPartitionPoolIndexes(active []int, dirty []int, total int) []int {
	if total <= 0 {
		return active[:0]
	}
	selected := make([]bool, total)
	for _, index := range active {
		if index >= 0 && index < total {
			selected[index] = true
		}
	}
	for _, index := range dirty {
		if index >= 0 && index < total {
			selected[index] = true
		}
	}
	active = active[:0]
	for index, ok := range selected {
		if ok {
			active = append(active, index)
		}
	}
	return active
}

// controllerPoolActivityDeltas returns one activity bit per sampled Pool index.
func controllerPoolActivityDeltas(indexes []int, window PoolPartitionWindow, dirtyIndexes []int) []bool {
	active := make([]bool, len(indexes))
	for offset, index := range indexes {
		if partitionIndexSetContains(dirtyIndexes, index) {
			active[offset] = true
			continue
		}
		if offset < len(window.Pools) && !window.Pools[offset].Delta.IsZero() {
			active[offset] = true
		}
	}
	return active
}

// partitionIndexSetContains reports whether indexes contains index.
func partitionIndexSetContains(indexes []int, index int) bool {
	for _, candidate := range indexes {
		if candidate == index {
			return true
		}
	}
	return false
}

// controllerPoolBudgetTargets computes class-aware Pool budget targets for the
// sampled Pool windows in one controller cycle.
func (p *PoolPartition) controllerPoolBudgetTargets(
	generation Generation,
	runtime *partitionRuntimeSnapshot,
	window PoolPartitionWindow,
	elapsed time.Duration,
) []PoolBudgetTarget {
	if len(window.Pools) == 0 {
		return nil
	}

	inputs := make([]poolBudgetAllocationInput, 0, len(window.Pools))
	for _, poolWindow := range window.Pools {
		pool, ok := p.registry.pool(poolWindow.Name)
		if !ok {
			continue
		}
		inputs = append(inputs, poolBudgetAllocationInput{
			PoolName:          poolWindow.Name,
			BaseRetainedBytes: SizeFromBytes(poolWindowCurrentBudgetBytes(poolWindow)),
			MaxRetainedBytes:  SizeFromBytes(poolControllerRetainedBudget(pool)),
			Score:             poolWindowScore(p, poolWindow, elapsed),
		})
	}

	retainedBytes := runtime.Policy.Budget.MaxRetainedBytes
	if retainedBytes.IsZero() {
		retainedBytes = SizeFromBytes(partitionControllerDefaultRetainedBudget(p, inputs))
	}
	targets := allocatePoolBudgetTargets(generation, retainedBytes, inputs)
	for index := range targets {
		if poolWindow, ok := findPoolPartitionPoolWindow(window, targets[index].PoolName); ok {
			if pool, ok := p.registry.pool(targets[index].PoolName); ok {
				targets[index].ClassTargets = p.controllerClassBudgetTargets(generation, pool, poolWindow, targets[index].RetainedBytes, elapsed)
			}
		}
	}

	return targets
}

// controllerClassBudgetTargets computes class targets for one Pool budget target.
func (p *PoolPartition) controllerClassBudgetTargets(
	generation Generation,
	pool *Pool,
	window PoolPartitionPoolWindow,
	retainedBytes Size,
	elapsed time.Duration,
) []ClassBudgetTarget {
	inputs := make([]classBudgetAllocationInput, 0, len(window.Classes))
	policy := pool.currentRuntimeSnapshot().Policy
	for _, classWindow := range window.Classes {
		activity := newPoolPartitionClassActivity(classWindow, elapsed)
		key := poolClassKey{PoolName: window.Name, ClassID: classWindow.ClassID}
		ewma := p.controller.ewmaByPoolClass[key].WithUpdate(PoolPartitionEWMAConfig{}, elapsed, activity)
		p.controller.ewmaByPoolClass[key] = ewma

		inputs = append(inputs, classBudgetAllocationInput{
			ClassID:         classWindow.ClassID,
			BaseTargetBytes: SizeFromBytes(classWindow.Current.Budget.AssignedBytes),
			MaxTargetBytes:  SizeFromBytes(poolClassStaticBudgetCap(policy, classWindow.Class.Size())),
			Score:           poolPartitionClassScore(classWindow.Current, activity, ewma),
		})
	}
	return allocateClassBudgetTargets(generation, retainedBytes, inputs)
}

// poolWindowCurrentBudgetBytes returns the current assigned class budget total.
func poolWindowCurrentBudgetBytes(window PoolPartitionPoolWindow) uint64 {
	var total uint64
	for _, class := range window.Classes {
		total = poolSaturatingAdd(total, class.Current.Budget.AssignedBytes)
	}
	return total
}

// poolWindowScore aggregates class scores for Pool-level budget allocation.
func poolWindowScore(p *PoolPartition, window PoolPartitionPoolWindow, elapsed time.Duration) float64 {
	var score float64
	for _, class := range window.Classes {
		activity := newPoolPartitionClassActivity(class, elapsed)
		key := poolClassKey{PoolName: window.Name, ClassID: class.ClassID}
		ewma := p.controller.ewmaByPoolClass[key]
		score += poolPartitionClassScore(class.Current, activity, ewma)
	}
	return score
}

// poolControllerRetainedBudget returns the local retained target available to a
// Pool under its current static policy.
func poolControllerRetainedBudget(pool *Pool) uint64 {
	policy := pool.currentRuntimeSnapshot().Policy
	var totalCap uint64
	for index := range pool.classes {
		totalCap = poolSaturatingAdd(totalCap, poolClassStaticBudgetCap(policy, pool.classes[index].classSize()))
	}
	if policy.Retention.SoftRetainedBytes.IsZero() {
		return totalCap
	}
	return poolMinUint64(policy.Retention.SoftRetainedBytes.Bytes(), totalCap)
}

// partitionControllerDefaultRetainedBudget returns a parent budget when the
// partition policy is unbounded at retained scope.
func partitionControllerDefaultRetainedBudget(p *PoolPartition, inputs []poolBudgetAllocationInput) uint64 {
	var total uint64
	for _, input := range inputs {
		if input.MaxRetainedBytes.IsZero() {
			if pool, ok := p.registry.pool(input.PoolName); ok {
				total = poolSaturatingAdd(total, poolControllerRetainedBudget(pool))
			}
			continue
		}
		total = poolSaturatingAdd(total, input.MaxRetainedBytes.Bytes())
	}
	return total
}

// findPoolPartitionPoolWindow returns a Pool window by name.
func findPoolPartitionPoolWindow(window PoolPartitionWindow, name string) (PoolPartitionPoolWindow, bool) {
	for _, pool := range window.Pools {
		if pool.Name == name {
			return pool, true
		}
	}
	return PoolPartitionPoolWindow{}, false
}
