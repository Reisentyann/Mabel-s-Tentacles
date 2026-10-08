// 文件：mcp-server-go/internal/api/audit_test.go —— 对账入口权限测试：匿名与普通用户拒绝，管理员进入
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

func TestAuditAdminGate(t *testing.T) {
	cfg := config.Load()
	cfg.API.RequireAuth = true
	cfg.Security.SecretKey = "audit-test-secret"
	s := &Server{cfg: cfg}
	h := s.requireAdmin(http.HandlerFunc(s.auditFiles))
	for _, tc := range []struct {
		role   string
		status int
	}{
		{"", http.StatusUnauthorized},
		{"user", http.StatusForbidden},
		{"admin", http.StatusServiceUnavailable}, // 已通过权限检查，管理机未装配。
	} {
		t.Run(tc.role, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/admin/audit", nil)
			if tc.role != "" {
				tokens, err := service.GenerateTokens(cfg, 1, "tester", tc.role)
				if err != nil {
					t.Fatal(err)
				}
				r.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
		})
	}
}
