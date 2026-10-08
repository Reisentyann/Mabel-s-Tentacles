// 文件：manager-go/directory_context.go —— 入库事务的目录归属与复制来源上下文
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import "context"

type directoryIntakeKey struct{}
type directoryCopyKey struct{}

// DirectoryIntake 在数据库占位事务中固定目录及展示名称。
type DirectoryIntake struct{ ParentUUID, Name string }

// DirectoryIntakeFrom 返回本次占位事务的目录关联。
func DirectoryIntakeFrom(ctx context.Context) (DirectoryIntake, bool) {
	v, ok := ctx.Value(directoryIntakeKey{}).(DirectoryIntake)
	return v, ok
}

// WithDirectoryIntake 将目录关联传递给 Store.ReserveMeta。
func WithDirectoryIntake(ctx context.Context, parentUUID, name string) context.Context {
	return context.WithValue(ctx, directoryIntakeKey{}, DirectoryIntake{parentUUID, name})
}

// WithDirectoryCopy 由入库事务按源 UUID 固定复制谱系。
func WithDirectoryCopy(ctx context.Context, sourceUUID string) context.Context {
	return context.WithValue(ctx, directoryCopyKey{}, sourceUUID)
}

// DirectoryCopyFrom 返回本次占位事务的复制来源 UUID。
func DirectoryCopyFrom(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(directoryCopyKey{}).(string)
	return v, ok
}
