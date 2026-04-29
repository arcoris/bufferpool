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
	// errGroupClosed reports operations rejected after group shutdown starts.
	errGroupClosed = "bufferpool.PoolGroup: group is closed"
)

// Lifecycle returns the current group lifecycle state.
func (g *PoolGroup) Lifecycle() LifecycleState { g.mustBeInitialized(); return g.lifecycle.Load() }

// IsClosed reports whether the group is closed.
func (g *PoolGroup) IsClosed() bool { g.mustBeInitialized(); return g.lifecycle.IsClosed() }

// Close performs the current hard group shutdown.
//
// Close closes every group-owned PoolPartition and aggregates close errors. It
// is not a graceful drain controller, does not wait beyond partition Close
// behavior, does not coordinate physical trim, and does not coordinate with a
// background group scheduler.
func (g *PoolGroup) Close() error {
	g.mustBeInitialized()
	if g.lifecycle.IsClosed() {
		return nil
	}
	if !g.lifecycle.IsClosing() {
		g.lifecycle.BeginClose()
	}
	var err error
	if closeErr := g.registry.closeAll(); closeErr != nil {
		multierr.AppendInto(&err, closeErr)
	}
	g.lifecycle.MarkClosed()
	g.generation.Advance()
	return err
}
