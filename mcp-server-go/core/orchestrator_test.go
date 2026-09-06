// 文件：mcp-server-go/core/orchestrator_test.go —— 编排机骨架测试：执行器管线 / 队列生命周期 / Describe 闸门 / 检索降级 / 索引重建
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

package core

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/indexer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

// —— 测试替身 ——

// memStore Store 最小面的内存实现（近似 COALESCE 语义：指针 nil 不覆盖）。
type memStore struct {
	mu   sync.Mutex
	rows map[string]*repo.FileMetadata
}

func newMemStore() *memStore {
	return &memStore{rows: map[string]*repo.FileMetadata{}}
}

func (s *memStore) GetMetadata(_ context.Context, p string) (*repo.FileMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.rows[p]; ok {
		cp := *m
		return &cp, nil
	}
	return nil, pgx.ErrNoRows // 与 repo 同口径：无行返回 ErrNoRows
}

func (s *memStore) UpsertMetadata(_ context.Context, m *repo.FileMetadata) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.rows[m.FilePath]; ok {
		if m.Title != nil {
			old.Title = m.Title
		}
		if m.Description != nil {
			old.Description = m.Description
		}
		if m.Tags != nil {
			old.Tags = m.Tags
		}
		if m.FileType != nil {
			old.FileType = m.FileType
		}
		if m.MimeType != nil {
			old.MimeType = m.MimeType
		}
		if m.Extension != nil {
			old.Extension = m.Extension
		}
		if m.SizeBytes != nil {
			old.SizeBytes = m.SizeBytes
		}
		if m.Checksum != nil {
			old.Checksum = m.Checksum
		}
		if m.SessionID != nil {
			old.SessionID = m.SessionID
		}
		if len(m.Attributes) > 0 {
			old.Attributes = m.Attributes
		}
		if m.Scope != "" {
			old.Scope = m.Scope
		}
		return old.UUID, nil
	}
	cp := *m
	cp.UUID = "uuid-" + m.FilePath
	if cp.Scope == "" {
		cp.Scope = "global"
	}
	s.rows[m.FilePath] = &cp
	return cp.UUID, nil
}

