// 文件：mcp-server-go/cmd/vizdump/report.go —— 报告渲染：单文件自包含 HTML（内联 CSS/JS，浏览器直接打开，零外部依赖）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package main

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"
)

// fieldStat 字段目录统计行（main.fieldStats 的产出）。
type fieldStat struct {
	Field   string
	Kind    string
	Files   int
	Samples string
}

// famGroup 属性按键前缀归族（cod-<family>-* / llm-* / sp-* / 其他）。
func famGroup(m map[string]any) []famEntry {
	fams := map[string][]kv{}
	for k, v := range m {
		fam := "其他"
		if strings.HasPrefix(k, "cod-") {
			parts := strings.SplitN(k, "-", 3)
			if len(parts) >= 2 {
				fam = parts[0] + "-" + parts[1]
			}
		} else if i := strings.Index(k, "-"); i > 0 {
			fam = k[:i]
		}
		fams[fam] = append(fams[fam], kv{Key: k, Val: fmt.Sprint(v)})
	}
	names := make([]string, 0, len(fams))
	for f := range fams {
		names = append(names, f)
	}
	sort.Strings(names)
	out := make([]famEntry, 0, len(names))
	for _, f := range names {
		kvs := fams[f]
		sort.Slice(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })
		out = append(out, famEntry{Family: f, KVs: kvs})
	}
	return out
}

type kv struct{ Key, Val string }
type famEntry struct {
	Family string
	KVs    []kv
}

