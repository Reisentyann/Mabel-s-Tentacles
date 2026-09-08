// 文件：mcp-server-go/internal/service/ticket.go —— 限时下载票据：无状态 HMAC 签发/验证（单文件绑定 + 自动过期）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

// 下载票据（2026-09-08 设计，替代 ACCESS_TOKEN 静态口径）：
//
//		ticket = HMAC-SHA256(SECRET_KEY, path + "|" + uuid + "|" + exp)
//
//	  - 单文件绑定：票据按 path+uuid 签发，换一个文件即失效
//	  - 自动过期：exp（unix 秒）随 URL 明文携带，过期即拒
//	  - 无状态：验证时重算比对，零存储、零撤销面——过期就是撤销
//	  - 动机：静态 ACCESS_TOKEN 随链接扩散等于全站任意文件（含私密）
//	    永久可下载；票据把暴露面缩到"这一个文件这一天"
package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"
)

// DownloadTicketTTL 票据有效期（一天：链接发出后对方随时能点开，
// 次日失效——单文件绑定的暴露面仍然可控）。
const DownloadTicketTTL = 24 * time.Hour

// SignDownloadTicket 签发单文件限时票据。
func SignDownloadTicket(secret, path, uuid string, exp time.Time) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(path + "|" + uuid + "|" + strconv.FormatInt(exp.Unix(), 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyDownloadTicket 验票：重算比对（常量时间）+ 有效期检查。
// expStr 为 URL 携带的明文过期时刻（unix 秒）。
func VerifyDownloadTicket(secret, path, uuid, expStr, ticket string) error {
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return fmt.Errorf("ticket: bad exp")
	}
	if time.Now().Unix() > exp {
		return fmt.Errorf("ticket: expired")
	}
	want := SignDownloadTicket(secret, path, uuid, time.Unix(exp, 0))
	if subtle.ConstantTimeCompare([]byte(want), []byte(ticket)) != 1 {
		return fmt.Errorf("ticket: signature mismatch")
	}
	return nil
}

// DownloadTicketExpiry 便捷：从现在起算的过期时刻。
func DownloadTicketExpiry() time.Time {
	return time.Now().Add(DownloadTicketTTL)
}
