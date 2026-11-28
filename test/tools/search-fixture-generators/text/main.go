package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	maxFileBytes  = 16 * 1024
	maxBatchBytes = 256 * 1024
	fixtureDir    = "test/fixtures/检索字段/文本"
	generatorPath = "test/tools/search-fixture-generators/text/main.go"
)

type fileSpec struct {
	path     string
	encoding string
	marker   string
	coverage []string
	purpose  string
	expect   map[string]any
	omitted  []string
}

type fileEntry struct {
	Path             string         `json:"path"`
	Bytes            int            `json:"bytes"`
	SHA256           string         `json:"sha256"`
	Encoding         string         `json:"encoding"`
	Marker           string         `json:"marker,omitempty"`
	ExpectedCoverage []string       `json:"expected_coverage"`
	Purpose          string         `json:"purpose"`
	Expect           map[string]any `json:"expect,omitempty"`
	NotCovered       []string       `json:"not_covered,omitempty"`
}

type manifestLimits struct {
	MaxFileBytes          int  `json:"max_file_bytes"`
	MaxBatchBytes         int  `json:"max_batch_bytes"`
	BatchIncludesManifest bool `json:"batch_includes_manifest"`
}

type manifest struct {
	SchemaVersion int            `json:"schema_version"`
	Family        string         `json:"family"`
	GeneratedBy   string         `json:"generated_by"`
	PathBase      string         `json:"path_base"`
	Limits        manifestLimits `json:"limits"`
	Notes         []string       `json:"notes"`
	NotCovered    []string       `json:"not_covered"`
	Files         []fileEntry    `json:"files"`
}

