// 文件：describer-go/basic/basic.go —— cod-basic 插件：MIME 嗅探 / textish / 熵 / 文件名模式（一切文件必跑）
// 修改：2026-09-04（日期由 fresh-header.ps1 刷新）

// Package basic cod-basic 插件：一切文件必跑的确定性基础事实。
// 字段字典见 docs/元数据字段说明.md 第 4.1 节，键在此处字面量封闭。
package basic

import (
	"math"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

func init() { describer.Register(descriptor{}) }

type descriptor struct{}

func (descriptor) Family() string { return "basic" }

// FamilyVersion=3：v1 首发；v2 textish 认定带 BOM 的 UTF-16 为文本
// （此前其高低位交替 NUL 被误判二进制，text 家族整体不跑）；
// v3 新增 cod-basic-executable（PE/ELF/Mach-O 魔数）与 name-pattern 增补 uuid-like。
func (descriptor) FamilyVersion() int                            { return 3 }
func (descriptor) Supports(string, []byte, describer.Basic) bool { return true }
func (descriptor) SPNamespaces() []string                        { return nil }
func (descriptor) Analyze(in describer.Input, _ []byte) (map[string]any, map[string]string) {
	a := map[string]any{}

	a["cod-basic-mime"] = http.DetectContentType(in.Head)
	if in.ExtMime != "" {
		if m, ok := mimeMatch(http.DetectContentType(in.Head), in.ExtMime); ok {
			a["cod-basic-mime-match"] = m
		}
	}
	if !in.MTime.IsZero() {
		a["cod-basic-mtime"] = in.MTime.Unix()
	}
	a["cod-basic-textish"] = looksTexty(in.Head)
	a["cod-basic-executable"] = looksExecutable(in.Head)
	if len(in.Head) > 0 {
		a["cod-basic-entropy"] = describer.Round2(entropy(in.Head))
	}
	if p := namePattern(in.Path); p != "" {
		a["cod-basic-name-pattern"] = p
	}
	return a, nil
}

// mimeMatch 嗅探 MIME 与扩展名推断 MIME 的一致性。
// 嗅探失败（octet-stream）时无法判定，返回 ok=false 不落字段。
// 文本族宽容：json/yaml/xml/markdown 嗅探均为 text/plain，一方为
// text/plain 且另一方是文本类 MIME 即视为一致。
func mimeMatch(sniffed, extMime string) (bool, bool) {
	s := strings.TrimSpace(strings.SplitN(sniffed, ";", 2)[0])
	e := strings.TrimSpace(strings.SplitN(extMime, ";", 2)[0])
	if s == "" || e == "" || s == "application/octet-stream" {
		return false, false
	}
	if s == e {
		return true, true
	}
	if s == "text/plain" && isTextyMime(e) {
		return true, true
	}
	if e == "text/plain" && isTextyMime(s) {
		return true, true
	}
	return false, true
}

func isTextyMime(m string) bool {
	return strings.HasPrefix(m, "text/") ||
		m == "application/json" || m == "application/xml" ||
		m == "application/yaml" || m == "application/x-yaml" ||
		m == "application/toml" || m == "application/javascript"
}

// looksTexty 文本判定：无 NUL 字节且控制字符占比低（GBK 等非 UTF-8 文本也算文本，
// 编码细节由 text 插件处理）。带 BOM 的 UTF-16 例外——高低位交替 NUL 是其
// 正常形态，属文本，交 text 插件解码；无 BOM 的 UTF-16 无从确证，维持二进制判定。
func looksTexty(head []byte) bool {
	if len(head) == 0 {
		return true
	}
	if _, ok := describer.UTF16BOM(head); ok {
		return true
	}
	ctrl := 0
	for _, b := range head {
		if b == 0 {
			return false
		}
		if b < 0x09 || (b > 0x0D && b < 0x20) {
			ctrl++
		}
	}
	return float64(ctrl)/float64(len(head)) <= 0.15
}

// looksExecutable 可执行文件魔数：PE（MZ）/ ELF / Mach-O（含通用二进制）。
// 纯魔数判定，恒产 bool——"找出误存成数据目录的程序"与"伪装的exe"检索用。
func looksExecutable(head []byte) bool {
	if len(head) < 4 {
		return false
	}
	if head[0] == 'M' && head[1] == 'Z' { // PE / DOS 可执行
		return true
	}
	if head[0] == 0x7F && head[1] == 'E' && head[2] == 'L' && head[3] == 'F' {
		return true
	}
	macho := [][4]byte{
		{0xFE, 0xED, 0xFA, 0xCE}, {0xCE, 0xFA, 0xED, 0xFE}, // 32 位（BE/LE）
		{0xFE, 0xED, 0xFA, 0xCF}, {0xCF, 0xFA, 0xED, 0xFE}, // 64 位（BE/LE）
		{0xCA, 0xFE, 0xBA, 0xBE}, // 通用二进制（fat）
	}
	for _, m := range macho {
		if head[0] == m[0] && head[1] == m[1] && head[2] == m[2] && head[3] == m[3] {
			return true
		}
	}
	return false
}

// entropy 香农熵（bits/byte，0-8）。
func entropy(b []byte) float64 {
	if len(b) == 0 {
		return 0
	}
	var freq [256]int
	for _, x := range b {
		freq[x]++
	}
	h := 0.0
	n := float64(len(b))
	for _, c := range freq {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}

var (
	reCamera      = regexp.MustCompile(`(?i)^(img[_-]?\d+|dsc[_-]?\d+|dji_?\d+|pxl_?~?\d+|go\w{0,2}\d{4}|mvi_?\d+)`)
	reScreenshot  = regexp.MustCompile(`(?i)(screenshot|screen[_ -]?shot|screen[_ -]?capture|截屏|截图|屏幕截图)`)
	reTimestamped = regexp.MustCompile(`\d{4}[-_.]\d{1,2}[-_.]\d{1,2}|\d{8}[_-]\d{6}`)
	reHashlike    = regexp.MustCompile(`^[0-9a-f]{16,}$`)
	reUUIDlike    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reVersioned   = regexp.MustCompile(`(?i)[._-]v?\d+\.\d+(\.\d+)*$`)
)

// namePattern 文件名模式（纯正则，可复现）。判定顺序即优先级。
func namePattern(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	switch {
	case reCamera.MatchString(base):
		return "camera"
	case reScreenshot.MatchString(base):
		return "screenshot"
	case reTimestamped.MatchString(base):
		return "timestamped"
	case reUUIDlike.MatchString(base):
		return "uuid-like"
	case reVersioned.MatchString(base):
		return "versioned"
	case reHashlike.MatchString(base):
		return "hashlike"
	default:
		return "plain"
	}
}
