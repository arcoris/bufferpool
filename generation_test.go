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

// TestGenerationConstants verifies the canonical generation values.
//
// Generation zero is reserved as the "not published yet" marker. The first real
// published runtime state starts at InitialGeneration. This distinction is
// important for snapshots, policy views, controller cycles, and budget
// publication records.
func TestGenerationConstants(t *testing.T) {
	t.Parallel()

	if NoGeneration != Generation(0) {
		t.Fatalf("NoGeneration = %d, want 0", NoGeneration)
	}

	if InitialGeneration != Generation(1) {
		t.Fatalf("InitialGeneration = %d, want 1", InitialGeneration)
	}

	if MaxGeneration != ^Generation(0) {
		t.Fatalf("MaxGeneration = %d, want %d", MaxGeneration, ^Generation(0))
	}
}

// TestGenerationUint64 verifies raw numeric extraction.
//
// Uint64 is useful for metrics, snapshots, logs, and tests where the generation
// has to be emitted as a plain numeric value.
func TestGenerationUint64(t *testing.T) {
	t.Parallel()

	generation := Generation(42)

	if got := generation.Uint64(); got != 42 {
		t.Fatalf("Generation(42).Uint64() = %d, want 42", got)
	}
}

// TestGenerationString verifies diagnostic string formatting.
//
// String should use the decimal representation because generations are ordinary
// monotonic runtime version numbers, not hexadecimal identifiers.
func TestGenerationString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// generation is the value being formatted.
		generation Generation

		// want is the expected decimal string.
		want string
	}{
		{
			name:       "zero generation",
			generation: NoGeneration,
			want:       "0",
		},
		{
			name:       "initial generation",
			generation: InitialGeneration,
			want:       "1",
		},
		{
			name:       "ordinary generation",
			generation: Generation(42),
			want:       "42",
		},
		{
			name:       "max generation",
			generation: MaxGeneration,
			want:       "18446744073709551615",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.generation.String()
			if got != tt.want {
				t.Fatalf("Generation(%d).String() = %q, want %q", tt.generation, got, tt.want)
			}
		})
	}
}

// TestGenerationIsZero verifies the unpublished-state predicate.
//
// IsZero should be true only for NoGeneration. Any non-zero generation represents
// an ordinary published or advanced runtime generation.
func TestGenerationIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// generation is the candidate value.
		generation Generation

		// want is true only for NoGeneration.
		want bool
	}{
		{
			name:       "no generation",
			generation: NoGeneration,
			want:       true,
		},
		{
			name:       "initial generation",
			generation: InitialGeneration,
			want:       false,
		},
		{
			name:       "ordinary generation",
			generation: Generation(42),
			want:       false,
		},
		{
			name:       "max generation",
			generation: MaxGeneration,
			want:       false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.generation.IsZero()
			if got != tt.want {
				t.Fatalf("Generation(%d).IsZero() = %t, want %t", tt.generation, got, tt.want)
			}
		})
	}
}

// TestGenerationNext verifies ordinary monotonic advancement.
//
// Advancing NoGeneration should produce InitialGeneration. Advancing an ordinary
// generation should produce the next numeric generation.
func TestGenerationNext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// generation is the current generation.
		generation Generation

		// want is the next generation.
		want Generation
	}{
		{
			name:       "no generation advances to initial generation",
			generation: NoGeneration,
			want:       InitialGeneration,
		},
		{
			name:       "initial generation advances to second generation",
			generation: InitialGeneration,
			want:       Generation(2),
		},
		{
			name:       "ordinary generation advances by one",
			generation: Generation(42),
			want:       Generation(43),
		},
		{
			name:       "generation before max advances to max",
			generation: MaxGeneration - 1,
			want:       MaxGeneration,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.generation.Next()
			if got != tt.want {
				t.Fatalf("Generation(%d).Next() = %d, want %d", tt.generation, got, tt.want)
			}
		})
	}
}

