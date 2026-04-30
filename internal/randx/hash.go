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

package randx

const (
	hashString64Offset = uint64(14695981039346656037)
	hashString64Prime  = uint64(1099511628211)
)

// HashString64 returns a deterministic non-security hash for placement keys.
//
// The function uses FNV-1a over the input bytes and applies MixUint64 to spread
// the result before bounded index selection. It is stable, allocation-free, and
// suitable for internal placement decisions. It is not a cryptographic hash and
// must not be used for secrets or adversarial hashing.
func HashString64(value string) uint64 {
	hash := hashString64Offset
	for index := 0; index < len(value); index++ {
		hash ^= uint64(value[index])
		hash *= hashString64Prime
	}
	return MixUint64(hash)
}
