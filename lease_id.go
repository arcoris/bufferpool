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

import "strconv"

// LeaseID identifies one checked-out buffer lease within a LeaseRegistry.
//
// LeaseID is intentionally registry-local. A future PoolPartition may expose
// both registry and pool context around a lease id, but the id by itself is not
// globally unique across registries, partitions, groups, processes, or program
// restarts. Keeping this type small makes snapshots and tests easy to read while
// avoiding a false global identity guarantee.
type LeaseID uint64

// Uint64 returns id as a raw uint64 value.
//
// This is useful for metrics exporters, logs, snapshots, and tests. Ownership
// code should continue using LeaseID at API boundaries so registry-local
// semantics remain visible in signatures.
func (id LeaseID) Uint64() uint64 { return uint64(id) }

// IsZero reports whether id is the zero invalid lease id.
//
// LeaseRegistry never allocates id 0. The zero value therefore represents "no
// lease" in snapshots, tests, and zero-value Lease handles.
func (id LeaseID) IsZero() bool { return id == 0 }

// String returns the decimal representation of id.
//
// The format is stable for diagnostics. It intentionally omits registry context
// because LeaseID alone is not a global identifier.
func (id LeaseID) String() string { return strconv.FormatUint(uint64(id), 10) }
