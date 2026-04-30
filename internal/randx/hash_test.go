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

import "testing"

func TestHashString64IsDeterministic(t *testing.T) {
	values := []string{"", "alpha", "beta", "partitioned-pool"}
	for _, value := range values {
		first := HashString64(value)
		second := HashString64(value)
		if first != second {
			t.Fatalf("HashString64(%q) not deterministic: %d != %d", value, first, second)
		}
	}
}

func TestHashString64GoldenValues(t *testing.T) {
	tests := []struct {
		value string
		want  uint64
	}{
		{value: "", want: 17665956581633026203},
		{value: "alpha", want: 8596495612706370024},
		{value: "beta", want: 17294041484173018516},
		{value: "partitioned-pool", want: 4648288770799375346},
	}
	for _, tt := range tests {
		got := HashString64(tt.value)
		if got != tt.want {
			t.Fatalf("HashString64(%q) = %d, want %d", tt.value, got, tt.want)
		}
	}
}
