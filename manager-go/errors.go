// 文件：manager-go/errors.go —— 调用方可通过 errors.Is 判定的公共错误
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import "errors"

var (
	// ErrNotFound 表示元数据中不存在指定文件。
	ErrNotFound = errors.New("manager: uuid not found")
	// ErrDeleted 表示文件已软删除，不再提供内容。
	ErrDeleted = errors.New("manager: file soft-deleted")
	// ErrGhost 表示元数据存在但物理文件缺失。
	ErrGhost = errors.New("manager: file gone on disk")
	// ErrPending 表示入库或移动尚未完成发布。
	ErrPending = errors.New("manager: file intake not published")
	// ErrInvalidPath 表示逻辑路径或目录项名称不合法。
	ErrInvalidPath = errors.New("manager: invalid logic path")
	// ErrDirectoryExists 表示目标逻辑目录已存在。
	ErrDirectoryExists = errors.New("manager: directory already exists")
	// ErrKeyExists 表示目标逻辑键已被占用，包含软删除对象。
	ErrKeyExists = errors.New("manager: logic key already exists; choose another path, or use modify_file/patch_file to update an existing file")
)
