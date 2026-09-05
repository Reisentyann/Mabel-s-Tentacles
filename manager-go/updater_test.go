// 文件：manager-go/updater_test.go —— updater 域 L1：T3 执行器（路由/合并/穿越/喂食）+ T2 陈旧四条件 + 幽灵软删 + batch 上限
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/all"
	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
)

// fakeStore updater 域最小面的内存实现（path 键；软删行退出扫描，对齐
// repo 的 is_deleted=FALSE 过滤口径）。
type fakeStore struct {
	rows        map[string]*manager.MetaRow
	ups         map[string]manager.MetaRecord
	uuids       map[string]string
	missing     map[string]int
	softDeleted map[string]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		rows:        map[string]*manager.MetaRow{},
		ups:         map[string]manager.MetaRecord{},
		uuids:       map[string]string{},
		missing:     map[string]int{},
		softDeleted: map[string]bool{},
	}
}

func (s *fakeStore) ListMetaPage(ctx context.Context, since string, limit int) ([]manager.MetaRow, error) {
	paths := make([]string, 0, len(s.rows))
	for p := range s.rows {
		if p > since {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	if len(paths) > limit {
		paths = paths[:limit]
	}
	out := make([]manager.MetaRow, 0, len(paths))
	for _, p := range paths {
		out = append(out, *s.rows[p])
	}
	return out, nil
}

func (s *fakeStore) GetMeta(ctx context.Context, path string) (*manager.MetaRow, error) {
	if r, ok := s.rows[path]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, nil
}

func (s *fakeStore) UpsertMeta(ctx context.Context, rec manager.MetaRecord) (string, error) {
	uuid, ok := s.uuids[rec.Path]
	if !ok {
		uuid = "u-" + rec.Path
		s.uuids[rec.Path] = uuid
	}
	s.ups[rec.Path] = rec
	if r, ok := s.rows[rec.Path]; ok {
		r.Checksum = rec.Checksum
		r.Attributes = rec.Attributes
	} else {
		s.rows[rec.Path] = &manager.MetaRow{Path: rec.Path, Checksum: rec.Checksum, Attributes: rec.Attributes}
	}
	s.missing[rec.Path] = 0 // Upsert 即文件存在证据（与 repo 清零语义一致）
	return uuid, nil
}

func (s *fakeStore) MarkMissing(ctx context.Context, path string) (int, error) {
	s.missing[path]++
	return s.missing[path], nil
}

func (s *fakeStore) SoftDeleteMeta(ctx context.Context, path string) error {
	s.softDeleted[path] = true
	delete(s.rows, path)
	return nil
}

// fakeSink 索引喂食钩子的录音机。
type fakeSink struct {
	feeds []sinkFeed
}

type sinkFeed struct {
	uuid string
	old  map[string]any
	new  map[string]any
}

func (s *fakeSink) Update(uuid string, old, new map[string]any) {
	s.feeds = append(s.feeds, sinkFeed{uuid: uuid, old: old, new: new})
}

func testExtMime(path string) string {
	if strings.HasSuffix(path, ".txt") || strings.HasSuffix(path, ".md") {
		return "text/plain"
	}
	return ""
}

func newTestManager(t *testing.T) (*manager.Manager, *fakeStore, *fakeSink, string) {
	t.Helper()
	dir := t.TempDir()
	st, sink := newFakeStore(), &fakeSink{}
	m := manager.New(st, dir, sink, testExtMime)
	return m, st, sink, dir
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func upsertedAttrs(t *testing.T, st *fakeStore, path string) map[string]any {
	t.Helper()
	rec, ok := st.ups[path]
	if !ok {
		t.Fatalf("no UpsertMeta recorded for %s", path)
	}
	return describer.AttrsFromJSON(rec.Attributes)
}

const novelSample = "# 第一章 深夜来电\n\n梅贝尔放下了手中的茶杯。\n\n「铃仙，文件整理好了吗？」她问道。\n\n「马上就好。」\n"

func TestAnalyzeFileFresh(t *testing.T) {
	m, st, sink, dir := newTestManager(t)
	writeFile(t, dir, "小说/第一章.txt", novelSample)

	report, err := m.AnalyzeFile(context.Background(), "小说/第一章.txt")
	if err != nil {
		t.Fatal(err)
	}
	fams := map[string]bool{}
	for _, f := range report.Families {
		fams[f] = true
	}
	if !fams["basic"] || !fams["text"] {
		t.Fatalf("families = %v, want basic+text", report.Families)
	}
	if fams["image"] || fams["code"] {
		t.Fatalf("families = %v, text file must not hit image/code", report.Families)
	}
	if report.Attrs["cod-text-language"] != "zh" {
		t.Fatalf("cod-text-language = %v, want zh", report.Attrs["cod-text-language"])
	}
	if v, ok := report.Attrs["cod-basic-mime-match"].(bool); !ok || !v {
		t.Fatalf("cod-basic-mime-match = %v, want true (extMime injected)", report.Attrs["cod-basic-mime-match"])
	}

	// 落库载荷：size / checksum / 合并后的 attributes（含 ver 与 at）
	rec := st.ups["小说/第一章.txt"]
	if rec.SizeBytes != int64(len(novelSample)) {
		t.Fatalf("size = %d, want %d", rec.SizeBytes, len(novelSample))
	}
	if len(rec.Checksum) != 64 {
		t.Fatalf("checksum = %q, want sha-256 hex", rec.Checksum)
	}
	attrs := upsertedAttrs(t, st, "小说/第一章.txt")
	if v, ok := attrs["cod-text-ver"].(float64); !ok || v < 1 {
		t.Fatalf("cod-text-ver = %v, want >= 1", attrs["cod-text-ver"])
	}
	if _, ok := attrs["cod-basic-at"]; !ok {
		t.Fatal("cod-basic-at missing in merged attributes")
	}

	// 喂食钩子：新文件 old 为空、new 为合并结果、uuid 对得上
	if len(sink.feeds) != 1 {
		t.Fatalf("sink feeds = %d, want 1", len(sink.feeds))
	}
	f := sink.feeds[0]
	if f.uuid != st.uuids["小说/第一章.txt"] {
		t.Fatalf("sink uuid = %q, want %q", f.uuid, st.uuids["小说/第一章.txt"])
	}
	if len(f.old) != 0 {
		t.Fatalf("old attrs = %v, want empty (fresh file)", f.old)
	}
	if f.new["cod-text-language"] != "zh" {
		t.Fatalf("sink new attrs missing cod-text-language: %v", f.new)
	}
}

func TestAnalyzeFilePreservesLLMFields(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	path := "note.txt"
	writeFile(t, dir, path, "hello")

	// 预置 llm 轨字段与一个过期的 cod 旧值：重分析后 llm 保留、cod 刷新
	st.rows[path] = &manager.MetaRow{
		Path:       path,
		Attributes: describer.JSONFromAttrs(map[string]any{"llm-tone": "暖橙", "cod-text-lines": 999}),
	}
	if _, err := m.AnalyzeFile(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	attrs := upsertedAttrs(t, st, path)
	if attrs["llm-tone"] != "暖橙" {
		t.Fatalf("llm-tone = %v, must survive cod merge", attrs["llm-tone"])
	}
	if attrs["cod-text-lines"] == 999 {
		t.Fatalf("cod-text-lines = 999, must be refreshed by family replace")
	}
}

func TestAnalyzeFileMissing(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	if _, err := m.AnalyzeFile(context.Background(), "no/such.txt"); err == nil {
		t.Fatal("missing file must error")
	}
}

func TestAnalyzeFileTraversalBlocked(t *testing.T) {
	m, _, _, dir := newTestManager(t)
	writeFile(t, dir, "inside.txt", "x")
	// dataDir 外的文件：.. 穿越——resolve 必须拒绝（内化 placement 域）
	if _, err := m.AnalyzeFile(context.Background(), "../outside.txt"); err == nil {
		t.Fatal("directory traversal must be blocked")
	}
}

func TestBackfillFreshNotStale(t *testing.T) {
	m, _, sink, dir := newTestManager(t)
	writeFile(t, dir, "a.txt", novelSample)
	if _, err := m.AnalyzeFile(context.Background(), "a.txt"); err != nil {
		t.Fatal(err)
	}
	feedsBefore := len(sink.feeds)
	n, err := m.Backfill(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("analyzed = %d, want 0 (ver fresh / checksum same / mtime old)", n)
	}
	if len(sink.feeds) != feedsBefore {
		t.Fatalf("sink feeds = %d, want %d (fresh file must not be re-analyzed)", len(sink.feeds), feedsBefore)
	}
}

func TestBackfillVerBump(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	writeFile(t, dir, "a.txt", novelSample)
	if _, err := m.AnalyzeFile(context.Background(), "a.txt"); err != nil {
		t.Fatal(err)
	}
	// 伪造旧版本：ver 落后（条件 2），checksum 与 mtime 保持新鲜
	row := st.rows["a.txt"]
	attrs := upsertedAttrs(t, st, "a.txt")
	attrs["cod-basic-ver"] = 1
	attrs["cod-text-ver"] = 1
	row.Attributes = describer.JSONFromAttrs(attrs)

	n, err := m.Backfill(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("analyzed = %d, want 1 (family version bump)", n)
	}
}

func TestBackfillChecksumDrift(t *testing.T) {
	m, _, _, dir := newTestManager(t)
	writeFile(t, dir, "a.txt", novelSample)
	if _, err := m.AnalyzeFile(context.Background(), "a.txt"); err != nil {
		t.Fatal(err)
	}

	// 模拟 execute_command 绕口直改：内容变 + mtime 改回原值——
	// 快路径（条件 4）不命中，只留给 checksum 漂移（条件 3）兜底
	abs := filepath.Join(dir, "a.txt")
	before, err := os.Stat(abs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(novelSample+"\n新的一行，命令直改。"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(abs, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}

	n, err := m.Backfill(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("analyzed = %d, want 1 (checksum drift)", n)
	}
	// 重分析后 checksum 已刷新：再跑一轮不再命中
	n, err = m.Backfill(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("second round analyzed = %d, want 0 (idempotent)", n)
	}
}

func TestBackfillMtimeNew(t *testing.T) {
	m, _, _, dir := newTestManager(t)
	writeFile(t, dir, "a.txt", novelSample)
	if _, err := m.AnalyzeFile(context.Background(), "a.txt"); err != nil {
		t.Fatal(err)
	}
	// 内容不变、mtime 推到未来（条件 4 快路径）
	abs := filepath.Join(dir, "a.txt")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(abs, future, future); err != nil {
		t.Fatal(err)
	}
	n, err := m.Backfill(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("analyzed = %d, want 1 (mtime newer than cod-*-at)", n)
	}
}

func TestBackfillGhostSoftDelete(t *testing.T) {
	m, st, _, _ := newTestManager(t)
	// DB 有行、盘上无文件：连续 3 轮 → 软删除（字典 10.4）
	st.rows["ghost.txt"] = &manager.MetaRow{
		Path:       "ghost.txt",
		Attributes: describer.JSONFromAttrs(map[string]any{"cod-basic-ver": 3, "cod-basic-at": time.Now().Unix()}),
	}
	ctx := context.Background()
	for round := 1; round <= 2; round++ {
		if _, err := m.Backfill(ctx, 10); err != nil {
			t.Fatal(err)
		}
		if st.softDeleted["ghost.txt"] {
			t.Fatalf("soft-deleted at round %d, too early", round)
		}
	}
	if _, err := m.Backfill(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if !st.softDeleted["ghost.txt"] {
		t.Fatal("ghost must be soft-deleted after 3 missing rounds")
	}
	if st.missing["ghost.txt"] != 3 {
		t.Fatalf("missing rounds = %d, want 3", st.missing["ghost.txt"])
	}
}

func TestBackfillBatchLimit(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	// 两条同为版本落后的行：batch=1 只重分析一个，其余仅做缺失检查
	for _, p := range []string{"b.txt", "a.txt"} {
		writeFile(t, dir, p, novelSample)
		st.rows[p] = &manager.MetaRow{
			Path:       p,
			Attributes: describer.JSONFromAttrs(map[string]any{"cod-basic-ver": 1, "cod-basic-at": time.Now().Unix()}),
		}
	}
	n, err := m.Backfill(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("analyzed = %d, want 1 (batch limit)", n)
	}
}

func TestBackfillNeverAppliedFamilyIgnored(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	// 老数据只有 basic 家族（text 从未适用）：basic 新鲜时不得误判 text 陈旧
	writeFile(t, dir, "a.txt", novelSample)
	st.rows["a.txt"] = &manager.MetaRow{
		Path:     "a.txt",
		Checksum: mustChecksum(t, dir, "a.txt"),
		Attributes: describer.JSONFromAttrs(map[string]any{
			"cod-basic-ver": 999, // 远超当前版本：永不落后
			"cod-basic-at":  time.Now().Unix(),
		}),
	}
	// mtime 归零到过去：快路径不命中（at 在未来即可）
	abs := filepath.Join(dir, "a.txt")
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(abs, past, past); err != nil {
		t.Fatal(err)
	}
	n, err := m.Backfill(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("analyzed = %d, want 0 (never-applied text family must not trigger)", n)
	}
}

func mustChecksum(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
