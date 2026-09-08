// 文件：manager-go/buffer_test.go —— 取件缓冲区 L1（包内测试，直摸 unexported）：快照复制 / stat 新鲜度 / LRU 逐出 / maxEntry 旁路 / drop / 覆盖
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newBufFile 测试便捷：临时目录里放一个真实盘上文件，返回绝对路径。
func newBufFile(t *testing.T, content string) string {
	t.Helper()
	abs := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return abs
}

// entryOf 盘上文件的当前状态 + 指定内容 → bufEntry。
func entryOf(t *testing.T, abs, content string) bufEntry {
	t.Helper()
	info, err := os.Stat(abs)
	if err != nil {
		t.Fatal(err)
	}
	return bufEntry{path: abs, size: info.Size(), modTime: info.ModTime(), content: []byte(content)}
}

// TestBufferPutGetHit 基本闭环：put 后 get 命中，内容一致；stats 计量正确。
func TestBufferPutGetHit(t *testing.T) {
	abs := newBufFile(t, "hello-触手")
	buf := newFileBuffer(64<<20, 5<<20)
	buf.put("u1", entryOf(t, abs, "hello-触手"))

	got, ok := buf.get("u1")
	if !ok || string(got) != "hello-触手" {
		t.Fatalf("get = (%q, %v)", got, ok)
	}
	if n, b := buf.stats(); n != 1 || b != int64(len("hello-触手")) {
		t.Fatalf("stats = (%d, %d)", n, b)
	}
}

// TestBufferSnapshot 快照语义：put 后调用方改写原数组，缓存内容不变
// （put 复制入缓——盘外改动不许污染快照）。
func TestBufferSnapshot(t *testing.T) {
	abs := newBufFile(t, "v1")
	buf := newFileBuffer(64<<20, 5<<20)
	orig := []byte("v1")
	buf.put("u1", bufEntry{path: abs, size: int64(len(orig)), modTime: time.Now(), content: orig})
	orig[0] = 'X' // 调用方后续改原数组

	got, ok := buf.get("u1")
	if !ok || string(got) != "v1" {
		t.Fatalf("snapshot broken: get = (%q, %v), want v1", got, ok)
	}
}

// TestBufferFreshnessDrift 新鲜度：盘上改动（size 漂）→ get 失效（false），
// 条目被顺带 drop。
func TestBufferFreshnessDrift(t *testing.T) {
	abs := newBufFile(t, "short")
	buf := newFileBuffer(64<<20, 5<<20)
	buf.put("u1", entryOf(t, abs, "short"))

	if err := os.WriteFile(abs, []byte("much-longer-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := buf.get("u1"); ok {
		t.Fatal("get after size drift should miss")
	}
	if n, _ := buf.stats(); n != 0 {
		t.Fatalf("drifted entry should be dropped, stats = %d", n)
	}
}

// TestBufferLRUEviction LRU 逐出：容量装不下新条目时，最久未用的先出局；
// 最近使用过的（get 提升位次）留任。
func TestBufferLRUEviction(t *testing.T) {
	dir := t.TempDir()
	mk := func(name, content string) string {
		abs := filepath.Join(dir, name)
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return abs
	}
	a, b, c := mk("a.txt", "aaaa"), mk("b.txt", "bbbb"), mk("c.txt", "cccc")

	// 容量只够两条（每条 4 字节），第三条进来逐出最久未用
	buf := newFileBuffer(8, 5<<20)
	buf.put("a", entryOf(t, a, "aaaa"))
	buf.put("b", entryOf(t, b, "bbbb"))
	buf.get("a")                        // a 最近用过 → 提升位次
	buf.put("c", entryOf(t, c, "cccc")) // 应逐出 b（最久未用）

	if _, ok := buf.get("b"); ok {
		t.Fatal("b (LRU tail) should be evicted")
	}
	if _, ok := buf.get("a"); !ok {
		t.Fatal("a (recently used) should survive")
	}
	if _, ok := buf.get("c"); !ok {
		t.Fatal("c (just put) should be present")
	}
	if n, used := buf.stats(); n != 2 || used != 8 {
		t.Fatalf("stats = (%d, %d), want (2, 8)", n, used)
	}
}

// TestBufferMaxEntryBypass 单条目上限：size > maxEntry 拒入（大文件旁路，
// Open 直流——内存峰值防线）。
func TestBufferMaxEntryBypass(t *testing.T) {
	abs := newBufFile(t, "ten-bytes!") // 10 字节 > maxEntry=8
	buf := newFileBuffer(64<<20, 8)
	buf.put("u1", entryOf(t, abs, "ten-bytes!"))

	if n, _ := buf.stats(); n != 0 {
		t.Fatalf("oversized entry should be rejected, stats = %d", n)
	}
	if _, ok := buf.get("u1"); ok {
		t.Fatal("get should miss (never cached)")
	}
}

// TestBufferDrop drop 显式失效（删除路径的支撑）；不存在为 no-op。
func TestBufferDrop(t *testing.T) {
	abs := newBufFile(t, "x")
	buf := newFileBuffer(64<<20, 5<<20)
	buf.put("u1", entryOf(t, abs, "x"))
	buf.drop("u1")
	buf.drop("u-none") // no-op 不 panic

	if n, used := buf.stats(); n != 0 || used != 0 {
		t.Fatalf("stats after drop = (%d, %d), want (0, 0)", n, used)
	}
	if _, ok := buf.get("u1"); ok {
		t.Fatal("get after drop should miss")
	}
}

// TestBufferOverwrite 同 uuid 重复 put = 新快照覆盖（旧占用先归还，
// stats 不虚高）。
func TestBufferOverwrite(t *testing.T) {
	abs := newBufFile(t, "old-value")
	buf := newFileBuffer(64<<20, 5<<20)
	buf.put("u1", entryOf(t, abs, "old-value"))
	time.Sleep(2 * time.Millisecond) // 保证 modTime 可区分（文件系统精度）
	if err := os.WriteFile(abs, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	buf.put("u1", entryOf(t, abs, "new"))

	if n, used := buf.stats(); n != 1 || used != 3 {
		t.Fatalf("stats after overwrite = (%d, %d), want (1, 3)", n, used)
	}
	got, ok := buf.get("u1")
	if !ok || string(got) != "new" {
		t.Fatalf("get after overwrite = (%q, %v), want new", got, ok)
	}
}
