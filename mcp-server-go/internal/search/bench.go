// 文件：mcp-server-go/internal/search/bench.go —— 检索字段基准注册表：每字段一句含义 + 分档基准（自然语言 → 查询值的翻译表）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// Package search 的基准面：agent 拿到字段目录（list_index_fields /
// GET /api/index/fields）时只知道"有哪些字段、什么桶型、当前取值"，
// 不知道每个字段在量什么、值到多大算"高"——本注册表补这一层：
// Desc = 字段含义一句话；Bench = 分档基准（经验校准，供 agent 把
// "长文/对话多/暗图"这类自然语言翻译成查询值）。
//
// 维护规则（与 logging 双侧同步同款）：
//   - 语义权威 = docs/元数据字段说明.md（先改字典再写代码）；
//     分档基准的人类审读版 = docs/检索契约.md——三处同步改
//   - 注册表只收确定会出现在 attributes 的键（cod-* 硬编码面 +
//     llm-* 受控词表）；sp-* 自由区长尾不枚举（目录的 kind/values
//     本身就是它们的发现面）
//   - 分档是经验锚点不是真理：库内实测值域（目录的 min/max）优先，
//     基准用于跨库的先验分档
package search

// FieldBench 单字段的查询基准。
type FieldBench struct {
	Desc  string // 含义一句话（量的是什么）
	Bench string // 分档基准（低/中/高档位与典型场景）
}

