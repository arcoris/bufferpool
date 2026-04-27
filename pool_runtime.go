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
	// errPoolRuntimeSnapshotNil is used when runtime snapshot publication is
	// asked to publish no snapshot.
	errPoolRuntimeSnapshotNil = "bufferpool.Pool: runtime snapshot must not be nil"
)

// poolRuntimeSnapshot is the immutable runtime view consumed by Pool hot paths.
//
// Pool is a data-plane owner. Future PoolPartition control-plane code can
// publish a new immutable snapshot without mutating Pool fields in place and
// without calling into Pool from the hot path. Get and Put load the pointer once
// near operation start and use that stable view for admission decisions.
//
// A snapshot is not a bucket rebuild. Shrinking policy limits through a future
// publication restricts new retention immediately, while physical correction of
// already retained buffers still belongs to bounded trim or close cleanup.
type poolRuntimeSnapshot struct {
	// Generation identifies the publication version of this runtime view.
	//
	// Pool does not interpret generation as a clock. It is carried into
	// snapshots, metrics, and internal samples so controller code can correlate
	// observations with the policy view that produced them.
	Generation Generation

	// Policy is the effective immutable policy for admission decisions.
	//
	// The value stored here owns its class-size slice. Publishers must never
	// mutate a Policy after publication; publishRuntimeSnapshot defensively
	// clones to enforce that boundary inside Pool.
	Policy Policy
}

// newPoolRuntimeSnapshot returns an immutable runtime snapshot value.
//
// The constructor clones Policy slice storage so callers can continue using or
// modifying their local Policy value without racing with Pool hot paths.
func newPoolRuntimeSnapshot(generation Generation, policy Policy) *poolRuntimeSnapshot {
	return &poolRuntimeSnapshot{
		Generation: generation,
		Policy:     clonePoolPolicy(policy),
	}
}

// clonePolicy returns a caller-owned Policy copy from the runtime snapshot.
//
// Public accessors and public snapshots use this helper so callers cannot
// mutate the live runtime policy slice.
func (s *poolRuntimeSnapshot) clonePolicy() Policy {
	if s == nil {
		return Policy{}
	}

	return clonePoolPolicy(s.Policy)
}

// publishRuntimeSnapshot atomically publishes a new Pool runtime view.
//
// This is intentionally a narrow internal hook. It updates admission data only;
// it does not rebuild the class table, resize buckets, redistribute existing
// retained buffers, trim over-target storage, or call any controller. Those
// actions belong to construction or cold control-plane paths.
func (p *Pool) publishRuntimeSnapshot(snapshot *poolRuntimeSnapshot) {
	if snapshot == nil {
		panic(errPoolRuntimeSnapshotNil)
	}

	p.runtimeSnapshot.Store(newPoolRuntimeSnapshot(snapshot.Generation, snapshot.Policy))
}

// currentRuntimeSnapshot returns the currently published runtime view.
//
// A nil pointer means Pool construction did not complete. The panic mirrors the
// rest of Pool's fail-fast zero-value receiver behavior.
func (p *Pool) currentRuntimeSnapshot() *poolRuntimeSnapshot {
	snapshot := p.runtimeSnapshot.Load()
	if snapshot == nil {
		panic(errUninitializedPool)
	}

	return snapshot
}
