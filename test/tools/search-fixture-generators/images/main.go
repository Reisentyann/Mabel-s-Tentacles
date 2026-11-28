package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	maxFileBytes  = int64(64 << 10)
	maxBatchBytes = int64(512 << 10)
	pngSignature  = "\x89PNG\r\n\x1a\n"
	exifDateTime  = "2026:11:24 12:34:56"
)

var commonImageFields = []string{
	"cod-image-width",
	"cod-image-height",
	"cod-image-aspect",
	"cod-image-orientation",
	"cod-image-megapixels",
	"cod-image-brightness",
	"cod-image-contrast",
	"cod-image-dark",
	"cod-image-light",
	"cod-image-warm-ratio",
	"cod-image-cool-ratio",
	"cod-image-skin-tone-ratio",
	"cod-image-alpha-ratio",
	"cod-image-color-count",
	"cod-image-flat-ratio",
	"cod-image-entropy",
	"cod-image-palette",
	"cod-image-dominant",
	"cod-image-saturation",
	"cod-image-family",
	"cod-image-edge-density",
	"cod-image-sharpness",
	"cod-image-symmetry",
}

type dimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type fixtureEntry struct {
	Path                       string         `json:"path"`
	MediaType                  string         `json:"media_type"`
	Bytes                      int64          `json:"bytes"`
	SHA256                     string         `json:"sha256"`
	Dimensions                 *dimensions    `json:"dimensions,omitempty"`
	Frames                     int            `json:"frames,omitempty"`
	Decodable                  bool           `json:"decodable"`
	CommonImageFieldsApply     bool           `json:"common_image_fields_apply"`
	AllCommonImageFieldsAbsent bool           `json:"all_common_image_fields_absent,omitempty"`
	ExpectedFields             map[string]any `json:"expected_fields"`
	ExpectedAbsentFields       []string       `json:"expected_absent_fields,omitempty"`
	Purpose                    string         `json:"purpose"`
}

type fixtureManifest struct {
	SchemaVersion     int            `json:"schema_version"`
	PathBase          string         `json:"path_base"`
	Generator         string         `json:"generator"`
	CommonImageFields []string       `json:"common_image_fields"`
	FixtureBytes      int64          `json:"fixture_bytes"`
	Files             []fixtureEntry `json:"files"`
}