func (s *memStore) ListMetadataPage(_ context.Context, since string, limit int) ([]repo.FileMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var paths []string
	for p := range s.rows {
		if p > since {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	if len(paths) > limit {
		paths = paths[:limit]
	}
	out := make([]repo.FileMetadata, 0, len(paths))
	for _, p := range paths {
		out = append(out, *s.rows[p])
	}
	return out, nil
}

func (s *memStore) row(t *testing.T, p string) *repo.FileMetadata {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.rows[p]; ok {
		return m
	}
	t.Fatalf("no row for %s", p)
	return nil
}

// fakeSink 记录型喂食替身。
type sinkFeed struct {
	uuid     string
	old, new map[string]any
}

type fakeSink struct {
	mu    sync.Mutex
	feeds []sinkFeed
}

func (s *fakeSink) Update(uuid string, old, new map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.feeds = append(s.feeds, sinkFeed{uuid: uuid, old: old, new: new})
}

func (s *fakeSink) snapshot() []sinkFeed {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sinkFeed(nil), s.feeds...)
}

// stubSearcher SQL 兜底替身（计调用次数）。
type stubSearcher struct {
	mu    sync.Mutex
	calls int
}

func (s *stubSearcher) Search(_ context.Context, _ search.Query) ([]repo.FileMetadata, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return nil, 0, nil
}

func (s *stubSearcher) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// memIndex IndexSource 替身（记录重建载荷）。
type memIndex struct {
	mu       sync.Mutex
	rebuilt  map[string]map[string]any
	rebuilds int
}

func (m *memIndex) Query(_ []indexer.Condition, _ indexer.Combine) ([]string, error) {
	return nil, nil
}

func (m *memIndex) Rebuild(all map[string]map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rebuilt = all
	m.rebuilds++
	return nil
}

func (m *memIndex) snapshot() (map[string]map[string]any, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rebuilt, m.rebuilds
}

func ptr[T any](v T) *T { return &v }

// waitFor 轮询等待条件成立（异步测试的会合点）。
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for condition")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// —— 用例 ——

func TestNewValidation(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("expected error for empty options")
	}
	if _, err := New(Options{DataDir: "x"}); err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestExecutePipeline(t *testing.T) {
	dir := t.TempDir()
	content := "# 深夜书架\n\n第一章 来电\n\n梅贝尔整理着书架上的旧书。\n"
	mustWrite(t, dir, "note.txt", content)

	ms := newMemStore()
	sn := &fakeSink{}
	o, err := New(Options{DataDir: dir, Store: ms, Sink: sn})
	if err != nil {
		t.Fatal(err)
	}

	rep, err := o.execute(context.Background(), Event{
		Kind: KindWrite, Path: "note.txt", SessionID: "s1",
		Agent: &AgentMeta{Title: ptr("书架笔记")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.UUID == "" {
		t.Fatal("uuid empty")
	}
	fams := strings.Join(rep.Families, ",")
	if !strings.Contains(fams, "basic") || !strings.Contains(fams, "text") {
		t.Fatalf("families missing basic/text: %v", rep.Families)
	}
	if rep.CodKeys == 0 {
		t.Fatal("no cod keys produced")
	}

	row := ms.row(t, "note.txt")
	attrs := describer.AttrsFromJSON(row.Attributes)
	if attrs["cod-text-language"] != "zh" {
		t.Fatalf("cod-text-language = %v, want zh", attrs["cod-text-language"])
	}
	if row.Title == nil || *row.Title != "书架笔记" {
		t.Fatalf("agent title not persisted: %v", row.Title)
	}
	if row.Checksum == nil || *row.Checksum != service.ChecksumSHA256([]byte(content)) {
		t.Fatal("checksum mismatch with content hash")
	}
	if row.SessionID == nil || *row.SessionID != "s1" {
		t.Fatalf("session not persisted: %v", row.SessionID)
	}

	feeds := sn.snapshot()
	if len(feeds) != 1 {
		t.Fatalf("sink feeds = %d, want 1", len(feeds))
	}
	if feeds[0].uuid != rep.UUID {
		t.Fatalf("feed uuid %q != %q", feeds[0].uuid, rep.UUID)
	}
	if feeds[0].new == nil || feeds[0].new["cod-text-language"] != "zh" {
		t.Fatal("feed new attrs missing cod-text-language")
	}
}

func TestExecuteMissingFile(t *testing.T) {
	o, err := New(Options{DataDir: t.TempDir(), Store: newMemStore()})
	if err != nil {
		t.Fatal(err)
	}
	// 容灾路径：盘上竞态消失 → 返回错误（调用方丢弃，T2 兜底），不 panic
	if _, err := o.execute(context.Background(), Event{Kind: KindWrite, Path: "ghost.txt"}); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestQueueLifecycle(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "q.txt", "hello world\n")

	ms := newMemStore()
	sn := &fakeSink{}
	o, err := New(Options{DataDir: dir, Store: ms, Sink: sn})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o.Start(ctx)

	if !o.Submit(Event{Kind: KindWrite, Path: "q.txt", SessionID: "s1"}) {
		t.Fatal("submit rejected")
	}
	waitFor(t, func() bool { return o.Stats().Processed >= 1 })

	st := o.Stats()
	if st.Submitted != 1 || st.Dropped != 0 || st.InFlight != 0 {
		t.Fatalf("stats = %+v", st)
	}
	if len(sn.snapshot()) != 1 {
		t.Fatal("sink not fed by worker")
	}

	o.Stop()
	if o.Submit(Event{Kind: KindWrite, Path: "q.txt"}) {
		t.Fatal("submit after stop should be rejected")
	}
}

func TestSubmitOverflow(t *testing.T) {
	o, err := New(Options{DataDir: "x", Store: newMemStore(), QueueCap: 1})
	if err != nil {
		t.Fatal(err)
	}
	// 不 Start：事件滞留队列占满容量，第二个事件按容灾立场丢弃
	if !o.Submit(Event{Kind: KindWrite, Path: "a"}) {
		t.Fatal("first submit should succeed")
	}
	if o.Submit(Event{Kind: KindWrite, Path: "b"}) {
		t.Fatal("second submit should be dropped")
	}
	if o.Stats().Dropped != 1 {
		t.Fatalf("dropped = %d, want 1", o.Stats().Dropped)
	}
}

func TestDescribeGateAndTombstone(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "d.txt", "你好世界\n")

	ms := newMemStore()
	sn := &fakeSink{}
	o, err := New(Options{DataDir: dir, Store: ms, Sink: sn})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// 第一轮：cod 越权 + 词表外被拒；合法 sp-llm 落库
	res, err := o.Describe(ctx, DescribeRequest{
		Path:        "d.txt",
		Description: ptr("初稿"),
		Attributes: map[string]any{
			"cod-image-width":   999,
			"llm-semantic-type": "科幻小说",
			"sp-llm-观察":         "有趣",
		},
	}, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rejected) != 2 {
		t.Fatalf("rejected = %v, want 2 entries", res.Rejected)
	}
	row := ms.row(t, "d.txt")
	attrs := describer.AttrsFromJSON(row.Attributes)
	if attrs["sp-llm-观察"] != "有趣" {
		t.Fatalf("sp-llm-观察 = %v, want 有趣", attrs["sp-llm-观察"])
	}
	if _, ok := attrs["cod-image-width"]; ok {
		t.Fatal("cod-image-width should be rejected (cod 区只读)")
	}
	if attrs["llm-source"] != "agent" {
		t.Fatalf("llm-source = %v, want agent", attrs["llm-source"])
	}
	if row.Description == nil || *row.Description != "初稿" {
		t.Fatalf("description = %v, want 初稿", row.Description)
	}
	if len(sn.snapshot()) != 1 {
		t.Fatal("describe should feed sink")
	}

	// 第二轮：null 墓碑删除 sp 键
	if _, err := o.Describe(ctx, DescribeRequest{
		Path:       "d.txt",
		Attributes: map[string]any{"sp-llm-观察": nil},
	}, "sess-2"); err != nil {
		t.Fatal(err)
	}
	attrs = describer.AttrsFromJSON(ms.row(t, "d.txt").Attributes)
	if _, ok := attrs["sp-llm-观察"]; ok {
		t.Fatal("sp-llm-观察 should be tombstoned")
	}

	// 第三轮：append 模式拼段
	if _, err := o.Describe(ctx, DescribeRequest{
		Path:        "d.txt",
		Description: ptr("补记"),
		Mode:        "append",
	}, "sess-3"); err != nil {
		t.Fatal(err)
	}
	if d := ms.row(t, "d.txt").Description; d == nil || *d != "初稿\n\n补记" {
		t.Fatalf("append description = %v, want 初稿\\n\\n补记", d)
	}
}

func TestSearchDegradation(t *testing.T) {
	ctx := context.Background()

	fb := &stubSearcher{}
	o, err := New(Options{DataDir: "x", Store: newMemStore(), Fallback: fb})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.Search(ctx, search.Query{Text: "q"}); err != nil {
		t.Fatal(err)
	}
	if fb.count() != 1 {
		t.Fatalf("fallback calls = %d, want 1", fb.count())
	}

	// 无兜底也无索引 → 明确报错（不静默空结果）
	o2, err := New(Options{DataDir: "x", Store: newMemStore()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := o2.Search(ctx, search.Query{}); err == nil {
		t.Fatal("expected error without fallback")
	}
}

func TestRebuildIndex(t *testing.T) {
	ctx := context.Background()
	ms := newMemStore()
	if _, err := ms.UpsertMetadata(ctx, &repo.FileMetadata{
		FilePath:   "a.txt",
		Attributes: describer.JSONFromAttrs(map[string]any{"cod-text-lines": 3}),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ms.UpsertMetadata(ctx, &repo.FileMetadata{
		FilePath:   "b.txt",
		Attributes: describer.JSONFromAttrs(map[string]any{"cod-text-lines": 7}),
	}); err != nil {
		t.Fatal(err)
	}

	idx := &memIndex{}
	o, err := New(Options{DataDir: "x", Store: ms, Index: idx})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	all, rebuilds := idx.snapshot()
	if rebuilds != 1 || len(all) != 2 {
		t.Fatalf("rebuild = %d entries, %d calls; want 2 entries, 1 call", len(all), rebuilds)
	}
	if _, ok := all["uuid-a.txt"]; !ok {
		t.Fatalf("uuid-a.txt missing from rebuild: %v", all)
	}

	// 未装配索引源 → 明确报错
	o2, err := New(Options{DataDir: "x", Store: newMemStore()})
	if err != nil {
		t.Fatal(err)
	}
	if err := o2.RebuildIndex(ctx); err == nil {
		t.Fatal("expected error without index source")
	}
}
