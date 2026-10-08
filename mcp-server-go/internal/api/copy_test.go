// 文件：mcp-server-go/internal/api/copy_test.go —— HTTP 复制重名返回 409，源文件授权先于落盘
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package api

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	manager "github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/authz"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
)

type copyConflictStore struct {
	repo.Store
	reserved bool
}

func (s *copyConflictStore) GetMetadata(context.Context, string) (*repo.FileMetadata, error) {
	owner := "alice"
	return &repo.FileMetadata{UUID: "ab-source", FilePath: "source.txt", OwnerID: &owner, Visibility: "private"}, nil
}

func (s *copyConflictStore) ReserveMeta(context.Context, string) (string, error) {
	s.reserved = true
	return "", repo.ErrKeyExists
}

func (s *copyConflictStore) DirectoryExists(context.Context, string) (bool, error) { return false, nil }

func (s *copyConflictStore) IsIntakePending(context.Context, string) (bool, error) { return false, nil }

func TestCopyConflictAndAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		reserve bool
	}{
		{"alice", 409, true}, {"bob", 404, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &copyConflictStore{}
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "ab"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "ab", "ab-source.txt"), []byte("source"), 0644); err != nil {
				t.Fatal(err)
			}
			s := &Server{repo: st, manager: manager.New(repo.NewManagerStore(st), dir, nil, nil, manager.DownloadConfig{})}
			r := httptest.NewRequest("POST", "/api/files/copy", strings.NewReader(`{"source":"source.txt","target":"existing.txt"}`))
			r = r.WithContext(authz.WithPrincipal(r.Context(), &authz.Principal{Name: tc.name, Role: "user"}))
			w := httptest.NewRecorder()
			s.copyFile(w, r)
			if w.Code != tc.status || st.reserved != tc.reserve {
				t.Fatalf("status=%d reserved=%v body=%s", w.Code, st.reserved, w.Body.String())
			}
		})
	}
}
