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
	"sync"
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestShardCreditLimitZero verifies disabled limit semantics.
//
// A zero limit is a valid disabled credit assignment. It means the shard should
// reject new retained buffers through the credit gate, but it does not imply that
// already retained buffers were physically removed.
func TestShardCreditLimitZero(t *testing.T) {
	t.Parallel()

	limit := shardCreditLimit{}

	if !limit.IsZero() {
		t.Fatal("zero limit IsZero() = false, want true")
	}

	if limit.IsEnabled() {
		t.Fatal("zero limit IsEnabled() = true, want false")
	}

	if limit.IsPartial() {
		t.Fatal("zero limit IsPartial() = true, want false")
	}

	if !limit.IsConsistent() {
		t.Fatal("zero limit IsConsistent() = false, want true")
	}
}

// TestNewShardCreditLimit verifies construction of valid shard-local credit
// limits.
//
// shardCreditLimit represents an already-derived local shard limit. It does not
// compute itself from ClassSize. That computation belongs to class_budget.go.
func TestNewShardCreditLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		targetBuffers uint64
		targetBytes   uint64

		wantEnabled bool
		wantZero    bool
	}{
		{
			name:          "disabled",
			targetBuffers: 0,
			targetBytes:   0,
			wantEnabled:   false,
			wantZero:      true,
		},
		{
			name:          "minimal enabled",
			targetBuffers: 1,
			targetBytes:   1,
			wantEnabled:   true,
			wantZero:      false,
		},
		{
			name:          "ordinary enabled",
			targetBuffers: 4,
			targetBytes:   4096,
			wantEnabled:   true,
			wantZero:      false,
		},
		{
			name:          "large enabled",
			targetBuffers: 1024,
			targetBytes:   64 * 1024 * 1024,
			wantEnabled:   true,
			wantZero:      false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			limit := newShardCreditLimit(tt.targetBuffers, tt.targetBytes)

			if limit.TargetBuffers != tt.targetBuffers {
				t.Fatalf("TargetBuffers = %d, want %d", limit.TargetBuffers, tt.targetBuffers)
			}

			if limit.TargetBytes != tt.targetBytes {
				t.Fatalf("TargetBytes = %d, want %d", limit.TargetBytes, tt.targetBytes)
			}

			if got := limit.IsEnabled(); got != tt.wantEnabled {
				t.Fatalf("IsEnabled() = %t, want %t", got, tt.wantEnabled)
			}

			if got := limit.IsZero(); got != tt.wantZero {
				t.Fatalf("IsZero() = %t, want %t", got, tt.wantZero)
			}

			if limit.IsPartial() {
				t.Fatal("IsPartial() = true, want false")
			}

			if !limit.IsConsistent() {
				t.Fatal("IsConsistent() = false, want true")
			}
		})
	}
}

// TestNewShardCreditLimitPanicsForPartialLimit verifies that credit dimensions
// must be enabled or disabled together.
func TestNewShardCreditLimitPanicsForPartialLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		targetBuffers uint64
		targetBytes   uint64
	}{
		{
			name:          "buffers only",
			targetBuffers: 1,
			targetBytes:   0,
		},
		{
			name:          "bytes only",
			targetBuffers: 0,
			targetBytes:   1,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, errShardCreditPartialLimit, func() {
				_ = newShardCreditLimit(tt.targetBuffers, tt.targetBytes)
			})
		})
	}
}

// TestNewShardCreditLimitPanicsForInconsistentLimit verifies the basic enabled
// credit consistency invariant.
//
// TargetBytes must be able to fund at least one byte for each retained-buffer
// credit. Otherwise the limit is internally contradictory.
func TestNewShardCreditLimitPanicsForInconsistentLimit(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errShardCreditInconsistentLimit, func() {
		_ = newShardCreditLimit(2, 1)
	})
}

