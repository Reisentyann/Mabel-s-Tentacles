package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
)

const (
	fixtureRoot   = "test/fixtures/检索字段/图像补充"
	generatorPath = "test/tools/search-fixture-generators/images-extra/main.go"
	maxFileBytes  = 32 << 10
	maxBatchBytes = 128 << 10
	pngSignature  = "\x89PNG\r\n\x1a\n"
)

type dimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type fixtureEntry struct {
	Path                 string         `json:"path"`
	MediaType            string         `json:"media_type"`
	Bytes                int            `json:"bytes"`
	SHA256               string         `json:"sha256"`
	Dimensions           dimensions     `json:"dimensions"`
	Decodable            bool           `json:"decodable"`
	ExpectedFields       map[string]any `json:"expected_fields"`
	ExpectedAbsentFields []string       `json:"expected_absent_fields"`
	Purpose              string         `json:"purpose"`
}

type fixtureManifest struct {
	SchemaVersion int            `json:"schema_version"`
	PathBase      string         `json:"path_base"`
	Generator     string         `json:"generator"`
	FixtureBytes  int            `json:"fixture_bytes"`
	Files         []fixtureEntry `json:"files"`
}

type generatedFixture struct {
	entry fixtureEntry
	data  []byte
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "generate image-extra fixtures:", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := os.Stat(generatorPath); err != nil {
		return fmt.Errorf("run from project root: %w", err)
	}
	fixtures, err := buildFixtures()
	if err != nil {
		return err
	}
	manifest := fixtureManifest{SchemaVersion: 1, PathBase: ".", Generator: generatorPath, Files: make([]fixtureEntry, 0, len(fixtures))}
	total := 0
	for i := range fixtures {
		data := fixtures[i].data
		if len(data) == 0 || len(data) > maxFileBytes {
			return fmt.Errorf("%s has invalid size %d", fixtures[i].entry.Path, len(data))
		}
		digest := sha256.Sum256(data)
		fixtures[i].entry.Bytes = len(data)
		fixtures[i].entry.SHA256 = hex.EncodeToString(digest[:])
		manifest.Files = append(manifest.Files, fixtures[i].entry)
		total += len(data)
	}
	manifest.FixtureBytes = total
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestData = append(manifestData, '\n')
	if len(manifestData) > maxFileBytes || total+len(manifestData) > maxBatchBytes {
		return fmt.Errorf("image-extra batch exceeds safety limits")
	}
	if err := os.MkdirAll(fixtureRoot, 0o755); err != nil {
		return err
	}
	paths := make([]string, 0, len(fixtures)+1)
	for _, fixture := range fixtures {
		paths = append(paths, filepath.Join(fixtureRoot, fixture.entry.Path))
	}
	paths = append(paths, filepath.Join(fixtureRoot, "manifest.json"))
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing to overwrite %s", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for _, fixture := range fixtures {
		if err := writeNew(filepath.Join(fixtureRoot, fixture.entry.Path), fixture.data); err != nil {
			return err
		}
	}
	if err := writeNew(filepath.Join(fixtureRoot, "manifest.json"), manifestData); err != nil {
		return err
	}
	fmt.Printf("fixtures=%d fixtureBytes=%d manifestBytes=%d total=%d\n", len(fixtures), total, len(manifestData), total+len(manifestData))
	return nil
}

