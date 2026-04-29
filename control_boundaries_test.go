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
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	controlBoundaryRootImport = "arcoris.dev/bufferpool"
	testutilImportPath        = "arcoris.dev/bufferpool/internal/testutil"
)

var forbiddenHotPathControlImports = map[string]struct{}{
	"arcoris.dev/bufferpool/internal/control/activity": {},
	"arcoris.dev/bufferpool/internal/control/decision": {},
	"arcoris.dev/bufferpool/internal/control/risk":     {},
	"arcoris.dev/bufferpool/internal/control/score":    {},
}

var hotDataPlaneFiles = []string{
	"bucket.go",
	"bucket_trim.go",
	"pool_get.go",
	"pool_put.go",
	"shard.go",
	"shard_counters.go",
	"shard_selection.go",
}

// TestControlImportBoundaries verifies the static dependency rules that keep
// shared algorithms out of the hot data plane and keep internal/control
// independent from root bufferpool domain types.
func TestControlImportBoundaries(t *testing.T) {
	t.Parallel()

	t.Run("internal control does not import root package", func(t *testing.T) {
		t.Parallel()

		files := goFilesUnder(t, "internal/control")
		for _, file := range files {
			for _, importPath := range importsForFile(t, file) {
				if importPath == controlBoundaryRootImport {
					t.Fatalf("%s imports root package %q", file, controlBoundaryRootImport)
				}
			}
		}
	})

	t.Run("hot data-plane files do not import scoring packages", func(t *testing.T) {
		t.Parallel()

		for _, file := range hotDataPlaneFiles {
			for _, importPath := range importsForFile(t, file) {
				if _, forbidden := forbiddenHotPathControlImports[importPath]; forbidden {
					t.Fatalf("%s imports control-plane package %q", file, importPath)
				}
			}
		}
	})

	t.Run("testutil is imported only from tests", func(t *testing.T) {
		t.Parallel()

		files := goFilesUnder(t, ".")
		for _, file := range files {
			for _, importPath := range importsForFile(t, file) {
				if importPath == testutilImportPath && !strings.HasSuffix(file, "_test.go") {
					t.Fatalf("%s imports %q outside a test file", file, testutilImportPath)
				}
			}
		}
	})
}

func goFilesUnder(t *testing.T, root string) []string {
	t.Helper()

	var files []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if shouldSkipBoundaryDir(path, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, filepath.ToSlash(path))
		}
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return files
}

func shouldSkipBoundaryDir(path, name string) bool {
	if path == "." {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	return name == "vendor"
}

func importsForFile(t *testing.T, file string) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports for %s: %v", file, err)
	}
	imports := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("unquote import path %s in %s: %v", spec.Path.Value, file, err)
		}
		imports = append(imports, importPath)
	}
	return imports
}
