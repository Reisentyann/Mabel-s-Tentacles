// 文件：mcp-server-go/internal/api/auth.go —— 认证端点：login / refresh / logout / register（开放注册 + 密码策略 + 限流）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package api

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

// usernamePattern 用户名白名单（3-50 位字母数字下划线连字符）。
var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,50}$`)

// register 开放注册（权限批次 2026-09-06 解禁）：密码 ≥8 位、用户名白名单、
// 一律 role=user（admin 只能由 bootstrap 或既有 admin 产生）。注册即发 token。
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if !usernamePattern.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "username must be 3-50 chars of letters, digits, '_' or '-'")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}

	hash, err := service.HashPassword(req.Password)
	if err != nil {
		slog.Error("register hash password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "register failed")
		return
	}
	user, err := s.repo.CreateUser(r.Context(), req.Username, hash, req.Email, "user")
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			slog.Warn("register rejected: username taken", "username", req.Username, "ip", common.ClientIP(r))
			writeError(w, http.StatusConflict, "username already taken")
			return
		}
		slog.Error("register create user failed", "username", req.Username, "error", err)
		writeError(w, http.StatusInternalServerError, "register failed")
		return
	}

	tokens, err := service.GenerateTokens(s.cfg, user.ID, user.Username, user.Role)
	if err != nil {
		slog.Error("register generate tokens failed", "username", req.Username, "error", err)
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	slog.Info("register ok", "username", user.Username, "uid", user.ID, "ip", common.ClientIP(r))
	writeJSON(w, http.StatusCreated, tokens)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusUnauthorized, "Incorrect username or password")
		return
	}

	if s.repo == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}

	user, err := s.repo.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("login failed", "username", req.Username, "ip", common.ClientIP(r), "reason", "user_not_found")
			writeError(w, http.StatusUnauthorized, "Incorrect username or password")
			return
		}
		slog.Error("login query user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	if !service.CheckPassword(user.PasswordHash, req.Password) {
		slog.Warn("login failed", "username", req.Username, "ip", common.ClientIP(r), "reason", "bad_password")
		writeError(w, http.StatusUnauthorized, "Incorrect username or password")
		return
	}
	if !user.IsActive {
		slog.Warn("login rejected: account disabled", "username", req.Username, "ip", common.ClientIP(r))
		writeError(w, http.StatusForbidden, "account disabled")
		return
	}

	tokens, err := service.GenerateTokens(s.cfg, user.ID, user.Username, user.Role)
	if err != nil {
		slog.Error("generate tokens failed", "error", err)
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	slog.Info("login ok", "username", user.Username, "ip", common.ClientIP(r))
	writeJSON(w, http.StatusOK, tokens)
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	claims, err := service.ParseToken(s.cfg.Security.SecretKey, req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid refresh token")
		return
	}

	if s.repo != nil {
		blacklisted, err := s.repo.IsTokenBlacklisted(r.Context(), claims.ID)
		if err != nil {
			slog.Error("check blacklist failed", "error", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if blacklisted {
			writeError(w, http.StatusUnauthorized, "Token has been revoked")
			return
		}

		exp := time.Time{}
		if claims.ExpiresAt != nil {
			exp = claims.ExpiresAt.Time
		}
		if err := s.repo.InsertBlacklist(r.Context(), claims.ID, exp); err != nil {
			slog.Error("blacklist token failed", "error", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
	}

	// 角色实时化：refresh 时查库重签（角色调整后最长一个 access 周期内生效）
	role := claims.Role
	if s.repo != nil {
		if u, err := s.repo.GetUserByID(r.Context(), claims.UserID); err == nil && u != nil {
			role = u.Role
		}
	}
	tokens, err := service.GenerateTokens(s.cfg, claims.UserID, claims.Subject, role)
	if err != nil {
		slog.Error("generate tokens failed", "error", err)
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	slog.Info("refresh ok", "username", claims.Subject)
	writeJSON(w, http.StatusOK, tokens)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Successfully logged out"})
		return
	}

	claims, err := service.ParseToken(s.cfg.Security.SecretKey, req.RefreshToken)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Successfully logged out"})
		return
	}

	if s.repo != nil {
		exp := time.Time{}
		if claims.ExpiresAt != nil {
			exp = claims.ExpiresAt.Time
		}
		if err := s.repo.InsertBlacklist(r.Context(), claims.ID, exp); err != nil {
			slog.Warn("logout blacklist failed", "error", err)
		}
	}

	slog.Info("logout ok", "username", claims.Subject)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Successfully logged out"})
}
