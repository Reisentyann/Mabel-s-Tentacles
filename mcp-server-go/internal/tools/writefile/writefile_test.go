// 文件：mcp-server-go/internal/tools/writefile/writefile_test.go —— writefile/upload_file Base64 与 Data URL 解码测试
// 修改：2026-10-14（日期由 fresh-header.ps1 刷新）

package writefile

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestDecodeBase64Payload(t *testing.T) {
	want := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x01, 0x02, 0x03}
	std := base64.StdEncoding.EncodeToString(want)
	raw := base64.RawStdEncoding.EncodeToString(want)
	dataURL := "data:image/png;base64," + std[:4] + "\n" + std[4:]

	for _, tc := range []struct {
		name  string
		input string
	}{
		{"std", std},
		{"raw_no_padding", raw},
		{"data_url_with_newline", dataURL},
	} {
		got, err := DecodeBase64Payload(tc.input)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: got %x, want %x", tc.name, got, want)
		}
	}

	if _, err := DecodeBase64Payload("   "); err == nil {
		t.Fatal("expected error on empty base64 payload")
	}
}
