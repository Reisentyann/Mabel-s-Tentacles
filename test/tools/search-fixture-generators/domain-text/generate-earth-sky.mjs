import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const dir = path.dirname(fileURLToPath(import.meta.url));
const spec = JSON.parse(fs.readFileSync(path.join(dir, 'spec.json'), 'utf8'));
const root = path.resolve(dir, '../../../fixtures/检索字段/领域文本');
const template = JSON.parse(fs.readFileSync(path.join(root, 'economics/manifest.json'), 'utf8'));
const ids = ['astronomy', 'geography', 'geology', 'meteorology'];

function marker(id, name) {
  return `SEARCH_DOMAIN_${id.toUpperCase()}_${name.toUpperCase()}_2026`;
}

function special(domain) {
  const kind = domain.special;
  if (kind === 'shebang') return [
    `#!/usr/bin/env ${domain.id}-notes`, marker(domain.id, 'special'),
    `${domain.terms[0].toUpperCase()} ${domain.name.toUpperCase()} NOTE: The ${domain.terms[0]} register lists the ${domain.topics[0]} review.`,
    `${domain.terms[1].toUpperCase()} ${domain.name.toUpperCase()} NOTE: The ${domain.terms[1]} file records the ${domain.topics[1]} source.`,
    `${domain.terms[2].toUpperCase()} ${domain.name.toUpperCase()} NOTE: The ${domain.terms[2]} inspection marks the ${domain.topics[2]} observation.`,
    `${domain.terms[3].toUpperCase()} ${domain.name.toUpperCase()} NOTE: The ${domain.terms[3]} ledger separates four ${domain.name} records.`,
    `${domain.name.toUpperCase()} REVIEW NOTE: Each ${domain.terms[0]} term points to a dated source.`,
    `RETRIEVAL NOTE: The ${domain.terms[1]} and ${domain.terms[2]} records share one case identifier.`, '',
  ].join('\n');
  if (kind === 'crlf') return [
    `# ${marker(domain.id, 'special')} ${domain.name} notes.`, '', '## Retrieval path', '',
    `- ${domain.terms[0]} ${domain.topics[0]}记录已登记。`, `- ${domain.terms[1]} ${domain.topics[1]}记录已归档。`, `- ${domain.terms[2]} ${domain.topics[2]}字段已标注。`, '',
    '> “本记录用于验证 CRLF 行尾与结构字段。”', '', '| component | note |', '| --- | --- |', `| ${domain.terms[3]} | ${domain.topics[3]}边界案例 |`, '',
  ].join('\r\n');
  if (kind === 'glossary') return [
    `# ${marker(domain.id, 'special')} ${domain.name}术语表。`, '',
    ...domain.topics.map((topic, index) => `- ${domain.terms[index]} ${topic}：“目标应当可观察。”`),
    `- ${domain.terms[0]} 跨主题整合：“证据应当可追踪。”`, `- ${domain.terms[1]} 记录复核：“记录应当保真。”`,
    `- ${domain.terms[2]} 修订说明：“误差应当可解释。”`, `- ${domain.terms[3]} 资料归档：“档案应当可复核。”`, '',
    '| term | note |', '| --- | --- |', `| ${domain.terms[0]} | 术语与${domain.topics[0]}记录 |`, '',
  ].join('\n');
  const lines = [`# ${marker(domain.id, 'special')} ${domain.name} index.`, ''];
  for (let index = 1; index <= 31; index++) lines.push(`## Section ${String(index).padStart(2, '0')} ${domain.terms[(index - 1) % 4]}`, `https://${domain.id}-${String(index).padStart(2, '0')}.example.org`, `\`\`\`lang${String(index).padStart(2, '0')}`, '\`\`\`', '');
  lines.push(`| ${Array.from({ length: 31 }, (_, index) => `cell${String(index + 1).padStart(2, '0')}`).join(' | ')} |`);
  lines.push(`| ${Array.from({ length: 31 }, () => '---').join(' | ')} |`);
  lines.push(`| ${Array.from({ length: 31 }, (_, index) => `v${String(index + 1).padStart(2, '0')}`).join(' | ')} |`);
  return lines.join('\n') + '\n';
}