type generatedFile struct {
	name string
	data []byte
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "search fixture generator:", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := os.Stat(generatorPath); err != nil {
		return fmt.Errorf("run from the project root so %s is visible: %w", generatorPath, err)
	}

	gbkData, err := encodeGBK("SEARCH_GBK 你好，世界！文本编码。\n")
	if err != nil {
		return err
	}
	latinData, err := encodeLatin1("SEARCH_LATIN1 Noël à Paris, déjà vu, café, façade; Grüße aus Köln. ÿ\n")
	if err != nil {
		return err
	}
	utf8BOM := append([]byte{0xEF, 0xBB, 0xBF}, []byte("SEARCH_UTF8_BOM 文本编码：UTF-8 BOM，你好。\n")...)
	generated := []generatedFile{
		{name: "utf8-bom.txt", data: utf8BOM},
		{name: "utf16le.txt", data: encodeUTF16("SEARCH_UTF16LE 文本编码：小端 UTF-16LE，你好。\n", true, true)},
		{name: "utf16be.txt", data: encodeUTF16("SEARCH_UTF16BE 文本编码：大端 UTF-16BE，你好。\n", false, true)},
		{name: "gbk.txt", data: gbkData},
		{name: "latin1.txt", data: latinData},
		{name: "no-bom-utf16le.txt", data: encodeUTF16("SEARCH_NO_BOM_UTF16LE 无 BOM 的 UTF-16LE 负例。\n", true, false)},
		{name: "nul-sample.bin", data: []byte{0x00, 0x01, 0x02, 0x00, 0xFF, 0x10}},
	}

	directNames := []string{
		"search-markdown.md", "search-log.txt", "search-chat.txt", "minified.json",
		"regular.csv", "broken.csv", "empty.txt", "crlf.txt",
	}
	allData := make(map[string][]byte, len(directNames)+len(generated))
	for _, name := range directNames {
		data, err := os.ReadFile(filepath.Join(fixtureDir, name))
		if err != nil {
			return fmt.Errorf("read direct UTF-8 fixture %s: %w", name, err)
		}
		allData[name] = data
	}
	for _, item := range generated {
		path := filepath.Join(fixtureDir, item.name)
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing to overwrite existing %s", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
		allData[item.name] = item.data
	}
	manifestPath := filepath.Join(fixtureDir, "manifest.json")
	if _, err := os.Lstat(manifestPath); err == nil {
		return fmt.Errorf("refusing to overwrite existing %s", manifestPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect %s: %w", manifestPath, err)
	}

	if err := validateDirectFixtures(allData); err != nil {
		return err
	}
	totalFixtures := 0
	for name, data := range allData {
		if len(data) > maxFileBytes {
			return fmt.Errorf("%s is %d bytes; limit is %d", name, len(data), maxFileBytes)
		}
		totalFixtures += len(data)
	}

	specs := fixtureSpecs()
	entries := make([]fileEntry, 0, len(specs))
	for _, spec := range specs {
		data, ok := allData[spec.path]
		if !ok {
			return fmt.Errorf("missing fixture data for %s", spec.path)
		}
		sum := sha256.Sum256(data)
		entries = append(entries, fileEntry{
			Path:             spec.path,
			Bytes:            len(data),
			SHA256:           hex.EncodeToString(sum[:]),
			Encoding:         spec.encoding,
			Marker:           spec.marker,
			ExpectedCoverage: spec.coverage,
			Purpose:          spec.purpose,
			Expect:           spec.expect,
			NotCovered:       spec.omitted,
		})
	}

	m := manifest{
		SchemaVersion: 1,
		Family:        "text",
		GeneratedBy:   generatorPath,
		PathBase:      fixtureDir,
		Limits: manifestLimits{
			MaxFileBytes:          maxFileBytes,
			MaxBatchBytes:         maxBatchBytes,
			BatchIncludesManifest: true,
		},
		Notes: []string{
			"path 均相对于 path_base；bytes 是物理字节数；sha256 覆盖原始文件字节。",
			"expected_coverage 是预期可检索字段；expect 仅列关键确定性值。",
			"text 族按 textish 而非扩展名路由；JSON/CSV 没有专用提取器，只验证通用 cod-text-* 字段。",
			"empty.txt 的 textish=true，但 0 字节文件在 text Analyze 入口返回 nil，不产 cod-text-*。",
			"批次上限包含 manifest.json；manifest 不记录自身哈希。",
		},
		NotCovered: []string{
			"JSON/CSV 专用字段：当前 text 族不存在此类提取器。",
			"cod-text-shebang：本批没有 shebang 样本。",
			"cod-text-structure 的 frontmatter 值：本批没有 YAML frontmatter。",
			"cod-text-*-truncated=true：本批没有超过 30 项或 200 rune 的内容列表。",
			"cod-text-partial=true：本批没有模拟截断读取。",
			"cod-text-eol=mixed：本批只有 LF、CRLF 和 none。",
		},
		Files: entries,
	}
	var manifestBuf bytes.Buffer
	encoder := json.NewEncoder(&manifestBuf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(m); err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	manifestData := manifestBuf.Bytes()
	if len(manifestData) > maxFileBytes {
		return fmt.Errorf("manifest is %d bytes; limit is %d", len(manifestData), maxFileBytes)
	}
	totalBatch := totalFixtures + len(manifestData)
	if totalBatch > maxBatchBytes {
		return fmt.Errorf("batch is %d bytes; limit is %d", totalBatch, maxBatchBytes)
	}

	for _, item := range generated {
		if err := writeNew(filepath.Join(fixtureDir, item.name), item.data); err != nil {
			return err
		}
	}
	if err := writeNew(manifestPath, manifestData); err != nil {
		return err
	}
	fmt.Printf("wrote %d fixtures and manifest.json; total=%d bytes\n", len(entries), totalBatch)
	for _, entry := range entries {
		fmt.Printf("%s\t%d\t%s\n", entry.Path, entry.Bytes, entry.SHA256)
	}
	fmt.Printf("manifest.json\t%d\t%s\n", len(manifestData), sha256Hex(manifestData))
	return nil
}

func validateDirectFixtures(data map[string][]byte) error {
	markers := []struct{ path, marker string }{
		{"search-markdown.md", "SEARCH_MD_2026"},
		{"search-log.txt", "SEARCH_LOG_2026"},
		{"search-chat.txt", "SEARCH_CHAT_2026"},
		{"minified.json", "SEARCH_JSON_2026"},
	}
	for _, item := range markers {
		if !bytes.Contains(data[item.path], []byte(item.marker)) {
			return fmt.Errorf("%s is missing marker %s", item.path, item.marker)
		}
	}
	mini := data["minified.json"]
	if !json.Valid(mini) || bytes.ContainsRune(mini, '\n') || utf8.RuneCount(mini) <= 500 {
		return fmt.Errorf("minified.json must be valid single-line JSON without a final newline and longer than 500 runes")
	}
	if len(data["empty.txt"]) != 0 {
		return fmt.Errorf("empty.txt must be zero bytes")
	}
	crlf := data["crlf.txt"]
	if bytes.Count(crlf, []byte("\r\n")) == 0 || bytes.Count(crlf, []byte("\r\n")) != bytes.Count(crlf, []byte("\n")) {
		return fmt.Errorf("crlf.txt must contain CRLF and no bare LF")
	}
	return nil
}

func fixtureSpecs() []fileSpec {
	common := func(extra ...string) []string {
		fields := []string{
			"cod-basic-textish", "cod-text-encoding", "cod-text-lines", "cod-text-chars",
			"cod-text-language", "cod-text-title-line", "cod-text-final-newline",
			"cod-text-longest-line", "cod-text-minified", "cod-text-eol", "cod-text-has-bom",
		}
		return append(fields, extra...)
	}
	stats := []string{
		"cod-text-top-keywords", "cod-text-cjk-ratio", "cod-text-non-ascii-ratio",
		"cod-text-word-count", "cod-text-avg-word-len", "cod-text-repeat-line-ratio",
		"cod-text-avg-line-len", "cod-text-line-len-std", "cod-text-bracket-balance",
	}
	timestamps := []string{"cod-text-timestamp-count", "cod-text-time-span", "cod-text-timestamp-line-ratio"}
	entities := []string{
		"cod-text-url-count", "cod-text-email-count", "cod-text-mention-count",
		"cod-text-hashtag-count", "cod-text-emoji-count", "cod-text-link-count",
	}
	markdown := []string{
		"cod-text-headings", "cod-text-structure", "cod-text-table-blocks", "cod-text-table-rows",
		"cod-text-table-cols", "cod-text-list-items", "cod-text-list-nesting-max",
		"cod-text-ordered-ratio", "cod-text-checkboxes", "cod-text-code-lines",
		"cod-text-quote-lines", "cod-text-indent-max", "cod-text-indent-style",
		"cod-text-link-domains", "cod-text-code-languages", "cod-text-section-paths",
		"cod-text-table-headers",
	}
	return []fileSpec{
		{
			path: "search-markdown.md", encoding: "utf-8", marker: "SEARCH_MD_2026",
			coverage: common(append(append(append([]string{}, stats...), timestamps...), append(entities, markdown...)...)...),
			purpose:  "Markdown 综合正例：两级标题、两种管道表、列表、围栏、引用、混合缩进及实体/时间戳。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "utf-8", "cod-text-language": "mixed",
				"cod-text-has-bom": false, "cod-text-eol": "lf", "cod-text-final-newline": true,
				"cod-text-minified": false, "cod-text-table-blocks": 2, "cod-text-table-rows": 7,
				"cod-text-table-cols": 3, "cod-text-list-items": 6, "cod-text-list-nesting-max": 3,
				"cod-text-code-lines": 3, "cod-text-quote-lines": 3, "cod-text-indent-max": 2,
				"cod-text-indent-style": "mixed", "cod-text-url-count": 2, "cod-text-email-count": 1,
				"cod-text-mention-count": 1, "cod-text-hashtag-count": 1, "cod-text-emoji-count": 2,
				"cod-text-link-count": 1, "cod-text-bracket-balance": 1.0,
				"cod-text-structure":      []string{"lists", "code-blocks", "tables", "headings"},
				"cod-text-code-languages": []string{"go"},
				"cod-text-link-domains":   []string{"docs.example.org", "status.example.net"},
				"cod-text-table-headers":  []string{"字段", "示例", "说明", "域名", "用途"},
			},
		},
		{
			path: "search-log.txt", encoding: "utf-8", marker: "SEARCH_LOG_2026",
			coverage: common(append(append([]string{}, stats...), timestamps...)...),
			purpose:  "8 行服务日志：行首时间戳、多级别、两组精确重复行，验证时间跨度与重复行比例。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "utf-8", "cod-text-language": "en",
				"cod-text-lines": 9, "cod-text-eol": "lf", "cod-text-final-newline": true,
				"cod-text-timestamp-count": 8, "cod-text-time-span": 30,
				"cod-text-timestamp-line-ratio": 1.0, "cod-text-repeat-line-ratio": 0.5,
				"cod-text-bracket-balance": 1.0,
			},
		},
		{
			path: "search-chat.txt", encoding: "utf-8", marker: "SEARCH_CHAT_2026",
			coverage: common(append(append(append([]string{}, stats...), timestamps...), entities...)...),
			purpose:  "短行聊天：方括号时间戳、@提及、emoji、话题、邮箱和 URL，与行首日志时间戳形成对照。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "utf-8", "cod-text-language": "mixed",
				"cod-text-lines": 7, "cod-text-eol": "lf", "cod-text-timestamp-count": 6,
				"cod-text-time-span": 145, "cod-text-timestamp-line-ratio": 0.0,
				"cod-text-mention-count": 6, "cod-text-hashtag-count": 2, "cod-text-emoji-count": 4,
				"cod-text-url-count": 1, "cod-text-email-count": 1, "cod-text-link-count": 0,
				"cod-text-bracket-balance": 1.0,
			},
		},
		{
			path: "minified.json", encoding: "utf-8", marker: "SEARCH_JSON_2026",
			coverage: common(append(append([]string{}, stats...), entities...)...),
			purpose:  "合法单行 JSON，无末尾换行且最长行超过 500 rune；只验证 minified/longest-line 等通用文本字段。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "utf-8", "cod-text-language": "en",
				"cod-text-lines": 1, "cod-text-eol": "none", "cod-text-final-newline": false,
				"cod-text-minified": true, "cod-text-bracket-balance": 1.0,
				"cod-text-url-count": 1, "cod-text-email-count": 1,
			},
			omitted: []string{"JSON 专用字段（无专用提取器）"},
		},
		{
			path: "regular.csv", encoding: "utf-8", marker: "SEARCH_CSV_REGULAR",
			coverage: common("cod-text-top-keywords", "cod-text-cjk-ratio", "cod-text-bracket-balance"),
			purpose:  "6 行规整 CSV（表头加 5 行等宽记录）；只验证通用行数、title-line、语言和括号字段。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "utf-8", "cod-text-language": "mixed",
				"cod-text-lines": 7, "cod-text-title-line": "id,name,department,note",
				"cod-text-eol": "lf", "cod-text-bracket-balance": 1.0,
			},
			omitted: []string{"CSV 专用字段（无专用提取器）", "cod-text-table-headers（仅识别 Markdown 管道表）"},
		},
		{
			path: "broken.csv", encoding: "utf-8", marker: "SEARCH_CSV_BROKEN",
			coverage: common("cod-text-bracket-balance"),
			purpose:  "5 行错位 CSV：数据行分别为 3、5、2、4 列，并含未配平括号；只与 regular.csv 做通用文本对照。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "utf-8", "cod-text-language": "mixed",
				"cod-text-lines": 6, "cod-text-title-line": "id,name,department,note",
				"cod-text-eol": "lf", "cod-text-bracket-balance": 0.0,
			},
			omitted: []string{"CSV 专用字段（无专用提取器）", "cod-text-table-headers（仅识别 Markdown 管道表）"},
		},
		{
			path: "empty.txt", encoding: "empty", coverage: []string{"cod-basic-textish"},
			purpose: "0 字节边界：textish=true，但 text Analyze 返回 nil，不应出现 cod-text-* 字段。",
			expect:  map[string]any{"cod-basic-textish": true},
			omitted: []string{"全部 cod-text-*（0 字节在 Analyze 入口直接返回 nil）"},
		},
		{
			path: "nul-sample.bin", encoding: "binary", coverage: []string{"cod-basic-textish"},
			purpose: "6 字节 NUL 二进制负例：textish=false，text 族不运行。",
			expect:  map[string]any{"cod-basic-textish": false},
			omitted: []string{"全部 cod-text-*（textish=false）"},
		},
		{
			path: "utf8-bom.txt", encoding: "utf-8 with BOM", marker: "SEARCH_UTF8_BOM", coverage: common(),
			purpose: "UTF-8 BOM 正例：验证编码仍为 utf-8，has-bom=true。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "utf-8", "cod-text-language": "mixed",
				"cod-text-has-bom": true, "cod-text-eol": "lf", "cod-text-lines": 2,
			},
		},
		{
			path: "utf16le.txt", encoding: "utf-16le with BOM", marker: "SEARCH_UTF16LE", coverage: common(),
			purpose: "带 BOM 的 UTF-16LE 中文正例：验证 textish 闸门、字节序解码和 has-bom。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "utf-16le", "cod-text-language": "mixed",
				"cod-text-has-bom": true, "cod-text-eol": "lf", "cod-text-lines": 2,
			},
		},
		{
			path: "utf16be.txt", encoding: "utf-16be with BOM", marker: "SEARCH_UTF16BE", coverage: common(),
			purpose: "带 BOM 的 UTF-16BE 中文正例：与 LE 样本区分字节序。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "utf-16be", "cod-text-language": "mixed",
				"cod-text-has-bom": true, "cod-text-eol": "lf", "cod-text-lines": 2,
			},
		},
		{
			path: "gbk.txt", encoding: "gbk", marker: "SEARCH_GBK", coverage: common(),
			purpose: "短中文 GBK 正例：无 BOM、无 NUL，验证非 UTF-8 文本经 GBK 链解码。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "gbk", "cod-text-language": "mixed",
				"cod-text-has-bom": false, "cod-text-eol": "lf", "cod-text-lines": 2,
			},
		},
		{
			path: "latin1.txt", encoding: "latin-1", marker: "SEARCH_LATIN1", coverage: common(),
			purpose: "法语/德语 Latin-1 兜底正例：含重音字符及无效 GBK 字节 0xFF，验证 latin-1 回退。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "latin-1", "cod-text-language": "en",
				"cod-text-has-bom": false, "cod-text-eol": "lf", "cod-text-lines": 2,
			},
		},
		{
			path: "no-bom-utf16le.txt", encoding: "utf-16le without BOM", marker: "SEARCH_NO_BOM_UTF16LE",
			coverage: []string{"cod-basic-textish"},
			purpose:  "无 BOM UTF-16LE 负例：交替 NUL 使 textish=false，不应被猜测为文本。",
			expect:   map[string]any{"cod-basic-textish": false},
			omitted:  []string{"全部 cod-text-*（textish=false）"},
		},
		{
			path: "crlf.txt", encoding: "utf-8", marker: "SEARCH_CRLF_2026",
			coverage: common("cod-text-top-keywords"),
			purpose:  "3 行明确 CRLF 文本：验证 eol=crlf、final-newline=true 及物理分行。",
			expect: map[string]any{
				"cod-basic-textish": true, "cod-text-encoding": "utf-8", "cod-text-language": "en",
				"cod-text-lines": 4, "cod-text-eol": "crlf", "cod-text-final-newline": true,
				"cod-text-has-bom": false, "cod-text-minified": false,
			},
		},
	}
}