// TestShardCreditLimitPredicates verifies direct predicate behavior for manually
// constructed limits.
//
// Manual construction can represent invalid states that newShardCreditLimit
// rejects. Predicates should classify those states without panicking.
func TestShardCreditLimitPredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		limit shardCreditLimit

		wantZero       bool
		wantEnabled    bool
		wantPartial    bool
		wantConsistent bool
	}{
		{
			name: "zero",
			limit: shardCreditLimit{
				TargetBuffers: 0,
				TargetBytes:   0,
			},
			wantZero:       true,
			wantEnabled:    false,
			wantPartial:    false,
			wantConsistent: true,
		},
		{
			name: "partial buffers only",
			limit: shardCreditLimit{
				TargetBuffers: 1,
				TargetBytes:   0,
			},
			wantZero:       false,
			wantEnabled:    false,
			wantPartial:    true,
			wantConsistent: false,
		},
		{
			name: "partial bytes only",
			limit: shardCreditLimit{
				TargetBuffers: 0,
				TargetBytes:   1,
			},
			wantZero:       false,
			wantEnabled:    false,
			wantPartial:    true,
			wantConsistent: false,
		},
		{
			name: "inconsistent enabled",
			limit: shardCreditLimit{
				TargetBuffers: 2,
				TargetBytes:   1,
			},
			wantZero:       false,
			wantEnabled:    true,
			wantPartial:    false,
			wantConsistent: false,
		},
		{
			name: "consistent enabled",
			limit: shardCreditLimit{
				TargetBuffers: 2,
				TargetBytes:   4096,
			},
			wantZero:       false,
			wantEnabled:    true,
			wantPartial:    false,
			wantConsistent: true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.limit.IsZero(); got != tt.wantZero {
				t.Fatalf("IsZero() = %t, want %t", got, tt.wantZero)
			}

			if got := tt.limit.IsEnabled(); got != tt.wantEnabled {
				t.Fatalf("IsEnabled() = %t, want %t", got, tt.wantEnabled)
			}

			if got := tt.limit.IsPartial(); got != tt.wantPartial {
				t.Fatalf("IsPartial() = %t, want %t", got, tt.wantPartial)
			}

			if got := tt.limit.IsConsistent(); got != tt.wantConsistent {
				t.Fatalf("IsConsistent() = %t, want %t", got, tt.wantConsistent)
			}
		})
	}
}

// TestShardCreditZeroValueSnapshot verifies zero-value shardCredit behavior.
//
// A zero-value shardCredit is disabled and should reject retention, but it should
// be safe to snapshot and evaluate.
func TestShardCreditZeroValueSnapshot(t *testing.T) {
	t.Parallel()

	var credit shardCredit

	snapshot := credit.snapshot()

	if !snapshot.IsZero() {
		t.Fatalf("zero-value snapshot IsZero() = false, snapshot=%+v", snapshot)
	}

	if snapshot.IsEnabled() {
		t.Fatal("zero-value snapshot IsEnabled() = true, want false")
	}

	if snapshot.IsPartial() {
		t.Fatal("zero-value snapshot IsPartial() = true, want false")
	}

	if !snapshot.IsConsistent() {
		t.Fatal("zero-value snapshot IsConsistent() = false, want true")
	}

	decision := credit.evaluateRetain(shardCreditUsage{}, 1)
	if decision != shardCreditRejectNoCredit {
		t.Fatalf("zero-value evaluateRetain() = %s, want %s", decision, shardCreditRejectNoCredit)
	}

	if credit.allowsRetain(shardCreditUsage{}, 1) {
		t.Fatal("zero-value allowsRetain() = true, want false")
	}
}

