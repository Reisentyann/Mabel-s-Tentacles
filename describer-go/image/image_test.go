// 文件：describer-go/image/image_test.go —— cod-image 单元测试
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package image

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

// warmPNG 生成 64×64 测试图：3/4 暖色 #D4A373，1/4 冷色 #5A78B4。
func warmPNG(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	warm := color.RGBA{212, 163, 115, 255}
	cool := color.RGBA{90, 120, 180, 255}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if x < 48 {
				img.Set(x, y, warm)
			} else {
				img.Set(x, y, cool)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// withTextChunk 在 IEND 前插入一个 tEXt 块。
func withTextChunk(t *testing.T, raw []byte, keyword, value string) []byte {
	if string(raw[len(raw)-8:len(raw)-4]) != "IEND" {
		t.Fatal("tail chunk is not IEND")
	}
	body := append([]byte(keyword), 0)
	body = append(body, []byte(value)...)
	chunk := make([]byte, 0, 12+len(body))
	var lenb [4]byte
	binary.BigEndian.PutUint32(lenb[:], uint32(len(body)))
	chunk = append(chunk, lenb[:]...)
	chunk = append(chunk, []byte("tEXt")...)
	chunk = append(chunk, body...)
	var crcb [4]byte
	binary.BigEndian.PutUint32(crcb[:], crc32.ChecksumIEEE(chunk[4:8+len(body)]))
	chunk = append(chunk, crcb[:]...)

	out := make([]byte, 0, len(raw)+len(chunk))
	out = append(out, raw[:len(raw)-12]...) // 去掉 IEND 的长度+类型（保留 IEND 完整尾部）
	out = append(out, chunk...)
	out = append(out, raw[len(raw)-12:]...)
	return out
}

func TestImageAnalyze(t *testing.T) {
	raw := warmPNG(t)
	d := descriptor{}
	attrs, sp := d.Analyze(describer.Input{Path: "test.png", ExtMime: "image/png"}, raw)

	if attrs["cod-image-width"] != 64 || attrs["cod-image-height"] != 64 {
		t.Fatalf("dims = %v x %v", attrs["cod-image-width"], attrs["cod-image-height"])
	}
	if attrs["cod-image-aspect"] != "1:1" {
		t.Fatalf("aspect = %v", attrs["cod-image-aspect"])
	}
	if attrs["cod-image-orientation"] != "square" {
		t.Fatalf("orientation = %v", attrs["cod-image-orientation"])
	}
	pal := attrs["cod-image-palette"].([]map[string]any)
	if len(pal) == 0 || len(pal) > 5 {
		t.Fatalf("palette len = %d", len(pal))
	}
	if fam := attrs["cod-image-family"]; fam != "orange" {
		t.Fatalf("family = %v, want orange (warm dominant)", fam)
	}
	if dom := attrs["cod-image-dominant"]; dom == nil || dom.(string)[0] != '#' {
		t.Fatalf("dominant = %#v", attrs["cod-image-dominant"])
	}
	if b := attrs["cod-image-brightness"].(float64); b < 0 || b > 100 {
		t.Fatalf("brightness out of range: %v", b)
	}
	if len(sp) != 0 {
		t.Fatalf("plain png should have no sp, got %v", sp)
	}
}

func TestImageDeterminism(t *testing.T) {
	raw := warmPNG(t)
	d := descriptor{}
	a1, s1 := d.Analyze(describer.Input{Path: "test.png"}, raw)
	a2, s2 := d.Analyze(describer.Input{Path: "test.png"}, raw)
	j1, _ := json.Marshal([]any{a1, s1})
	j2, _ := json.Marshal([]any{a2, s2})
	if string(j1) != string(j2) {
		t.Fatalf("image analyze not deterministic:\n%s\n%s", j1, j2)
	}
}

func TestPNGTextChunk(t *testing.T) {
	raw := withTextChunk(t, warmPNG(t), "prompt", "a cute anime girl, warm light")
	d := descriptor{}
	_, sp := d.Analyze(describer.Input{Path: "gen.png"}, raw)
	if sp["png-prompt"] != "a cute anime girl, warm light" {
		t.Fatalf("sp = %#v, want png-prompt", sp)
	}
}

func TestSupports(t *testing.T) {
	d := descriptor{}
	if !d.Supports("x.png", nil, describer.Basic{Mime: "image/png"}) {
		t.Fatal("image mime should be supported")
	}
	if d.Supports("x.png", nil, describer.Basic{Mime: "text/plain"}) {
		t.Fatal("text mime should not hit image plugin")
	}
}