type generatedFixture struct {
	Meta fixtureEntry
	Data []byte
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "generate image fixtures: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	outputDir := filepath.Join("test", "fixtures", "检索字段", "图像")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	solid, err := encodeSolidPNG(8, 8, color.NRGBA{R: 231, G: 138, B: 60, A: 255})
	if err != nil {
		return fmt.Errorf("encode solid-warm.png: %w", err)
	}
	transparent, err := encodeTransparentPNG()
	if err != nil {
		return fmt.Errorf("encode transparent.png: %w", err)
	}
	metadataBase, err := encodeSolidPNG(8, 8, color.NRGBA{R: 112, G: 48, B: 160, A: 255})
	if err != nil {
		return fmt.Errorf("encode metadata.png base: %w", err)
	}
	metadata, err := insertPNGText(metadataBase,
		[2]string{"prompt", "SEARCH_PNG_PROMPT_2026"},
		[2]string{"parameters", "SEARCH_PNG_PARAMS_2026"},
	)
	if err != nil {
		return fmt.Errorf("insert metadata.png text: %w", err)
	}
	exifJPEG, err := encodeEXIFJPEG()
	if err != nil {
		return fmt.Errorf("encode exif-sample.jpg: %w", err)
	}
	animated, err := encodeAnimatedGIF()
	if err != nil {
		return fmt.Errorf("encode animated.gif: %w", err)
	}
	corrupt := encodeCorruptPNG()

	if err := validateSolidPNG(solid, 8, 8, color.NRGBA{R: 231, G: 138, B: 60, A: 255}); err != nil {
		return fmt.Errorf("validate solid-warm.png: %w", err)
	}
	if err := validateTransparentPNG(transparent); err != nil {
		return fmt.Errorf("validate transparent.png: %w", err)
	}
	if err := validateMetadataPNG(metadata); err != nil {
		return fmt.Errorf("validate metadata.png: %w", err)
	}
	if err := validateEXIFJPEG(exifJPEG); err != nil {
		return fmt.Errorf("validate exif-sample.jpg: %w", err)
	}
	if err := validateAnimatedGIF(animated); err != nil {
		return fmt.Errorf("validate animated.gif: %w", err)
	}
	if err := validateCorruptPNG(corrupt); err != nil {
		return fmt.Errorf("validate corrupt.png: %w", err)
	}

	takenAt, err := time.Parse("2006:01:02 15:04:05", exifDateTime)
	if err != nil {
		return fmt.Errorf("parse EXIF time: %w", err)
	}

	fixtures := []generatedFixture{
		{
			Data: solid,
			Meta: fixtureEntry{
				Path:                   "solid-warm.png",
				MediaType:              "image/png",
				Dimensions:             &dimensions{Width: 8, Height: 8},
				Decodable:              true,
				CommonImageFieldsApply: true,
				ExpectedFields: map[string]any{
					"cod-image-width":       8,
					"cod-image-height":      8,
					"cod-image-aspect":      "1:1",
					"cod-image-orientation": "square",
					"cod-image-warm-ratio":  1,
					"cod-image-cool-ratio":  0,
					"cod-image-alpha-ratio": 0,
					"cod-image-color-count": 1,
					"cod-image-flat-ratio":  1,
					"cod-image-entropy":     0,
					"cod-image-dominant":    "#e78a3c",
					"cod-image-family":      "orange",
				},
				ExpectedAbsentFields: []string{
					"cod-image-has-alpha",
					"sp-cod-png-prompt",
					"sp-cod-png-parameters",
					"cod-image-taken-at",
					"cod-image-camera",
					"cod-image-software",
					"cod-image-animated",
					"cod-image-frames",
				},
				Purpose: "8×8 暖色纯色不透明 PNG；用于尺寸、纯色/扁平率、暖色与无透明通道检索。",
			},
		},
		{
			Data: transparent,
			Meta: fixtureEntry{
				Path:                   "transparent.png",
				MediaType:              "image/png",
				Dimensions:             &dimensions{Width: 4, Height: 4},
				Decodable:              true,
				CommonImageFieldsApply: true,
				ExpectedFields: map[string]any{
					"cod-image-width":       4,
					"cod-image-height":      4,
					"cod-image-has-alpha":   true,
					"cod-image-alpha-ratio": 0.5,
					"cod-image-warm-ratio":  0.5,
					"cod-image-cool-ratio":  0.5,
					"cod-image-color-count": 2,
					"cod-image-flat-ratio":  1,
					"cod-image-entropy":     1,
				},
				ExpectedAbsentFields: []string{
					"sp-cod-png-prompt",
					"sp-cod-png-parameters",
					"cod-image-taken-at",
					"cod-image-camera",
					"cod-image-software",
					"cod-image-animated",
					"cod-image-frames",
				},
				Purpose: "4×4 棋盘格 PNG，一半像素 Alpha=96；用于 has-alpha 与 alpha-ratio 检索。",
			},
		},
		{
			Data: metadata,
			Meta: fixtureEntry{
				Path:                   "metadata.png",
				MediaType:              "image/png",
				Dimensions:             &dimensions{Width: 8, Height: 8},
				Decodable:              true,
				CommonImageFieldsApply: true,
				ExpectedFields: map[string]any{
					"cod-image-width":       8,
					"cod-image-height":      8,
					"cod-image-alpha-ratio": 0,
					"cod-image-color-count": 1,
					"cod-image-flat-ratio":  1,
					"cod-image-dominant":    "#7030a0",
					"cod-image-family":      "purple",
					"sp-cod-png-prompt":     "SEARCH_PNG_PROMPT_2026",
					"sp-cod-png-parameters": "SEARCH_PNG_PARAMS_2026",
				},
				ExpectedAbsentFields: []string{
					"cod-image-has-alpha",
					"cod-image-taken-at",
					"cod-image-camera",
					"cod-image-software",
					"cod-image-animated",
					"cod-image-frames",
				},
				Purpose: "8×8 PNG 内嵌 prompt/parameters tEXt；用唯一标记验证 sp-cod-png-* 检索。",
			},
		},
		{
			Data: exifJPEG,
			Meta: fixtureEntry{
				Path:                   "exif-sample.jpg",
				MediaType:              "image/jpeg",
				Dimensions:             &dimensions{Width: 8, Height: 8},
				Decodable:              true,
				CommonImageFieldsApply: true,
				ExpectedFields: map[string]any{
					"cod-image-width":    8,
					"cod-image-height":   8,
					"cod-image-camera":   "SEARCH_CAMERA_2026 SEARCH_MODEL_2026",
					"cod-image-software": "SEARCH_SOFTWARE_2026",
					"cod-image-taken-at": takenAt.Unix(),
				},
				ExpectedAbsentFields: []string{
					"sp-cod-png-prompt",
					"sp-cod-png-parameters",
					"cod-image-animated",
					"cod-image-frames",
				},
				Purpose: "8×8 JPEG 内嵌最小 EXIF；验证 camera/software/taken-at 固定字段及唯一相机标记。",
			},
		},
		{
			Data: animated,
			Meta: fixtureEntry{
				Path:                   "animated.gif",
				MediaType:              "image/gif",
				Dimensions:             &dimensions{Width: 4, Height: 4},
				Frames:                 2,
				Decodable:              true,
				CommonImageFieldsApply: true,
				ExpectedFields: map[string]any{
					"cod-image-width":       4,
					"cod-image-height":      4,
					"cod-image-animated":    true,
					"cod-image-frames":      2,
					"cod-image-color-count": 1,
					"cod-image-dominant":    "#e63946",
				},
				ExpectedAbsentFields: []string{
					"cod-image-has-alpha",
					"sp-cod-png-prompt",
					"sp-cod-png-parameters",
					"cod-image-taken-at",
					"cod-image-camera",
					"cod-image-software",
				},
				Purpose: "4×4 两帧 GIF，红/蓝纯色帧；验证多帧 animated/frames。",
			},
		},
		{
			Data: corrupt,
			Meta: fixtureEntry{
				Path:                       "corrupt.png",
				MediaType:                  "image/png",
				Decodable:                  false,
				CommonImageFieldsApply:     false,
				AllCommonImageFieldsAbsent: true,
				ExpectedFields: map[string]any{
					"cod-basic-mime": "image/png",
					"cod-image-ver":  3,
				},
				ExpectedAbsentFields: []string{
					"sp-cod-png-prompt",
					"sp-cod-png-parameters",
					"cod-image-animated",
					"cod-image-frames",
				},
				Purpose: "保留 PNG 魔数，IHDR 尺寸为 0 且 CRC 损坏；验证 MIME 命中 image 后解码失败、图像事实为零。",
			},
		},
	}

	var fixtureBytes int64
	for i := range fixtures {
		data := fixtures[i].Data
		if int64(len(data)) > maxFileBytes {
			return fmt.Errorf("%s exceeds 64 KiB: %d bytes", fixtures[i].Meta.Path, len(data))
		}
		fixtureBytes += int64(len(data))
		if fixtureBytes > maxBatchBytes {
			return fmt.Errorf("fixture batch exceeds 512 KiB: %d bytes", fixtureBytes)
		}
		digest := sha256.Sum256(data)
		fixtures[i].Meta.Bytes = int64(len(data))
		fixtures[i].Meta.SHA256 = hex.EncodeToString(digest[:])
	}

	manifest := fixtureManifest{
		SchemaVersion:     1,
		PathBase:          ".",
		Generator:         "test/tools/search-fixture-generators/images/main.go",
		CommonImageFields: commonImageFields,
		FixtureBytes:      fixtureBytes,
		Files:             make([]fixtureEntry, len(fixtures)),
	}
	for i := range fixtures {
		manifest.Files[i] = fixtures[i].Meta
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')
	if int64(len(manifestData)) > maxFileBytes {
		return fmt.Errorf("manifest.json exceeds 64 KiB: %d bytes", len(manifestData))
	}
	if fixtureBytes+int64(len(manifestData)) > maxBatchBytes {
		return fmt.Errorf("fixture batch plus manifest exceeds 512 KiB: %d bytes", fixtureBytes+int64(len(manifestData)))
	}

	paths := make([]string, 0, len(fixtures)+1)
	for i := range fixtures {
		paths = append(paths, filepath.Join(outputDir, fixtures[i].Meta.Path))
	}
	manifestPath := filepath.Join(outputDir, "manifest.json")
	paths = append(paths, manifestPath)
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing to overwrite existing file: %s", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect output path %s: %w", path, err)
		}
	}

	for i := range fixtures {
		path := filepath.Join(outputDir, fixtures[i].Meta.Path)
		if err := writeNewFile(path, fixtures[i].Data); err != nil {
			return err
		}
		fmt.Printf("wrote %s\t%d bytes\tsha256=%s\n", fixtures[i].Meta.Path, fixtures[i].Meta.Bytes, fixtures[i].Meta.SHA256)
	}
	if err := writeNewFile(manifestPath, manifestData); err != nil {
		return err
	}
	fmt.Printf("wrote manifest.json\t%d bytes\n", len(manifestData))
	fmt.Printf("fixture bytes=%d; batch bytes including manifest=%d\n", fixtureBytes, fixtureBytes+int64(len(manifestData)))
	return nil
}

