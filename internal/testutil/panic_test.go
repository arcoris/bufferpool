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

package testutil

import (
	"errors"
	"testing"
)

func TestMustPanicAcceptsAnyPanicValue(t *testing.T) {
	t.Parallel()

	MustPanic(t, func() {
		panic(struct{ reason string }{reason: "boom"})
	})
}

func TestMustPanicWithMessageAcceptsStringPanic(t *testing.T) {
	t.Parallel()

	MustPanicWithMessage(t, "boom", func() {
		panic("boom")
	})
}

func TestMustPanicWithMessageAcceptsErrorPanic(t *testing.T) {
	t.Parallel()

	MustPanicWithMessage(t, "boom", func() {
		panic(errors.New("boom"))
	})
}