for (const domain of spec.domains.filter((item) => ids.includes(item.id))) {
  const target = path.join(root, domain.id);
  fs.mkdirSync(target, { recursive: true });
  const files = [];
  for (const entry of template.files) {
    let name = entry.path;
    let text = fs.readFileSync(path.join(root, 'economics', entry.path), 'utf8');
    const mark = entry.path === '08-long-index.md' ? marker(domain.id, 'special') : entry.marker.replaceAll('ECONOMICS', domain.id.toUpperCase());
    if (entry.path === '08-long-index.md') {
      name = spec.archetypes['08-special.txt'].variants[domain.special].filename;
      text = special(domain);
    } else {
      text = text.replaceAll('ECONOMICS', domain.id.toUpperCase()).replaceAll('economics', domain.id).replaceAll('ECON-', domain.id.slice(0, 4).toUpperCase() + '-')
        .replaceAll('inflation', domain.terms[0]).replaceAll('fiscal', domain.terms[1]).replaceAll('market', domain.terms[2]).replaceAll('exchange', domain.terms[3])
        .replaceAll('通货膨胀与财政检索报告', `${domain.topics[0]}与${domain.topics[1]}检索报告`)
        .replaceAll('Inflation review.', `${domain.name} observation review.`).replaceAll('消费者价格观察', domain.topics[0])
        .replaceAll('财政收支表', domain.topics[1]).replaceAll('指数成份', domain.topics[2]).replaceAll('汇率变化', domain.topics[3])
        .replaceAll('不构成投资建议', '不构成专业结论').replaceAll('四项经济主题', `四项${domain.name}主题`)
        .replaceAll('经济指标序列', `${domain.name}指标序列`).replaceAll('经济检索完成', `${domain.name}检索完成`).replaceAll('经济检索样本', `${domain.name}检索样本`)
        .replaceAll('物价观察档案', `${domain.topics[0]}档案`).replaceAll('财政收支材料', `${domain.topics[1]}材料`)
        .replaceAll('指数成份记录', `${domain.topics[2]}记录`).replaceAll('汇率变化台账', `${domain.topics[3]}台账`)
        .replaceAll('财政收支记录', `${domain.topics[1]}记录`).replaceAll('经济 boundary', `${domain.name} boundary`)
        .replaceAll('物价观察台账', `${domain.topics[0]}台账`).replaceAll('市场指数与汇率变化记录', `${domain.topics[2]}与${domain.topics[3]}记录`);
    }
    if (entry.path === '09-pseudo-table.md') text = text.slice(0, -1);
    const data = Buffer.from(text, 'utf8');
    fs.writeFileSync(path.join(target, name), data);
    const expectations = entry.path === '08-long-index.md' && domain.special === 'long-index'
      ? { ...entry.expect, 'cod-text-table-rows': 3 }
      : entry.expect;
    files.push({ ...entry, path: name, bytes: data.length, sha256: crypto.createHash('sha256').update(data).digest('hex'), marker: mark, expect: expectations });
  }
  for (const filename of fs.readdirSync(target)) {
    if (filename.startsWith('08-') && !files.some((entry) => entry.path === filename)) fs.unlinkSync(path.join(target, filename));
  }
  fs.writeFileSync(path.join(target, 'manifest.json'), `${JSON.stringify({ schema_version: 1, domain_id: domain.id, domain_name: domain.name, path_base: '.', generated_by: 'subagent:domain-batch-earth-sky-v2', files }, null, 2)}\n`);
  console.log(`${domain.id}: files=${files.length} bytes=${files.reduce((sum, item) => sum + item.bytes, 0)}`);
}