// TestShardCreditUpdate verifies publication of a new credit limit.
//
// update stores the target dimensions and advances the generation.
func TestShardCreditUpdate(t *testing.T) {
	t.Parallel()

	var credit shardCredit

	before := credit.snapshot()
	generation := credit.update(newShardCreditLimit(4, 4096))
	after := credit.snapshot()

	if after.Generation != generation {
		t.Fatalf("snapshot Generation = %v, want returned generation %v", after.Generation, generation)
	}

	if after.Generation == before.Generation {
		t.Fatalf("Generation did not advance: before=%v after=%v", before.Generation, after.Generation)
	}

	if after.TargetBuffers != 4 {
		t.Fatalf("TargetBuffers = %d, want 4", after.TargetBuffers)
	}

	if after.TargetBytes != 4096 {
		t.Fatalf("TargetBytes = %d, want 4096", after.TargetBytes)
	}

	if !after.IsEnabled() {
		t.Fatal("IsEnabled() = false, want true")
	}

	if !after.IsConsistent() {
		t.Fatal("IsConsistent() = false, want true")
	}
}

// TestShardCreditDisable verifies disabling already configured credit.
//
// Disabling prevents new retention through the credit gate. It does not imply
// physical cleanup of already retained buffers.
func TestShardCreditDisable(t *testing.T) {
	t.Parallel()

	var credit shardCredit

	firstGeneration := credit.update(newShardCreditLimit(4, 4096))
	disabledGeneration := credit.disable()

	if disabledGeneration == firstGeneration {
		t.Fatalf("disable generation = %v, want generation after %v", disabledGeneration, firstGeneration)
	}

	snapshot := credit.snapshot()

	if snapshot.Generation != disabledGeneration {
		t.Fatalf("Generation = %v, want %v", snapshot.Generation, disabledGeneration)
	}

	if !snapshot.IsZero() {
		t.Fatalf("disabled snapshot IsZero() = false, snapshot=%+v", snapshot)
	}

	if snapshot.IsEnabled() {
		t.Fatal("disabled snapshot IsEnabled() = true, want false")
	}

	decision := credit.evaluateRetain(shardCreditUsage{}, 1)
	if decision != shardCreditRejectNoCredit {
		t.Fatalf("disabled evaluateRetain() = %s, want %s", decision, shardCreditRejectNoCredit)
	}
}

// TestShardCreditSnapshotLimit verifies conversion from snapshot to limit.
//
// The conversion should preserve the observed target dimensions without
// validating them, because snapshots may represent mixed values during a
// concurrent update.
func TestShardCreditSnapshotLimit(t *testing.T) {
	t.Parallel()

	snapshot := shardCreditSnapshot{
		TargetBuffers: 4,
		TargetBytes:   4096,
	}

	limit := snapshot.Limit()

	if limit.TargetBuffers != 4 {
		t.Fatalf("Limit().TargetBuffers = %d, want 4", limit.TargetBuffers)
	}

	if limit.TargetBytes != 4096 {
		t.Fatalf("Limit().TargetBytes = %d, want 4096", limit.TargetBytes)
	}
}

// TestShardCreditSnapshotPredicates verifies snapshot classification.
//
// Snapshot predicates must classify valid, disabled, partial, and inconsistent
// observed target pairs without panicking.
func TestShardCreditSnapshotPredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		snapshot shardCreditSnapshot

		wantZero       bool
		wantEnabled    bool
		wantPartial    bool
		wantConsistent bool
	}{
		{
			name: "zero",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 0,
				TargetBytes:   0,
			},
			wantZero:       true,
			wantEnabled:    false,
			wantPartial:    false,
			wantConsistent: true,
		},
		{
			name: "partial buffers only",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 1,
				TargetBytes:   0,
			},
			wantZero:       false,
			wantEnabled:    false,
			wantPartial:    true,
			wantConsistent: false,
		},
		{
			name: "partial bytes only",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 0,
				TargetBytes:   1,
			},
			wantZero:       false,
			wantEnabled:    false,
			wantPartial:    true,
			wantConsistent: false,
		},
		{
			name: "inconsistent enabled",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 2,
				TargetBytes:   1,
			},
			wantZero:       false,
			wantEnabled:    true,
			wantPartial:    false,
			wantConsistent: false,
		},
		{
			name: "consistent enabled",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 2,
				TargetBytes:   4096,
			},
			wantZero:       false,
			wantEnabled:    true,
			wantPartial:    false,
			wantConsistent: true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.snapshot.IsZero(); got != tt.wantZero {
				t.Fatalf("IsZero() = %t, want %t", got, tt.wantZero)
			}

			if got := tt.snapshot.IsEnabled(); got != tt.wantEnabled {
				t.Fatalf("IsEnabled() = %t, want %t", got, tt.wantEnabled)
			}

			if got := tt.snapshot.IsPartial(); got != tt.wantPartial {
				t.Fatalf("IsPartial() = %t, want %t", got, tt.wantPartial)
			}

			if got := tt.snapshot.IsConsistent(); got != tt.wantConsistent {
				t.Fatalf("IsConsistent() = %t, want %t", got, tt.wantConsistent)
			}
		})
	}
}

