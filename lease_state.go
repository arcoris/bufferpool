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

// LeaseState describes the lifecycle of one checked-out lease record.
//
// Lease state is separate from Pool lifecycle. Pool may be active, closing, or
// closed while a lease is active. LeaseRegistry owns this state and uses it to
// decide whether Release is valid, whether double release has occurred, and what
// snapshot diagnostics should report.
type LeaseState uint8

const (
	// LeaseStateInvalid is the zero diagnostic state.
	//
	// Live lease records are never intentionally created in this state. It is
	// useful for zero-value LeaseSnapshot values and unknown handles.
	LeaseStateInvalid LeaseState = iota

	// LeaseStateActive means the lease is checked out and exactly one successful
	// ownership release is still possible.
	LeaseStateActive

	// LeaseStateReleased means ownership has completed.
	//
	// A released lease may already have failed its best-effort Pool.Put handoff;
	// that does not reactivate the lease.
	LeaseStateReleased
)

// String returns a stable diagnostic label for s.
//
// The labels are intended for tests, snapshots, logs, and future controller
// diagnostics. They are not parsed by runtime logic.
func (s LeaseState) String() string {
	switch s {
	case LeaseStateInvalid:
		return "invalid"
	case LeaseStateActive:
		return "active"
	case LeaseStateReleased:
		return "released"
	default:
		return "unknown"
	}
}
