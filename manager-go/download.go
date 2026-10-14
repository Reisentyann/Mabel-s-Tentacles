// 文件：manager-go/download.go —— 下载票据域：短期签名下载地址的唯一产出方（签 + 验对称，无状态）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// download 域职责：文件下载访问策略——**管理机给出下载文件的地址**。
//
// 票据公式：ticket = HMAC-SHA256(SECRET_KEY, path|uuid|exp)
//   - 单文件绑定：票据按 path+uuid 签发，换文件/换 uuid 即失效
//   - 自动过期：exp（unix 秒）随 URL 明文携带，过期即拒
//   - 无状态：验证时重算比对，零存储、零撤销面——过期就是撤销
//   - 动机：静态 ACCESS_TOKEN 随链接扩散等于全站任意文件（含私密）永久
//     可下载；票据把暴露面缩到"这一个文件这一天"
//
// 归属（2026-09-11 归位）：本域原为 manager 骨架、真实现曾漂到装配层
// service/ticket.go（安全热修批次），现收归管理机——位置与下载策略的
// 唯一知情者。Secret/BaseURL 是部署配置，由装配层解析后经 DownloadConfig
// 注入（manager 不 import config）。
//
// 路由常量 DownloadEndpoint 是下载地址的对外契约（装配层 handler 挂同路径）。
package manager

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DownloadEndpoint 下载路由（管理机签出的地址端点；装配层 handler 必须挂同路径）。
const DownloadEndpoint = "/api/files/download"

// DownloadTicketTTL 票据默认有效期（一天：链接发出后对方随时能点开，次日失效）。
const DownloadTicketTTL = 24 * time.Hour

// DownloadConfig 下载域配置（装配层从 config 解析后注入）。
// Secret 空 = 下载票据未启用（IssueDownloadURL 返回空串）；BaseURL 空 = 同款。
type DownloadConfig struct {
	Secret  string // HMAC 密钥（SECRET_KEY）
	BaseURL string // 对外下载地址前缀（download_base_url 优先，回退 server.base_url）
}

// IssueDownloadURL 为文件签发限时下载地址（单文件绑定 + 自动过期）。
// ttl <= 0 用 DownloadTicketTTL。Secret/BaseURL 未配 → 返回空串
// （调用方走降级提示，不卡回执）。
func (m *Manager) IssueDownloadURL(logicPath, uuid string, ttl time.Duration) string {
	if m.download.Secret == "" || m.download.BaseURL == "" {
		return ""
	}
	if ttl <= 0 {
		ttl = DownloadTicketTTL
	}
	exp := time.Now().Add(ttl)

	// path 里的斜杠不编码（query 值中裸 / 合法）：QQ 等消息层会把 %2F
	// 二次转义成 %252F 导致服务端查无此键（实测 404）——源头少一个
	// 可被转义的靶子；接收侧另有容错解码兜底（api.normalizeDownloadPath）。
	escapedPath := strings.ReplaceAll(url.QueryEscape(logicPath), "%2F", "/")
	base := strings.TrimRight(m.download.BaseURL, "/")
	return base + DownloadEndpoint +
		"?path=" + escapedPath +
		"&exp=" + strconv.FormatInt(exp.Unix(), 10) +
		"&ticket=" + url.QueryEscape(m.signTicket(logicPath, uuid, exp))
}

// VerifyDownloadTicket 验票：重算比对（常量时间）+ 有效期检查。
// expStr 为 URL 携带的明文过期时刻（unix 秒）；下载 handler 唯一调用点。
func (m *Manager) VerifyDownloadTicket(logicPath, uuid, expStr, ticket string) error {
	if m.download.Secret == "" {
		return fmt.Errorf("download: ticket disabled")
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return fmt.Errorf("download: bad exp")
	}
	if time.Now().Unix() > exp {
		return fmt.Errorf("download: ticket expired")
	}
	want := m.signTicket(logicPath, uuid, time.Unix(exp, 0))
	if subtle.ConstantTimeCompare([]byte(want), []byte(ticket)) != 1 {
		return fmt.Errorf("download: ticket signature mismatch")
	}
	return nil
}

// signTicket HMAC-SHA256(SECRET_KEY, path|uuid|exp)（签发/验签共用）。
func (m *Manager) signTicket(logicPath, uuid string, exp time.Time) string {
	mac := hmac.New(sha256.New, []byte(m.download.Secret))
	mac.Write([]byte(logicPath + "|" + uuid + "|" + strconv.FormatInt(exp.Unix(), 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