func encodeSolidPNG(width, height int, c color.NRGBA) ([]byte, error) {
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

func encodeTransparentPNG() ([]byte, error) {
	const size = 4
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	opaque := color.NRGBA{R: 231, G: 138, B: 60, A: 255}
	translucent := color.NRGBA{R: 40, G: 120, B: 220, A: 96}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if (x+y)%2 == 0 {
				img.SetNRGBA(x, y, opaque)
			} else {
				img.SetNRGBA(x, y, translucent)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func insertPNGText(raw []byte, texts ...[2]string) ([]byte, error) {
	if !bytes.HasPrefix(raw, []byte(pngSignature)) || len(raw) < 12 {
		return nil, errors.New("invalid base PNG")
	}
	if string(raw[len(raw)-8:len(raw)-4]) != "IEND" {
		return nil, errors.New("base PNG does not end with IEND")
	}
	out := append([]byte(nil), raw[:len(raw)-12]...)
	for _, text := range texts {
		if text[0] == "" || text[1] == "" || bytes.IndexByte([]byte(text[0]), 0) >= 0 || bytes.IndexByte([]byte(text[1]), 0) >= 0 {
			return nil, fmt.Errorf("invalid tEXt key/value: %q", text)
		}
		body := append([]byte(text[0]), 0)
		body = append(body, []byte(text[1])...)
		chunk := make([]byte, 12+len(body))
		binary.BigEndian.PutUint32(chunk[0:4], uint32(len(body)))
		copy(chunk[4:8], "tEXt")
		copy(chunk[8:8+len(body)], body)
		binary.BigEndian.PutUint32(chunk[8+len(body):12+len(body)], crc32.ChecksumIEEE(chunk[4:8+len(body)]))
		out = append(out, chunk...)
	}
	out = append(out, raw[len(raw)-12:]...)
	return out, nil
}

func encodeEXIFJPEG() ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	c := color.NRGBA{R: 70, G: 130, B: 180, A: 255}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	raw := jpegBuf.Bytes()
	if len(raw) < 2 || !bytes.Equal(raw[:2], []byte{0xff, 0xd8}) {
		return nil, errors.New("JPEG encoder did not produce SOI")
	}
	tiff := buildEXIF(
		"SEARCH_CAMERA_2026",
		"SEARCH_MODEL_2026",
		"SEARCH_SOFTWARE_2026",
		exifDateTime,
	)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	if len(payload)+2 > 0xffff {
		return nil, errors.New("EXIF APP1 segment too large")
	}
	out := make([]byte, 0, len(raw)+len(payload)+4)
	out = append(out, raw[:2]...)
	out = append(out, 0xff, 0xe1)
	var segmentLength [2]byte
	binary.BigEndian.PutUint16(segmentLength[:], uint16(len(payload)+2))
	out = append(out, segmentLength[:]...)
	out = append(out, payload...)
	out = append(out, raw[2:]...)
	return out, nil
}

func buildEXIF(makeValue, modelValue, softwareValue, dateValue string) []byte {
	tiff := make([]byte, 0, 256)
	tiff = append(tiff, 'I', 'I', 0x2a, 0x00, 8, 0, 0, 0)

	rootIFD := len(tiff)
	tiff = append(tiff, 4, 0) // Four IFD0 entries.
	tiff = append(tiff, make([]byte, 4*12+4)...)

	appendASCII := func(value string) uint32 {
		if len(tiff)%2 != 0 {
			tiff = append(tiff, 0)
		}
		offset := uint32(len(tiff))
		tiff = append(tiff, value...)
		tiff = append(tiff, 0)
		return offset
	}
	makeOffset := appendASCII(makeValue)
	modelOffset := appendASCII(modelValue)
	softwareOffset := appendASCII(softwareValue)

	if len(tiff)%2 != 0 {
		tiff = append(tiff, 0)
	}
	exifIFD := len(tiff)
	tiff = append(tiff, 1, 0) // One Exif IFD entry.
	tiff = append(tiff, make([]byte, 12+4)...)
	dateOffset := appendASCII(dateValue)

	putEntry := func(base int, tag, fieldType uint16, count, value uint32) {
		entry := tiff[base : base+12]
		binary.LittleEndian.PutUint16(entry[0:2], tag)
		binary.LittleEndian.PutUint16(entry[2:4], fieldType)
		binary.LittleEndian.PutUint32(entry[4:8], count)
		binary.LittleEndian.PutUint32(entry[8:12], value)
	}
	entryBase := rootIFD + 2
	putEntry(entryBase+0*12, 0x010f, 2, uint32(len(makeValue)+1), makeOffset)         // Make.
	putEntry(entryBase+1*12, 0x0110, 2, uint32(len(modelValue)+1), modelOffset)       // Model.
	putEntry(entryBase+2*12, 0x0131, 2, uint32(len(softwareValue)+1), softwareOffset) // Software.
	putEntry(entryBase+3*12, 0x8769, 4, 1, uint32(exifIFD))                           // ExifIFDPointer.
	putEntry(exifIFD+2, 0x9003, 2, uint32(len(dateValue)+1), dateOffset)              // DateTimeOriginal.
	return tiff
}

func encodeAnimatedGIF() ([]byte, error) {
	const size = 4
	colors := []color.RGBA{
		{R: 230, G: 57, B: 70, A: 255},
		{R: 69, G: 123, B: 157, A: 255},
	}
	frames := make([]*image.Paletted, len(colors))
	for i, c := range colors {
		frame := image.NewPaletted(image.Rect(0, 0, size, size), color.Palette{c})
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				frame.SetColorIndex(x, y, 0)
			}
		}
		frames[i] = frame
	}
	animation := &gif.GIF{
		Image:     frames,
		Delay:     []int{40, 40},
		LoopCount: 0,
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, animation); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeCorruptPNG() []byte {
	data := append([]byte(pngSignature), make([]byte, 25)...)
	binary.BigEndian.PutUint32(data[8:12], 13) // IHDR length.
	copy(data[12:16], "IHDR")
	// Zero width/height make the IHDR invalid; bit depth/color type are plausible.
	data[16+8] = 8
	data[16+9] = 6
	// The CRC remains zero and is intentionally invalid.
	return data
}

func validateSolidPNG(data []byte, width, height int, want color.NRGBA) error {
	img, err := decodeAndCheck(data, "png", dimensions{Width: width, Height: height})
	if err != nil {
		return err
	}
	if _, err := inspectPNG(data); err != nil {
		return err
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if uint8(r>>8) != want.R || uint8(g>>8) != want.G || uint8(b>>8) != want.B || uint8(a>>8) != want.A {
				return fmt.Errorf("unexpected pixel at (%d,%d)", x, y)
			}
		}
	}
	return nil
}

