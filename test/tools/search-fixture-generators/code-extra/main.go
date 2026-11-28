// code-extra 生成检索字段/代码补充批次；只创建新文件，拒绝覆盖。
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	fixtureRoot   = "test/fixtures/检索字段/代码补充"
	generatorPath = "test/tools/search-fixture-generators/code-extra/main.go"
	maxFileBytes  = 8 * 1024
	maxBatchBytes = 64 * 1024
	maxFixtures   = 12
)

type fixtureSpec struct {
	path     string
	content  string
	marker   string
	coverage []string
	facts    map[string]any
	purpose  string
}

type fixtureEntry struct {
	Path             string         `json:"path"`
	Bytes            int            `json:"bytes"`
	SHA256           string         `json:"sha256"`
	Marker           string         `json:"marker"`
	ExpectedCoverage []string       `json:"expectedCoverage"`
	ExpectedFacts    map[string]any `json:"expectedFacts"`
	Purpose          string         `json:"purpose"`
}

type batchLimits struct {
	FixtureBytes         int    `json:"fixtureBytes"`
	MaxFixtureBytes      int    `json:"maxFixtureBytes"`
	MaxBatchBytes        int    `json:"maxBatchBytes"`
	FixtureBytesExcludes string `json:"fixtureBytesExcludes"`
}

type fixtureManifest struct {
	SchemaVersion   int            `json:"schemaVersion"`
	Family          string         `json:"family"`
	PathsRelativeTo string         `json:"pathsRelativeTo"`
	Description     string         `json:"description"`
	GeneratedBy     string         `json:"generatedBy"`
	BatchLimits     batchLimits    `json:"batchLimits"`
	Fixtures        []fixtureEntry `json:"fixtures"`
}

