// 文件：common/fsutil_test.go —— 文件安全层 L1：ResolveWithin 跨平台语义（正常/穿越/盘符/symlink 逃逸/symlink 改写/写场景尾段不存在）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package common

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resolveErrText 便捷：断言错误文案（对外契约，逐句钉死）。
func resolveErrText(t *testing.T, baseDir, rel, wantPrefix string) {
	t.Helper()
	_, err := ResolveWithin(baseDir, rel)
	if err == nil || !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("ResolveWithin(%q) err = %v, want prefix %q", rel, err, wantPrefix)
	}
}

func TestResolveWithinNormal(t *testing.T) {
	dir := realBase(t, t.TempDir())
	for _, rel := range []string{"a.txt", "sub/b.txt", "./c.txt", "sub/../d.txt"} {
		abs, err := ResolveWithin(dir, rel)
		if err != nil {
			t.Fatalf("ResolveWithin(%q) err = %v", rel, err)
		}
		if !strings.HasPrefix(abs, dir) {
			t.Fatalf("ResolveWithin(%q) = %q, want inside %q", rel, abs, dir)
		}
	}
}

func TestResolveWithinTraversal(t *testing.T) {
	dir := t.TempDir()
	resolveErrText(t, dir, "../outside.txt", "security error: directory traversal detected and blocked")
	resolveErrText(t, dir, "sub/../../outside.txt", "security error: directory traversal detected and blocked")
}

// TestResolveWithinAbsNormalized 绝对路径的既有语义：前导分隔符被剥掉
// 归一为界内相对路径（防穿越方式是归一而非拒绝——原实现行为，契约保持）。
func TestResolveWithinAbsNormalized(t *testing.T) {
	dir := t.TempDir()
	abs, err := ResolveWithin(dir, "/abs.txt")
	if err != nil {
		t.Fatalf("absolute path err = %v", err)
	}
	if want := filepath.Join(realBase(t, dir), "abs.txt"); abs != want {
		t.Fatalf("absolute path = %q, want %q", abs, want)
	}
}

func TestResolveWithinDriveLetter(t *testing.T) {
	dir := t.TempDir()
	resolveErrText(t, dir, "C:/Windows/system32", "security error: path cannot contain drive letters")
}

func TestResolveWithinSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir() // 界外目录
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// data 内 symlink 指向界外文件
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "escape.txt")); err != nil {
		t.Skipf("symlink not permitted on this platform/privilege: %v", err)
	}
	resolveErrText(t, dir, "escape.txt", "security error: symlink")
}

func TestResolveWithinSymlinkRewrite(t *testing.T) {
	dir := t.TempDir()
	// data 内 symlink 指向 data 内另一处（不逃逸但改写路径）——fail-closed 同拒
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(dir, "alias.txt")); err != nil {
		t.Skipf("symlink not permitted on this platform/privilege: %v", err)
	}
	resolveErrText(t, dir, "alias.txt", "security error: symlink")
}

// realBase 解析短名/长名差异（Windows t.TempDir 可能给 8.3 短路径，
// securejoin 解析后是真实长名——同指一处）。
func realBase(t *testing.T, dir string) string {
	t.Helper()
	rb, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return rb
}

func TestResolveWithinWriteScene(t *testing.T) {
	// 写场景：尾段（及父段）不存在——词法拼接照常通过（MkdirAll 由调用方做）
	dir := t.TempDir()
	abs, err := ResolveWithin(dir, "新目录/新文件.txt")
	if err != nil {
		t.Fatalf("write-scene err = %v", err)
	}
	if want := filepath.Join(realBase(t, dir), "新目录", "新文件.txt"); abs != want {
		t.Fatalf("write-scene = %q, want %q", abs, want)
	}
}

func TestResolveWithinBaseSymlink(t *testing.T) {
	// base 自身是 symlink：解析后基准对齐，界内路径照常通过
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "datalink")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not permitted on this platform/privilege: %v", err)
	}
	if err := os.WriteFile(filepath.Join(real, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, err := ResolveWithin(link, "a.txt")
	if err != nil {
		t.Fatalf("base-symlink err = %v", err)
	}
	if _, serr := os.Stat(abs); serr != nil {
		t.Fatalf("resolved path not stat-able: %v", serr)
	}
}

func TestWithinDirSemantics(t *testing.T) {
	if !WithinDir("/base", "/base/a.txt") {
		t.Fatal("inside should pass")
	}
	if WithinDir("/base", "/base2/a.txt") {
		t.Fatal("sibling prefix should fail")
	}
	if WithinDir("/base", "../x") {
		t.Fatal("dot-dot should fail")
	}
}

// 编译期确认哨兵错误语义未被误改（fsutil 无哨兵，此处保留给未来扩展的占位惯例）。
var _ = errors.Is