// TestGenerationNextPanicsOnOverflow verifies that generation wrap-around is
// forbidden.
//
// Runtime code compares generations using ordinary unsigned ordering. If
// MaxGeneration wrapped to NoGeneration, an older state could appear newer or an
// unpublished state could appear after a published state.
func TestGenerationNextPanicsOnOverflow(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errGenerationOverflow, func() {
		_ = MaxGeneration.Next()
	})
}

// TestGenerationCompare verifies three-way generation ordering.
//
// Compare is useful when code wants to branch on older/equal/newer state without
// duplicating comparison logic at call sites.
func TestGenerationCompare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// generation is the left-hand value.
		generation Generation

		// other is the right-hand value.
		other Generation

		// want is -1, 0, or +1.
		want int
	}{
		{
			name:       "older",
			generation: Generation(1),
			other:      Generation(2),
			want:       -1,
		},
		{
			name:       "equal",
			generation: Generation(2),
			other:      Generation(2),
			want:       0,
		},
		{
			name:       "newer",
			generation: Generation(3),
			other:      Generation(2),
			want:       1,
		},
		{
			name:       "no generation before initial generation",
			generation: NoGeneration,
			other:      InitialGeneration,
			want:       -1,
		},
		{
			name:       "max generation after ordinary generation",
			generation: MaxGeneration,
			other:      Generation(42),
			want:       1,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.generation.Compare(tt.other)
			if got != tt.want {
				t.Fatalf("Generation(%d).Compare(%d) = %d, want %d", tt.generation, tt.other, got, tt.want)
			}
		})
	}
}

// TestGenerationBeforeAfter verifies convenience ordering predicates.
//
// These methods make snapshot and publication comparisons easier to read at
// higher layers.
func TestGenerationBeforeAfter(t *testing.T) {
	t.Parallel()

	older := Generation(10)
	equal := Generation(10)
	newer := Generation(11)

	if !older.Before(newer) {
		t.Fatal("older.Before(newer) = false, want true")
	}

	if older.After(newer) {
		t.Fatal("older.After(newer) = true, want false")
	}

	if equal.Before(older) {
		t.Fatal("equal.Before(older) = true, want false")
	}

	if equal.After(older) {
		t.Fatal("equal.After(older) = true, want false")
	}

	if !newer.After(older) {
		t.Fatal("newer.After(older) = false, want true")
	}

	if newer.Before(older) {
		t.Fatal("newer.Before(older) = true, want false")
	}
}

// TestAtomicGenerationZeroValue verifies that AtomicGeneration is usable without
// explicit initialization.
//
// A zero-value holder starts at NoGeneration. This is important for publishers
// that embed AtomicGeneration directly into runtime state structs.
func TestAtomicGenerationZeroValue(t *testing.T) {
	t.Parallel()

	var generation AtomicGeneration

	if got := generation.Load(); got != NoGeneration {
		t.Fatalf("zero-value AtomicGeneration Load() = %d, want %d", got, NoGeneration)
	}
}

// TestAtomicGenerationStoreAndLoad verifies explicit publication.
//
// Store is intended for initialization, restoration, or controlled publication.
// Ordinary monotonic movement should use Advance.
func TestAtomicGenerationStoreAndLoad(t *testing.T) {
	t.Parallel()

	var generation AtomicGeneration

	generation.Store(Generation(42))

	if got := generation.Load(); got != Generation(42) {
		t.Fatalf("Load after Store = %d, want 42", got)
	}
}

// TestAtomicGenerationAdvance verifies monotonic atomic advancement.
//
// Advance should move the holder from NoGeneration to InitialGeneration and then
// by one for each subsequent call.
func TestAtomicGenerationAdvance(t *testing.T) {
	t.Parallel()

	var generation AtomicGeneration

	first := generation.Advance()
	if first != InitialGeneration {
		t.Fatalf("first Advance() = %d, want %d", first, InitialGeneration)
	}

	second := generation.Advance()
	if second != Generation(2) {
		t.Fatalf("second Advance() = %d, want 2", second)
	}

	if got := generation.Load(); got != Generation(2) {
		t.Fatalf("Load after two advances = %d, want 2", got)
	}
}

