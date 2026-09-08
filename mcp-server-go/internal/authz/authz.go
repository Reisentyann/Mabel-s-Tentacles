// 文件：mcp-server-go/internal/authz/authz.go —— 授权单点：Principal 主体模型 + CanRead/CanWrite 策略
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

// Package authz 是权限判定的唯一策略源：
// HTTP（api 层）与 MCP（tools 层）两侧的每一次读写判定都来这里，
// 不允许各自内联 if——策略改动一处生效，审计日志一处对齐。
//
// 两级可见性模型（2026-09-08 简化定稿，替代 owner/group/visibility 三轴矩阵）：
//
//	防线在门口：接入校验（master key / external key 绑定用户）防陌生人，
//	接入之后不做授权矩阵——信任接入者。唯一保留的文件级判定是
//	private 与 group（存量兼容按 public 对待）：
//
//	          读 public        读 private         写 public        写 private
//	接入者    ✅              ❌（仅主人）      ✅（协作）       ❌（仅主人）
//	admin     ✅              ✅                ✅              ✅
//	管家      ✅              ✅                ✅              ✅
//
//	private 的语义 = 仅主人与管家可见可动（防搞乱的核心：私人物品不许碰）；
//	public（新默认）= 共享协作，任何接入者可读写。新文件默认 public。
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
	Visibility string // public / private；group 与空 = 存量兼容，按 public 口径
	GroupID    *int64 // 组判定已退役（2026-09-08 两级模型），列保留投影兼容
}

func (f FileACL) visibility() string {
	// 两级归一（2026-09-08）：group 是三轴矩阵的存量值，退役按 public 对待
	if f.Visibility == "private" {
		return "private"
	}
	return "public"
}

func (f FileACL) ownedBy(p *Principal) bool {
	return f.Owner != nil && p != nil && *f.Owner == p.Name
}

// ACLOf 从 repo 行投影（调用方组装 FileACL 的便捷口）。
func ACLOf(owner *string, visibility string, groupID *int64) FileACL {
	return FileACL{Owner: owner, Visibility: visibility, GroupID: groupID}
}

// CanRead 读判定（两级模型）：admin/管家全通；主人全通；public 任何
// 接入者可读（共享）；private 他人拒（私密 = 仅主人与管家）。
// 返回 (是否允许, 拒绝原因)——原因供拒绝日志与回执，允许时为空串。
func CanRead(p *Principal, f FileACL) (bool, string) {
	if p == nil {
		return false, "未认证主体"
	}
	if p.IsAdmin() || f.ownedBy(p) {
		return true, ""
	}
	if f.visibility() == "private" {
		return false, "他人私文件"
	}
	return true, ""
}

// CanWrite 写判定（两级模型）：接入即信任——public 任何接入者可写
// （协作共享，含无主存量）；private 仅主人与管家（防搞乱的核心）。
func CanWrite(p *Principal, f FileACL) (bool, string) {
	if p == nil {
		return false, "未认证主体"
	}
	if p.IsAdmin() || f.ownedBy(p) {
		return true, ""
	}
	if f.visibility() == "private" {
		return false, "他人私文件（private 仅主人可动）"
	}
	return true, ""
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