func validateTransparentPNG(data []byte) error {
	img, err := decodeAndCheck(data, "png", dimensions{Width: 4, Height: 4})
	if err != nil {
		return err
	}
	if _, err := inspectPNG(data); err != nil {
		return err
	}
	opaque, translucent := 0, 0
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			switch uint8(a >> 8) {
			case 255:
				opaque++
			case 96:
				translucent++
			default:
				return fmt.Errorf("unexpected alpha at (%d,%d): %d", x, y, uint8(a>>8))
			}
		}
	}
	if opaque != 8 || translucent != 8 {
		return fmt.Errorf("alpha counts: opaque=%d translucent=%d", opaque, translucent)
	}
	return nil
}

func validateMetadataPNG(data []byte) error {
	if _, err := decodeAndCheck(data, "png", dimensions{Width: 8, Height: 8}); err != nil {
		return err
	}
	texts, err := inspectPNG(data)
	if err != nil {
		return err
	}
	if texts["prompt"] != "SEARCH_PNG_PROMPT_2026" || texts["parameters"] != "SEARCH_PNG_PARAMS_2026" {
		return fmt.Errorf("unexpected tEXt values: %#v", texts)
	}
	return nil
}

func validateEXIFJPEG(data []byte) error {
	if _, err := decodeAndCheck(data, "jpeg", dimensions{Width: 8, Height: 8}); err != nil {
		return err
	}
	if len(data) < 10 || !bytes.Equal(data[:2], []byte{0xff, 0xd8}) || !bytes.Equal(data[2:4], []byte{0xff, 0xe1}) {
		return errors.New("missing EXIF APP1 after SOI")
	}
	segmentLength := int(binary.BigEndian.Uint16(data[4:6]))
	if segmentLength < 8 || 4+segmentLength > len(data) {
		return errors.New("invalid EXIF APP1 length")
	}
	payload := data[6 : 4+segmentLength]
	if !bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
		return errors.New("invalid EXIF APP1 signature")
	}
	return validateEXIFTIFF(payload[6:])
}

