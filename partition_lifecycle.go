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

import "arcoris.dev/bufferpool/internal/multierr"

const (
	// errPartitionClosed reports operations rejected after partition shutdown starts.
	errPartitionClosed = "bufferpool.PoolPartition: partition is closed"
)

// Lifecycle returns the current partition lifecycle state.
func (p *PoolPartition) Lifecycle() LifecycleState { p.mustBeInitialized(); return p.lifecycle.Load() }

// IsClosed reports whether the partition is closed.
func (p *PoolPartition) IsClosed() bool { p.mustBeInitialized(); return p.lifecycle.IsClosed() }

// Close performs the current hard partition shutdown.
//
// Close first closes the partition LeaseRegistry to reject new ownership
// acquisition, then closes owned Pools. Active leases may still complete
// ownership after registry close; if Pools are already closed, Lease.Release
// records Pool handoff failure diagnostically.
//
// Close serializes hard shutdown with closeMu and foregroundMu before
// publishing Closing. foregroundMu prevents new partition foreground work and
// waits for already-admitted Acquire, Release, TickInto, pressure, budget, and
// trim operations to finish before child cleanup starts. BeginClose then remains
// the hard-close cleanup ownership gate for concurrent Close callers. If a
// previous CloseGracefully timed out and left the partition in Closing, Close
// explicitly continues the already-started shutdown and completes the hard
// cleanup under the same gates.
//
// This is not a graceful drain controller. It does not wait for active leases
// beyond any previous CloseGracefully attempt, does not run background trim, and
// does not coordinate with PoolGroup.
func (p *PoolPartition) Close() error {
	p.mustBeInitialized()
	p.closeMu.Lock()
	defer p.closeMu.Unlock()

	p.lockForegroundClose()
	defer p.foregroundMu.Unlock()

	if !beginOrContinueSerializedCloseCleanupLocked(&p.lifecycle) {
		return nil
	}

	var err error
	if closeErr := p.leases.Close(); closeErr != nil {
		multierr.AppendInto(&err, closeErr)
	}
	if closeErr := p.registry.closeAll(); closeErr != nil {
		multierr.AppendInto(&err, closeErr)
	}
	p.lifecycle.MarkClosed()
	p.generation.Advance()
	return err
}
