// 文件：manager-go/intake.go —— 入库域：文本写入与外部文件导入（逻辑键 + uuid 派生物理随机路径）
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

// maxIntakeBytes 单次写入内容上限（= describer.MaxFullBytes 单一来源，
// 与分析预算同口径：值得入库的分析粒度）。
const maxIntakeBytes = describer.MaxFullBytes

// WriteReceipt 入库回执：uuid（后续操作凭证）+ 逻辑键 + 物理相对路径
// （审计/排障用，agent 场景不依赖它）。
type WriteReceipt struct {
	UUID       string `json:"uuid"`
	LogicPath  string `json:"logic_path"`
	StorageRel string `json:"storage_rel"`
	SizeBytes  int64  `json:"size_bytes"`
}

// ImportReceipt 外部文件导入回执：与 WriteReceipt 同形，但内容来自已有的
// 本地文件。下载器等内部组件只需把源文件路径交给管理机，不需要知道
// uuid 派生的物理布局。
type ImportReceipt struct {
	UUID       string `json:"uuid"`
	LogicPath  string `json:"logic_path"`
	StorageRel string `json:"storage_rel"`
	SizeBytes  int64  `json:"size_bytes"`
}

// Write 新文件入库口：Reserve 原子占位 → uuid 派生物理路径 →
// 落盘。描述/落库/喂索引归编排机事件流（调用方提交），本口只管"文件在哪
// +内容落盘"。同逻辑键（包括软删、幽灵、占位行）拒绝；更新使用 Modify。
func (m *Manager) Write(ctx context.Context, logicPath, content string) (*WriteReceipt, error) {
	if err := validLogicPath(logicPath); err != nil {
		return nil, err
	}
	if len(content) > maxIntakeBytes {
		return nil, fmt.Errorf("security error: file content exceeds 5MB limit")
	}
	if err := m.ensureFilePathAvailable(ctx, logicPath); err != nil {
		return nil, err
	}
	return m.storeNewFile(ctx, logicPath, int64(len(content)), func(abs string) error {
		if err := publishNewFile(abs, strings.NewReader(content)); err != nil {
			return fmt.Errorf("write file: %w", err)
		}
		return nil
	})
}

// ImportFile 将本地普通文件流式复制到新 UUID 的物理路径并发布。
// 源文件保持原样，目标逻辑键必须未占用。
func (m *Manager) ImportFile(ctx context.Context, logicPath, sourcePath string) (*ImportReceipt, error) {
	if err := validLogicPath(logicPath); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sourcePath) == "" {
		return nil, errors.New("import file: source path is empty")
	}

	info, err := os.Lstat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("stat source file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("import file: symbolic links are not accepted")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("import file: source is not a regular file")
	}

	r, err := m.storeNewFile(ctx, logicPath, info.Size(), func(abs string) error {
		if err := transferImportedFile(sourcePath, abs); err != nil {
			return fmt.Errorf("store imported file: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &ImportReceipt{UUID: r.UUID, LogicPath: r.LogicPath, StorageRel: r.StorageRel, SizeBytes: r.SizeBytes}, nil
}

// storeNewFile 统一占位、物理发布与状态确认。publish 只有完整发布成功才返回 nil。
// 发布前失败归档占位；发布后确认失败保留 pending，供启动恢复。
// 来源校验和各入口的逻辑路径占用规则由调用方执行。
func (m *Manager) storeNewFile(ctx context.Context, logicPath string, size int64, publish func(string) error) (_ *WriteReceipt, resultErr error) {
	m.intake.RLock()
	defer m.intake.RUnlock()
	uuid, err := m.store.ReserveMeta(ctx, logicPath)
	if err != nil {
		return nil, fmt.Errorf("reserve meta: %w", err)
	}
	stored := false
	defer func() {
		if !stored {
			m.recoverFailedIntake(ctx, logicPath, uuid, &resultErr)
		}
	}()
	rel, err := StoragePathOf(uuid, logicPath)
	if err != nil {
		return nil, err
	}
	abs, err := m.resolve(rel)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}
	if err := publish(abs); err != nil {
		return nil, err
	}
	stored = true
	if err := m.store.CompleteIntake(ctx, logicPath, uuid); err != nil {
		return nil, fmt.Errorf("confirm stored file: %w", err)
	}
	return &WriteReceipt{UUID: uuid, LogicPath: logicPath, StorageRel: rel, SizeBytes: size}, nil
}

// transferImportedFile 流式复制源文件，排他发布目标并保留源文件。
func transferImportedFile(sourcePath, targetPath string) error {
	in, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer in.Close()

	return publishNewFile(targetPath, in)
}

// publishNewFile 正式路径只在完整写入并同步后出现。硬链接发布排他且不覆盖，
// 临时文件保留作证据；不支持硬链接的文件系统明确失败，不降级到非原子覆盖。
func publishNewFile(targetPath string, src io.Reader) error {
	out, err := os.OpenFile(targetPath+".partial", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, src)
	var syncErr error
	if copyErr == nil {
		syncErr = out.Sync()
	}
	closeErr := out.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := os.Link(targetPath+".partial", targetPath); err != nil {
		return fmt.Errorf("publish file: %w", err)
	}
	// 临时目录项仅作为写入证据，不参与读取和启动发布判断。
	return nil
}

func (m *Manager) ensureFilePathAvailable(ctx context.Context, logicPath string) error {
	for _, prefix := range logicPrefixes(logicPath) {
		if prefix == logicPath {
			exists, err := m.store.DirectoryExists(ctx, prefix)
			if err != nil {
				return fmt.Errorf("get directory: %w", err)
			}
			if exists {
				return ErrKeyExists
			}
			continue
		}
		if row, err := m.store.GetMeta(ctx, prefix); err != nil {
			return fmt.Errorf("get meta: %w", err)
		} else if row != nil {
			return fmt.Errorf("manager: path parent is a file: %s", prefix)
		}
	}
	return nil
}
