// 文件：describer-go/describer_test.go —— 引擎端到端测试：basic+路由+sp 前缀拼装 / 文本路由不串味 / 惰性加载只调一次 / 预算截断
// 修改：2026-09-04（日期由 fresh-header.ps1 刷新）

// 端到端：引擎把 basic + 路由 + sp 前缀拼起来。
package describer_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/all"
)

func pngWithPrompt(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{200, 120, 60, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	body := append(append([]byte("prompt"), 0), []byte("masterpiece")...)
	var lenb, crcb [4]byte
	binary.BigEndian.PutUint32(lenb[:], uint32(len(body)))
	binary.BigEndian.PutUint32(crcb[:], crc32.ChecksumIEEE(append([]byte("tEXt"), body...)))
	chunk := append(append(append(lenb[:], []byte("tEXt")...), body...), crcb[:]...)
	out := append([]byte{}, raw[:len(raw)-12]...)
	out = append(out, chunk...)
	out = append(out, raw[len(raw)-12:]...)
	return out
}

func TestAnalyzeEnginePNG(t *testing.T) {
	raw := pngWithPrompt(t)
	mtime := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	in := describer.Input{
		Path:    "gen.png",
		Head:    raw,
		Full:    raw,
		Size:    int64(len(raw)),
		MTime:   mtime,
		ExtMime: "image/png",
	}
	rs := describer.Analyze(in, nil)
	if len(rs) != 2 {
		t.Fatalf("families = %d, want 2 (basic+image)", len(rs))
	}

	fam := map[string]describer.Result{}
	for _, r := range rs {
		fam[r.Family] = r
	}
	basic, image := fam["basic"], fam["image"]

	if basic.Attrs["cod-basic-mime"] != "image/png" {
		t.Fatalf("mime = %v", basic.Attrs["cod-basic-mime"])
	}
	if basic.Attrs["cod-basic-textish"] != false {
		t.Fatal("png should not be textish")
	}
	if basic.Attrs["cod-basic-mime-match"] != true {
		t.Fatalf("mime-match = %v", basic.Attrs["cod-basic-mime-match"])
	}
	if basic.Attrs["cod-basic-mtime"] != mtime.Unix() {
		t.Fatalf("mtime = %v", basic.Attrs["cod-basic-mtime"])
	}
	if basic.Attrs["cod-basic-name-pattern"] != "plain" {
		t.Fatalf("name-pattern = %v", basic.Attrs["cod-basic-name-pattern"])
	}
	if image.Attrs["cod-image-width"] != 32 {
		t.Fatalf("width = %v", image.Attrs["cod-image-width"])
	}
	if image.Attrs["sp-cod-png-prompt"] != "masterpiece" {
		t.Fatalf("sp = %#v", image.Attrs["sp-cod-png-prompt"])
	}

	// 合并 + llm 共存
	existing := map[string]any{"llm-tone": "暖橙"}
	merged := describer.MergeResults(existing, rs, mtime)
	if merged["llm-tone"] != "暖橙" {
		t.Fatal("llm field must survive cod merge")
	}
	if merged["cod-image-at"] != mtime.Unix() {
		t.Fatal("cod-image-at should be set")
	}
	if merged["cod-image-ver"] != 3 {
		t.Fatal("cod-image-ver should be set (image v3)")
	}
	if merged["cod-basic-ver"] != 3 {
		t.Fatal("cod-basic-ver should be set (basic v3)")
	}
	b, _ := json.Marshal(merged)
	if string(b) == "" {
		t.Fatal("unreachable")
	}
}

func TestAnalyzeEngineTextRoutesTextOnly(t *testing.T) {
	content := []byte("# 标题\n\n梅贝尔整理文件。")
	rs := describer.Analyze(describer.Input{
		Path: "note.md", Head: content, Full: content,
		Size: int64(len(content)), ExtMime: "text/plain",
	}, nil)
	fams := map[string]bool{}
	for _, r := range rs {
		fams[r.Family] = true
	}
	if !fams["basic"] || !fams["text"] {
		t.Fatalf("families = %v, want basic+text", fams)
	}
	if fams["image"] {
		t.Fatal("text file must NOT produce cod-image-* keys at all")
	}
	for _, r := range rs {
		for k := range r.Attrs {
			if fams["image"] && len(k) >= 9 && k[:9] == "cod-image" {
				t.Fatalf("text file leaked image key %s", k)
			}
		}
	}
}

func TestAnalyzeEngineLazyLoad(t *testing.T) {
	content := []byte("hello world")
	calls := 0
	rs := describer.Analyze(describer.Input{
		Path: "a.txt", Head: content[:5], ExtMime: "text/plain",
	}, func() ([]byte, error) {
		calls++
		return content, nil
	})
	if calls != 1 {
		t.Fatalf("loader called %d times, want exactly 1 (shared)", calls)
	}
	found := false
	for _, r := range rs {
		if r.Family == "text" {
			found = true
		}
	}
	if !found {
		t.Fatal("text family missing after lazy load")
	}
}

func TestAnalyzeEngineCorruptImagePurgesOldKeys(t *testing.T) {
	// PNG 魔数齐全但 IHDR 损坏：image 路由命中、解码失败零产出。
	// 引擎仍须给出 image 空结果，合并时清掉旧 cod-image-* / sp-cod-png-*，
	// 否则损坏文件永远残留旧事实（IsStale 反复重分析也清不掉）。
	raw := append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D},
		[]byte("IHDR")...)
	raw = append(raw, make([]byte, 17)...) // 13B 数据 + 4B CRC 全零 → 解码必败
	in := describer.Input{
		Path: "broken.png", Head: raw, Full: raw,
		Size: int64(len(raw)), ExtMime: "image/png",
	}
	rs := describer.Analyze(in, nil)
	var img *describer.Result
	for i, r := range rs {
		if r.Family == "image" {
			img = &rs[i]
		}
	}
	if img == nil {
		t.Fatal("image family must be present (ran but empty)")
	}
	if len(img.Attrs) != 0 {
		t.Fatalf("corrupt image attrs = %#v, want empty", img.Attrs)
	}

	now := time.Unix(1700000000, 0)
	m := describer.MergeResults(map[string]any{
		"cod-image-width":   32,
		"sp-cod-png-prompt": "旧参数",
		"cod-image-at":      int64(1),
		"cod-text-lines":    5,
		"llm-tone":          "保留",
	}, rs, now)
	if _, ok := m["cod-image-width"]; ok {
		t.Fatal("old cod-image-width must be purged on empty output")
	}
	if _, ok := m["sp-cod-png-prompt"]; ok {
		t.Fatal("old sp-cod-png-prompt must be purged on empty output")
	}
	if m["cod-image-ver"] != img.Ver {
		t.Fatalf("cod-image-ver = %v, want %d", m["cod-image-ver"], img.Ver)
	}
	if m["cod-image-at"] != now.Unix() {
		t.Fatal("cod-image-at should be stamped even on empty output")
	}
	if m["cod-text-lines"] != 5 {
		t.Fatal("other cod family must survive")
	}
	if m["llm-tone"] != "保留" {
		t.Fatal("llm field must survive")
	}
}

