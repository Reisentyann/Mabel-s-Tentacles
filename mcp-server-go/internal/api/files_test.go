// 文件：mcp-server-go/internal/api/files_test.go —— HTTP 大文件 multipart 上传回归
// 修改：2026-11-28

package api

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/authz"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
)

type largeUploadStore struct {
	repo.Store
	reserved  bool
	confirmed bool
}

func (s *largeUploadStore) GetMetadata(context.Context, string) (*repo.FileMetadata, error) {
	return nil, pgx.ErrNoRows
}

func (s *largeUploadStore) ReserveMeta(context.Context, string) (string, error) {
	s.reserved = true
	return "ab-large", nil
}

func (s *largeUploadStore) CompleteIntake(context.Context, string, string) error {
	s.confirmed = true
	return nil
}

func (s *largeUploadStore) ArchiveFailedIntake(context.Context, string, string) error {
	return nil
}

func (s *largeUploadStore) DirectoryExists(context.Context, string) (bool, error) {
	return false, nil
}

func (s *largeUploadStore) PrepareMaintenance(context.Context, string, string, string, string) (int64, error) {
	return 0, nil
}

func (s *largeUploadStore) FinishMaintenance(context.Context, int64, string, string) error {
	return nil
}

func (s *largeUploadStore) IsIntakePending(context.Context, string) (bool, error) {
	return false, nil
}

type uploadZeroReader struct{}

func (uploadZeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestUploadMultipartLargeFile(t *testing.T) {
	const size = int64(6<<20 + 1)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "large.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(part, uploadZeroReader{}, size); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	st := &largeUploadStore{}
	dir := t.TempDir()
	s := &Server{repo: st, manager: manager.New(repo.NewManagerStore(st), dir, nil, nil, manager.DownloadConfig{})}
	r := httptest.NewRequest(http.MethodPost, "/api/files/upload", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r = r.WithContext(authz.WithPrincipal(r.Context(), &authz.Principal{Name: "admin", Role: "admin"}))
	w := httptest.NewRecorder()

	s.uploadFile(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !st.reserved || !st.confirmed {
		t.Fatalf("reserved = %v, confirmed = %v", st.reserved, st.confirmed)
	}
	stored := filepath.Join(dir, "ab", "ab-large.bin")
	info, err := os.Stat(stored)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != size {
		t.Fatalf("stored size = %d, want %d", info.Size(), size)
	}
}
