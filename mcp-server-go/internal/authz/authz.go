// 文件：mcp-server-go/internal/authz/authz.go —— 授权单点：Principal 主体模型 + CanRead/CanWrite 策略（docs/权限设计.md 的唯一实现）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

// Package authz 是权限判定的唯一策略源（docs/权限设计.md）：
// HTTP（api 层）与 MCP（tools 层）两侧的每一次读写判定都来这里，
// 不允许各自内联 if——策略改动一处生效，审计日志一处对齐。
//
// 主体 × 资源 × 操作矩阵（2026-09-06 定稿）：
//
//	主体：user（JWT）/ agent-master（.env master key，管家，无限权限）/
//	      agent-external（受限 key，绑定用户身份，权限=该用户）
//	资源：文件三档可见性 public（全局）/ group（组内）/ private（个人）
//
//	          读 public   读 group     读 private   写 public   写 group      写 private
//	user      ✅          本组          自己         ❌只读共享   本组协作      自己
//	admin     ✅          ✅            ✅           ✅          ✅            ✅
//	管家      ✅          ✅            ✅           ✅          ✅            ✅
//	外部agent  =绑定用户   =绑定用户     =绑定用户    =绑定用户   =绑定用户     =绑定用户
//
// 存量口径：owner_id 为 NULL 的老文件视为"无主遗产"——人人可读（迁移给
// visibility=public），写权归 admin/管家。新文件默认 private（安全默认）。
package authz

import (
	"context"
)

// Kind 主体种类。
type Kind string

const (
	KindUser          Kind = "user"           // 人类（HTTP / JWT）
	KindAgentMaster   Kind = "agent-master"   // 管家 agent（.env master key）
	KindAgentExternal Kind = "agent-external" // 外部 agent（受限 key，绑定用户）
)

// Principal 请求主体：认证中间件（HTTP requireAuth / MCP AuthMiddleware）
// 的产物，授权判定的入参。Name 是 owner 匹配口径（用户名；管家固定 "agent"）。
type Principal struct {
	Kind     Kind
	UID      int64   // users.id（user 与 external 有效；管家为 0）
	Name     string  // 用户名 / "agent"；同时是 owner_id 落库值
	Role     string  // admin / user；管家视作 admin
	GroupIDs []int64 // 所属组（group 可见性判定用）
}

// IsAdmin admin 或管家（两者都越过一切归属检查）。
func (p *Principal) IsAdmin() bool {
	return p != nil && (p.Role == "admin" || p.Kind == KindAgentMaster)
}

// IsMaster 仅管家（execute_command 的 shell 特权口径）。
func (p *Principal) IsMaster() bool {
	return p != nil && p.Kind == KindAgentMaster
}

// Subject 日志与审计的主体标识（未认证时 "anonymous"）。
func (p *Principal) Subject() string {
	if p == nil {
		return "anonymous"
	}
	return string(p.Kind) + ":" + p.Name
}

func (p *Principal) inGroup(gid int64) bool {
	for _, g := range p.GroupIDs {
		if g == gid {
			return true
		}
	}
	return false
}

// FileACL 文件侧授权输入（repo.FileMetadata 的投影，两侧共用）。
type FileACL struct {
	Owner      *string
	Visibility string // public / group / private；空 = 存量行，按 public 口径
	GroupID    *int64
}

func (f FileACL) visibility() string {
	if f.Visibility == "" {
		return "public"
	}
	return f.Visibility
}

func (f FileACL) ownedBy(p *Principal) bool {
	return f.Owner != nil && p != nil && *f.Owner == p.Name
}

// ACLOf 从 repo 行投影（调用方组装 FileACL 的便捷口）。
func ACLOf(owner *string, visibility string, groupID *int64) FileACL {
	return FileACL{Owner: owner, Visibility: visibility, GroupID: groupID}
}

// CanRead 读判定。返回 (是否允许, 拒绝原因)——原因供拒绝日志与回执，
// 允许时为空串。
func CanRead(p *Principal, f FileACL) (bool, string) {
	if p == nil {
		return false, "未认证主体"
	}
	if p.IsAdmin() {
		return true, ""
	}
	if f.ownedBy(p) {
		return true, ""
	}
	switch f.visibility() {
	case "public":
		return true, ""
	case "group":
		if f.GroupID != nil && p.inGroup(*f.GroupID) {
			return true, ""
		}
		return false, "组外文件"
	default:
		return false, "他人私文件"
	}
}

// CanWrite 写判定：admin/管家全权；owner 恒可写自己的；group 文件组内
// 协作；public 他人只读（共享不互踩）；无主遗产归 admin。
func CanWrite(p *Principal, f FileACL) (bool, string) {
	if p == nil {
		return false, "未认证主体"
	}
	if p.IsAdmin() {
		return true, ""
	}
	if f.ownedBy(p) {
		return true, ""
	}
	if f.Owner == nil {
		return false, "无主存量文件（写权归管理员）"
	}
	if f.visibility() == "group" && f.GroupID != nil && p.inGroup(*f.GroupID) {
		return true, ""
	}
	return false, "非本人文件（public 文件对他人只读）"
}

// —— context 贯通：HTTP 与 MCP 共用同一键，工具/handler 侧统一取用 ——

type ctxKey int

const principalKey ctxKey = 0

// WithPrincipal 把主体放进 context（认证中间件调用）。
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFrom 取主体；nil = 未经认证中间件（策略上按拒绝处理，
// 正常装配下不会出现——dev 无 key 模式也会注入管家主体）。
func PrincipalFrom(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey).(*Principal)
	return p
}
