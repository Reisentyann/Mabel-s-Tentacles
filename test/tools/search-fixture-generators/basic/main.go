package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	fixtureRoot     = "test/fixtures/检索字段/基础"
	maxFileBytes    = 2 * 1024
	maxBatchBytes   = 32 * 1024
	highEntropySeed = uint64(0x4d4142454c323536)
)

type fixture struct {
	name    string
	data    []byte
	pattern string
}

type profile struct {
	mime       string
	textish    bool
	executable bool
}

type mtimeSpec struct {
	Fixed  string `json:"fixed"`
	Unix   int64  `json:"unix"`
	Others string `json:"others"`
}

type manifest struct {
	Version  int            `json:"v"`
	Root     string         `json:"root"`
	Schema   string         `json:"schema"`
	Profiles map[string]any `json:"p"`
	MTime    mtimeSpec      `json:"mtime"`
	Entropy  string         `json:"entropy"`
	PRNG     string         `json:"prng"`
	Platform string         `json:"platform"`
	Limits   []string       `json:"limits"`
	Files    [][]any        `json:"files"`
}

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, "generate basic fixtures:", err)
		os.Exit(1)
	}
}

func generate() error {
	fixedTime := time.Date(2026, time.November, 24, 12, 34, 56, 0, time.UTC)
	fixtures := []fixture{
		{name: "IMG_20261124_123456.bin", data: []byte("SEARCH_BASIC_CAMERA_2026\n"), pattern: "camera"},
		{name: "Screenshot_2026-11-24-123456.bin", data: []byte("SEARCH_BASIC_SCREENSHOT_2026\n"), pattern: "screenshot"},
		{name: "2026-11-24T12-34-56.log", data: []byte("SEARCH_BASIC_TIMESTAMPED_2026\n"), pattern: "timestamped"},
		{name: "550e8400-e29b-41d4-a716-446655440000.txt", data: []byte("SEARCH_BASIC_UUID_2026\n"), pattern: "uuid-like"},
		{name: "release-v1.2.3.txt", data: []byte("SEARCH_BASIC_VERSIONED_2026\n"), pattern: "versioned"},
		{name: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef.bin", data: []byte("SEARCH_BASIC_HASHLIKE_2026\n"), pattern: "hashlike"},
		{name: "plain.txt", data: []byte("SEARCH_BASIC_PLAIN_2026\n"), pattern: "plain"},
		{name: "low-entropy.bin", data: make([]byte, 512), pattern: "plain"},
		{name: "high-entropy.bin", data: deterministicHighEntropy(), pattern: "plain"},
		{name: "empty.bin", data: []byte{}, pattern: "plain"},
		{name: "executable.sh", data: []byte("#!/bin/sh\nprintf '%s\\n' 'SEARCH_BASIC_SCRIPT_2026'\n"), pattern: "plain"},
	}

	profiles := map[string]profile{
		"t": {mime: "text/plain; charset=utf-8", textish: true, executable: false},
		"b": {mime: "application/octet-stream", textish: false, executable: false},
	}
	manifestProfiles := make(map[string]any, len(profiles))
	for code, p := range profiles {
		manifestProfiles[code] = []any{p.mime, p.textish, p.executable}
	}

	m := manifest{
		Version:  1,
		Root:     fixtureRoot,
		Schema:   "path,bytes,sha256,pattern,p,entropy; p=[mime,textish,executable]",
		Profiles: manifestProfiles,
		MTime: mtimeSpec{
			Fixed:  "2026-11-24T12-34-56.log",
			Unix:   fixedTime.Unix(),
			Others: "generation-time",
		},
		Entropy:  "Shannon(head<=512B) bits/byte, Round2; null=omitted(empty)",
		PRNG:     "xorshift64*: x^=x>>12; x^=x<<25; x^=x>>27; byte(x*0x9e3779b97f4a7c15); seed=0x4d4142454c323536",
		Platform: runtime.GOOS,
		Limits: []string{
			"Windows mode bits unreliable; no chmod/ACL/permission changes.",
			"executable=PE/ELF/Mach-O magic, not mode bits; shebang=false.",
			"manifest omits self-hash but counts toward limits.",
		},
	}

	sampleBytes := 0
	for _, f := range fixtures {
		if len(f.data) > maxFileBytes {
			return fmt.Errorf("%s exceeds %d bytes", f.name, maxFileBytes)
		}
		code, err := profileCode(f.data, profiles)
		if err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
		var entropy any
		if len(f.data) > 0 {
			entropy = round2(shannonEntropy(f.data))
		}
		m.Files = append(m.Files, []any{f.name, len(f.data), sha256Hex(f.data), f.pattern, code, entropy})
		sampleBytes += len(f.data)
	}

	manifestBytes, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if len(manifestBytes) > maxFileBytes {
		return fmt.Errorf("manifest.json is %d bytes, exceeding %d", len(manifestBytes), maxFileBytes)
	}
	if sampleBytes+len(manifestBytes) > maxBatchBytes {
		return fmt.Errorf("batch is %d bytes, exceeding %d", sampleBytes+len(manifestBytes), maxBatchBytes)
	}

	paths := make([]string, 0, len(fixtures)+1)
	for _, f := range fixtures {
		paths = append(paths, filepath.Join(fixtureRoot, f.name))
	}
	manifestPath := filepath.Join(fixtureRoot, "manifest.json")
	paths = append(paths, manifestPath)
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing to overwrite existing file: %s", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
	}

	for _, f := range fixtures {
		path := filepath.Join(fixtureRoot, f.name)
		if err := writeNew(path, f.data); err != nil {
			return err
		}
		fmt.Printf("%s\t%d bytes\t%s\n", filepath.ToSlash(filepath.Join(fixtureRoot, f.name)), len(f.data), sha256Hex(f.data))
		if f.name == m.MTime.Fixed {
			if err := os.Chtimes(path, fixedTime, fixedTime); err != nil {
				return fmt.Errorf("set fixed mtime on %s: %w", path, err)
			}
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("stat %s: %w", path, err)
			}
			if info.ModTime().Unix() != fixedTime.Unix() {
				return fmt.Errorf("fixed mtime on %s is %d, want %d", path, info.ModTime().Unix(), fixedTime.Unix())
			}
		}
	}
	if err := writeNew(manifestPath, manifestBytes); err != nil {
		return err
	}
	fmt.Printf("%s\t%d bytes\t%s\n", filepath.ToSlash(manifestPath), len(manifestBytes), sha256Hex(manifestBytes))
	fmt.Printf("batch: %d bytes; fixed mtime: %s (%d)\n", sampleBytes+len(manifestBytes), m.MTime.Fixed, m.MTime.Unix)
	return nil
}

