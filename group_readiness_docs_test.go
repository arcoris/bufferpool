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
	"os"
	"strings"
	"testing"
)

const groupReadinessDocumentPath = "docs/design/group-readiness.md"

func TestGroupReadinessDocumentExistsAndIsLinked(t *testing.T) {
	t.Parallel()

	document := readTextForTest(t, groupReadinessDocumentPath)
	if strings.TrimSpace(document) == "" {
		t.Fatalf("%s is empty", groupReadinessDocumentPath)
	}
	index := readTextForTest(t, "docs/design/index.md")
	if !strings.Contains(index, "group-readiness.md") {
		t.Fatalf("docs/design/index.md does not link %s", groupReadinessDocumentPath)
	}
}

func TestGroupReadinessDocumentStatesGoNoGoBoundary(t *testing.T) {
	t.Parallel()

	document := readTextForTest(t, groupReadinessDocumentPath)
	required := []string{
		"GO for observational PoolGroup only",
		"NO-GO",
		"automatic policy mutation",
		"background",
		"physical trim",
		"Pool hot-path",
		"group.go",
		"group_config.go",
		"group_policy.go",
		"group_lifecycle.go",
		"group_registry.go",
		"group_sample.go",
		"group_window.go",
		"group_rate.go",
		"group_score.go",
		"group_score_evaluator.go",
		"group_snapshot.go",
		"group_metrics.go",
		"group_coordinator.go",
		"group_controller_report.go",
	}
	for _, text := range required {
		if !strings.Contains(document, text) {
			t.Fatalf("%s does not contain %q", groupReadinessDocumentPath, text)
		}
	}
}

func readTextForTest(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
