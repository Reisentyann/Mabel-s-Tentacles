// 文件：manager-go/lineage_test.go —— 谱系域 L1：复制来源、复制衍生与移动关系
// 修改：2026-09-23（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"context"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
)

func TestRelatedReturnsCopyAndMoveRelations(t *testing.T) {
	m, st, _, _ := newTestManager(t)
	st.refs["u-source"] = &manager.FileRef{UUID: "u-source", Path: "source.txt"}
	st.refs["u-copy"] = &manager.FileRef{UUID: "u-copy", Path: "copy.txt"}
	st.rows["source.txt"] = &manager.MetaRow{Path: "source.txt", UUID: "u-source"}
	st.rows["copy.txt"] = &manager.MetaRow{Path: "copy.txt", UUID: "u-copy", CopiedFrom: "source.txt"}

	got, err := m.Related(context.Background(), "u-source")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (manager.Relation{UUID: "u-copy", Kind: "copied"}) {
		t.Fatalf("source relations = %+v", got)
	}

	got, err = m.Related(context.Background(), "u-copy")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (manager.Relation{UUID: "u-source", Kind: "copied"}) {
		t.Fatalf("copy relations = %+v", got)
	}

	st.rows["moved.txt"] = &manager.MetaRow{Path: "moved.txt", UUID: "u-copy", MovedFrom: "copy.txt"}
	st.refs["u-copy"].Path = "moved.txt"
	got, err = m.Related(context.Background(), "u-copy")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (manager.Relation{UUID: "u-copy", Kind: "moved"}) {
		t.Fatalf("moved relations = %+v", got)
	}
}

func TestRelatedMissingUUID(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	if _, err := m.Related(context.Background(), "missing"); err != manager.ErrNotFound {
		t.Fatalf("Related missing error = %v", err)
	}
}