var todoPattern = regexp.MustCompile(`(?i)\b(TODO|FIXME)\b`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "generate code-extra fixtures:", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := os.Stat(generatorPath); err != nil {
		return fmt.Errorf("run from the project root so %s is visible: %w", generatorPath, err)
	}
	specs := fixtureSpecs()
	if len(specs) > maxFixtures {
		return fmt.Errorf("%d fixtures exceed limit %d", len(specs), maxFixtures)
	}
	seenPaths := make(map[string]bool, len(specs))
	seenMarkers := make(map[string]bool, len(specs))
	covered := make(map[string]bool)
	entries := make([]fixtureEntry, 0, len(specs))
	files := make([]generatedFile, 0, len(specs)+1)
	fixtureBytes := 0

	for _, spec := range specs {
		if !filepath.IsLocal(spec.path) || filepath.Base(spec.path) != spec.path || strings.ContainsAny(spec.path, `/\`) {
			return fmt.Errorf("path must be a local basename: %q", spec.path)
		}
		if seenPaths[spec.path] {
			return fmt.Errorf("duplicate fixture path %q", spec.path)
		}
		seenPaths[spec.path] = true
		if seenMarkers[spec.marker] {
			return fmt.Errorf("duplicate marker %q", spec.marker)
		}
		seenMarkers[spec.marker] = true
		data := []byte(spec.content)
		if len(data) == 0 || len(data) > maxFileBytes {
			return fmt.Errorf("%s is %d bytes; limit is 1..%d", spec.path, len(data), maxFileBytes)
		}
		if count := bytes.Count(data, []byte(spec.marker)); count != 1 {
			return fmt.Errorf("%s contains marker %q %d times, want exactly once", spec.path, spec.marker, count)
		}
		if got, ok := spec.facts["cod-code-todo-count"].(int); !ok || got != len(todoPattern.FindAll(data, -1)) {
			return fmt.Errorf("%s TODO/FIXME fact disagrees with source", spec.path)
		}
		if len(spec.coverage) == 0 {
			return fmt.Errorf("%s has no expectedCoverage", spec.path)
		}
		coverageSeen := make(map[string]bool, len(spec.coverage))
		for _, field := range spec.coverage {
			if !strings.HasPrefix(field, "cod-") || coverageSeen[field] {
				return fmt.Errorf("%s has invalid or duplicate coverage field %q", spec.path, field)
			}
			coverageSeen[field] = true
			covered[field] = true
		}
		factNames := make([]string, 0, len(spec.facts))
		for field := range spec.facts {
			factNames = append(factNames, field)
		}
		sort.Strings(factNames)
		for _, field := range factNames {
			if !coverageSeen[field] {
				return fmt.Errorf("%s expectedFacts field %q is absent from expectedCoverage", spec.path, field)
			}
		}
		entry := fixtureEntry{
			Path:             spec.path,
			Bytes:            len(data),
			SHA256:           sha256Hex(data),
			Marker:           spec.marker,
			ExpectedCoverage: spec.coverage,
			ExpectedFacts:    spec.facts,
			Purpose:          spec.purpose,
		}
		entries = append(entries, entry)
		files = append(files, generatedFile{path: filepath.Join(fixtureRoot, spec.path), data: data})
		fixtureBytes += len(data)
	}

	manifest := fixtureManifest{
		SchemaVersion:   1,
		Family:          "检索字段/代码",
		PathsRelativeTo: "manifest.json",
		Description:     "独立小型代码检索夹具：生成标记、零函数/零注释、Apache-2.0、TypeScript/Java/SQL/Shell/Vue/CSS/HTML/C++/Rust 扩展名及分析器实际支持的语言相关字段。",
		GeneratedBy:     generatorPath,
		BatchLimits: batchLimits{
			FixtureBytes:         fixtureBytes,
			MaxFixtureBytes:      maxFileBytes,
			MaxBatchBytes:        maxBatchBytes,
			FixtureBytesExcludes: "manifest.json",
		},
		Fixtures: entries,
	}
	var manifestBuf bytes.Buffer
	encoder := json.NewEncoder(&manifestBuf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(manifest); err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	manifestData := manifestBuf.Bytes()
	if len(manifestData) > maxFileBytes {
		return fmt.Errorf("manifest.json is %d bytes; limit is %d", len(manifestData), maxFileBytes)
	}
	totalBatch := fixtureBytes + len(manifestData)
	if totalBatch > maxBatchBytes {
		return fmt.Errorf("batch is %d bytes; limit is %d", totalBatch, maxBatchBytes)
	}
	files = append(files, generatedFile{path: filepath.Join(fixtureRoot, "manifest.json"), data: manifestData})

	for _, file := range files {
		if _, err := os.Lstat(file.path); err == nil {
			return fmt.Errorf("refusing to overwrite existing file: %s", file.path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect %s: %w", file.path, err)
		}
	}
	for _, file := range files {
		if err := writeNew(file.path, file.data); err != nil {
			return err
		}
	}
	for _, file := range files {
		written, err := os.ReadFile(file.path)
		if err != nil {
			return fmt.Errorf("verify read %s: %w", file.path, err)
		}
		if len(written) != len(file.data) || sha256Hex(written) != sha256Hex(file.data) {
			return fmt.Errorf("verify size/SHA-256 failed: %s", file.path)
		}
		fmt.Printf("%s\t%d bytes\t%s\n", filepath.ToSlash(file.path), len(written), sha256Hex(written))
	}
	fmt.Printf("batch: %d fixtures; fixtureBytes=%d; manifestBytes=%d; total=%d bytes\n", len(entries), fixtureBytes, len(manifestData), totalBatch)
	fmt.Printf("covered fields (%d): %s\n", len(covered), strings.Join(sortedKeys(covered), ", "))
	return nil
}

func fixtureSpecs() []fixtureSpec {
	common := func(extra ...string) []string {
		return append([]string{
			"cod-basic-textish",
			"cod-code-lang",
			"cod-code-todo-count",
			"cod-code-generated",
			"cod-code-test-file",
		}, extra...)
	}
	return []fixtureSpec{
		{
			path: "generated.go",
			content: `// Code generated by search-fixture-generator. DO NOT EDIT.
// SEARCH_CODE_EXTRA_GENERATED_2026
// source: generated.proto

package generatedfixture

import "os"

const DefaultMode = "safe"

var GeneratedMode = os.Getenv("SEARCH_CODE_EXTRA_GENERATED_ENV_2026")

func Mode() string {
	return GeneratedMode
}
`,
			marker:   "SEARCH_CODE_EXTRA_GENERATED_2026",
			coverage: common("cod-code-package", "cod-code-imports", "cod-code-func-count", "cod-code-comment-ratio", "cod-code-exported-names", "cod-code-env-vars", "cod-code-declared-symbols", "cod-code-env-vars-truncated", "cod-code-declared-symbols-truncated"),
			facts: map[string]any{
				"cod-basic-textish":                   true,
				"cod-code-lang":                       "go",
				"cod-code-imports":                    []string{"os"},
				"cod-code-todo-count":                 0,
				"cod-code-generated":                  true,
				"cod-code-func-count":                 1,
				"cod-code-comment-ratio":              0.3,
				"cod-code-test-file":                  false,
				"cod-code-exported-names":             []string{"DefaultMode", "GeneratedMode", "Mode"},
				"cod-code-package":                    "generatedfixture",
				"cod-code-env-vars":                   []string{"SEARCH_CODE_EXTRA_GENERATED_ENV_2026"},
				"cod-code-declared-symbols":           []string{"DefaultMode", "GeneratedMode", "Mode"},
				"cod-code-env-vars-truncated":         false,
				"cod-code-declared-symbols-truncated": false,
			},
			purpose: "前 5 行有生成标记的合法 Go；generated=true，覆盖导入、导出与 AST 环境变量。",
		},
		{
			path: "no-functions.go",
			content: `package constantsonly

const (
	First         = 1
	Second        = 2
	FixtureMarker = "SEARCH_CODE_EXTRA_ZERO_2026"
)
`,
			marker:   "SEARCH_CODE_EXTRA_ZERO_2026",
			coverage: common("cod-code-package", "cod-code-func-count", "cod-code-comment-ratio", "cod-code-declared-symbols", "cod-code-declared-symbols-truncated"),
			facts: map[string]any{
				"cod-basic-textish":                   true,
				"cod-code-lang":                       "go",
				"cod-code-todo-count":                 0,
				"cod-code-generated":                  false,
				"cod-code-func-count":                 0,
				"cod-code-comment-ratio":              0,
				"cod-code-test-file":                  false,
				"cod-code-package":                    "constantsonly",
				"cod-code-declared-symbols":           []string{"First", "Second", "FixtureMarker"},
				"cod-code-declared-symbols-truncated": false,
			},
			purpose: "合法 Go 仅 package/常量；marker 存于常量以保持零注释，函数数与注释率均为 0；无环境变量事实。",
		},
		{
			path: "apache-licensed.ts",
			content: `// SPDX-License-Identifier: Apache-2.0
// SEARCH_CODE_EXTRA_APACHE_2026
import { join } from "node:path";

// TODO: validate the Apache fixture.
export function apacheLabel(name: string): string {
  return join("fixtures", name);
}
`,
			marker:   "SEARCH_CODE_EXTRA_APACHE_2026",
			coverage: common("cod-code-license", "cod-code-imports", "cod-code-func-count", "cod-code-comment-ratio"),
			facts: map[string]any{
				"cod-basic-textish":      true,
				"cod-code-lang":          "typescript",
				"cod-code-license":       "Apache-2.0",
				"cod-code-imports":       []string{"node:path"},
				"cod-code-todo-count":    1,
				"cod-code-generated":     false,
				"cod-code-func-count":    1,
				"cod-code-comment-ratio": 0.43,
				"cod-code-test-file":     false,
			},
			purpose: "TypeScript 非 MIT 正例：SPDX 识别为 Apache-2.0，覆盖导入、函数、TODO 与注释。",
		},
		{
			path:     "todo.java",
			content:  "// SEARCH_CODE_EXTRA_JAVA_2026\n// TODO: finish the Java fixture.\npublic class Fixture {\n    public int size() {\n        return 1;\n    }\n}\n",
			marker:   "SEARCH_CODE_EXTRA_JAVA_2026",
			purpose:  "Java 扩展名、C 系函数计数、TODO 与 // 注释；不声明不支持的 import。",
			coverage: common("cod-code-func-count", "cod-code-comment-ratio"),
			facts: map[string]any{
				"cod-basic-textish":      true,
				"cod-code-lang":          "java",
				"cod-code-todo-count":    1,
				"cod-code-generated":     false,
				"cod-code-func-count":    1,
				"cod-code-comment-ratio": 0.29,
				"cod-code-test-file":     false,
			},
		},
		{
			path:     "todo.sql",
			content:  "-- SEARCH_CODE_EXTRA_SQL_2026\n-- TODO: add an index after review.\nSELECT id, name\nFROM search_fixture\nWHERE id = 1;\n",
			marker:   "SEARCH_CODE_EXTRA_SQL_2026",
			purpose:  "SQL 扩展名、TODO 与 -- 注释率；不声明不支持的 import/function。",
			coverage: common("cod-code-comment-ratio"),
			facts: map[string]any{
				"cod-basic-textish":      true,
				"cod-code-lang":          "sql",
				"cod-code-todo-count":    1,
				"cod-code-generated":     false,
				"cod-code-comment-ratio": 0.4,
				"cod-code-test-file":     false,
			},
		},
		{
			path:     "todo.sh",
			content:  "#!/bin/sh\n# SEARCH_CODE_EXTRA_SHELL_2026\n# TODO: quote the caller-supplied value.\nshow_value() {\n  printf '%s\\n' \"$1\"\n}\nshow_value \"fixture\"\n",
			marker:   "SEARCH_CODE_EXTRA_SHELL_2026",
			purpose:  "Shell 扩展名、shebang、TODO 与 # 注释率；不声明不支持的 import/function。",
			coverage: common("cod-code-comment-ratio"),
			facts: map[string]any{
				"cod-basic-textish":      true,
				"cod-code-lang":          "shell",
				"cod-code-todo-count":    1,
				"cod-code-generated":     false,
				"cod-code-comment-ratio": 0.43,
				"cod-code-test-file":     false,
			},
		},
		{
			path: "todo.vue",
			content: `<template>
  <!-- SEARCH_CODE_EXTRA_VUE_2026 -->
  <!-- TODO: add an accessible label. -->
  <button>{{ label }}</button>
</template>

<script lang="ts">
import { defineComponent } from "vue";

export default defineComponent({
  data() {
    return { label: "Fixture" };
  }
});
</script>
`,
			marker:   "SEARCH_CODE_EXTRA_VUE_2026",
			purpose:  "Vue 扩展名、from 风格导入与 TODO；HTML 注释不计入 // 注释率。",
			coverage: common("cod-code-imports", "cod-code-comment-ratio"),
			facts: map[string]any{
				"cod-basic-textish":      true,
				"cod-code-lang":          "vue",
				"cod-code-imports":       []string{"vue"},
				"cod-code-todo-count":    1,
				"cod-code-generated":     false,
				"cod-code-comment-ratio": 0,
				"cod-code-test-file":     false,
			},
		},
		{
			path:     "todo.css",
			content:  "/* SEARCH_CODE_EXTRA_CSS_2026 */\n/* TODO: replace the temporary color. */\n.fixture {\n  color: #336699;\n}\n",
			marker:   "SEARCH_CODE_EXTRA_CSS_2026",
			purpose:  "CSS 扩展名、TODO 与块注释；含 /* 的非空行计入注释率。",
			coverage: common("cod-code-comment-ratio"),
			facts: map[string]any{
				"cod-basic-textish":      true,
				"cod-code-lang":          "css",
				"cod-code-todo-count":    1,
				"cod-code-generated":     false,
				"cod-code-comment-ratio": 0.4,
				"cod-code-test-file":     false,
			},
		},
		{
			path:     "todo.html",
			content:  "<!doctype html>\n<!-- SEARCH_CODE_EXTRA_HTML_2026 -->\n<!-- TODO: add a document language. -->\n<title>Fixture</title>\n<p>HTML comment ratio is unsupported.</p>\n",
			marker:   "SEARCH_CODE_EXTRA_HTML_2026",
			purpose:  "HTML 扩展名与 TODO；实现不支持注释率/函数数，故不声明这两项。",
			coverage: common(),
			facts: map[string]any{
				"cod-basic-textish":   true,
				"cod-code-lang":       "html",
				"cod-code-todo-count": 1,
				"cod-code-generated":  false,
				"cod-code-test-file":  false,
			},
		},
		{
			path: "todo.cpp",
			content: `// SEARCH_CODE_EXTRA_CPP_2026
// TODO: finish the C++ fixture.
#include <string>

int fixture_size(const std::string& value) {
    return static_cast<int>(value.size());
}
`,
			marker:   "SEARCH_CODE_EXTRA_CPP_2026",
			purpose:  "C++ 扩展名、include、C 系函数计数、TODO 与 // 注释。",
			coverage: common("cod-code-imports", "cod-code-func-count", "cod-code-comment-ratio"),
			facts: map[string]any{
				"cod-basic-textish":      true,
				"cod-code-lang":          "cpp",
				"cod-code-imports":       []string{"string"},
				"cod-code-todo-count":    1,
				"cod-code-generated":     false,
				"cod-code-func-count":    1,
				"cod-code-comment-ratio": 0.33,
				"cod-code-test-file":     false,
			},
		},
		{
			path: "todo.rs",
			content: `// SEARCH_CODE_EXTRA_RUST_2026
// TODO: finish the Rust fixture.
use std::collections::HashMap;

fn fixture_size(values: &HashMap<String, String>) -> usize {
    values.len()
}
`,
			marker:   "SEARCH_CODE_EXTRA_RUST_2026",
			purpose:  "Rust 扩展名、fn 计数、TODO 与 // 注释；不误报 use 为 import。",
			coverage: common("cod-code-func-count", "cod-code-comment-ratio"),
			facts: map[string]any{
				"cod-basic-textish":      true,
				"cod-code-lang":          "rust",
				"cod-code-todo-count":    1,
				"cod-code-generated":     false,
				"cod-code-func-count":    1,
				"cod-code-comment-ratio": 0.33,
				"cod-code-test-file":     false,
			},
		},
	}
}

type generatedFile struct {
	path string
	data []byte
}

func writeNew(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	n, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write %s: %w", path, writeErr)
	}
	if n != len(data) {
		return fmt.Errorf("write %s: wrote %d of %d bytes", path, n, len(data))
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
	}
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