// TestAtomicGenerationAdvanceFromStoredValue verifies advancement from an
// explicitly stored generation.
//
// This covers publisher paths that restore or publish a known generation before
// continuing monotonic advancement.
func TestAtomicGenerationAdvanceFromStoredValue(t *testing.T) {
	t.Parallel()

	var generation AtomicGeneration

	generation.Store(Generation(41))

	got := generation.Advance()
	if got != Generation(42) {
		t.Fatalf("Advance from 41 = %d, want 42", got)
	}

	if loaded := generation.Load(); loaded != Generation(42) {
		t.Fatalf("Load after Advance from 41 = %d, want 42", loaded)
	}
}

// TestAtomicGenerationAdvancePanicsOnOverflow verifies that atomic advancement
// preserves the same no-wrap invariant as Generation.Next.
func TestAtomicGenerationAdvancePanicsOnOverflow(t *testing.T) {
	t.Parallel()

	var generation AtomicGeneration

	generation.Store(MaxGeneration)

	testutil.MustPanicWithMessage(t, errGenerationOverflow, func() {
		_ = generation.Advance()
	})
}

// TestAtomicGenerationCompareAndSwap verifies exact expected-value publication.
//
// CompareAndSwap is useful for advanced internal state transitions where a
// publisher must update a generation only if the observed generation still
// matches the expected value.
func TestAtomicGenerationCompareAndSwap(t *testing.T) {
	t.Parallel()

	var generation AtomicGeneration

	generation.Store(Generation(10))

	if swapped := generation.CompareAndSwap(Generation(9), Generation(11)); swapped {
		t.Fatal("CompareAndSwap succeeded with wrong old generation")
	}

	if got := generation.Load(); got != Generation(10) {
		t.Fatalf("Load after failed CompareAndSwap = %d, want 10", got)
	}

	if swapped := generation.CompareAndSwap(Generation(10), Generation(11)); !swapped {
		t.Fatal("CompareAndSwap failed with correct old generation")
	}

	if got := generation.Load(); got != Generation(11) {
		t.Fatalf("Load after successful CompareAndSwap = %d, want 11", got)
	}
}

// TestAtomicGenerationConcurrentAdvance verifies atomic correctness under
// concurrent publishers.
//
// This test does not define a production publication topology. It only proves
// that concurrent Advance calls do not lose increments.
func TestAtomicGenerationConcurrentAdvance(t *testing.T) {
	t.Parallel()

	const goroutines = 16
	const iterations = 4096

	var generation AtomicGeneration
	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				generation.Advance()
			}
		}()
	}

	wg.Wait()

	want := Generation(goroutines * iterations)
	if got := generation.Load(); got != want {
		t.Fatalf("generation after concurrent Advance = %d, want %d", got, want)
	}
}

// TestAtomicGenerationNilReceiverPanics verifies explicit nil-receiver
// diagnostics.
//
// Methods on nil pointer receivers are legal to call in Go. AtomicGeneration
// deliberately checks this case and panics with a stable package-specific
// message instead of allowing a generic nil-pointer panic.
func TestAtomicGenerationNilReceiverPanics(t *testing.T) {
	t.Parallel()

	var generation *AtomicGeneration

	t.Run("Load", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicGeneration, func() {
			_ = generation.Load()
		})
	})

	t.Run("Store", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicGeneration, func() {
			generation.Store(InitialGeneration)
		})
	})

	t.Run("Advance", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicGeneration, func() {
			_ = generation.Advance()
		})
	})

	t.Run("CompareAndSwap", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilAtomicGeneration, func() {
			_ = generation.CompareAndSwap(NoGeneration, InitialGeneration)
		})
	})
}
