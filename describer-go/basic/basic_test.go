// 文件：describer-go/basic/basic_test.go —— cod-basic 单元测试
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package basic

import "testing"

func TestNamePattern(t *testing.T) {
	cases := map[string]string{
		"IMG_1234.jpg":        "camera",
		"DSC00856.png":        "camera",
		"截图2026-09-01.png":    "screenshot",
		"Screenshot_12.png":   "screenshot",
		"2026-09-01_2353.md":  "timestamped",
		"20260901_235301.txt": "timestamped",
		"deadbeefdeadbeef":    "hashlike",
		"report_v1.2.pdf":     "versioned",
		"随便什么.md":             "plain",
	}
	for name, want := range cases {
		if got := namePattern(name); got != want {
			t.Errorf("namePattern(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestLooksTexty(t *testing.T) {
	if looksTexty([]byte{0x00, 0x01}) {
		t.Fatal("NUL byte must be binary")
	}
	if !looksTexty([]byte("你好，世界 hello\n")) {
		t.Fatal("plain utf-8 text should be texty")
	}
	ctrl := make([]byte, 100)
	for i := range ctrl {
		ctrl[i] = 0x1F
	}
	if looksTexty(ctrl) {
		t.Fatal("control-char heavy bytes must not be texty")
	}
	// UTF-16 BOM：属文本编码（高低位交替 NUL 是其正常形态）
	if !looksTexty([]byte{0xFF, 0xFE, 'h', 0x00, 'i', 0x00}) {
		t.Fatal("utf-16le BOM must be texty")
	}
	if !looksTexty([]byte{0xFE, 0xFF, 0x00, 'h', 0x00, 'i'}) {
		t.Fatal("utf-16be BOM must be texty")
	}
	// UTF-32 LE BOM（FF FE 00 00）不认：仍按 NUL 判二进制
	if looksTexty([]byte{0xFF, 0xFE, 0x00, 0x00, 0x00, 0x00, 'A', 0x00, 0x00, 0x00}) {
		t.Fatal("utf-32le BOM must stay binary")
	}
}

func TestMimeMatch(t *testing.T) {
	if m, ok := mimeMatch("text/plain; charset=utf-8", "text/plain"); !ok || !m {
		t.Fatalf("text/plain with params should match, got %v %v", m, ok)
	}
	if m, ok := mimeMatch("image/jpeg", "image/png"); ok && m {
		t.Fatal("jpeg vs png must be mismatch")
	}
	if _, ok := mimeMatch("application/octet-stream", "image/png"); ok {
		t.Fatal("octet-stream sniff is unknowable, ok must be false")
	}
	if m, ok := mimeMatch("text/plain; charset=utf-8", "application/json"); !ok || !m {
		t.Fatalf("json sniffed as text/plain should match, got %v %v", m, ok)
	}
}