func buildFixtures() ([]generatedFixture, error) {
	dark, err := solidPNG(8, 8, color.NRGBA{R: 10, G: 10, B: 10, A: 255})
	if err != nil {
		return nil, err
	}
	light, err := solidPNG(8, 8, color.NRGBA{R: 240, G: 240, B: 240, A: 255})
	if err != nil {
		return nil, err
	}
	portrait, err := solidPNG(4, 8, color.NRGBA{R: 198, G: 134, B: 107, A: 255})
	if err != nil {
		return nil, err
	}
	landscape, err := solidPNG(8, 4, color.NRGBA{R: 40, G: 120, B: 220, A: 255})
	if err != nil {
		return nil, err
	}
	metadataBase, err := solidPNG(8, 8, color.NRGBA{R: 112, G: 48, B: 160, A: 255})
	if err != nil {
		return nil, err
	}
	metadata, err := addCompressedPNGText(metadataBase)
	if err != nil {
		return nil, err
	}
	singleGIF, err := singleFrameGIF()
	if err != nil {
		return nil, err
	}
	bmp := encodeBMP(4, 6, color.NRGBA{R: 60, G: 180, B: 90, A: 255})

	commonAbsent := []string{"cod-image-has-alpha", "cod-image-taken-at", "cod-image-camera", "cod-image-software"}
	return []generatedFixture{
		{entry: fixtureEntry{Path: "dark-grayscale.png", MediaType: "image/png", Dimensions: dimensions{8, 8}, Decodable: true,
			ExpectedFields:       map[string]any{"cod-image-width": 8, "cod-image-height": 8, "cod-image-aspect": "1:1", "cod-image-orientation": "square", "cod-image-dark": true, "cod-image-light": false, "cod-image-grayscale": true, "cod-image-family": "grayscale", "cod-image-color-count": 1, "cod-image-flat-ratio": 1, "cod-image-entropy": 0},
			ExpectedAbsentFields: commonAbsent, Purpose: "深灰纯色 PNG：dark/grayscale 正例及 light=false。"}, data: dark},
		{entry: fixtureEntry{Path: "light-grayscale.png", MediaType: "image/png", Dimensions: dimensions{8, 8}, Decodable: true,
			ExpectedFields:       map[string]any{"cod-image-width": 8, "cod-image-height": 8, "cod-image-aspect": "1:1", "cod-image-orientation": "square", "cod-image-dark": false, "cod-image-light": true, "cod-image-grayscale": true, "cod-image-family": "grayscale", "cod-image-color-count": 1, "cod-image-flat-ratio": 1, "cod-image-entropy": 0},
			ExpectedAbsentFields: commonAbsent, Purpose: "浅灰纯色 PNG：light/grayscale 正例及 dark=false。"}, data: light},
		{entry: fixtureEntry{Path: "portrait-skin.png", MediaType: "image/png", Dimensions: dimensions{4, 8}, Decodable: true,
			ExpectedFields:       map[string]any{"cod-image-width": 4, "cod-image-height": 8, "cod-image-aspect": "1:2", "cod-image-orientation": "portrait", "cod-image-skin-tone-ratio": 1, "cod-image-warm-ratio": 1, "cod-image-cool-ratio": 0, "cod-image-family": "orange"},
			ExpectedAbsentFields: commonAbsent, Purpose: "4x8 肤色纯色竖图：portrait、1:2 与肤色占比正例。"}, data: portrait},
		{entry: fixtureEntry{Path: "landscape-cool.png", MediaType: "image/png", Dimensions: dimensions{8, 4}, Decodable: true,
			ExpectedFields:       map[string]any{"cod-image-width": 8, "cod-image-height": 4, "cod-image-aspect": "2:1", "cod-image-orientation": "landscape", "cod-image-cool-ratio": 1, "cod-image-warm-ratio": 0, "cod-image-family": "blue"},
			ExpectedAbsentFields: commonAbsent, Purpose: "8x4 冷色纯色横图：landscape、2:1 与冷色占比正例。"}, data: landscape},
		{entry: fixtureEntry{Path: "compressed-metadata.png", MediaType: "image/png", Dimensions: dimensions{8, 8}, Decodable: true,
			ExpectedFields:       map[string]any{"cod-image-width": 8, "cod-image-height": 8, "cod-image-family": "purple", "sp-cod-png-prompt": "SEARCH_PNG_ZTXT_2026", "sp-cod-png-parameters": "SEARCH_PNG_ITXT_2026"},
			ExpectedAbsentFields: commonAbsent, Purpose: "PNG zTXt/iTXt 元数据正例，验证压缩文本提取路径。"}, data: metadata},
		{entry: fixtureEntry{Path: "single-frame.gif", MediaType: "image/gif", Dimensions: dimensions{4, 4}, Decodable: true,
			ExpectedFields:       map[string]any{"cod-image-width": 4, "cod-image-height": 4, "cod-image-aspect": "1:1", "cod-image-orientation": "square", "cod-image-family": "red"},
			ExpectedAbsentFields: append(append([]string{}, commonAbsent...), "cod-image-animated", "cod-image-frames"), Purpose: "单帧 GIF 负例：animated/frames 应缺键。"}, data: singleGIF},
		{entry: fixtureEntry{Path: "sample.bmp", MediaType: "image/bmp", Dimensions: dimensions{4, 6}, Decodable: true,
			ExpectedFields:       map[string]any{"cod-image-width": 4, "cod-image-height": 6, "cod-image-aspect": "2:3", "cod-image-orientation": "portrait", "cod-image-family": "green"},
			ExpectedAbsentFields: commonAbsent, Purpose: "最小 24-bit BMP，验证 BMP 解码与竖图字段。"}, data: bmp},
	}, nil
}

