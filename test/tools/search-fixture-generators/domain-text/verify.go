package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"mime"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/all"
	"github.com/Reisentyann/Mabel-s-Tentacles/indexer-go"
)

type spec struct {
	Root           string               `json:"root"`
	Limits         limits               `json:"limits"`
	MarkerPattern  string               `json:"marker_pattern"`
	Domains        []domain             `json:"domains"`
	CommonCoverage []string             `json:"common_coverage"`
	Archetypes     map[string]archetype `json:"archetypes"`
	HardRules      []string             `json:"hard_rules"`
}

type limits struct {
	MinFileBytes   int    `json:"min_file_bytes"`
	MaxFileBytes   int    `json:"max_file_bytes"`
	MaxDomainBytes int    `json:"max_domain_bytes"`
	MaxTotalBytes  int    `json:"max_total_bytes"`
	Encoding       string `json:"encoding"`
	AllowBOM       bool   `json:"allow_bom"`
}

type domain struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Special string   `json:"special"`
	Topics  []string `json:"topics"`
	Terms   []string `json:"terms"`
}

type archetype struct {
	Kind                 string             `json:"kind"`
	Requirements         []string           `json:"requirements"`
	ExtraCoverage        []string           `json:"extra_coverage"`
	Expect               map[string]any     `json:"expect"`
	ExpectedAbsentFields []string           `json:"expected_absent_fields"`
	Variants             map[string]variant `json:"variants"`
}

type variant struct {
	Filename             string         `json:"filename"`
	Requirements         []string       `json:"requirements"`
	ExtraCoverage        []string       `json:"extra_coverage"`
	Expect               map[string]any `json:"expect"`
	ExpectedAbsentFields []string       `json:"expected_absent_fields"`
}

type manifest struct {
	SchemaVersion int     `json:"schema_version"`
	DomainID      string  `json:"domain_id"`
	DomainName    string  `json:"domain_name"`
	PathBase      string  `json:"path_base"`
	GeneratedBy   string  `json:"generated_by"`
	Files         []entry `json:"files"`
}

type entry struct {
	Path                 string         `json:"path"`
	Archetype            string         `json:"archetype"`
	Bytes                int            `json:"bytes"`
	SHA256               string         `json:"sha256"`
	Marker               string         `json:"marker"`
	Purpose              string         `json:"purpose"`
	ExpectedCoverage     []string       `json:"expected_coverage"`
	Expect               map[string]any `json:"expect"`
	ExpectedAbsentFields []string       `json:"expected_absent_fields"`
	Notes                string         `json:"notes,omitempty"`
}

type fileResult struct {
	Path          string         `json:"path"`
	Domain        string         `json:"domain"`
	Bytes         int64          `json:"bytes"`
	Fields        map[string]any `json:"fields"`
	QueryFailures []string       `json:"query_failures,omitempty"`
}

type stats struct {
	Files            int   `json:"files"`
	Fields           int   `json:"fields"`
	ManifestChecks   int   `json:"manifest_checks"`
	ManifestFailures int   `json:"manifest_failures"`
	Queries          int   `json:"queries"`
	QueryFailures    int   `json:"query_failures"`
	MissingChecks    int   `json:"missing_checks"`
	TotalBytes       int64 `json:"total_bytes"`
	MaxFileBytes     int64 `json:"max_file_bytes"`
}

