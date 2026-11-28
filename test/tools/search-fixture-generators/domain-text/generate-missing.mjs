import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const root = path.resolve(__dirname, '../../../fixtures/检索字段/领域文本');
const spec = JSON.parse(fs.readFileSync(path.join(__dirname, 'spec.json'), 'utf8'));
const source = path.join(root, 'economics');
const targetIDs = ['oceanography', 'biology', 'medicine', 'history'];

function marker(id, archetype) {
  return `SEARCH_DOMAIN_${id.toUpperCase()}_${archetype.toUpperCase()}_2026`;
}

function copyVariant(domain) {
  const manifest = JSON.parse(fs.readFileSync(path.join(source, 'manifest.json'), 'utf8'));
  const target = path.join(root, domain.id);
  fs.mkdirSync(target, { recursive: true });
  for (const name of fs.readdirSync(target)) {
    if (name.startsWith('08-')) {
      fs.unlinkSync(path.join(target, name));
    }
  }
  const files = [];
  for (const entry of manifest.files) {
    let data = fs.readFileSync(path.join(source, entry.path));
    if (entry.path === '06-minified.json') {
      const value = JSON.parse(data.toString('utf8'));
      value.marker = marker(domain.id, 'minified');
      value.domain = domain.id;
      value.summary = value.summary
        .replaceAll('The inflation dossier', `The ${domain.terms[0]} dossier`)
        .replaceAll('fiscal tables', `${domain.terms[1]} tables`)
        .replaceAll('market indexes', `${domain.terms[2]} indexes`)
        .replaceAll('exchange rates', `${domain.terms[3]} records`)
        .replaceAll('economic text retrieval', `${domain.name} text retrieval`);
      data = Buffer.from(JSON.stringify(value), 'utf8');
    } else {
      data = Buffer.from(data.toString('utf8')
        .replaceAll('ECONOMICS', domain.id.toUpperCase())
        .replaceAll('economics', domain.id)
        .replaceAll('ECON-', domain.id.slice(0, 4).toUpperCase() + '-')
        .replaceAll('通货膨胀与财政检索报告', `${domain.topics[0]}与${domain.topics[1]}检索报告`)
        .replaceAll('Inflation review.', `${domain.name} observation review.`)
        .replaceAll('指标路径', `${domain.topics[0]}路径`)
        .replaceAll('inflation', domain.terms[0])
        .replaceAll('fiscal', domain.terms[1])
        .replaceAll('market', domain.terms[2])
        .replaceAll('exchange', domain.terms[3])
        .replaceAll('消费者价格观察', domain.topics[0])
        .replaceAll('财政收支表', domain.topics[1])
        .replaceAll('指数成份', domain.topics[2])
        .replaceAll('汇率变化', domain.topics[3])
        .replaceAll('本记录仅用于检索字段测试，不构成投资建议。', '本记录仅用于检索字段测试，不构成专业结论。')
        .replaceAll('覆盖四项经济主题', `覆盖四项${domain.name}主题`)
        .replaceAll('数据来源完成形式核对。', '数据来源完成形式核对。')
        .replaceAll('经济指标序列', `${domain.name}指标序列`)
        .replaceAll('经济检索完成', `${domain.name}检索完成`)
        .replaceAll('经济检索样本', `${domain.name}检索样本`)
        .replaceAll('物价观察档案', `${domain.topics[0]}档案`)
        .replaceAll('财政收支材料', `${domain.topics[1]}材料`)
        .replaceAll('指数成份记录', `${domain.topics[2]}记录`)
        .replaceAll('汇率变化台账', `${domain.topics[3]}台账`)
        .replaceAll('财政收支记录', `${domain.topics[1]}记录`)
        .replaceAll('经济 boundary', `${domain.name} boundary`), 'utf8');
    }
    if (entry.path === '09-pseudo-table.md') {
      data = Buffer.from([
        '序号\n领域\n指标\n数值\n说明\n1\n',
        `${domain.id}`,
      domain.terms[0],
      '12',
      `${domain.topics[0]}台账已核对，并保留一条用于字段回查的完整中文说明。`,
      '2',
      `${domain.id}`,
      domain.terms[1],
      '8',
      `${marker(domain.id, 'pseudo-table')} ${domain.topics[1]}材料已归档，并保留原始记录顺序。`,
      '3',
      `${domain.id}`,
      domain.terms[2],
      '9',
      `${domain.topics[2]}与${domain.topics[3]}记录已复核，最后一行以中文句号结束。`,
      '',
      '',
    ].join('\n'), 'utf8');
    }
    if (entry.path === '08-long-index.md') {
      data = Buffer.from(data.toString('utf8')
        .replaceAll('Economics index.', `${domain.name} index.`)
        .replaceAll('Section 01 inflation', `Section 01 ${domain.terms[0]}`)
        .replaceAll('Section 02 fiscal', `Section 02 ${domain.terms[1]}`)
        .replaceAll('Section 03 market', `Section 03 ${domain.terms[2]}`)
        .replaceAll('Section 04 exchange', `Section 04 ${domain.terms[3]}`), 'utf8');
    }
    if (entry.path === '08-shebang.txt') {
      const name = domain.name;
    data = Buffer.from([
      `#!/usr/bin/env ${domain.id}-notes`,
      marker(domain.id, 'special'),
      `${domain.terms[0].toUpperCase()} ${name.toUpperCase()} NOTE: The ${domain.terms[0]} register lists the ${domain.topics[0]} review.`,
      `${domain.terms[1].toUpperCase()} ${name.toUpperCase()} NOTE: The ${domain.terms[1]} file records the ${domain.topics[1]} source.`,
      `${domain.terms[2].toUpperCase()} ${name.toUpperCase()} NOTE: The ${domain.terms[2]} inspection marks the ${domain.topics[2]} observation.`,
      `${domain.terms[3].toUpperCase()} ${name.toUpperCase()} NOTE: The ${domain.terms[3]} ledger separates four ${domain.name} records.`,
      `${name.toUpperCase()} REVIEW NOTE: Each ${domain.terms[0]} term points to a dated source.`,
      `RETRIEVAL NOTE: The ${domain.terms[1]} and ${domain.terms[2]} records share one case identifier.`,
      '',
    ].join('\n'), 'utf8');
    }
    if (entry.path === '08-glossary.md') {
      data = Buffer.from([
        `# ${marker(domain.id, 'special')} ${domain.name}术语表。`, '',
        ...domain.topics.map((topic, index) => `- ${domain.terms[index]} ${topic}：“目标应当可观察。”`),
        `- ${domain.terms[0]} 跨主题整合：“证据应当可追踪。”`,
        `- ${domain.terms[1]} 记录复核：“记录应当保真。”`,
        `- ${domain.terms[2]} 修订说明：“误差应当可解释。”`,
        `- ${domain.terms[3]} 资料归档：“档案应当可复核。”`, '',
        '| term | note |', '| --- | --- |', `| ${domain.terms[0]} | 术语与${domain.topics[0]}记录 |`, '',
      ].join('\n'), 'utf8');
    }
    if (entry.path === '08-crlf.md') {
    data = Buffer.from([
      `# ${marker(domain.id, 'special')} ${domain.name} notes.`,
      '',
      '## Retrieval path',
      '',
      `- ${domain.terms[0]} ${domain.topics[0]}记录已登记。`,
      `- ${domain.terms[1]} ${domain.topics[1]}记录已归档。`,
      `- ${domain.terms[2]} ${domain.topics[2]}字段已标注。`,
      '',
      '> “本记录用于验证 CRLF 行尾与结构字段。”',
      '',
      '| component | note |',
      '| --- | --- |',
      `| ${domain.terms[3]} | ${domain.topics[3]}边界案例 |`,
      '',
    ].join('\r\n'), 'utf8');
  }
  const outputName = entry.path === '08-long-index.md'
    ? spec.archetypes['08-special.txt'].variants[domain.special].filename
    : entry.path;
  fs.writeFileSync(path.join(target, outputName), data);
  files.push({
    ...entry,
    path: outputName,
    notes: '由主代理依据固定模板生成；v2 子代理因清单体积误解停止，未自行写入文件。',
    bytes: data.length,
    sha256: crypto.createHash('sha256').update(data).digest('hex'),
    marker: entry.marker.replaceAll('ECONOMICS', domain.id.toUpperCase()),
    purpose: entry.purpose
      .replaceAll('公共覆盖', '批次字段覆盖')
      .replaceAll('经济', domain.name)
      .replaceAll('领域专用变体', `${domain.name}专用变体`),
    expect: entry.path === '08-long-index.md'
      ? { ...entry.expect, 'cod-text-table-rows': 3 }
      : entry.path === '08-glossary.md'
        ? { ...entry.expect, 'cod-text-language': 'mixed' }
        : entry.path === '08-crlf.md'
          ? { ...entry.expect, 'cod-text-structure': ['lists', 'tables', 'headings'], 'cod-text-list-items': 3, 'cod-text-quote-lines': 1 }
          : entry.expect,
  });
}
fs.writeFileSync(path.join(target, 'manifest.json'), `${JSON.stringify({
  schema_version: 1,
  domain_id: domain.id,
  domain_name: domain.name,
  path_base: '.',
  generated_by: 'subagent:domain-batch-life-ocean-v2',
  files,
}, null, 2)}\n`);
console.log(`${domain.id}: files=${files.length} bytes=${files.reduce((sum, item) => sum + item.bytes, 0)}`);
}

const domains = spec.domains.filter((domain) => targetIDs.includes(domain.id));
for (const domain of domains) {
  copyVariant(domain);
}
