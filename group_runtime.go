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
	// errGroupRuntimeSnapshotNil reports an internal nil group publication.
	errGroupRuntimeSnapshotNil = "bufferpool.PoolGroup: runtime snapshot must not be nil"
)

// groupRuntimeSnapshot is the immutable group-level runtime policy view.
type groupRuntimeSnapshot struct {
	// Generation is the group policy publication generation.
	Generation Generation

	// Policy is the immutable group policy used by manual coordinator cycles.
	Policy PoolGroupPolicy
}

// newGroupRuntimeSnapshot returns a normalized immutable group runtime view.
func newGroupRuntimeSnapshot(generation Generation, policy PoolGroupPolicy) *groupRuntimeSnapshot {
	return &groupRuntimeSnapshot{Generation: generation, Policy: policy.Normalize()}
}

// publishRuntimeSnapshot atomically publishes a group policy view.
//
// Group policy publication itself does not publish partition policies, execute
// trim, or advance the group state generation. Manual TickInto reads this
// snapshot and performs budget publication as an explicit foreground
// coordinator cycle.
func (g *PoolGroup) publishRuntimeSnapshot(snapshot *groupRuntimeSnapshot) {
	if snapshot == nil {
		panic(errGroupRuntimeSnapshotNil)
	}
	g.runtimeSnapshot.Store(newGroupRuntimeSnapshot(snapshot.Generation, snapshot.Policy))
}

// currentRuntimeSnapshot returns the currently published group policy view.
func (g *PoolGroup) currentRuntimeSnapshot() *groupRuntimeSnapshot {
	snapshot := g.runtimeSnapshot.Load()
	if snapshot == nil {
		panic(errNilPoolGroup)
	}
	return snapshot
}
