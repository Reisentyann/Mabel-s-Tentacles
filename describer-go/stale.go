// 文件：describer-go/stale.go —— 陈旧判定纯函数 IsStale（字典 10.2 四条：缺 ver / 版本落后 / checksum 变 / mtime 新）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package describer

import (
	"strconv"
	"time"
)

// IsStale 判定某家族的存量元数据是否需要重分析（字段字典第 10.2 节，纯函数）。
// 任一命中即陈旧：
//
//  1. 缺 cod-<family>-ver（老数据 / 新增插件）
//  2. ver < curVer（算法升级，FamilyVersion 提升）
//  3. storedChecksum != curChecksum（文件内容变了，绕过写入路径的改动也能发现；
//     curChecksum 为空表示调用方未提供，跳过本条；stored 为空视为老数据）
//  4. 文件 mtime 晚于 cod-<family>-at（快路径，先 stat 再决定要不要哈希）
//
// attrs 数值经 JSON 反序列化为 float64，亦兼容 int / int64 / 字符串数字。
func IsStale(attrs map[string]any, family string, curVer int, storedChecksum, curChecksum string, mtime time.Time) bool {
	verKey := "cod-" + family + "-ver"
	atKey := "cod-" + family + "-at"

	raw, ok := attrs[verKey]
	if !ok {
		return true // 条件 1
	}
	ver, ok := toInt(raw)
	if !ok || ver < curVer {
		return true // 条件 2（含脏数据）
	}
	if curChecksum != "" && storedChecksum != curChecksum {
		return true // 条件 3
	}
	if !mtime.IsZero() {
		if at, ok := toInt(attrs[atKey]); ok && mtime.Unix() > int64(at) {
			return true // 条件 4
		}
	}
	return false
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case string:
		n, err := strconv.Atoi(x)
		return n, err == nil
	}
	return 0, false
}