func validateAnimatedGIF(data []byte) error {
	animation, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if len(animation.Image) != 2 || len(animation.Delay) != 2 {
		return fmt.Errorf("frames=%d delays=%d", len(animation.Image), len(animation.Delay))
	}
	for i, frame := range animation.Image {
		if frame.Bounds() != image.Rect(0, 0, 4, 4) {
			return fmt.Errorf("frame %d bounds=%v", i, frame.Bounds())
		}
		if animation.Delay[i] != 40 {
			return fmt.Errorf("frame %d delay=%d", i, animation.Delay[i])
		}
	}
	r0, g0, b0, _ := animation.Image[0].At(0, 0).RGBA()
	r1, g1, b1, _ := animation.Image[1].At(0, 0).RGBA()
	dr := int(r0>>8) - int(r1>>8)
	dg := int(g0>>8) - int(g1>>8)
	db := int(b0>>8) - int(b1>>8)
	if dr*dr+dg*dg+db*db < 10000 {
		return errors.New("GIF frame colors are not clearly different")
	}
	return nil
}

func validateCorruptPNG(data []byte) error {
	if !bytes.HasPrefix(data, []byte(pngSignature)) {
		return errors.New("missing PNG signature")
	}
	if len(data) < 33 || binary.BigEndian.Uint32(data[8:12]) != 13 || string(data[12:16]) != "IHDR" {
		return errors.New("missing damaged IHDR")
	}
	if binary.BigEndian.Uint32(data[16:20]) != 0 || binary.BigEndian.Uint32(data[20:24]) != 0 {
		return errors.New("corrupt IHDR does not have zero dimensions")
	}
	if crc32.ChecksumIEEE(data[12:29]) == binary.BigEndian.Uint32(data[29:33]) {
		return errors.New("corrupt IHDR unexpectedly has a valid CRC")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err == nil {
		return errors.New("corrupt PNG unexpectedly decodes")
	}
	if mime := http.DetectContentType(data); mime != "image/png" {
		return fmt.Errorf("MIME=%q, want image/png", mime)
	}
	return nil
}

func decodeAndCheck(data []byte, wantFormat string, want dimensions) (image.Image, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if format != wantFormat {
		return nil, fmt.Errorf("format=%q, want %q", format, wantFormat)
	}
	if img.Bounds() != image.Rect(0, 0, want.Width, want.Height) {
		return nil, fmt.Errorf("bounds=%v, want %dx%d", img.Bounds(), want.Width, want.Height)
	}
	return img, nil
}

func inspectPNG(data []byte) (map[string]string, error) {
	if !bytes.HasPrefix(data, []byte(pngSignature)) {
		return nil, errors.New("missing PNG signature")
	}
	texts := make(map[string]string)
	position := 8
	first := true
	sawIEND := false
	for position < len(data) {
		if len(data)-position < 12 {
			return nil, errors.New("truncated PNG chunk")
		}
		length64 := binary.BigEndian.Uint32(data[position : position+4])
		if length64 > uint32(len(data)-position-12) {
			return nil, errors.New("PNG chunk length exceeds file")
		}
		length := int(length64)
		chunk := data[position : position+12+length]
		chunkType := string(chunk[4:8])
		if first && chunkType != "IHDR" {
			return nil, errors.New("first PNG chunk is not IHDR")
		}
		storedCRC := binary.BigEndian.Uint32(chunk[8+length : 12+length])
		actualCRC := crc32.ChecksumIEEE(chunk[4 : 8+length])
		if storedCRC != actualCRC {
			return nil, fmt.Errorf("bad CRC for %s", chunkType)
		}
		body := chunk[8 : 8+length]
		if chunkType == "tEXt" {
			separator := bytes.IndexByte(body, 0)
			if separator <= 0 || separator+1 >= len(body) {
				return nil, errors.New("invalid tEXt chunk")
			}
			key := string(body[:separator])
			if _, exists := texts[key]; exists {
				return nil, fmt.Errorf("duplicate tEXt key %q", key)
			}
			texts[key] = string(body[separator+1:])
		}
		position += 12 + length
		first = false
		if chunkType == "IEND" {
			if length != 0 || position != len(data) {
				return nil, errors.New("invalid IEND or trailing PNG data")
			}
			sawIEND = true
			break
		}
	}
	if !sawIEND {
		return nil, errors.New("missing IEND")
	}
	return texts, nil
}

func validateEXIFTIFF(data []byte) error {
	if len(data) < 8 || string(data[:2]) != "II" || binary.LittleEndian.Uint16(data[2:4]) != 42 {
		return errors.New("invalid little-endian TIFF header")
	}
	rootOffset := int(binary.LittleEndian.Uint32(data[4:8]))
	root, err := readIFD(data, rootOffset)
	if err != nil {
		return fmt.Errorf("read EXIF IFD0: %w", err)
	}
	checks := []struct {
		tag   uint16
		value string
	}{
		{0x010f, "SEARCH_CAMERA_2026"},
		{0x0110, "SEARCH_MODEL_2026"},
		{0x0131, "SEARCH_SOFTWARE_2026"},
	}
	for _, check := range checks {
		entry, ok := root[check.tag]
		if !ok {
			return fmt.Errorf("missing IFD0 tag %#x", check.tag)
		}
		value, err := tiffASCII(data, entry)
		if err != nil {
			return fmt.Errorf("read IFD0 tag %#x: %w", check.tag, err)
		}
		if value != check.value {
			return fmt.Errorf("IFD0 tag %#x=%q", check.tag, value)
		}
	}
	pointer, ok := root[0x8769]
	if !ok {
		return errors.New("missing ExifIFDPointer")
	}
	exifOffset, err := tiffLong(pointer)
	if err != nil {
		return fmt.Errorf("read ExifIFDPointer: %w", err)
	}
	exifIFD, err := readIFD(data, int(exifOffset))
	if err != nil {
		return fmt.Errorf("read Exif IFD: %w", err)
	}
	dateEntry, ok := exifIFD[0x9003]
	if !ok {
		return errors.New("missing DateTimeOriginal")
	}
	date, err := tiffASCII(data, dateEntry)
	if err != nil {
		return fmt.Errorf("read DateTimeOriginal: %w", err)
	}
	if date != exifDateTime {
		return fmt.Errorf("DateTimeOriginal=%q", date)
	}
	return nil
}

func readIFD(data []byte, offset int) (map[uint16][]byte, error) {
	if offset < 0 || offset+2 > len(data) {
		return nil, errors.New("IFD offset outside TIFF")
	}
	count := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
	if count > 256 || offset+2+count*12+4 > len(data) {
		return nil, errors.New("invalid IFD entry count")
	}
	entries := make(map[uint16][]byte, count)
	for i := 0; i < count; i++ {
		start := offset + 2 + i*12
		tag := binary.LittleEndian.Uint16(data[start : start+2])
		if _, exists := entries[tag]; exists {
			return nil, fmt.Errorf("duplicate IFD tag %#x", tag)
		}
		entries[tag] = data[start : start+12]
	}
	return entries, nil
}

func tiffASCII(data, entry []byte) (string, error) {
	if len(entry) != 12 || binary.LittleEndian.Uint16(entry[2:4]) != 2 {
		return "", errors.New("tag is not ASCII")
	}
	count := int(binary.LittleEndian.Uint32(entry[4:8]))
	if count <= 0 {
		return "", errors.New("empty ASCII value")
	}
	var raw []byte
	if count <= 4 {
		raw = entry[8 : 8+count]
	} else {
		offset := int(binary.LittleEndian.Uint32(entry[8:12]))
		if offset < 0 || offset+count > len(data) {
			return "", errors.New("ASCII offset outside TIFF")
		}
		raw = data[offset : offset+count]
	}
	if raw[len(raw)-1] != 0 {
		return "", errors.New("ASCII value is not NUL-terminated")
	}
	return string(raw[:len(raw)-1]), nil
}

func tiffLong(entry []byte) (uint32, error) {
	if len(entry) != 12 || binary.LittleEndian.Uint16(entry[2:4]) != 4 || binary.LittleEndian.Uint32(entry[4:8]) != 1 {
		return 0, errors.New("tag is not a single LONG")
	}
	return binary.LittleEndian.Uint32(entry[8:12]), nil
}

func writeNewFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	written, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write %s: %w", path, writeErr)
	}
	if written != len(data) {
		return fmt.Errorf("write %s: %d of %d bytes", path, written, len(data))
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
	}
	return nil
}
