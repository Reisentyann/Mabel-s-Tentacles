// 文件：mcp-server-go/internal/authz/authz_test.go —— 授权单点 L1：两级可见性矩阵 + 存量口径 + 未认证主体
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package authz

import "testing"

func strPtr(s string) *string { return &s }

func gid(v int64) *int64 { return &v }

var (
	alice  = &Principal{Kind: KindUser, Name: "alice", Role: "user", GroupIDs: []int64{7}}
	bob    = &Principal{Kind: KindUser, Name: "bob", Role: "user"}
	admin  = &Principal{Kind: KindUser, Name: "root", Role: "admin"}
	master = &Principal{Kind: KindAgentMaster, Name: "agent", Role: "admin"}
	extAl  = &Principal{Kind: KindAgentExternal, Name: "alice", Role: "user", GroupIDs: []int64{7}}
)

// TestCanReadMatrix 两级读矩阵（2026-09-08 简化定稿）：public 任何接入者
// 可读；private 仅主人与管家/admin；group 存量按 public 对待。
func TestCanReadMatrix(t *testing.T) {
	cases := []struct {
		name string
		p    *Principal
		f    FileACL
		want bool
	}{
		{"公开人人可读", bob, ACLOf(strPtr("alice"), "public", nil), true},
		{"私文件仅 owner", alice, ACLOf(strPtr("alice"), "private", nil), true},
		{"他人私文件拒绝", bob, ACLOf(strPtr("alice"), "private", nil), false},
		{"admin 全读", admin, ACLOf(strPtr("alice"), "private", nil), true},
		{"管家全读", master, ACLOf(strPtr("alice"), "private", nil), true},
		{"外部agent=绑定用户读自己的私文件", extAl, ACLOf(strPtr("alice"), "private", nil), true},
		{"group 存量按 public（组判定退役）", bob, ACLOf(strPtr("carol"), "group", gid(7)), true},
		{"存量无主可读", bob, ACLOf(nil, "", nil), true},
		{"未认证拒绝", nil, ACLOf(strPtr("alice"), "public", nil), false},
	}
	for _, c := range cases {
		if got, _ := CanRead(c.p, c.f); got != c.want {
			t.Errorf("%s: CanRead = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestCanWriteMatrix 两级写矩阵：接入即信任——public 协作可写（含无主
// 存量）；private 仅主人与管家（防搞乱的核心）。
func TestCanWriteMatrix(t *testing.T) {
	cases := []struct {
		name string
		p    *Principal
		f    FileACL
		want bool
	}{
		{"owner 写自己的公开文件", alice, ACLOf(strPtr("alice"), "public", nil), true},
		{"接入者可写他人公开文件（协作共享）", bob, ACLOf(strPtr("alice"), "public", nil), true},
		{"私文件仅 owner", alice, ACLOf(strPtr("alice"), "private", nil), true},
		{"他人私文件拒绝", bob, ACLOf(strPtr("alice"), "private", nil), false},
		{"admin 全写", admin, ACLOf(strPtr("alice"), "private", nil), true},
		{"管家全写", master, ACLOf(strPtr("alice"), "private", nil), true},
		{"外部agent写自己的私文件", extAl, ACLOf(strPtr("alice"), "private", nil), true},
		{"group 存量按 public 可写（组判定退役）", bob, ACLOf(strPtr("carol"), "group", gid(7)), true},
		{"无主存量任何接入者可写（信任接入）", bob, ACLOf(nil, "", nil), true},
		{"未认证拒绝", nil, ACLOf(strPtr("alice"), "public", nil), false},
	}
	for _, c := range cases {
		if got, _ := CanWrite(c.p, c.f); got != c.want {
			t.Errorf("%s: CanWrite = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDenialReasons(t *testing.T) {
	// 拒绝必须带原因（埋日志的素材，空原因=判定实现漂移）
	if _, reason := CanWrite(bob, ACLOf(strPtr("alice"), "private", nil)); reason == "" {
		t.Fatal("private denial must carry a reason")
	}
	if _, reason := CanRead(bob, ACLOf(strPtr("alice"), "private", nil)); reason == "" {
		t.Fatal("private read denial must carry a reason")
	}
	if _, reason := CanRead(nil, ACLOf(nil, "public", nil)); reason == "" {
		t.Fatal("unauthenticated denial must carry a reason")
	}
}

func TestIsAdminAndMaster(t *testing.T) {
	if !master.IsMaster() || alice.IsMaster() || admin.IsMaster() {
		t.Fatal("IsMaster must hold only for agent-master")
	}
	if !admin.IsAdmin() || !master.IsAdmin() || alice.IsAdmin() || extAl.IsAdmin() {
		t.Fatal("IsAdmin must hold for admin role and agent-master only")
	}
	var nilP *Principal
	if nilP.IsAdmin() || nilP.IsMaster() || nilP.Subject() != "anonymous" {
		t.Fatal("nil principal must be non-privileged with anonymous subject")
	}
}
