// 文件：chaos-go/truerandom/anu.go —— 真随机熵源·ANU 量子随机数（真空量子涨落，备选）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// ANU 是澳大利亚国立大学的量子随机数服务：平衡零差探测测真空量子涨落，
// 真量子随机（属"器件可信"型，非器件无关；输出经哈希后处理）。
//
// 接口（公开、无需 key，旧端点限 1 次/分钟）：
//
//	GET https://qrng.anu.edu.au/API/jsonI.php?length=N&type=uint8
//	→ {"type":"uint8","length":N,"data":[...],"success":true}
//
// 注意：超限时服务返回 HTTP 200 的**纯文本**拒绝（不是 429、不是 JSON）——
// 解析失败即按错误处理（不回退）。
package truerandom

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
)

const anuDefaultEndpoint = "https://qrng.anu.edu.au/API/jsonI.php"

// ANU 澳国立真空量子涨落熵源（备选）。
type ANU struct {
	endpoint string
	client   *http.Client

	mu   sync.Mutex
	meta map[string]any
}

var (
	_ chaos.EntropySource = (*ANU)(nil)
	_ chaos.InfoSource    = (*ANU)(nil)
)

// NewANU 构造 ANU 熵源。endpoint 空 = 内置默认；timeout<=0 = 10s。
func NewANU(endpoint string, timeout time.Duration) *ANU {
	if endpoint == "" {
		endpoint = anuDefaultEndpoint
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ANU{endpoint: endpoint, client: &http.Client{Timeout: timeout}}
}

func (a *ANU) Name() string { return "quantum:au-anu" }

func (a *ANU) Info() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]any, len(a.meta))
	for k, v := range a.meta {
		out[k] = v
	}
	return out
}

// Bytes 取 n 字节（1-1024）真随机。失败返回 error（不回退）。
func (a *ANU) Bytes(n int) ([]byte, error) {
	if n < 1 || n > 1024 {
		return nil, fmt.Errorf("anu: 单次可取 1-1024 字节，请求 %d", n)
	}
	resp, err := a.client.Get(a.endpoint + "?length=" + strconv.Itoa(n) + "&type=uint8")
	if err != nil {
		return nil, fmt.Errorf("anu: 请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("anu: 读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anu: HTTP %d", resp.StatusCode)
	}
	var r anuResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		// 限流拒绝是纯文本，会走到这里——如实报错
		return nil, fmt.Errorf("anu: 响应解析失败（可能触发限流）: %w", err)
	}
	if !r.Success || len(r.Data) < n {
		return nil, fmt.Errorf("anu: 响应无效（success=%v, 数据 %d 字节，请求 %d）", r.Success, len(r.Data), n)
	}
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = byte(r.Data[i])
	}
	a.mu.Lock()
	a.meta = map[string]any{"type": r.Type, "length": r.Length}
	a.mu.Unlock()
	return b, nil
}

type anuResponse struct {
	Type    string `json:"type"`
	Length  int    `json:"length"`
	Data    []int  `json:"data"`
	Success bool   `json:"success"`
}