// TestShardCreditEvaluateRetain verifies all credit admission decisions.
//
// Evaluation is purely local and does not mutate the credit state or retained
// usage. Admission code will later map these internal decisions to drop reasons.
func TestShardCreditEvaluateRetain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		snapshot          shardCreditSnapshot
		usage             shardCreditUsage
		candidateCapacity uint64

		want shardCreditDecision
	}{
		{
			name: "invalid zero capacity",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage:             shardCreditUsage{},
			candidateCapacity: 0,
			want:              shardCreditRejectInvalidCapacity,
		},
		{
			name: "disabled credit",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 0,
				TargetBytes:   0,
			},
			usage:             shardCreditUsage{},
			candidateCapacity: 1,
			want:              shardCreditRejectNoCredit,
		},
		{
			name: "partial credit",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   0,
			},
			usage:             shardCreditUsage{},
			candidateCapacity: 1,
			want:              shardCreditRejectInconsistentCredit,
		},
		{
			name: "inconsistent enabled credit",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   3,
			},
			usage:             shardCreditUsage{},
			candidateCapacity: 1,
			want:              shardCreditRejectInconsistentCredit,
		},
		{
			name: "buffer credit exhausted",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 4,
				RetainedBytes:   1024,
			},
			candidateCapacity: 1,
			want:              shardCreditRejectBufferCreditExhausted,
		},
		{
			name: "byte credit exhausted at target",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 1,
				RetainedBytes:   4096,
			},
			candidateCapacity: 1,
			want:              shardCreditRejectByteCreditExhausted,
		},
		{
			name: "byte credit exhausted above target",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 1,
				RetainedBytes:   4097,
			},
			candidateCapacity: 1,
			want:              shardCreditRejectByteCreditExhausted,
		},
		{
			name: "byte accounting overflow",
			snapshot: shardCreditSnapshot{
				TargetBuffers: maxShardCreditValue,
				TargetBytes:   maxShardCreditValue,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 1,
				RetainedBytes:   maxShardCreditValue,
			},
			candidateCapacity: 1,
			want:              shardCreditRejectByteCreditExhausted,
		},
		{
			name: "candidate would overflow accounting",
			snapshot: shardCreditSnapshot{
				TargetBuffers: maxShardCreditValue,
				TargetBytes:   maxShardCreditValue,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 1,
				RetainedBytes:   maxShardCreditValue - 1,
			},
			candidateCapacity: 2,
			want:              shardCreditRejectByteAccountingOverflow,
		},
		{
			name: "candidate exceeds remaining byte credit",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 1,
				RetainedBytes:   3072,
			},
			candidateCapacity: 2048,
			want:              shardCreditRejectByteCreditExhausted,
		},
		{
			name: "candidate exactly fits remaining credit",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 1,
				RetainedBytes:   3072,
			},
			candidateCapacity: 1024,
			want:              shardCreditAccept,
		},
		{
			name: "candidate fits ordinary credit",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 1,
				RetainedBytes:   1024,
			},
			candidateCapacity: 512,
			want:              shardCreditAccept,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.snapshot.evaluateRetain(tt.usage, tt.candidateCapacity)
			if got != tt.want {
				t.Fatalf("evaluateRetain() = %s, want %s", got, tt.want)
			}

			if got.AllowsRetain() != (tt.want == shardCreditAccept) {
				t.Fatalf("AllowsRetain() = %t, want %t", got.AllowsRetain(), tt.want == shardCreditAccept)
			}

			if tt.snapshot.allowsRetain(tt.usage, tt.candidateCapacity) != (tt.want == shardCreditAccept) {
				t.Fatalf("allowsRetain() mismatch for decision %s", tt.want)
			}
		})
	}
}

