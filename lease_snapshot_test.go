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
	"testing"
	"time"
)

// TestLeaseSnapshotStatePredicates verifies active/released helpers.
func TestLeaseSnapshotStatePredicates(t *testing.T) {
	t.Parallel()

	active := LeaseSnapshot{State: LeaseStateActive}
	if !active.IsActive() {
		t.Fatal("active snapshot did not report IsActive")
	}
	if active.IsReleased() {
		t.Fatal("active snapshot reported IsReleased")
	}

	released := LeaseSnapshot{State: LeaseStateReleased}
	if !released.IsReleased() {
		t.Fatal("released snapshot did not report IsReleased")
	}
	if released.IsActive() {
		t.Fatal("released snapshot reported IsActive")
	}
}

// TestLeaseRecordSnapshotActiveAndReleased verifies record snapshot hold-duration
// behavior for active and released records.
func TestLeaseRecordSnapshotActiveAndReleased(t *testing.T) {
	t.Parallel()

	acquiredAt := time.Now().Add(-time.Second)
	record := &leaseRecord{
		id:               LeaseID(7),
		state:            LeaseStateActive,
		requestedSize:    300,
		originClass:      ClassSizeFromBytes(512),
		acquiredCapacity: 512,
		acquiredAt:       acquiredAt,
	}

	active := record.snapshot()
	if !active.IsActive() {
		t.Fatalf("state = %s, want active", active.State)
	}
	if active.HoldDuration <= 0 {
		t.Fatalf("active HoldDuration = %s, want positive", active.HoldDuration)
	}
	if active.ReturnedCapacity != 0 {
		t.Fatalf("active ReturnedCapacity = %d, want 0", active.ReturnedCapacity)
	}

	releasedAt := acquiredAt.Add(500 * time.Millisecond)
	record.state = LeaseStateReleased
	record.returnedCapacity = 512
	record.releasedAt = releasedAt
	released := record.snapshot()
	if !released.IsReleased() {
		t.Fatalf("state = %s, want released", released.State)
	}
	if released.HoldDuration != 500*time.Millisecond {
		t.Fatalf("released HoldDuration = %s, want 500ms", released.HoldDuration)
	}
	if released.ReturnedCapacity != 512 {
		t.Fatalf("released ReturnedCapacity = %d, want 512", released.ReturnedCapacity)
	}
}

// TestLeaseRegistrySnapshotPredicates verifies snapshot utility methods.
func TestLeaseRegistrySnapshotPredicates(t *testing.T) {
	t.Parallel()

	var snapshot LeaseRegistrySnapshot
	if snapshot.ActiveCount() != 0 {
		t.Fatalf("zero ActiveCount() = %d, want 0", snapshot.ActiveCount())
	}
	if !snapshot.IsEmpty() {
		t.Fatal("zero LeaseRegistrySnapshot did not report IsEmpty")
	}

	snapshot.Active = []LeaseSnapshot{{ID: 1, State: LeaseStateActive}}
	if snapshot.ActiveCount() != 1 {
		t.Fatalf("ActiveCount() = %d, want 1", snapshot.ActiveCount())
	}
	if snapshot.IsEmpty() {
		t.Fatal("snapshot with active lease reported IsEmpty")
	}
}
