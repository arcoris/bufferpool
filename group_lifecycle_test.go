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

import "testing"

func TestPoolGroupClose(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha", "beta"))
	requireGroupNoError(t, err)

	requireGroupNoError(t, group.Close())
	if !group.IsClosed() {
		t.Fatalf("group should be closed")
	}
	requireGroupNoError(t, group.Close())
}

func TestPoolGroupTickAfterCloseRejected(t *testing.T) {
	group, err := NewPoolGroup(testGroupConfig("alpha"))
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.Close())

	_, err = group.Tick()
	requireGroupErrorIs(t, err, ErrClosed)
}
