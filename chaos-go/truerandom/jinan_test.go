// 文件：chaos-go/truerandom/jinan_test.go —— 济南 DIQRNG 信标客户端单测：解析/元数据/服务错/HTTP 错
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package truerandom

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const jinanSampleValue = "e3b16da07479ce8b8b0abda22b80eb01c46e92e8d01cb567330b897dd779a5fa" +
	"9e8fb81903660cd03585e2f0b46136473cf8abc8aa087445538ea2328d065151"

func jinanServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestJinanBytes(t *testing.T) {
	body := `{"code":0,"msg":"","data":{"output_value":"` + jinanSampleValue + `",` +
		`"output_value_pqc":"aa","pulse_index":609341,"timestamp":"2025-10-17 02:14:00",` +
		`"type":"DI QRNG","chsh":"2.0194599628448","nist_test":"Pass"}}`
	srv := jinanServer(t, body, http.StatusOK)
	defer srv.Close()

	j := NewJinan(srv.URL, 5*time.Second)
	if j.Name() != "quantum:cn-jinan" {
		t.Fatalf("来源名 = %q", j.Name())
	}
	b, err := j.Bytes(8)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString(jinanSampleValue)
	for i := range b {
		if b[i] != want[i] {
			t.Fatalf("第 %d 字节 = %x, want %x", i, b[i], want[i])
		}
	}
	info := j.Info()
	if info["pulse_index"] != int64(609341) || info["type"] != "DI QRNG" {
		t.Fatalf("元数据异常: %+v", info)
	}

	if _, err := j.Bytes(33); err == nil {
		t.Fatal(">32 字节应报错")
	}
	if _, err := j.Bytes(0); err == nil {
		t.Fatal("0 字节应报错")
	}
}

func TestJinanServiceError(t *testing.T) {
	srv := jinanServer(t, `{"code":1,"msg":"Undefined index: external_id","data":""}`, http.StatusOK)
	defer srv.Close()
	if _, err := NewJinan(srv.URL, time.Second).Bytes(8); err == nil {
		t.Fatal("code!=0 应报错")
	}
}

func TestJinanHTTPError(t *testing.T) {
	srv := jinanServer(t, "nope", http.StatusInternalServerError)
	defer srv.Close()
	if _, err := NewJinan(srv.URL, time.Second).Bytes(8); err == nil {
		t.Fatal("HTTP 500 应报错")
	}
}