// renderReport 渲染自包含 HTML。
func renderReport(dataDir string, rows []fileRow) (string, error) {
	active, deleted, ghost, drift, diskBytes := 0, 0, 0, 0, int64(0)
	for _, r := range rows {
		if r.IsDeleted {
			deleted++
		} else {
			active++
		}
		if !r.DiskOK {
			ghost++
		}
		if r.HashMatch == "drift" {
			drift++
		}
		if r.DiskOK {
			diskBytes += r.DiskSize
		}
	}

	cards := make([]fileCard, 0, len(rows))
	for i := range rows {
		cards = append(cards, toCard(&rows[i]))
	}

	data := struct {
		Generated time.Time
		DataDir   string
		Active    int
		Deleted   int
		Ghost     int
		Drift     int
		Files     int
		DiskBytes string
		Cards     []fileCard
		FieldDir  []fieldStat
	}{
		Generated: time.Now(),
		DataDir:   dataDir,
		Active:    active,
		Deleted:   deleted,
		Ghost:     ghost,
		Drift:     drift,
		Files:     len(rows),
		DiskBytes: human(diskBytes),
		Cards:     cards,
		FieldDir:  fieldStats(rows),
	}

	tpl, err := template.New("r").Funcs(template.FuncMap{"hum": human}).Parse(reportHTML)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// fileCard 模板视图（fileRow 的展示投影）。
type fileCard struct {
	LogicPath   string
	UUID        string
	StorageAbs  string
	Title       string
	Description string
	Tags        []string
	FileType    string
	MimeType    string
	Extension   string
	Scope       string
	OwnerID     string
	Visibility  string
	SizeBytes   string
	Checksum    string
	MissRounds  int
	IsDeleted   bool
	CopiedFrom  string
	DLCnt       int64
	UpdatedAt   string
	CreatedAt   string
	DiskOK      bool
	DiskSize    string
	DiskMtime   string
	HashMatch   string
	Families    []famEntry
	AttrCount   int
}

func toCard(r *fileRow) fileCard {
	c := fileCard{
		LogicPath:   r.LogicPath,
		UUID:        r.UUID,
		StorageAbs:  r.StorageAbs,
		Title:       r.Title,
		Description: r.Description,
		Tags:        r.Tags,
		FileType:    r.FileType,
		MimeType:    r.MimeType,
		Extension:   r.Extension,
		Scope:       r.Scope,
		OwnerID:     r.OwnerID,
		Visibility:  r.Visibility,
		SizeBytes:   human(r.SizeBytes),
		Checksum:    r.Checksum,
		MissRounds:  r.MissRounds,
		IsDeleted:   r.IsDeleted,
		CopiedFrom:  r.CopiedFrom,
		DLCnt:       r.DLCnt,
		UpdatedAt:   r.UpdatedAt.Format("2006-01-02 15:04:05"),
		CreatedAt:   r.CreatedAt.Format("2006-01-02 15:04:05"),
		DiskOK:      r.DiskOK,
		HashMatch:   r.HashMatch,
		Families:    famGroup(r.Attrs),
		AttrCount:   len(r.Attrs),
	}
	if c.Title == "" {
		c.Title = "—"
	}
	if c.Description == "" {
		c.Description = "—"
	}
	if len(c.Tags) == 0 {
		c.Tags = []string{}
	}
	if c.StorageAbs == "" {
		c.StorageAbs = "（uuid 异常，未派生）"
	}
	if c.DiskOK {
		c.DiskSize = human(r.DiskSize)
		c.DiskMtime = r.DiskMtime.Format("2006-01-02 15:04:05")
	} else {
		c.DiskSize, c.DiskMtime = "—", "—"
	}
	if len(c.Checksum) > 16 {
		c.Checksum = c.Checksum[:16] + "…"
	}
	return c
}

// human 字节的人类读法。
func human(n int64) string {
	const u = "BKMGT"
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(u)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %cB", f, u[i])
}

// reportHTML 模板本体：内联样式 + 一段过滤 JS；卡片按 updated_at 倒序由调用方保证。
const reportHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>文件可视化导出 · Mabel's Tentacles</title>
<style>
  :root { --bg:#f6f7f9; --card:#fff; --ink:#1c2430; --dim:#68758a; --line:#e3e7ee;
          --accent:#3b6fd4; --bad:#c23a3a; --warn:#b07a1e; --ok:#2c7a4b; --chip:#eef2f9; }
  * { box-sizing: border-box; }
  body { margin:0; background:var(--bg); color:var(--ink);
         font:14px/1.6 "Segoe UI","Microsoft YaHei",system-ui,sans-serif; }
  header { position:sticky; top:0; z-index:9; background:var(--card); border-bottom:1px solid var(--line);
           padding:12px 20px; display:flex; flex-wrap:wrap; gap:14px; align-items:center; }
  header h1 { font-size:17px; margin:0; }
  .meta { color:var(--dim); font-size:12.5px; }
  .chips { display:flex; flex-wrap:wrap; gap:8px; }
  .chip { background:var(--chip); border-radius:12px; padding:2px 10px; font-size:12.5px; }
  .chip b { color:var(--ink); }
  .chip.bad b { color:var(--bad); }
  #q { margin-left:auto; padding:6px 12px; border:1px solid var(--line); border-radius:8px;
       width:260px; font-size:13px; outline-color:var(--accent); }
  main { max-width:1080px; margin:0 auto; padding:18px 20px 60px; }
  .card { background:var(--card); border:1px solid var(--line); border-radius:10px;
          padding:14px 18px; margin:14px 0; }
  .card.hide { display:none; }
  .path { font-size:15.5px; font-weight:600; word-break:break-all; }
  .badges { margin-left:8px; white-space:nowrap; }
  .bd { display:inline-block; font-size:11.5px; border-radius:5px; padding:1px 7px;
        margin-right:4px; vertical-align:2px; }
  .bd.t { background:#e7eefc; color:#2b4f9e; } .bd.v { background:#e9f5ee; color:var(--ok); }
  .bd.v.pri { background:#fdeeee; color:var(--bad); } .bd.o { background:#f1ecfa; color:#6448b0; }
  .bd.del { background:#fdeeee; color:var(--bad); font-weight:700; }
  .bd.ghost { background:#fff3e0; color:var(--warn); font-weight:700; }
  .bd.drift { background:#fdeeee; color:var(--bad); font-weight:700; }
  .bd.ok { background:#e9f5ee; color:var(--ok); }
  table { border-collapse:collapse; width:100%; margin-top:8px; font-size:13px; }
  td { border-top:1px solid var(--line); padding:4px 8px; vertical-align:top; }
  td.k { color:var(--dim); white-space:nowrap; width:120px; }
  td.v { word-break:break-all; font-family:Consolas,"Microsoft YaHei",monospace; }
  .tags span { display:inline-block; background:var(--chip); border-radius:10px;
               padding:0 9px; margin:0 4px 4px 0; font-size:12px; }
  .fam { margin-top:10px; }
  .fam h4 { margin:10px 0 4px; font-size:13px; color:var(--accent); }
  .kv { display:grid; grid-template-columns:repeat(auto-fill,minmax(320px,1fr)); gap:2px 14px; }
  .kv div { font-size:12.5px; border-bottom:1px dashed var(--line); padding:2px 0;
            display:flex; gap:8px; }
  .kv .k2 { color:var(--dim); min-width:210px; word-break:break-all; }
  .kv .v2 { word-break:break-all; font-family:Consolas,monospace; }
  section.dir h2, main > h2 { font-size:16px; margin:26px 0 6px; }
  .note { color:var(--dim); font-size:12.5px; margin-bottom:10px; }
  footer { color:var(--dim); font-size:12px; margin-top:30px; border-top:1px solid var(--line);
           padding-top:10px; }
</style>
</head>
<body>
<header>
  <h1>文件可视化导出</h1>
  <div class="chips">
    <span class="chip">文件 <b>{{.Files}}</b></span>
    <span class="chip">活跃 <b>{{.Active}}</b></span>
    {{if .Deleted}}<span class="chip bad">软删 <b>{{.Deleted}}</b></span>{{end}}
    {{if .Ghost}}<span class="chip bad">盘缺 <b>{{.Ghost}}</b></span>{{end}}
    {{if .Drift}}<span class="chip bad">校验漂移 <b>{{.Drift}}</b></span>{{end}}
    <span class="chip">盘上总量 <b>{{.DiskBytes}}</b></span>
  </div>
  <div class="meta">DATA_DIR：{{.DataDir}}<br>生成：{{.Generated.Format "2006-01-02 15:04:05"}}</div>
  <input id="q" type="search" placeholder="过滤：路径 / 标题 / 描述 / 标签 / 字段…">
</header>
<main>
  <h2>文件清单</h2>
  <div class="note">逻辑键 → uuid → 物理绝对路径 → 盘上实况（存在/大小/sha-256 对账）→ 描述机全属性。
  软删行照列（红标）；盘缺 = DB 有行但 uuid 派生位无文件（幽灵，3 轮后自动软删）；
  校验漂移 = 盘上内容与 DB checksum 不符（多半被命令直改过）。</div>
  {{range .Cards}}
  <div class="card" data-s="{{.LogicPath}} {{.Title}} {{.Description}} {{range .Tags}}{{.}} {{end}}{{range .Families}}{{range .KVs}}{{.Key}} {{end}}{{end}}">
    <div><span class="path">{{.LogicPath}}</span>
      <span class="badges">
        {{if .FileType}}<span class="bd t">{{.FileType}}</span>{{end}}
        {{if eq .Visibility "private"}}<span class="bd v pri">private{{else}}<span class="bd v">{{.Visibility}}{{end}}</span>
        {{if .OwnerID}}<span class="bd o">owner:{{.OwnerID}}</span>{{end}}
        {{if .IsDeleted}}<span class="bd del">已软删</span>{{end}}
        {{if not .DiskOK}}<span class="bd ghost">盘上缺失</span>{{end}}
        {{if eq .HashMatch "drift"}}<span class="bd drift">校验漂移</span>
        {{else if eq .HashMatch "ok"}}<span class="bd ok">校验一致</span>{{end}}
      </span>
    </div>
    <table>
      <tr><td class="k">uuid</td><td class="v">{{.UUID}}</td>
          <td class="k">物理路径</td><td class="v">{{.StorageAbs}}</td></tr>
      <tr><td class="k">盘上</td><td class="v">{{if .DiskOK}}存在 · {{.DiskSize}} · mtime {{.DiskMtime}}{{else}}<b style="color:var(--bad)">缺失</b>{{end}}</td>
          <td class="k">DB size / checksum</td><td class="v">{{.SizeBytes}} / {{.Checksum}}</td></tr>
      <tr><td class="k">title</td><td class="v">{{.Title}}</td>
          <td class="k">description</td><td class="v">{{.Description}}</td></tr>
      <tr><td class="k">mime / ext</td><td class="v">{{.MimeType}} / {{.Extension}}</td>
          <td class="k">scope / miss / dl</td><td class="v">{{.Scope}} / {{.MissRounds}} 轮 / {{.DLCnt}} 次</td></tr>
      {{if .CopiedFrom}}<tr><td class="k">谱系</td><td class="v" colspan="3">copied_from：{{.CopiedFrom}}</td></tr>{{end}}
      {{if .Tags}}<tr><td class="k">tags</td><td class="v tags" colspan="3">{{range .Tags}}<span>{{.}}</span>{{end}}</td></tr>{{end}}
      <tr><td class="k">时间</td><td class="v" colspan="3">建 {{.CreatedAt}} · 更 {{.UpdatedAt}}</td></tr>
    </table>
    {{range .Families}}
    <div class="fam"><h4>{{.Family}}（{{len .KVs}}）</h4>
      <div class="kv">{{range .KVs}}<div><span class="k2">{{.Key}}</span><span class="v2">{{.Val}}</span></div>{{end}}</div>
    </div>
    {{end}}
  </div>
  {{else}}<div class="card">（file_metadata 无行——库是空的？）</div>{{end}}

  <section class="dir">
    <h2>字段目录统计</h2>
    <div class="note">描述机产出键的覆盖面（活跃行口径，与索引机目录一致；kind 为粗判）。</div>
    <table>
      <tr><td class="k">字段</td><td class="k">kind</td><td class="k">覆盖文件</td><td class="k">样本值</td></tr>
      {{range .FieldDir}}<tr><td class="v">{{.Field}}</td><td class="v">{{.Kind}}</td><td class="v">{{.Files}}</td><td class="v">{{.Samples}}</td></tr>{{end}}
    </table>
  </section>

  <footer>vizdump · 观察者工具：直读 PostgreSQL（含软删）+ 盘面 stat + sha-256 对账 ·
  数据以 DB 为事实源，索引/缓存不在报告口径内</footer>
</main>
<script>
  const q = document.getElementById('q');
  q.addEventListener('input', () => {
    const s = q.value.trim().toLowerCase();
    document.querySelectorAll('.card').forEach(c => {
      c.classList.toggle('hide', s && !c.dataset.s.toLowerCase().includes(s));
    });
  });
</script>
</body>
</html>`