// TestShardCreditEvaluateRetainOverflowDecision verifies the overflow path
// specifically.
//
// This test uses a target larger than retained usage so byte exhaustion does not
// mask the overflow guard.
func TestShardCreditEvaluateRetainOverflowDecision(t *testing.T) {
	t.Parallel()

	snapshot := shardCreditSnapshot{
		TargetBuffers: maxShardCreditValue,
		TargetBytes:   maxShardCreditValue,
	}

	usage := shardCreditUsage{
		RetainedBuffers: 1,
		RetainedBytes:   maxShardCreditValue - 1,
	}

	decision := snapshot.evaluateRetain(usage, 2)
	if decision != shardCreditRejectByteAccountingOverflow {
		t.Fatalf("evaluateRetain() = %s, want %s", decision, shardCreditRejectByteAccountingOverflow)
	}
}

// TestShardCreditEvaluateRetainThroughAtomicCredit verifies the receiver methods
// on shardCredit itself.
//
// This covers the path that admission code is expected to use: snapshot through
// the current atomic credit state, then evaluate a candidate.
func TestShardCreditEvaluateRetainThroughAtomicCredit(t *testing.T) {
	t.Parallel()

	var credit shardCredit

	credit.update(newShardCreditLimit(4, 4096))

	usage := shardCreditUsage{
		RetainedBuffers: 2,
		RetainedBytes:   2048,
	}

	decision := credit.evaluateRetain(usage, 1024)
	if decision != shardCreditAccept {
		t.Fatalf("evaluateRetain() = %s, want %s", decision, shardCreditAccept)
	}

	if !credit.allowsRetain(usage, 1024) {
		t.Fatal("allowsRetain() = false, want true")
	}

	exhaustedUsage := shardCreditUsage{
		RetainedBuffers: 4,
		RetainedBytes:   2048,
	}

	decision = credit.evaluateRetain(exhaustedUsage, 1)
	if decision != shardCreditRejectBufferCreditExhausted {
		t.Fatalf("evaluateRetain() with exhausted buffers = %s, want %s", decision, shardCreditRejectBufferCreditExhausted)
	}
}

// TestShardCreditUsageIsZero verifies retained usage zero classification.
func TestShardCreditUsageIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		usage shardCreditUsage

		want bool
	}{
		{
			name:  "zero",
			usage: shardCreditUsage{},
			want:  true,
		},
		{
			name: "buffers only",
			usage: shardCreditUsage{
				RetainedBuffers: 1,
			},
			want: false,
		},
		{
			name: "bytes only",
			usage: shardCreditUsage{
				RetainedBytes: 1,
			},
			want: false,
		},
		{
			name: "buffers and bytes",
			usage: shardCreditUsage{
				RetainedBuffers: 1,
				RetainedBytes:   64,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.usage.IsZero(); got != tt.want {
				t.Fatalf("IsZero() = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestShardCreditRemaining verifies saturated remaining-credit calculation.
func TestShardCreditRemaining(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		snapshot shardCreditSnapshot
		usage    shardCreditUsage

		want shardCreditRemaining
	}{
		{
			name: "disabled snapshot has no remaining credit",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 0,
				TargetBytes:   0,
			},
			usage: shardCreditUsage{},
			want:  shardCreditRemaining{},
		},
		{
			name: "partial snapshot has no remaining credit",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   0,
			},
			usage: shardCreditUsage{},
			want:  shardCreditRemaining{},
		},
		{
			name: "inconsistent snapshot has no remaining credit",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   3,
			},
			usage: shardCreditUsage{},
			want:  shardCreditRemaining{},
		},
		{
			name: "zero usage",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{},
			want: shardCreditRemaining{
				Buffers: 4,
				Bytes:   4096,
			},
		},
		{
			name: "partial usage",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 1,
				RetainedBytes:   1024,
			},
			want: shardCreditRemaining{
				Buffers: 3,
				Bytes:   3072,
			},
		},
		{
			name: "buffer target reached",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 4,
				RetainedBytes:   1024,
			},
			want: shardCreditRemaining{
				Buffers: 0,
				Bytes:   3072,
			},
		},
		{
			name: "byte target reached",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 1,
				RetainedBytes:   4096,
			},
			want: shardCreditRemaining{
				Buffers: 3,
				Bytes:   0,
			},
		},
		{
			name: "over target saturates at zero",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 6,
				RetainedBytes:   8192,
			},
			want: shardCreditRemaining{},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.snapshot.remaining(tt.usage)
			if got != tt.want {
				t.Fatalf("remaining() = %+v, want %+v", got, tt.want)
			}

			if got.IsZero() != (got.Buffers == 0 && got.Bytes == 0) {
				t.Fatalf("remaining IsZero() inconsistent for %+v", got)
			}
		})
	}
}