func TestAnalyzeEngineEmptyTextStampsFamily(t *testing.T) {
	// 空文本文件：textish=true 路由命中但零产出——text 仍入列，
	// 合并清旧键并刷 ver/at，防 IsStale 每轮重触发却清不掉旧键。
	in := describer.Input{Path: "empty.txt", Head: []byte{}, Full: []byte{}, ExtMime: "text/plain"}
	rs := describer.Analyze(in, nil)
	var text *describer.Result
	for i, r := range rs {
		if r.Family == "text" {
			text = &rs[i]
		}
	}
	if text == nil {
		t.Fatal("text family must be present (ran but empty)")
	}
	if len(text.Attrs) != 0 {
		t.Fatalf("empty file text attrs = %#v, want empty", text.Attrs)
	}

	now := time.Unix(1700000000, 0)
	m := describer.MergeResults(map[string]any{"cod-text-lines": 5, "llm-tone": "保留"}, rs, now)
	if _, ok := m["cod-text-lines"]; ok {
		t.Fatal("old cod-text-lines must be purged on empty output")
	}
	if m["cod-text-ver"] != text.Ver {
		t.Fatalf("cod-text-ver = %v, want %d", m["cod-text-ver"], text.Ver)
	}
	if m["cod-text-at"] != now.Unix() {
		t.Fatal("cod-text-at should be stamped even on empty output")
	}
	if m["llm-tone"] != "保留" {
		t.Fatal("llm field must survive")
	}
}

func TestAnalyzeEngineBudgetTruncation(t *testing.T) {
	// 全量预算：5MB+1000B 文本截到 5MB（尾部 b 不得进入分析）
	big := make([]byte, describer.MaxFullBytes+1000)
	for i := range big {
		big[i] = 'a'
	}
	copy(big[describer.MaxFullBytes:], []byte(strings.Repeat("b", 1000)))
	in := describer.Input{Path: "big.txt", Head: big[:512], Full: big, ExtMime: "text/plain"}
	rs := describer.Analyze(in, nil)
	var chars any
	for _, r := range rs {
		if r.Family == "text" {
			chars = r.Attrs["cod-text-chars"]
		}
	}
	if chars == nil {
		t.Fatal("text family should run on truncated full")
	}
	if chars.(int) != describer.MaxFullBytes {
		t.Fatalf("chars = %v, want %d (5MB 截断)", chars, describer.MaxFullBytes)
	}

	// 头部预算：512B 全 'a' + 488B NUL → 截断后 entropy=0（纯 'a'）；
	// 未截断则混合 NUL 的 entropy > 0。textish=false 不影响 entropy 产出。
	head := append(bytes.Repeat([]byte{'a'}, 512), bytes.Repeat([]byte{0x00}, 488)...)
	rs = describer.Analyze(describer.Input{Path: "x.bin", Head: head, Full: head}, nil)
	for _, r := range rs {
		if r.Family == "basic" {
			if e, ok := r.Attrs["cod-basic-entropy"].(float64); !ok || e != 0 {
				t.Fatalf("entropy = %v, want 0 (head truncated to 512B of 'a')", r.Attrs["cod-basic-entropy"])
			}
		}
	}
}
