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
	"reflect"
	"testing"
)

func TestPoolGroupTickInto(t *testing.T) {
	group := testNewPoolGroup(t, "alpha", "beta")
	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	if report.Generation == 0 {
		t.Fatalf("Generation = 0, want advanced generation")
	}
	if report.Sample.PartitionCount != 2 {
		t.Fatalf("Sample.PartitionCount = %d, want 2", report.Sample.PartitionCount)
	}
	if report.Metrics.PartitionCount != 2 {
		t.Fatalf("Metrics.PartitionCount = %d, want 2", report.Metrics.PartitionCount)
	}
}

func TestPoolGroupTickIntoNilDstNoop(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	requireGroupNoError(t, group.TickInto(nil))
}

func TestPoolGroupTickReturnsReport(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	report, err := group.Tick()
	requireGroupNoError(t, err)
	if report.Sample.PartitionCount != 1 {
		t.Fatalf("Sample.PartitionCount = %d, want 1", report.Sample.PartitionCount)
	}
}

func TestPoolGroupTickDoesNotMutatePolicies(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	before := group.Policy()
	_, err := group.Tick()
	requireGroupNoError(t, err)
	after := group.Policy()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("Tick mutated policy: before=%#v after=%#v", before, after)
	}
}