// TestShardCreditOverage verifies over-target accounting.
func TestShardCreditOverage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		snapshot shardCreditSnapshot
		usage    shardCreditUsage

		want shardCreditAmount
	}{
		{
			name: "disabled credit treats retained usage as over target",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 0,
				TargetBytes:   0,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 2,
				RetainedBytes:   2048,
			},
			want: shardCreditAmount{
				Buffers: 2,
				Bytes:   2048,
			},
		},
		{
			name: "partial credit treats retained usage as over target",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 2,
				TargetBytes:   0,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 2,
				RetainedBytes:   2048,
			},
			want: shardCreditAmount{
				Buffers: 2,
				Bytes:   2048,
			},
		},
		{
			name: "within target",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 2,
				RetainedBytes:   2048,
			},
			want: shardCreditAmount{},
		},
		{
			name: "buffer over target",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 6,
				RetainedBytes:   2048,
			},
			want: shardCreditAmount{
				Buffers: 2,
				Bytes:   0,
			},
		},
		{
			name: "byte over target",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 2,
				RetainedBytes:   8192,
			},
			want: shardCreditAmount{
				Buffers: 0,
				Bytes:   4096,
			},
		},
		{
			name: "both over target",
			snapshot: shardCreditSnapshot{
				TargetBuffers: 4,
				TargetBytes:   4096,
			},
			usage: shardCreditUsage{
				RetainedBuffers: 6,
				RetainedBytes:   8192,
			},
			want: shardCreditAmount{
				Buffers: 2,
				Bytes:   4096,
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.snapshot.overage(tt.usage)
			if got != tt.want {
				t.Fatalf("overage() = %+v, want %+v", got, tt.want)
			}

			if got.IsZero() != (got.Buffers == 0 && got.Bytes == 0) {
				t.Fatalf("overage IsZero() inconsistent for %+v", got)
			}

			if tt.snapshot.isOverTarget(tt.usage) != !tt.want.IsZero() {
				t.Fatalf("isOverTarget() = %t, want %t", tt.snapshot.isOverTarget(tt.usage), !tt.want.IsZero())
			}
		})
	}
}