func deterministicHighEntropy() []byte {
	const multiplier = uint64(0x9e3779b97f4a7c15)
	state := highEntropySeed
	out := make([]byte, 512)
	for i := range out {
		state ^= state >> 12
		state ^= state << 25
		state ^= state >> 27
		out[i] = byte(state * multiplier)
	}
	return out
}

func profileCode(data []byte, profiles map[string]profile) (string, error) {
	actual := profile{
		mime:       http.DetectContentType(data),
		textish:    looksTextish(data),
		executable: looksExecutable(data),
	}
	for code, want := range profiles {
		if actual == want {
			return code, nil
		}
	}
	return "", fmt.Errorf("unexpected feature profile: %+v", actual)
}

func looksTextish(head []byte) bool {
	if len(head) == 0 {
		return true
	}
	controls := 0
	for _, b := range head {
		if b == 0 {
			return false
		}
		if b < 0x09 || (b > 0x0d && b < 0x20) {
			controls++
		}
	}
	return float64(controls)/float64(len(head)) <= 0.15
}

func looksExecutable(head []byte) bool {
	if len(head) < 4 {
		return false
	}
	if head[0] == 'M' && head[1] == 'Z' {
		return true
	}
	if head[0] == 0x7f && head[1] == 'E' && head[2] == 'L' && head[3] == 'F' {
		return true
	}
	magic := [][4]byte{
		{0xfe, 0xed, 0xfa, 0xce}, {0xce, 0xfa, 0xed, 0xfe},
		{0xfe, 0xed, 0xfa, 0xcf}, {0xcf, 0xfa, 0xed, 0xfe},
		{0xca, 0xfe, 0xba, 0xbe},
	}
	for _, want := range magic {
		if [4]byte{head[0], head[1], head[2], head[3]} == want {
			return true
		}
	}
	return false
}

func shannonEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}
	var counts [256]int
	for _, b := range data {
		counts[b]++
	}
	entropy := 0.0
	for _, count := range counts {
		if count == 0 {
			continue
		}
		p := float64(count) / float64(len(data))
		entropy -= p * math.Log2(p)
	}
	return entropy
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeNew(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