func encodeUTF16(text string, little, bom bool) []byte {
	units := utf16.Encode([]rune(text))
	out := make([]byte, 0, 2+len(units)*2)
	if bom {
		if little {
			out = append(out, 0xFF, 0xFE)
		} else {
			out = append(out, 0xFE, 0xFF)
		}
	}
	for _, unit := range units {
		var pair [2]byte
		if little {
			binary.LittleEndian.PutUint16(pair[:], unit)
		} else {
			binary.BigEndian.PutUint16(pair[:], unit)
		}
		out = append(out, pair[:]...)
	}
	return out
}

var gbkPairs = map[rune][2]byte{
	'你': {0xC4, 0xE3}, '好': {0xBA, 0xC3}, '，': {0xA3, 0xAC},
	'世': {0xCA, 0xCB}, '界': {0xBD, 0xE7}, '！': {0xA1, 0xA3},
	'文': {0xCE, 0xC4}, '本': {0xB1, 0xBE}, '编': {0xB1, 0xE0},
	'码': {0xC2, 0xEB}, '。': {0xA1, 0xA3},
}

func encodeGBK(text string) ([]byte, error) {
	var out bytes.Buffer
	for _, r := range text {
		if r < utf8.RuneSelf {
			out.WriteByte(byte(r))
			continue
		}
		pair, ok := gbkPairs[r]
		if !ok {
			return nil, fmt.Errorf("no GBK mapping for %q", r)
		}
		out.Write(pair[:])
	}
	return out.Bytes(), nil
}

func encodeLatin1(text string) ([]byte, error) {
	var out bytes.Buffer
	for _, r := range text {
		if r > 0xFF {
			return nil, fmt.Errorf("%q is not representable in Latin-1", r)
		}
		out.WriteByte(byte(r))
	}
	return out.Bytes(), nil
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
