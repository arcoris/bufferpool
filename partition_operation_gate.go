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

// beginForegroundOperation admits one partition-level foreground operation.
//
// The gate is an owner-boundary lock, not a Pool hot-path lock. It protects
// managed PoolPartition operations that can touch LeaseRegistry, owned Pools,
// partition runtime snapshots, pressure, budgets, or trim. Hard Close takes the
// write side before child cleanup so a foreground operation admitted here cannot
// race with LeaseRegistry.Close or Pool.Close.
func (p *PoolPartition) beginForegroundOperation() error {
	p.foregroundMu.RLock()
	if p.lifecycle.AllowsWork() {
		return nil
	}
	p.foregroundMu.RUnlock()
	return newError(ErrClosed, errPartitionClosed)
}

// beginReleaseOperation admits lease release into the partition foreground gate.
//
// Release intentionally does not require AllowsWork. A lease acquired before
// shutdown remains releasable after Closing or Closed so ownership accounting can
// complete and any closed-Pool handoff failure is recorded diagnostically.
func (p *PoolPartition) beginReleaseOperation() {
	p.foregroundMu.RLock()
}

// endForegroundOperation leaves a gate admitted by beginForegroundOperation or
// beginReleaseOperation.
func (p *PoolPartition) endForegroundOperation() {
	p.foregroundMu.RUnlock()
}

// lockForegroundClose prevents new foreground work and waits for admitted work.
//
// The caller must unlock p.foregroundMu after child cleanup is complete. Holding
// the write side while Closing is published and child resources are closed keeps
// applied partition work from observing partially closed children.
func (p *PoolPartition) lockForegroundClose() {
	p.foregroundMu.Lock()
}
