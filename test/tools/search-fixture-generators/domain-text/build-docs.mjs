import fs from 'node:fs';
import path from 'node:path';

const [root, specPath, reportPath, handoffPath] = process.argv.slice(2);
if (!root || !specPath || !reportPath) {
  console.error('usage: node build-docs.mjs <root> <spec.json> <report.json> [handoffs.json]');
  process.exit(2);
}

const spec = JSON.parse(fs.readFileSync(specPath, 'utf8'));
const report = JSON.parse(fs.readFileSync(reportPath, 'utf8'));
const handoffs = handoffPath && fs.existsSync(handoffPath)
  ? JSON.parse(fs.readFileSync(handoffPath, 'utf8'))
  : [];

const domains = [];
const files = [];
for (const domain of spec.domains) {
  const manifestPath = path.join(root, domain.id, 'manifest.json');
  const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
  const entries = manifest.files.map((entry) => ({
    ...entry,
    domain_id: domain.id,
    domain_name: domain.name,
    relative_path: `${domain.id}/${entry.path}`,
  }));
  domains.push({
    id: domain.id,
    name: domain.name,
    special: domain.special,
    topics: domain.topics,
    terms: domain.terms,
    manifest: `${domain.id}/manifest.json`,
    files: entries,
  });
  files.push(...entries);
}

const coverage = Object.entries(report.field_paths)
  .map(([field, paths]) => ({ field, count: paths.length, examples: paths.slice(0, 3) }))
  .sort((a, b) => a.field.localeCompare(b.field));

const central = {
  schema_version: 1,
  family: '领域文本',
  generated_at: '2026-11-26',
  path_base: '.',
  source_spec: path.relative(root, specPath).replaceAll('\\', '/'),
  verifier_report: path.relative(root, reportPath).replaceAll('\\', '/'),
  stats: {
    domains: domains.length,
    files: files.length,
    distinct_fields: coverage.length,
    cod_fields: coverage.filter((item) => item.field.startsWith('cod-')).length,
    fixture_bytes: report.stats.total_bytes,
    max_file_bytes: report.stats.max_file_bytes,
    manifest_checks: report.stats.manifest_checks,
    queries: report.stats.queries,
    missing_checks: report.stats.missing_checks,
    failures: report.failures.length,
  },
  domains,
  files,
  coverage,
  handoffs,
};

fs.writeFileSync(path.join(root, 'manifest.json'), `${JSON.stringify(central, null, 2)}\n`);

function cell(value) {
  return String(value ?? '').replaceAll('|', '\\|').replaceAll('\n', '<br>');
}
function jsonCell(value) {
  return cell(JSON.stringify(value));
}
function expectSummary(entry) {
  const items = Object.entries(entry.expect || {}).slice(0, 6);
  return items.map(([key, value]) => `${key}=${JSON.stringify(value)}`).join('<br>');
}

