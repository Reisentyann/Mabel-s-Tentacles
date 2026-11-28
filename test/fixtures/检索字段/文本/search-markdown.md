# SEARCH_MD_2026 检索手册

更新时间：2026-11-24T09:30:00Z；负责人 @MabelIndex；联系 search.owner@example.com。

## 表格与清单

| 字段 | 示例 | 说明 |
| --- | --- | --- |
| 编码 | UTF-8 | 文本编码 |
| 标记 | SEARCH_MD_2026 | 唯一检索标记 |

| 域名 | 用途 |
| :--- | ---: |
| docs.example.org | 文档检索 |

1. 先检查文本编码
2. 再核对结构
    - 嵌套列表：四级空格缩进
	- 嵌套列表：制表符缩进
- 无序列表：确认表格
- [ ] 待办列表：检查引用

## 代码与引用

```go
package fixture

func Marker() string { return "SEARCH_MD_2026" }
```

> 引用：文本族不按扩展名，只看 textish。
> 缩进示例：
> 	引用中的制表符缩进

项目文档：[检索入口](https://docs.example.org/search?q=SEARCH_MD_2026)
服务状态：https://status.example.net/health
聊天话题：#文本检索 #检索字段；表情：🧭✅
