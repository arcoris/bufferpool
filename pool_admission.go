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

// zeroDroppedBuffer clears a returned buffer when ZeroDroppedBuffers is enabled.
//
// This is used only for no-retain outcomes after Pool has decided that the
// returned buffer will not become visible through a bucket and Pool has accepted
// responsibility for handling that return. Close reject mode is different: it
// returns ErrClosed and leaves the caller responsible for the buffer, so this
// helper is not called. Retained-buffer zeroing is intentionally separate and
// runs inside the shard retain path before bucket publication, where shard.mu
// still prevents concurrent Get from popping the buffer.
func zeroDroppedBuffer(buffer []byte, policy Policy) {
	if !policy.Admission.ZeroDroppedBuffers {
		return
	}

	zeroBufferCapacity(buffer)
}

// zeroBufferCapacity clears the full capacity range of buffer.
//
// The caller must pass a non-nil positive-capacity buffer. Public Put validation
// enforces that for Pool return paths, and class/shard retain paths call this
// only after class admission has accepted the buffer.
func zeroBufferCapacity(buffer []byte) {
	clear(buffer[:cap(buffer)])
}
