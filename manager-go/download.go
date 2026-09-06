// 文件：manager-go/download.go —— 下载票据域：短期签名 URL 签发与验签，handler 不再拼链接
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

// download 域职责：下载访问策略。
// 现状（退役目标）：HTTP handler 拼 /api/files/download?path=...&token=静态ACCESS_TOKEN，
// 链接永久有效。目标形态：本域签发短期票据（file + 过期时间 + HMAC 签名），
// handler 只验签（纯函数）不出链接；download_count 计数归这里；
// 静态 ACCESS_TOKEN 降级为"谁可申请票据"的资格凭证。

package manager

import (
	"errors"
	"time"
)

// errNoDownload 域内通用未实现错误（骨架期）。
var errNoDownload = errors.New("manager: download not implemented")

// IssueTicket 为文件签发短期下载票据 URL（ttl 建议 10 分钟）。
// TODO 实现批次：hmac(secret, path+exp) 签名 → 拼 /api/files/dl?file=&exp=&sig=。
func (m *Manager) IssueTicket(path string, ttl time.Duration) (string, error) {
	return "", errNoDownload
}

// VerifyTicket 验签（纯函数）：校验签名与过期时间。handler 侧唯一调用。
// TODO 实现批次：与 IssueTicket 对称的 hmac 校验 + exp 比较。
func (m *Manager) VerifyTicket(path string, exp int64, sig string) bool {
	return false // TODO
}
