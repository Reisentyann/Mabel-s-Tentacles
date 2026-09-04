// 文件：describer-go/image/image_test.go —— cod-image 单元测试（含 v3：P2 画像字段）
// 修改：2026-09-04（日期由 fresh-header.ps1 刷新）

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

// solidPNG 生成单色 n×n PNG。
func solidPNG(t *testing.T, n int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestP2WarmCoolFlat(t *testing.T) {
	d := descriptor{}
	attrs, _ := d.Analyze(describer.Input{Path: "warm.png"}, warmPNG(t))

	// 3/4 暖 #D4A373（H≈30）+ 1/4 冷 #5A78B4（H≈220）
	if got := attrs["cod-image-warm-ratio"].(float64); got != 0.75 {
		t.Fatalf("warm-ratio = %v, want 0.75", got)
	}
	if got := attrs["cod-image-cool-ratio"].(float64); got != 0.25 {
		t.Fatalf("cool-ratio = %v, want 0.25", got)
	}
	// 两个量化色桶各占 75%/25%（均 ≥5%）→ flat=1.0；color-count=2
	if got := attrs["cod-image-flat-ratio"].(float64); got != 1.0 {
		t.Fatalf("flat-ratio = %v, want 1.0", got)
	}
	if got := attrs["cod-image-color-count"].(int); got != 2 {
		t.Fatalf("color-count = %v, want 2", got)
	}
	// 非暗非亮
	if attrs["cod-image-dark"] != false || attrs["cod-image-light"] != false {
		t.Fatalf("dark=%v light=%v, want false/false", attrs["cod-image-dark"], attrs["cod-image-light"])
	}
	// 无透明像素 → alpha-ratio=0
	if got := attrs["cod-image-alpha-ratio"].(float64); got != 0 {
		t.Fatalf("alpha-ratio = %v, want 0", got)
	}
	// warm #D4A373 落在肤色规则内（R>G>B、R-B>15、|R-G|<80）→ 0.75
	if got := attrs["cod-image-skin-tone-ratio"].(float64); got != 0.75 {
		t.Fatalf("skin-tone-ratio = %v, want 0.75", got)
	}
	// 2 个 luma 桶的熵 = -0.75log2(0.75)-0.25log2(0.25) ≈ 0.81
	if got := attrs["cod-image-entropy"].(float64); got != 0.81 {
		t.Fatalf("entropy = %v, want 0.81", got)
	}
	// 左右镜像：左 48 列暖右 16 列冷 → 前半差大后半差 0 → 0.5
	if got := attrs["cod-image-symmetry"].(float64); got != 0.5 {
		t.Fatalf("symmetry = %v, want 0.5", got)
	}
	// 饱和度均值（0.53×0.75 + 0.38×0.25 ≈ 0.49 → 49）
	if got := attrs["cod-image-saturation"].(float64); got <= 30 || got >= 60 {
		t.Fatalf("saturation = %v, want 30..60", got)
	}
	// 梯度：仅暖冷交界有边 → 低密度；sharpness 非负存在
	if got := attrs["cod-image-edge-density"].(float64); got <= 0 || got >= 0.2 {
		t.Fatalf("edge-density = %v, want (0,0.2)", got)
	}
	if got := attrs["cod-image-sharpness"].(float64); got < 0 {
		t.Fatalf("sharpness = %v, want >= 0", got)
	}
}

func TestP2DarkLightEntropy(t *testing.T) {
	d := descriptor{}
	// 全黑 32×32：dark=true、flat=1.0、color-count=1、entropy=0、冷暖 0（饱和度门槛）
	attrs, _ := d.Analyze(describer.Input{Path: "black.png"}, solidPNG(t, 32, color.RGBA{0, 0, 0, 255}))
	if attrs["cod-image-dark"] != true {
		t.Fatalf("dark = %v, want true", attrs["cod-image-dark"])
	}
	if got := attrs["cod-image-flat-ratio"].(float64); got != 1.0 {
		t.Fatalf("flat-ratio = %v, want 1.0", got)
	}
	if got := attrs["cod-image-color-count"].(int); got != 1 {
		t.Fatalf("color-count = %v, want 1", got)
	}
	if got := attrs["cod-image-entropy"].(float64); got != 0 {
		t.Fatalf("entropy = %v, want 0", got)
	}
	if got := attrs["cod-image-warm-ratio"].(float64); got != 0 {
		t.Fatalf("warm-ratio = %v, want 0 (灰图饱和度门槛)", got)
	}
	if got := attrs["cod-image-cool-ratio"].(float64); got != 0 {
		t.Fatalf("cool-ratio = %v, want 0", got)
	}

	// 全白 → light=true
	attrs, _ = d.Analyze(describer.Input{Path: "white.png"}, solidPNG(t, 32, color.RGBA{255, 255, 255, 255}))
	if attrs["cod-image-light"] != true {
		t.Fatalf("light = %v, want true", attrs["cod-image-light"])
	}
}