func solidPNG(width, height int, c color.NRGBA) ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func addCompressedPNGText(raw []byte) ([]byte, error) {
	if !bytes.HasPrefix(raw, []byte(pngSignature)) || len(raw) < 12 || string(raw[len(raw)-8:len(raw)-4]) != "IEND" {
		return nil, fmt.Errorf("invalid base PNG")
	}
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write([]byte("SEARCH_PNG_ZTXT_2026")); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	out := append([]byte(nil), raw[:len(raw)-12]...)
	out = appendPNGChunk(out, "zTXt", append([]byte("prompt\x00\x00"), compressed.Bytes()...))
	out = appendPNGChunk(out, "iTXt", append([]byte("parameters\x00\x00\x00en\x00parameters\x00"), []byte("SEARCH_PNG_ITXT_2026")...))
	out = append(out, raw[len(raw)-12:]...)
	return out, nil
}

func appendPNGChunk(dst []byte, kind string, body []byte) []byte {
	chunk := make([]byte, 12+len(body))
	binary.BigEndian.PutUint32(chunk[0:4], uint32(len(body)))
	copy(chunk[4:8], kind)
	copy(chunk[8:8+len(body)], body)
	binary.BigEndian.PutUint32(chunk[8+len(body):12+len(body)], crc32.ChecksumIEEE(chunk[4:8+len(body)]))
	return append(dst, chunk...)
}

func singleFrameGIF() ([]byte, error) {
	frame := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.RGBA{R: 230, G: 57, B: 70, A: 255}})
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			frame.SetColorIndex(x, y, 0)
		}
	}
	var buf bytes.Buffer
	if err := gif.Encode(&buf, frame, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeBMP(width, height int, c color.NRGBA) []byte {
	rowSize := ((width*3 + 3) / 4) * 4
	pixelOffset := 14 + 40
	fileSize := pixelOffset + rowSize*height
	out := make([]byte, fileSize)
	copy(out[0:2], "BM")
	binary.LittleEndian.PutUint32(out[2:6], uint32(fileSize))
	binary.LittleEndian.PutUint32(out[10:14], uint32(pixelOffset))
	binary.LittleEndian.PutUint32(out[14:18], 40)
	binary.LittleEndian.PutUint32(out[18:22], uint32(width))
	binary.LittleEndian.PutUint32(out[22:26], uint32(height))
	binary.LittleEndian.PutUint16(out[26:28], 1)
	binary.LittleEndian.PutUint16(out[28:30], 24)
	binary.LittleEndian.PutUint32(out[34:38], uint32(rowSize*height))
	for y := 0; y < height; y++ {
		row := pixelOffset + (height-1-y)*rowSize
		for x := 0; x < width; x++ {
			out[row+x*3+0] = c.B
			out[row+x*3+1] = c.G
			out[row+x*3+2] = c.R
		}
	}
	return out
}

func writeNew(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