type report struct {
	Root       string              `json:"root"`
	Spec       string              `json:"spec"`
	Files      []fileResult        `json:"files"`
	FieldPaths map[string][]string `json:"field_paths"`
	Stats      stats               `json:"stats"`
	Failures   []string            `json:"failures"`
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: verify <fixture-root> <spec.json> <report.json>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2], os.Args[3]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, specPath, output string) error {
	specData, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	var s spec
	if err := json.Unmarshal(specData, &s); err != nil {
		return err
	}
	idx := indexer.New()
	rep := report{Root: root, Spec: specPath, FieldPaths: map[string][]string{}, Failures: []string{}}
	check := func(ok bool, format string, args ...any) {
		rep.Stats.ManifestChecks++
		if !ok {
			rep.Stats.ManifestFailures++
			rep.Failures = append(rep.Failures, fmt.Sprintf(format, args...))
		}
	}

	for _, d := range s.Domains {
		dir := filepath.Join(root, d.ID)
		items, err := os.ReadDir(dir)
		if err != nil {
			check(false, "%s: read domain directory: %v", d.ID, err)
			continue
		}
		manifestPath := filepath.Join(dir, "manifest.json")
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			check(false, "%s: read manifest: %v", d.ID, err)
			continue
		}
		var m manifest
		if err := json.Unmarshal(manifestData, &m); err != nil {
			check(false, "%s: parse manifest: %v", d.ID, err)
			continue
		}
		check(m.SchemaVersion == 1, "%s: schema_version", d.ID)
		check(m.DomainID == d.ID, "%s: domain_id actual=%q", d.ID, m.DomainID)
		check(m.DomainName == d.Name, "%s: domain_name actual=%q expected=%q", d.ID, m.DomainName, d.Name)
		check(m.PathBase == ".", "%s: path_base actual=%q", d.ID, m.PathBase)
		check(strings.HasPrefix(m.GeneratedBy, "subagent:"), "%s: generated_by actual=%q", d.ID, m.GeneratedBy)
		expectedNames := expectedFilenames(d, s.Archetypes)
		check(len(m.Files) == len(expectedNames), "%s: manifest file count=%d expected=%d", d.ID, len(m.Files), len(expectedNames))
		seen := map[string]bool{}
		domainBytes := int64(0)
		for _, e := range m.Files {
			id := d.ID + "/" + e.Path
			check(!seen[e.Path], "%s: duplicate path %s", d.ID, e.Path)
			seen[e.Path] = true
			_, expected := expectedNames[e.Path]
			check(expected, "%s: unexpected path %s", d.ID, e.Path)
			a, ok := archetypeFor(d, e.Path, s.Archetypes)
			if !ok {
				check(false, "%s: no archetype for %s", d.ID, e.Path)
				continue
			}
			check(e.Archetype == a.Kind, "%s: archetype actual=%q expected=%q", id, e.Archetype, a.Kind)
			check(strings.TrimSpace(e.Purpose) != "", "%s: empty purpose", id)
			data, err := os.ReadFile(filepath.Join(dir, e.Path))
			if err != nil {
				check(false, "%s: read file: %v", id, err)
				continue
			}
			size := int64(len(data))
			domainBytes += size
			rep.Stats.TotalBytes += size
			if size > rep.Stats.MaxFileBytes {
				rep.Stats.MaxFileBytes = size
			}
			check(size >= int64(s.Limits.MinFileBytes), "%s: size %d below minimum", id, size)
			check(size <= int64(s.Limits.MaxFileBytes), "%s: size %d above maximum", id, size)
			check(int64(e.Bytes) == size, "%s: manifest bytes=%d actual=%d", id, e.Bytes, size)
			sum := sha256.Sum256(data)
			digest := hex.EncodeToString(sum[:])
			check(e.SHA256 == digest, "%s: sha256 actual=%s expected=%s", id, digest, e.SHA256)
			check(utf8.Valid(data), "%s: invalid UTF-8", id)
			check(!bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}), "%s: unexpected UTF-8 BOM", id)
			check(e.Marker != "" && bytes.Contains(data, []byte(e.Marker)), "%s: marker %q missing", id, e.Marker)
			check(strings.HasPrefix(e.Marker, "SEARCH_DOMAIN_"+strings.ToUpper(d.ID)+"_"), "%s: marker namespace %q", id, e.Marker)

			for _, field := range e.ExpectedCoverage {
				check(strings.HasPrefix(field, "cod-"), "%s: non-cod coverage field %q", id, field)
			}
			for field := range a.Expect {
				_, ok := e.Expect[field]
				check(ok, "%s: missing required expect %s", id, field)
			}
			for _, field := range a.ExpectedAbsentFields {
				check(contains(e.ExpectedAbsentFields, field), "%s: missing required absent assertion %s", id, field)
			}

			info, err := os.Stat(filepath.Join(dir, e.Path))
			if err != nil {
				check(false, "%s: stat: %v", id, err)
				continue
			}
			head := data
			if len(head) > 512 {
				head = head[:512]
			}
			results := describer.Analyze(describer.Input{
				Path: id, Head: head, Full: data, Size: size, MTime: info.ModTime(),
				ExtMime: mime.TypeByExtension(filepath.Ext(e.Path)),
			}, nil)
			attrs := describer.MergeResults(nil, results, time.Unix(0, 0))
			idx.Update(id, nil, attrs)
			rep.Files = append(rep.Files, fileResult{Path: id, Domain: d.ID, Bytes: size, Fields: attrs})
			rep.Stats.Files++
			for field := range attrs {
				rep.FieldPaths[field] = append(rep.FieldPaths[field], id)
			}
			for _, field := range e.ExpectedCoverage {
				_, ok := attrs[field]
				check(ok, "%s: expected field %s absent", id, field)
			}
			for field, expected := range e.Expect {
				actual, ok := attrs[field]
				check(ok && equalValue(actual, expected), "%s: %s actual=%s expected=%s", id, field, jsonValue(actual), jsonValue(expected))
			}
			for _, field := range e.ExpectedAbsentFields {
				_, ok := attrs[field]
				check(!ok, "%s: expected absent field %s present", id, field)
			}
		}
		for name := range expectedNames {
			check(seen[name], "%s: missing file %s", d.ID, name)
		}
		check(domainBytes <= int64(s.Limits.MaxDomainBytes), "%s: domain bytes=%d above maximum", d.ID, domainBytes)
		for _, item := range items {
			if item.IsDir() {
				check(false, "%s: unexpected subdirectory %s", d.ID, item.Name())
				continue
			}
			if item.Name() == "manifest.json" {
				continue
			}
			check(seen[item.Name()], "%s: unexpected extra file %s", d.ID, item.Name())
		}
	}
	check(rep.Stats.TotalBytes <= int64(s.Limits.MaxTotalBytes), "total bytes=%d above maximum", rep.Stats.TotalBytes)

	sort.Slice(rep.Files, func(i, j int) bool { return rep.Files[i].Path < rep.Files[j].Path })
	for field := range rep.FieldPaths {
		sort.Strings(rep.FieldPaths[field])
	}
	rep.Stats.Fields = len(rep.FieldPaths)
	for i := range rep.Files {
		for field, value := range rep.Files[i].Fields {
			for _, failure := range selfQueries(idx, rep.Files[i].Path, field, value, &rep.Stats.Queries) {
				rep.Files[i].QueryFailures = append(rep.Files[i].QueryFailures, failure)
				rep.Stats.QueryFailures++
				rep.Failures = append(rep.Failures, rep.Files[i].Path+": "+failure)
			}
		}
	}
	for _, file := range rep.Files {
		for field := range rep.FieldPaths {
			rep.Stats.Queries++
			got, err := idx.Query(indexer.Leaf(field, indexer.OpExists, false))
			present := hasField(rep.Files, file.Path, field)
			found := contains(got, file.Path)
			if err != nil || found == present {
				rep.Stats.MissingChecks++
				rep.Stats.QueryFailures++
				rep.Failures = append(rep.Failures, fmt.Sprintf("%s: exists=false %s", file.Path, field))
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(output, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("files=%d fields=%d manifest_checks=%d manifest_failures=%d queries=%d missing_checks=%d query_failures=%d bytes=%d max_file=%d report=%s\n",
		rep.Stats.Files, rep.Stats.Fields, rep.Stats.ManifestChecks, rep.Stats.ManifestFailures,
		rep.Stats.Queries, rep.Stats.MissingChecks, rep.Stats.QueryFailures,
		rep.Stats.TotalBytes, rep.Stats.MaxFileBytes, output)
	if rep.Stats.ManifestFailures+rep.Stats.QueryFailures > 0 {
		return fmt.Errorf("verification failed: %d manifest failures, %d query failures", rep.Stats.ManifestFailures, rep.Stats.QueryFailures)
	}
	return nil
}

func expectedFilenames(d domain, archetypes map[string]archetype) map[string]bool {
	out := map[string]bool{}
	for name := range archetypes {
		if name == "08-special.txt" {
			name = archetypes[name].Variants[d.Special].Filename
		}
		out[name] = true
	}
	return out
}

func archetypeFor(d domain, filename string, archetypes map[string]archetype) (archetype, bool) {
	if filename == archetypes["08-special.txt"].Variants[d.Special].Filename {
		a := archetypes["08-special.txt"]
		v := a.Variants[d.Special]
		a.ExtraCoverage = v.ExtraCoverage
		a.Expect = v.Expect
		a.ExpectedAbsentFields = v.ExpectedAbsentFields
		return a, true
	}
	a, ok := archetypes[filename]
	return a, ok
}

func equalValue(actual, expected any) bool {
	if af, ok := number(actual); ok {
		if ef, ok := number(expected); ok {
			return math.Abs(af-ef) < 1e-9
		}
	}
	switch e := expected.(type) {
	case []any:
		v := reflect.ValueOf(actual)
		if !v.IsValid() || (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) || v.Len() != len(e) {
			return false
		}
		for i := range e {
			if !equalValue(v.Index(i).Interface(), e[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		v := reflect.ValueOf(actual)
		if !v.IsValid() || v.Kind() != reflect.Map || v.Len() != len(e) {
			return false
		}
		for key, value := range e {
			mv := v.MapIndex(reflect.ValueOf(key))
			if !mv.IsValid() || !equalValue(mv.Interface(), value) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(actual, expected)
	}
}

func jsonValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func number(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func selfQueries(idx indexer.Indexer, id, field string, value any, count *int) []string {
	var failures []string
	check := func(op indexer.Op, queryValue any) {
		*count = *count + 1
		got, err := idx.Query(indexer.Leaf(field, op, queryValue))
		if err != nil || !contains(got, id) {
			failures = append(failures, fmt.Sprintf("%s %s", op, jsonValue(queryValue)))
		}
	}
	check(indexer.OpExists, true)
	switch v := value.(type) {
	case string:
		if v != "" {
			check(indexer.OpEq, v)
			check(indexer.OpContains, v)
		}
	case bool:
		check(indexer.OpEq, v)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		n, ok := number(value)
		if ok {
			check(indexer.OpEq, value)
			check(indexer.OpGt, n-1)
			check(indexer.OpLt, n+1)
			check(indexer.OpRange, [2]any{value, value})
		}
	case []string:
		for _, item := range v {
			check(indexer.OpEq, item)
			check(indexer.OpIn, []any{item})
			if item != "" {
				check(indexer.OpContains, item)
			}
		}
	case []any:
		for _, item := range v {
			if _, ok := item.(string); !ok {
				continue
			}
			check(indexer.OpEq, item)
			check(indexer.OpIn, []any{item})
		}
	}
	return failures
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasField(files []fileResult, id, field string) bool {
	for _, file := range files {
		if file.Path == id {
			_, ok := file.Fields[field]
			return ok
		}
	}
	return false
}