// TestShardCreditRemainingIsZero verifies remaining-credit zero classification.
func TestShardCreditRemainingIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		remaining shardCreditRemaining

		want bool
	}{
		{
			name:      "zero",
			remaining: shardCreditRemaining{},
			want:      true,
		},
		{
			name: "buffers only",
			remaining: shardCreditRemaining{
				Buffers: 1,
			},
			want: false,
		},
		{
			name: "bytes only",
			remaining: shardCreditRemaining{
				Bytes: 1,
			},
			want: false,
		},
		{
			name: "buffers and bytes",
			remaining: shardCreditRemaining{
				Buffers: 1,
				Bytes:   1,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.remaining.IsZero(); got != tt.want {
				t.Fatalf("IsZero() = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestShardCreditAmountIsZero verifies amount zero classification.
func TestShardCreditAmountIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		amount shardCreditAmount

		want bool
	}{
		{
			name:   "zero",
			amount: shardCreditAmount{},
			want:   true,
		},
		{
			name: "buffers only",
			amount: shardCreditAmount{
				Buffers: 1,
			},
			want: false,
		},
		{
			name: "bytes only",
			amount: shardCreditAmount{
				Bytes: 1,
			},
			want: false,
		},
		{
			name: "buffers and bytes",
			amount: shardCreditAmount{
				Buffers: 1,
				Bytes:   1,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.amount.IsZero(); got != tt.want {
				t.Fatalf("IsZero() = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestShardCreditDecisionString verifies stable diagnostic decision names.
//
// These strings are internal diagnostics. Public drop reasons should be modeled
// separately by admission/drop-reason code.
func TestShardCreditDecisionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		decision shardCreditDecision

		wantString string
		wantAllows bool
	}{
		{
			name:       "accept",
			decision:   shardCreditAccept,
			wantString: "accept",
			wantAllows: true,
		},
		{
			name:       "invalid capacity",
			decision:   shardCreditRejectInvalidCapacity,
			wantString: "reject_invalid_capacity",
			wantAllows: false,
		},
		{
			name:       "no credit",
			decision:   shardCreditRejectNoCredit,
			wantString: "reject_no_credit",
			wantAllows: false,
		},
		{
			name:       "inconsistent credit",
			decision:   shardCreditRejectInconsistentCredit,
			wantString: "reject_inconsistent_credit",
			wantAllows: false,
		},
		{
			name:       "buffer credit exhausted",
			decision:   shardCreditRejectBufferCreditExhausted,
			wantString: "reject_buffer_credit_exhausted",
			wantAllows: false,
		},
		{
			name:       "byte credit exhausted",
			decision:   shardCreditRejectByteCreditExhausted,
			wantString: "reject_byte_credit_exhausted",
			wantAllows: false,
		},
		{
			name:       "byte accounting overflow",
			decision:   shardCreditRejectByteAccountingOverflow,
			wantString: "reject_byte_accounting_overflow",
			wantAllows: false,
		},
		{
			name:       "not evaluated",
			decision:   shardCreditNotEvaluated,
			wantString: "not_evaluated",
			wantAllows: false,
		},
		{
			name:       "unknown",
			decision:   shardCreditDecision(255),
			wantString: "unknown",
			wantAllows: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.decision.String(); got != tt.wantString {
				t.Fatalf("String() = %q, want %q", got, tt.wantString)
			}

			if got := tt.decision.AllowsRetain(); got != tt.wantAllows {
				t.Fatalf("AllowsRetain() = %t, want %t", got, tt.wantAllows)
			}
		})
	}
}

// TestShardCreditConcurrentUpdateAndSnapshot verifies that concurrent credit
// updates and snapshots do not race or panic.
//
// The snapshot can observe old, new, or mixed values. Evaluation must be
// conservative and safe for all observed states.
func TestShardCreditConcurrentUpdateAndSnapshot(t *testing.T) {
	t.Parallel()

	var credit shardCredit
	var wg sync.WaitGroup

	const goroutines = 8
	const iterations = 1024

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i

		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				if (i+j)%2 == 0 {
					credit.update(newShardCreditLimit(4, 4096))
				} else {
					credit.disable()
				}

				snapshot := credit.snapshot()
				decision := snapshot.evaluateRetain(shardCreditUsage{}, 1)

				switch decision {
				case shardCreditAccept,
					shardCreditRejectNoCredit,
					shardCreditRejectInconsistentCredit:
					// These are the expected outcomes under concurrent update.
				default:
					t.Errorf("unexpected decision under concurrent update: %s", decision)
				}
			}
		}()
	}

	wg.Wait()
}