// registry 字段基准表。键 = attributes 里的字段名。
var registry = map[string]FieldBench{
	// —— cod-basic（一切文件）——
	"cod-basic-mime":         {Desc: "魔数嗅探的真实 MIME（读文件头 512B，不看扩展名）", Bench: "eq 精确匹配（image/png、text/plain…）；扩展名可疑时以它为准"},
	"cod-basic-mime-match":   {Desc: "内容 MIME 与扩展名推断是否一致；false = 扩展名伪造嫌疑", Bench: "eq false 圈可疑伪装文件"},
	"cod-basic-mtime":        {Desc: "文件修改时间（unix 秒，盘上事实；与元数据 updated_at 不同物）", Bench: "range [起,止] 圈时间窗；unix 秒口径（如 2026-09-01 ≈ 1788x 万级）"},
	"cod-basic-textish":      {Desc: "是否文本（无 NUL 且控制字符少；GBK/带BOM UTF-16 也算）", Bench: "eq true/false 分流文本与二进制"},
	"cod-basic-entropy":      {Desc: "字节香农熵 0-8 bits/byte", Bench: "文本≈4.5-6.5；结构化数据≈6-7.5；压缩/加密≈7.5-8——gt 7.5 基本是压缩件/密件"},
	"cod-basic-executable":   {Desc: "可执行魔数（PE/ELF/Mach-O）", Bench: "eq true 找误存的程序或伪装 exe"},
	"cod-basic-name-pattern": {Desc: "文件名模式（正则归类）", Bench: "取值 camera/screenshot/timestamped/uuid-like/versioned/hashlike/plain；eq screenshot 圈截图类"},
	"cod-basic-at":           {Desc: "basic 家族分析时间戳（unix 秒，重分析机制内部用）", Bench: "一般不作检索条件；误用请改 mtime/updated_at"},
	"cod-basic-ver":          {Desc: "basic 家族分析版本号（陈旧判定内部用）", Bench: "一般不作检索条件"},

	// —— cod-text 基础统计 ——
	"cod-text-lines":           {Desc: "行数——文章长度口径一", Bench: "微型<10；短文10-50；中等50-200；长文>200；超长>1000"},
	"cod-text-chars":           {Desc: "字符数——文章长度口径二", Bench: "便签<500；短文500-3000；中等3k-1万；长文>1万"},
	"cod-text-cjk-ratio":       {Desc: "中日韩字符占比 0-1", Bench: "纯中文>0.8；中文为主>0.5；混排0.2-0.5；几乎无中文<0.2"},
	"cod-text-language":        {Desc: "语言（字符集启发式判定）", Bench: "取值 zh/en/ja/mixed/unknown；中英混排常落 mixed——要圈中文正文用 cjk-ratio>0.5 更准"},
	"cod-text-encoding":        {Desc: "文本编码（解码链嗅探）", Bench: "utf-8 为主流；gbk=老中文文件；utf-16=Windows 程序产出"},
	"cod-text-top-keywords":    {Desc: "前 10 高频词（CJK 用 2-gram，无语义的频率统计）", Bench: "eq/in 匹配任一关键词，做'出现过某词'的粗筛；精确语义检索请用检索 Text 关键词"},
	"cod-text-title-line":      {Desc: "首个标题行或首行（截 80 字符）——标题猜测", Bench: "eq 精确匹配或 in 多候选；识别'这是什么文件'的最快路径"},
	"cod-text-headings":        {Desc: "前 8 个标题（章节名/文档目录）", Bench: "eq/in 找章节名或目录项"},
	"cod-text-structure":       {Desc: "检测到的文档结构（多选数组）", Bench: "取值 headings/lists/code-blocks/tables/frontmatter；eq 任一命中（multi 桶）"},
	"cod-text-blank-ratio":     {Desc: "空行率 0-1", Bench: "日志/数据<0.1；散文小说0.15-0.4；松散笔记>0.4"},
	"cod-text-shebang":         {Desc: "首行 #! 解释器声明（脚本指纹）", Bench: "eq '#!/bin/bash'、'#!/usr/bin/env python3' 等圈脚本"},
	"cod-text-final-newline":   {Desc: "文件是否以换行结尾（POSIX 文本规范）", Bench: "false=Windows 编辑痕迹或截断文件"},
	"cod-text-non-ascii-ratio": {Desc: "非 ASCII（>127）字符占比", Bench: "英文纯文本≈0；中文文本>0.3"},
	"cod-text-upper-ratio":     {Desc: "大写拉丁字母 / 拉丁字母总数（无拉丁字母不产）", Bench: "正常文本<0.3；常量/枚举定义文件>0.5；全大写吼叫体≈1"},
	"cod-text-word-count":      {Desc: "词数（拉丁词 + CJK 每字一词 + 假名）", Bench: "短段<50；短文50-300；中等300-1500；长文>1500"},
	"cod-text-avg-word-len":    {Desc: "拉丁词均长（rune；无拉丁词不产）", Bench: "日常英语3-5；技术/学术文档>6"},

	// —— cod-text 结构量化 ——
	"cod-text-table-blocks":          {Desc: "表格块数（连续 |…| 行的段数）", Bench: "gt 0 = 含 markdown 表格"},
	"cod-text-table-rows":            {Desc: "表格总行数", Bench: "gt 0 含表；>10 是数据密集表"},
	"cod-text-table-cols":            {Desc: "表格最大列数", Bench: "2-3 常规；≥5 宽表（数据表嫌疑）"},
	"cod-text-list-items":            {Desc: "列表项总数（-/+/数字前缀行）", Bench: "清单类>5；gt 10 重清单文件"},
	"cod-text-list-nesting-max":      {Desc: "列表最大嵌套深度（顶层=1）", Bench: "1=平铺清单；≥3=复杂大纲/脑图式"},
	"cod-text-ordered-ratio":         {Desc: "有序列表占比（无列表项不产）", Bench: ">0.5 偏步骤/教程/排行；≈0 全无序清单"},
	"cod-text-code-lines":            {Desc: "fence 围栏内的代码行数（围栏行不计）", Bench: "gt 0 = 文内带代码块；>50 教程/技术文档型"},
	"cod-text-quote-lines":           {Desc: "> 引用行数", Bench: "gt 0 = 邮件/摘录/引用体"},
	"cod-text-checkboxes":            {Desc: "复选框行计数（- [ ] / - [x]）——TODO 清单指纹", Bench: "gt 0 = 待办清单；≥5 重计划文件"},
	"cod-text-indent-max":            {Desc: "最大缩进层级（空格1、tab2，÷2 取整）", Bench: "1-2 常规文本；≥4 深嵌套结构/数据"},
	"cod-text-trailing-space-lines":  {Desc: "行尾空白行计数", Bench: "gt 0 = 脏文件（lint 视角）"},
	"cod-text-consecutive-blank-max": {Desc: "最大连续空行数", Bench: "≥3 = 排版松散/分段暴力"},
	"cod-text-indent-style":          {Desc: "缩进风格（按行首统计）", Bench: "取值 tab/space/mixed/none；mixed=多人/多工具编辑痕迹"},

	// —— cod-text 行文指纹 ——
	"cod-text-avg-line-len":         {Desc: "行均长（rune，全体行）", Bench: "日志/代码 20-80；硬换行散文 30-60；软换行长段 >100"},
	"cod-text-line-len-std":         {Desc: "行长标准差", Bench: "≈0 规整日志/机器产出；>10 自然文（长短错落）；>30 小说/混合体"},
	"cod-text-digit-ratio":          {Desc: "数字字符占比（Unicode 数字）", Bench: "文学<0.02；混合0.02-0.1；数据文件>0.3"},
	"cod-text-timestamp-count":      {Desc: "时间戳计数（日期可选时间）", Bench: "gt 5 大概率日志；0 = 非时间序列文"},
	"cod-text-time-span":            {Desc: "最早-最晚时间戳跨度（秒；≥2 个才产）", Bench: "日志特征字段；range 圈记录时长"},
	"cod-text-repeat-line-ratio":    {Desc: "重复行占比（出现 ≥2 次的非空行）", Bench: "≈0 原创文本；>0.3 复读/日志/模板"},
	"cod-text-paragraphs":           {Desc: "段落数（空行分隔的连续段）", Bench: "1=单段碎片；>10 多段长文"},
	"cod-text-avg-para-len":         {Desc: "段均长（rune）", Bench: "聊天/碎片<40；散文 40-150；小说长段>100"},
	"cod-text-sentences":            {Desc: "句数（句末标点切分）", Bench: "配合 avg-sent-len 判断文体；单独参考意义弱"},
	"cod-text-avg-sent-len":         {Desc: "句均长（rune）", Bench: "口语/聊天<15；常规 15-35；书面长句>35"},
	"cod-text-dialog-ratio":         {Desc: "对话密度（配对引号包裹字符占比 0-1）", Bench: "无对话<0.05；偶有对白0.05-0.15；对话体>0.15；对白为主>0.4"},
	"cod-text-punct-density":        {Desc: "标点密度（Unicode 标点 / 总字符）", Bench: "中文正文0.1-0.2；<0.05 列表/代码感；>0.25 强标点文体"},
	"cod-text-char-diversity":       {Desc: "字符多样性（不同字符 / 总字符）", Bench: "模板/复读<0.1；丰富自然文>0.3"},
	"cod-text-url-count":            {Desc: "URL 计数（http(s)://）", Bench: "gt 0 = 含链接；eq 0 排除链接体"},
	"cod-text-email-count":          {Desc: "邮箱计数", Bench: "gt 0 = 含联系方式/邮件体"},
	"cod-text-mention-count":        {Desc: "@提及计数", Bench: "gt 0 = 聊天/社群记录感"},
	"cod-text-hashtag-count":        {Desc: "#标签计数（不误判 # 标题）", Bench: "gt 0 = 社媒体"},
	"cod-text-emoji-count":          {Desc: "emoji 计数", Bench: "gt 0 = 聊天/轻松体；≥5 emoji 密集"},
	"cod-text-link-count":           {Desc: "markdown 链接计数（[t](u)）", Bench: "gt 0 = 文档/README 型"},
	"cod-text-eol":                  {Desc: "换行符类型", Bench: "取值 crlf/lf/mixed/none；crlf=Windows 产出；mixed=拼接痕迹"},
	"cod-text-has-bom":              {Desc: "是否带 BOM（UTF-8/UTF-16）", Bench: "true=Windows 编辑器产出"},
	"cod-text-longest-line":         {Desc: "最长行长度（rune）", Bench: ">500 单行超长=minified/数据 dump 嫌疑"},
	"cod-text-minified":             {Desc: "压缩单行判定（≤3 有效行且最长行>500）", Bench: "eq true 找 minified js/json"},
	"cod-text-timestamp-line-ratio": {Desc: "时间戳开头行占比", Bench: ">0.5 规整日志；<0.1 非日志"},
	"cod-text-bracket-balance":      {Desc: "括号配平率 0-1（全配平=1；无括号不产）", Bench: "1=配平健康；<0.8 截断/残缺嫌疑"},

	// —— cod-image（png/jpg/gif/webp/bmp）——
	"cod-image-width":           {Desc: "图片宽（像素）", Bench: "缩略图<300；常规300-1920；高清>1920"},
	"cod-image-height":          {Desc: "图片高（像素）", Bench: "同 width 分档"},
	"cod-image-aspect":          {Desc: "约分宽高比", Bench: "eq '16:9'/'1:1' 等圈画幅"},
	"cod-image-orientation":     {Desc: "朝向", Bench: "取值 portrait/landscape/square"},
	"cod-image-megapixels":      {Desc: "百万像素（1 位小数）", Bench: "图标<0.3；常规照片0.3-2；高清>2；大图>12"},
	"cod-image-palette":         {Desc: "前 5 主色（降采样 k-means，hex+占比）", Bench: "in 匹配色板（数组桶任一命中）"},
	"cod-image-dominant":        {Desc: "第一主色 hex", Bench: "eq 精确颜色值"},
	"cod-image-family":          {Desc: "主色调色系（HSL 聚类）", Bench: "取值 red/orange/yellow/green/cyan/blue/purple/neutral/grayscale；eq 圈主色调"},
	"cod-image-brightness":      {Desc: "平均亮度 0-100", Bench: "暗图<25；正常25-80；亮图>80"},
	"cod-image-contrast":        {Desc: "亮度标准差", Bench: "<10 平淡/扁平；>30 高对比"},
	"cod-image-grayscale":       {Desc: "是否黑白图", Bench: "eq true 圈黑白"},
	"cod-image-has-alpha":       {Desc: "是否带透明通道", Bench: "eq true 素材/图标嫌疑"},
	"cod-image-alpha-ratio":     {Desc: "半透明像素占比（a<250；无通道产 0）", Bench: ">0.1 透明素材；≈0 实拍/完整画面"},
	"cod-image-animated":        {Desc: "是否动图", Bench: "eq true 圈动图"},
	"cod-image-frames":          {Desc: "帧数（仅动图）", Bench: ">24 长动图"},
	"cod-image-taken-at":        {Desc: "EXIF 拍摄时间（unix 秒；截图/生成图无此键）", Bench: "range 圈时间线；无 EXIF 不产——'键不存在'本身可区分实拍与生成"},
	"cod-image-camera":          {Desc: "EXIF 相机型号", Bench: "eq 圈设备（Pixel/iPhone…）"},
	"cod-image-software":        {Desc: "EXIF 软件字段", Bench: "eq ComfyUI/Photoshop 等圈生成/加工图"},
	"cod-image-color-count":     {Desc: "量化唯一色数 0-4096", Bench: "扁平插画<64；照片>256"},
	"cod-image-flat-ratio":      {Desc: "大色桶（≥5% 覆盖）合计占比", Bench: ">0.9 扁平插画/图标；<0.5 照片"},
	"cod-image-dark":            {Desc: "整体暗图（亮度<25）", Bench: "eq true 夜景/暗调一键圈"},
	"cod-image-light":           {Desc: "整体亮图（亮度>80）", Bench: "eq true 过曝/白底"},
	"cod-image-saturation":      {Desc: "平均饱和度 0-100", Bench: "灰暗<15；正常15-60；艳丽>60"},
	"cod-image-warm-ratio":      {Desc: "暖色像素占比（红橙段，灰不计）", Bench: ">0.6 暖调；<0.2 偏冷"},
	"cod-image-cool-ratio":      {Desc: "冷色像素占比（蓝青紫段）", Bench: ">0.4 冷调"},
	"cod-image-edge-density":    {Desc: "边缘密度（Sobel 强梯度像素占比）", Bench: "线稿/文字截图>0.1；照片<0.05"},
	"cod-image-sharpness":       {Desc: "清晰度（拉普拉斯方差）", Bench: "<100 模糊；>500 锐利"},
	"cod-image-symmetry":        {Desc: "水平镜像对称度 0-1", Bench: ">0.7 海报/封面对称构图"},
	"cod-image-skin-tone-ratio": {Desc: "肤色规则像素占比", Bench: ">0.2 人像信号；>0.5 特写/半身"},
	"cod-image-entropy":         {Desc: "luma 直方图熵 0-5", Bench: "纯色≈0；复杂画面≈4"},

	// —— cod-code（源码扩展名追加）——
	"cod-code-lang":           {Desc: "编程语言（按扩展名）", Bench: "eq python/go/shell/javascript… 圈语言"},
	"cod-code-imports":        {Desc: "前 10 条 import/require", Bench: "in 找'用过某库'的代码"},
	"cod-code-todo-count":     {Desc: "TODO/FIXME 计数", Bench: "gt 0 有欠债；≥5 重灾文件"},
	"cod-code-generated":      {Desc: "自动生成文件标记（头部声明匹配）", Bench: "eq true 圈生成件（勿手改）"},
	"cod-code-package":        {Desc: "go 包名声明（仅 go）", Bench: "eq main 圈入口程序"},
	"cod-code-func-count":     {Desc: "函数/类定义计数", Bench: "小<10；中10-50；大>50"},
	"cod-code-comment-ratio":  {Desc: "注释行 / 非空行", Bench: "稀薄<0.05；健康0.1-0.3；文档型>0.4"},
	"cod-code-test-file":      {Desc: "测试文件判定（文件名模式）", Bench: "eq true 圈测试"},
	"cod-code-license":        {Desc: "许可证识别（前 10 行）", Bench: "eq MIT/Apache-2.0/GPL-3.0…"},
	"cod-code-exported-names": {Desc: "前 10 导出符号（go 大写 / python 公开）", Bench: "in 找 API 面"},

	// —— llm 轨（受控词表，仅经过 LLM 描述的文件才有）——
	"llm-semantic-type": {Desc: "模糊类型（模型判定的受控词表）", Bench: "取值 novel/game_guide/technical_doc/note/log/meme/illustration/photo/screenshot/code_artifact/data/other"},
	"llm-tone":          {Desc: "基调（图像=色调氛围；文本=行文风格）", Bench: "模型自由短语，eq 精确/in 候选；覆盖率取决于 LLM 描述普及度"},
	"llm-characters":    {Desc: "出现的人物/角色名（上限 10）", Bench: "eq/in 找'某人出场的文件'"},
	"llm-action":        {Desc: "图中/文中的主题动作一句话", Bench: "模型自由句，检索前先看目录当前取值"},
	"llm-style":         {Desc: "艺术风格或文体", Bench: "模型自由短语"},
	"llm-summary":       {Desc: "一句话总结（≤100 字）", Bench: "模型自由句"},
}

// Bench 取单字段基准；ok=false = 未收录（sp-* 自由区 / 未知新字段——
// 目录的 kind/values/min-max 是它们的发现面）。
func Bench(field string) (FieldBench, bool) {
	b, ok := registry[field]
	return b, ok
}

// BenchLen 收录条数（测试与自省用）。
func BenchLen() int { return len(registry) }

// All 全表只读快照（测试与自省用；调用方不得改写返回 map）。
func All() map[string]FieldBench {
	out := make(map[string]FieldBench, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}
