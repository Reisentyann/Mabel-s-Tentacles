// gen_png 生成图像测试夹具：warm.png（3/4 暖 + 1/4 冷）与 ai-gen.png（带 tEXt 块的"AI 生成图"）。
// 纯标准库，可在仓库根直接执行：go run ./test/tools/gen_png.go
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	outDir := filepath.Join("test", "fixtures", "图像")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal(err)
	}
	if err := writePNG(filepath.Join(outDir, "warm.png"), warmImage(), nil); err != nil {
		fatal(err)
	}
	text := map[string]string{
		"prompt":     "a cute anime girl reading in a warm library, golden hour",
		"parameters": "Steps: 28, Sampler: euler_ancestral, CFG: 7, Seed: 42",
	}
	if err := writePNG(filepath.Join(outDir, "ai-gen.png"), flatImage(), text); err != nil {
		fatal(err)
	}
	fmt.Println("written:", outDir)
}

func warmImage() image.Image {
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
	return img
}

func flatImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	c := color.RGBA{200, 120, 96, 255}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// writePNG 编码 PNG；textChunks 非空时在 IEND 前插入 tEXt 块（latin-1 文本）。
func writePNG(path string, img image.Image, textChunks map[string]string) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	raw := buf.Bytes()
	out := raw
	if len(textChunks) > 0 {
		out = append([]byte{}, raw[:len(raw)-12]...) // 去掉 IEND 的长度+类型，保留完整 IEND 尾部
		for _, k := range sortedKeys(textChunks) {
			out = append(out, textChunk(k, textChunks[k])...)
		}
		out = append(out, raw[len(raw)-12:]...)
	}
	return os.WriteFile(path, out, 0o644)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func textChunk(keyword, value string) []byte {
	body := append(append([]byte(keyword), 0), []byte(value)...)
	chunk := make([]byte, 0, 12+len(body))
	var lenb, crcb [4]byte
	binary.BigEndian.PutUint32(lenb[:], uint32(len(body)))
	binary.BigEndian.PutUint32(crcb[:], crc32.ChecksumIEEE(append([]byte("tEXt"), body...)))
	chunk = append(chunk, lenb[:]...)
	chunk = append(chunk, []byte("tEXt")...)
	chunk = append(chunk, body...)
	chunk = append(chunk, crcb[:]...)
	return chunk
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
