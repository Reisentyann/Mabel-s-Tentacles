// 文件：chaos-go/truerandom/lfdr.go —— 真随机熵源·lfdr.de（ID Quantique 量子随机数硬件）
// 修改：2026-09-12（日期由 fresh-header.ps1 刷新）

// LFDR 是 lfdr.de 的量子随机数服务，后端为 ID Quantique 的 QRNG PCIe 硬件
// （量子物理熵源）。公开、免费、无需 key。
//
// 接口：
//
//	GET https://lfdr.de/qrng_api/qrng?length=N&format=HEX
//	→ {"length":N,"qrn":"<hex>"}
//
// 与信标类源不同，本服务每次请求即取新值（非按分钟复用的脉冲）。
package truerandom

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
)

const lfdrDefaultEndpoint = "https://lfdr.de/qrng_api/qrng"

// LFDR lfdr.de（ID Quantique 硬件）熵源。
type LFDR struct {
	endpoint string
	client   *http.Client

	mu   sync.Mutex
	meta map[string]any
}

var (
	_ chaos.EntropySource = (*LFDR)(nil)
	_ chaos.InfoSource    = (*LFDR)(nil)
)

// NewLFDR 构造 lfdr 熵源。endpoint 空 = 内置默认；timeout<=0 = 10s。
func NewLFDR(endpoint string, timeout time.Duration) *LFDR {
	if endpoint == "" {
		endpoint = lfdrDefaultEndpoint
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &LFDR{endpoint: endpoint, client: &http.Client{Timeout: timeout}}
}

func (l *LFDR) Name() string { return "quantum:de-lfdr" }

func (l *LFDR) Info() map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]any, len(l.meta))
	for k, v := range l.meta {
		out[k] = v
	}
	return out
}

// Bytes 取 n 字节（1-1024）真随机。失败返回 error（不回退）。
func (l *LFDR) Bytes(n int) ([]byte, error) {
	if n < 1 || n > 1024 {
		return nil, fmt.Errorf("lfdr: 单次可取 1-1024 字节，请求 %d", n)
	}
	resp, err := l.client.Get(l.endpoint + "?length=" + strconv.Itoa(n) + "&format=HEX")
	if err != nil {
		return nil, fmt.Errorf("lfdr: 请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("lfdr: 读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lfdr: HTTP %d", resp.StatusCode)
	}
	var r lfdrResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("lfdr: 响应解析失败: %w", err)
	}
	b, err := hex.DecodeString(r.QRN)
	if err != nil {
		return nil, fmt.Errorf("lfdr: 随机值非 hex: %w", err)
	}
	if len(b) < n {
		return nil, fmt.Errorf("lfdr: 随机值仅 %d 字节，不足 %d", len(b), n)
	}
	l.mu.Lock()
	l.meta = map[string]any{"length": r.Length}
	l.mu.Unlock()
	return b[:n], nil
}

type lfdrResponse struct {
	Length int    `json:"length"`
	QRN    string `json:"qrn"`
}
