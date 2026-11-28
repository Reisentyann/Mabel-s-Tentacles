import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const dir = path.dirname(fileURLToPath(import.meta.url));
const spec = JSON.parse(fs.readFileSync(path.join(dir, 'spec.json'), 'utf8'));
const root = path.resolve(dir, '../../../fixtures/检索字段/领域文本');

function pseudo(domain) {
  return [
    '序号', '领域', '指标', '数值', '说明',
    '1', domain.id, domain.terms[0], '2', `${domain.topics[0]}台账已核对，并保留一条用于字段回查的完整中文说明。`,
    '2', domain.id, domain.terms[1], '5', `SEARCH_DOMAIN_${domain.id.toUpperCase()}_PSEUDO-TABLE_2026 ${domain.topics[1]}材料已归档，并保留原始记录顺序。`,
    '3', domain.id, domain.terms[2], '8', `${domain.topics[2]}与${domain.topics[3]}记录已复核，最后一行以中文句号结束。`, '',
  ].join('\n');
}

for (const domain of spec.domains) {
  const domainPath = path.join(root, domain.id);
  const manifestPath = path.join(domainPath, 'manifest.json');
  const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
  const variant = spec.archetypes['08-special.txt'].variants[domain.special];
  for (const entry of manifest.files) {
    if (entry.path === '09-pseudo-table.md') {
      const data = Buffer.from(pseudo(domain), 'utf8');
      fs.writeFileSync(path.join(domainPath, entry.path), data);
      entry.bytes = data.length;
      entry.sha256 = crypto.createHash('sha256').update(data).digest('hex');
    }
    if (entry.path === variant.filename) {
      entry.expected_coverage = [...variant.extra_coverage];
      entry.expect = { ...variant.expect };
      if (domain.special === 'long-index') entry.expect['cod-text-table-rows'] = 3;
      entry.expected_absent_fields = [...(variant.expected_absent_fields || [])];
    }
  }
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  console.log(`${domain.id}: manifest normalized`);
}
