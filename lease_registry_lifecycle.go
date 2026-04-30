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
	// errLeaseRegistryClosed is used when a caller tries to acquire a new lease
	// after registry shutdown has started.
	errLeaseRegistryClosed = "bufferpool.LeaseRegistry: registry is closed"
)

// Lifecycle returns the current registry lifecycle state.
//
// The returned value is observational. It should not be used as an external
// synchronization primitive because another goroutine may close the registry
// immediately after the value is read.
func (r *LeaseRegistry) Lifecycle() LifecycleState {
	r.mustBeInitialized()

	return r.lifecycle.Load()
}

// IsClosed reports whether the registry is closed.
//
// Closed registries reject new acquisition. Active leases that were acquired
// before Close remain releasable because checked-out ownership must be allowed
// to complete during shutdown.
func (r *LeaseRegistry) IsClosed() bool {
	r.mustBeInitialized()

	return r.lifecycle.IsClosed()
}

// Close prevents new acquisitions.
//
// Active leases are intentionally not force-released. A future Partition/Group
// close sequence can use Snapshot to observe outstanding leases and decide
// whether to wait, log, or force shutdown. Release remains allowed after Close.
// Owners that want clean retained-storage handoff should close the registry
// before closing its Pools; if a Pool is closed first, ownership release still
// completes and the failed Pool handoff is recorded diagnostically.
// Concurrent Close callers wait on the registry mutex so only one caller
// publishes Closed and advances generation. If Closing is already visible after
// the mutex is owned, Close finishes that already-started shutdown.
func (r *LeaseRegistry) Close() error {
	r.mustBeInitialized()
	r.mu.Lock()
	defer r.mu.Unlock()

	if !beginOrContinueSerializedCloseCleanupLocked(&r.lifecycle) {
		return nil
	}
	r.lifecycle.MarkClosed()
	r.generation.Advance()

	return nil
}
