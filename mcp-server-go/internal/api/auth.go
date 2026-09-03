// 文件：mcp-server-go/internal/api/auth.go —— 认证端点：login / refresh / logout / register
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusForbidden, "Registration is currently disabled.")
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
			slog.Warn("login failed", "username", req.Username, "reason", "user_not_found")
			writeError(w, http.StatusUnauthorized, "Incorrect username or password")
			return
		}
		slog.Error("login query user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	if !service.CheckPassword(user.PasswordHash, req.Password) {
		slog.Warn("login failed", "username", req.Username, "reason", "bad_password")
		writeError(w, http.StatusUnauthorized, "Incorrect username or password")
		return
	}

	tokens, err := service.GenerateTokens(s.cfg, user.ID, user.Username)
	if err != nil {
		slog.Error("generate tokens failed", "error", err)
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	slog.Info("login ok", "username", user.Username)
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

	tokens, err := service.GenerateTokens(s.cfg, claims.UserID, claims.Subject)
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
