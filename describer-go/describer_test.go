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