const lines = [];
lines.push('# 领域文本检索夹具');
lines.push('');
lines.push('生成日期：2026-11-26');
lines.push('');
lines.push('## 概览');
lines.push('');
lines.push(`- 领域：${domains.length} 个`);
lines.push(`- 样本：${files.length} 个，每领域 ${files.length / domains.length} 个`);
lines.push(`- 字段：${coverage.length} 个不同索引键，其中 ${central.stats.cod_fields} 个 cod-* 字段`);
lines.push(`- 大小：样本总计 ${report.stats.total_bytes} B，最大单文件 ${report.stats.max_file_bytes} B`);
lines.push(`- 验收：${report.stats.manifest_checks} 项清单检查、${report.stats.queries} 次索引回查、${report.stats.missing_checks} 次缺键检查，失败 ${report.failures.length} 项`);
lines.push('- 设计原则：领域内容只负责提供真实语境，字段断言只依赖 describer/indexer 的确定性输出。');
lines.push('');
lines.push('## 统一样本类型');
lines.push('');
lines.push('| 文件 | 类型 | 主要覆盖 |');
lines.push('| --- | --- | --- |');
const archetypeRows = [
  ['01-report.md', 'Markdown 综合报告', 'frontmatter、标题、列表、表格、围栏、引用、链接、章节路径'],
  ['02-observation-log.txt', '时间戳日志', '时间跨度、行首时间戳、重复行、URL、email'],
  ['03-collaboration-chat.txt', '协作对话', 'mention、hashtag、emoji、对话、方括号时间戳'],
  ['04-records.csv', '规整 CSV', '混合语言、标题行、括号配平'],
  ['05-records-broken.csv', '错位 CSV', ' ragged 行、括号不配平负例'],
  ['06-minified.json', '压缩 JSON', 'minified、最长行、无末尾换行'],
  ['07-boundary.txt', '边界文本', 'mixed EOL、行尾空白、连续空行、无末尾换行'],
  ['08-*', '领域特殊样本', 'shebang / 长索引截断 / 中文术语表 / CRLF'],
  ['09-pseudo-table.md', '伪表格负例', '逐单元格换行不能误报 Markdown 表格'],
];
for (const row of archetypeRows) lines.push(`| ${row.map(cell).join(' | ')} |`);
lines.push('');
lines.push('## 字段覆盖矩阵');
lines.push('');
lines.push('| 字段 | 命中文件数 | 示例文件 |');
lines.push('| --- | ---: | --- |');
for (const item of coverage) {
  lines.push(`| ${cell(item.field)} | ${item.count} | ${item.examples.map(cell).join('<br>')} |`);
}
lines.push('');
lines.push('## 逐文件用途');
lines.push('');
for (const domain of domains) {
  lines.push(`### ${domain.name}（${domain.id}）`);
  lines.push('');
  lines.push(`主题：${domain.topics.join('、')}。术语：${domain.terms.join('、')}。`);
  lines.push('');
  lines.push('| 文件 | 用途 | 关键预期 | 预期缺键 |');
  lines.push('| --- | --- | --- | --- |');
  for (const entry of domain.files) {
    lines.push(`| ${cell(entry.path)} | ${cell(entry.purpose)} | ${expectSummary(entry)} | ${cell((entry.expected_absent_fields || []).join('、'))} |`);
  }
  lines.push('');
}
lines.push('## 旧测试目录参考');
lines.push('');
lines.push('参考目录：`C:\\Users\\Reisentyan\\Downloads\\幻觉`。只复用结构模式，不复制大日志、图片、压缩包或未经核实的法律结论。');
lines.push('');
lines.push('- 调查摘要式 Markdown：H2 路径、两列表格、问题段落、引用块。');
lines.push('- 服务日志：时间戳、重复 run_id/conversation_id、多行提示词和长行。');
lines.push('- 伪表格负例：单元格逐行排列且不含管道符，不能误报为 Markdown 表格。');
lines.push('');
if (handoffs.length) {
  lines.push('## 子代理交付记录');
  lines.push('');
  lines.push('| 批次 | 代理 | 领域 | 交付结果 |');
  lines.push('| --- | --- | --- | --- |');
  for (const handoff of handoffs) {
    lines.push(`| ${cell(handoff.batch)} | ${cell(handoff.agent)} | ${cell(handoff.domains.join('、'))} | ${cell(handoff.result)} |`);
  }
  lines.push('');
}
lines.push('## 复现');
lines.push('');
lines.push('```bash');
lines.push('go run ../test/tools/search-fixture-generators/domain-text/verify.go \\');
lines.push('  ../test/fixtures/检索字段/领域文本 \\');
lines.push('  ../test/tools/search-fixture-generators/domain-text/spec.json \\');
lines.push('  ../test/.tmp-domain-verify/report.json');
lines.push('node ../test/tools/search-fixture-generators/domain-text/build-docs.mjs \\');
lines.push('  ../test/fixtures/检索字段/领域文本 \\');
lines.push('  ../test/tools/search-fixture-generators/domain-text/spec.json \\');
lines.push('  ../test/.tmp-domain-verify/report.json');
lines.push('```');
lines.push('');

fs.writeFileSync(path.join(root, 'README.md'), lines.join('\n'));
console.log(`domains=${domains.length} files=${files.length} fields=${coverage.length} README=${path.join(root, 'README.md')} manifest=${path.join(root, 'manifest.json')}`);
