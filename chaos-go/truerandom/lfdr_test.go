// 文件：chaos-go/truerandom/lfdr_test.go —— lfdr.de 客户端单测：解析/元数据/HTTP 错/坏 hex
// 修改：2026-09-12（日期由 fresh-header.ps1 刷新）

package truerandom

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func lfdrServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestLFDRBytes(t *testing.T) {
	srv := lfdrServer(t, `{"length":8,"qrn":"f65c925098a5457d"}`, http.StatusOK)
	defer srv.Close()

	l := NewLFDR(srv.URL, 5*time.Second)
	if l.Name() != "quantum:de-lfdr" {
		t.Fatalf("来源名 = %q", l.Name())
	}
	b, err := l.Bytes(8)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 8 || b[0] != 0xf6 || b[7] != 0x7d {
		t.Fatalf("字节异常: %x", b)
	}
	if l.Info()["length"] != 8 {
		t.Fatalf("元数据异常: %+v", l.Info())
	}
	if _, err := l.Bytes(1025); err == nil {
		t.Fatal(">1024 应报错")
	}
}

func TestLFDRHTTPError(t *testing.T) {
	srv := lfdrServer(t, "nope", http.StatusInternalServerError)
	defer srv.Close()
	if _, err := NewLFDR(srv.URL, time.Second).Bytes(4); err == nil {
		t.Fatal("HTTP 500 应报错")
	}
}

func TestLFDRBadHex(t *testing.T) {
	srv := lfdrServer(t, `{"length":4,"qrn":"zzzz"}`, http.StatusOK)
	defer srv.Close()
	if _, err := NewLFDR(srv.URL, time.Second).Bytes(4); err == nil {
		t.Fatal("坏 hex 应报错")
	}
}
