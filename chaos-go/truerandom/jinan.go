// 文件：chaos-go/truerandom/jinan.go —— 真随机熵源·济南器件无关量子随机数（DIQRNG）信标客户端
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// Jinan 是济南量子技术研究院 + 中科院量子信息与量子科技创新研究院的
// 器件无关量子随机数（DIQRNG）信标熵源。原理：基于无漏洞贝尔不等式检验、
// 利用量子力学非定域性产生，安全性最高等级（无需信任设备）。
//
// 实测接口（2026-09-11 逆向其前端）：
//
//	POST https://www.randomnessbeacon.com/index/random/find
//	Content-Type: application/json
//	Body: {"external_id":1}
//	→ {code:0, data:{output_value:"<64 hex>", output_value_pqc:"<64 hex>",
//	                  pulse_index, timestamp, type:"DI QRNG", chsh, nist_test, ...}}
//
// 注意：信标按分钟发布脉冲（period 60000ms），一脉冲一个 256bit 值——
// 同一分钟内的多次取数会得到同一脉冲值；脉冲时间戳随 meta 带出供判断新鲜度。
package truerandom

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
)

const jinanDefaultEndpoint = "https://www.randomnessbeacon.com/index/random/find"

// Jinan 济南 DIQRNG 信标熵源。
type Jinan struct {
	endpoint string
	client   *http.Client

	mu   sync.Mutex
	meta map[string]any
}

// 编译期断言：实现 chaos.EntropySource + chaos.InfoSource。
var (
	_ chaos.EntropySource = (*Jinan)(nil)
	_ chaos.InfoSource    = (*Jinan)(nil)
)

// NewJinan 构造济南信标熵源。endpoint 空 = 内置默认；timeout<=0 = 10s。
func NewJinan(endpoint string, timeout time.Duration) *Jinan {
	if endpoint == "" {
		endpoint = jinanDefaultEndpoint
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Jinan{endpoint: endpoint, client: &http.Client{Timeout: timeout}}
}

func (j *Jinan) Name() string { return "quantum:cn-jinan" }

func (j *Jinan) Info() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make(map[string]any, len(j.meta))
	for k, v := range j.meta {
		out[k] = v
	}
	return out
}

// Bytes 取 n 字节（1-32，单脉冲 256bit）真随机。失败返回 error（不回退）。
func (j *Jinan) Bytes(n int) ([]byte, error) {
	if n < 1 || n > 32 {
		return nil, fmt.Errorf("jinan: 单次可取 1-32 字节（单脉冲 256bit），请求 %d", n)
	}
	body, err := json.Marshal(map[string]any{"external_id": 1})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, j.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jinan: 请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("jinan: 读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jinan: HTTP %d", resp.StatusCode)
	}
	var r jinanResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("jinan: 响应解析失败: %w", err)
	}
	if r.Code != 0 {
		return nil, fmt.Errorf("jinan: 服务返回 code=%d msg=%s", r.Code, r.Msg)
	}
	hexValue := r.Data.OutputValue
	if hexValue == "" {
		hexValue = r.Data.OutputValuePQC
	}
	if hexValue == "" {
		return nil, fmt.Errorf("jinan: 响应无随机值（output_value 空）")
	}
	b, err := hex.DecodeString(hexValue)
	if err != nil {
		return nil, fmt.Errorf("jinan: 随机值非 hex: %w", err)
	}
	if len(b) < n {
		return nil, fmt.Errorf("jinan: 随机值仅 %d 字节，不足 %d", len(b), n)
	}
	j.mu.Lock()
	j.meta = map[string]any{
		"pulse_index": r.Data.PulseIndex,
		"timestamp":   r.Data.Timestamp,
		"type":        r.Data.Type,
		"chsh":        r.Data.CHSH,
		"nist_test":   r.Data.NistTest,
	}
	j.mu.Unlock()
	return b[:n], nil
}

// jinanResponse 济南信标响应（只取用到的字段）。
type jinanResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		OutputValue    string `json:"output_value"`
		OutputValuePQC string `json:"output_value_pqc"`
		PulseIndex     int64  `json:"pulse_index"`
		Timestamp      string `json:"timestamp"`
		Type           string `json:"type"`
		CHSH           string `json:"chsh"`
		NistTest       string `json:"nist_test"`
	} `json:"data"`
}
