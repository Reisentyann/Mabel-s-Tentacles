// 文件：manager-go/tree.go —— 从元数据构建逻辑目录树与路径列表
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// LogicNode 是逻辑树节点，Type 为 dir 或 file。
type LogicNode struct {
	Name      string       `json:"name"`
	Path      string       `json:"path"`
	Type      string       `json:"type"`
	SizeBytes int64        `json:"size_bytes,omitempty"`
	UpdatedAt float64      `json:"updated_at,omitempty"`
	Children  []*LogicNode `json:"children,omitempty"`
}

// LogicTree 按逻辑键构建目录树，包含空目录，不包含软删文件。
func (m *Manager) LogicTree(ctx context.Context) ([]*LogicNode, error) {
	root := &LogicNode{}
	since := ""
	for {
		rows, err := m.store.ListMetaPage(ctx, since, 200)
		if err != nil {
			return nil, fmt.Errorf("logic tree scan: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			since = r.Path
			insertLogicNode(root, r.Path, r.SizeBytes, r.UpdatedAt)
		}
	}
	dirs, err := m.store.ListDirectoryPaths(ctx)
	if err != nil {
		return nil, fmt.Errorf("logic directory scan: %w", err)
	}
	for _, dir := range dirs {
		insertLogicDir(root, dir)
	}
	return sortLogicChildren(root.Children), nil
}

// LogicPaths 按字典序返回可见文件的逻辑键。
func (m *Manager) LogicPaths(ctx context.Context) ([]string, error) {
	since := ""
	out := []string{}
	for {
		rows, err := m.store.ListMetaPage(ctx, since, 200)
		if err != nil {
			return nil, fmt.Errorf("logic paths scan: %w", err)
		}
		if len(rows) == 0 {
			return out, nil
		}
		for _, r := range rows {
			since = r.Path
			out = append(out, r.Path)
		}
	}
}

func insertLogicNode(root *LogicNode, p string, size int64, updated time.Time) {
	node := ensureLogicNode(root, p)
	node.Type = "file"
	node.SizeBytes = size
	node.UpdatedAt = float64(updated.UnixNano()) / 1e9
}

func insertLogicDir(root *LogicNode, p string) {
	ensureLogicNode(root, p)
}

// ensureLogicNode 复用文件与目录的路径遍历，已有文件节点不会被目录覆盖。
func ensureLogicNode(root *LogicNode, p string) *LogicNode {
	cur := root
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		var next *LogicNode
		for _, child := range cur.Children {
			if child.Name == seg {
				next = child
				break
			}
		}
		if next == nil {
			next = &LogicNode{Name: seg, Path: strings.Join(parts[:i+1], "/"), Type: "dir"}
			cur.Children = append(cur.Children, next)
		}
		if next.Type != "file" {
			next.Type = "dir"
		}
		cur = next
	}
	return cur
}

// sortLogicChildren 使目录在前，同类节点按名称排序。
func sortLogicChildren(nodes []*LogicNode) []*LogicNode {
	sort.Slice(nodes, func(i, j int) bool {
		ti, tj := nodes[i].Type == "dir", nodes[j].Type == "dir"
		if ti != tj {
			return ti
		}
		return nodes[i].Name < nodes[j].Name
	})
	for _, n := range nodes {
		if n.Children != nil {
			n.Children = sortLogicChildren(n.Children)
		}
	}
	return nodes
}
