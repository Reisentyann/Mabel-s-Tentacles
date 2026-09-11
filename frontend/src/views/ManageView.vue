<template>
  <div class="page-shell">
    <!-- 统计条 -->
    <div class="stat-bar">
      <div class="stat">
        <span class="stat-num">{{ stats.count }}</span>
        <span class="stat-label">文件</span>
      </div>
      <div class="stat">
        <span class="stat-num">{{ formatBytes(stats.totalBytes) }}</span>
        <span class="stat-label">总量</span>
      </div>
      <div class="stat">
        <span class="stat-num">{{ stats.dirs }}</span>
        <span class="stat-label">目录</span>
      </div>
      <div class="stat-tags">
        <el-tag
          v-for="(n, t) in stats.types"
          :key="t"
          size="small"
          effect="plain"
          class="type-tag"
        >{{ t }} · {{ n }}</el-tag>
      </div>
      <div class="flex-fill"></div>
      <el-input
        v-model="query"
        placeholder="搜索：路径 / 标签 / 描述"
        clearable
        class="search-box"
        @keyup.enter="doSearch"
        @clear="clearSearch"
      >
        <template #append>
          <el-button :icon="SearchIcon" @click="doSearch" />
        </template>
      </el-input>
      <el-button :icon="FilterIcon" @click="openCond">条件</el-button>
    </div>

    <!-- 主区：树 + 表 -->
    <div class="pane-row">
      <div class="pane pane-tree">
        <div class="pane-head">逻辑树（owner 键空间）</div>
        <div class="pane-body">
          <el-tree
            :data="tree"
            :props="{ label: 'name', children: 'children' }"
            node-key="path"
            highlight-current
            default-expand-all
            @node-click="onNodeClick"
          >
            <template #default="{ data }">
              <span class="tree-node">
                <span>{{ data.type === 'dir' ? '📁' : '📄' }}</span>
                <span class="tree-name">{{ data.name }}</span>
                <span v-if="data.type !== 'dir'" class="tree-size">{{
                  formatBytes(data.size_bytes)
                }}</span>
              </span>
            </template>
          </el-tree>
          <el-empty v-if="!tree.length" description="空书房（agent 还没写文件）" :image-size="60" />
        </div>
      </div>

      <div class="pane pane-table">
        <div class="pane-head">
          <span>{{ headTitle }}</span>
          <el-button
            v-if="mode === 'search' || mode === 'cond'"
            size="small"
            text
            @click="clearSearch"
          >
            返回全部
          </el-button>
          <div class="flex-fill"></div>
          <el-button size="small" text @click="refresh">刷新</el-button>
        </div>
        <div class="pane-body table-body">
          <el-table
            :data="rows"
            height="100%"
            size="small"
            highlight-current-row
            @row-click="(row) => openDetail(row.path)"
          >
            <el-table-column prop="path" label="路径" min-width="260" show-overflow-tooltip />
            <el-table-column label="类型" width="90">
              <template #default="{ row }">
                <el-tag size="small" effect="plain">{{ row.file_type || '-' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="大小" width="90">
              <template #default="{ row }">{{ formatBytes(row.size_bytes) }}</template>
            </el-table-column>
            <el-table-column label="更新" width="160">
              <template #default="{ row }">{{ formatDate(row.updated_at) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="170" fixed="right">
              <template #default="{ row }">
                <el-button size="small" text type="primary" @click.stop="openDetail(row.path)">
                  详情
                </el-button>
                <el-button size="small" text @click.stop="download(row.path)">下载</el-button>
                <el-button size="small" text @click.stop="openMove(row.path)">移动</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </div>
    </div>

    <!-- 详情抽屉 -->
    <el-drawer v-model="detailVisible" size="420px" :title="detail?.file_path || '详情'">
      <div v-if="detail" class="detail-body">
        <div class="detail-actions">
          <el-button size="small" type="primary" plain @click="download(detail.file_path)">
            下载
          </el-button>
          <el-button size="small" plain @click="openMove(detail.file_path)">移动</el-button>
          <el-button size="small" plain @click="openCopy(detail.file_path)">复制</el-button>
          <el-button
            size="small"
            plain
            :loading="analyzing"
            @click="reanalyze(detail.file_path)"
          >
            重分析
          </el-button>
        </div>

        <h4>基本信息</h4>
        <div class="meta-grid">
          <span class="k">uuid</span><span class="v mono">{{ detail.uuid }}</span>
          <span class="k">类型</span><span class="v">{{ detail.file_type || '-' }}</span>
          <span class="k">大小</span><span class="v">{{ formatBytes(detail.size_bytes) }}</span>
          <span class="k">归属</span><span class="v">{{ detail.owner_id || '无主（管家）' }}</span>
          <span class="k">可见性</span><span class="v">{{ detail.visibility || '-' }}</span>
          <span class="k">下载次数</span><span class="v">{{ detail.download_count ?? 0 }}</span>
          <span class="k">更新</span><span class="v">{{ formatDate(detail.updated_at) }}</span>
        </div>

        <h4>描述</h4>
        <p class="desc">{{ detail.description || '（无描述——agent 未提交）' }}</p>
        <div v-if="detail.tags?.length" class="attr-tag-list">
          <el-tag v-for="t in detail.tags" :key="t" size="small" effect="plain">{{ t }}</el-tag>
        </div>

        <template v-if="lineage.length">
          <h4>谱系</h4>
          <div class="lineage">
            <div v-for="(l, i) in lineage" :key="i" class="lineage-item">
              <span class="k">{{ l.k }}</span>
              <span class="v mono">{{ l.v }}</span>
            </div>
          </div>
        </template>

        <h4>事实字段（cod / llm）</h4>
        <div v-for="g in attrGroups" :key="g.name" class="attr-group">
          <div class="attr-group-name">{{ g.name }} · {{ g.items.length }}</div>
          <div class="attr-tag-list">
            <el-tooltip
              v-for="a in g.items"
              :key="a.key"
              :content="a.key"
              placement="top"
              :show-after="400"
            >
              <el-tag size="small" :type="g.tagType" effect="plain" class="attr-tag">
                {{ a.short }}: {{ a.value }}
              </el-tag>
            </el-tooltip>
          </div>
        </div>
        <el-empty
          v-if="!attrGroups.length"
          description="尚无事实字段（等编排机分析落库）"
          :image-size="50"
        />
      </div>
    </el-drawer>

    <!-- 移动对话框 -->
    <el-dialog v-model="moveVisible" title="移动（逻辑键改）" width="480px">
      <el-form label-width="70px">
        <el-form-item label="源">
          <el-input :model-value="moveFrom" disabled />
        </el-form-item>
        <el-form-item label="目标">
          <el-input v-model="moveTo" placeholder="例：~agent/书房档案/新名.txt" />
        </el-form-item>
      </el-form>
      <p class="hint">ext 不变 = 纯数据库键改；ext 变化会同步搬移存储位。uuid 与描述不变。</p>
      <template #footer>
        <el-button @click="moveVisible = false">取消</el-button>
        <el-button type="primary" :loading="moving" @click="doMove">移动</el-button>
      </template>
    </el-dialog>

    <!-- 复制对话框 -->
    <el-dialog v-model="copyVisible" title="复制（副本归我）" width="480px">
      <el-form label-width="70px">
        <el-form-item label="源">
          <el-input :model-value="copyFrom" disabled />
        </el-form-item>
        <el-form-item label="目标">
          <el-input v-model="copyTo" placeholder="目标逻辑键" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="copyVisible = false">取消</el-button>
        <el-button type="primary" :loading="copying" @click="doCopy">复制</el-button>
      </template>
    </el-dialog>

    <!-- 条件检索对话框（索引机直查，2026-09-10）：字段目录下拉 + 按桶型适配的
         op/值编辑器；字段带 desc/bench（契约机器面）；多行 = And 交集 -->
    <el-dialog v-model="condVisible" title="条件检索（索引机直查）" width="760px">
      <div v-if="catalogError" class="hint">{{ catalogError }}</div>
      <template v-else>
        <div v-for="(c, i) in condRows" :key="i" class="cond-item">
          <div class="cond-row">
            <el-select
              v-model="c.field"
              filterable
              placeholder="字段（来自目录）"
              class="cond-field"
              @change="onFieldChange(c)"
            >
              <el-option
                v-for="f in catalog"
                :key="f.field"
                :value="f.field"
                :label="`${f.field} · ${f.kind}`"
              />
            </el-select>
            <el-select v-model="c.op" class="cond-op" @change="onOpChange(c)">
              <el-option v-for="o in opsFor(c)" :key="o" :value="o" :label="o" />
            </el-select>

            <!-- 值编辑器：按（桶型, op）适配 -->
            <el-select
              v-if="kindOf(c) !== 'num' && c.op === 'eq'"
              v-model="c.value"
              filterable
              allow-create
              placeholder="取值"
              class="cond-value"
            >
              <el-option v-for="v in valuesOf(c)" :key="String(v)" :value="v" :label="String(v)" />
            </el-select>
            <el-select
              v-if="kindOf(c) !== 'num' && c.op === 'in'"
              v-model="c.inValues"
              multiple
              filterable
              allow-create
              placeholder="任一命中（可多选）"
              class="cond-value"
            >
              <el-option v-for="v in valuesOf(c)" :key="String(v)" :value="v" :label="String(v)" />
            </el-select>
            <el-input-number
              v-if="kindOf(c) === 'num' && ['eq', 'gt', 'lt'].includes(c.op)"
              v-model="c.value"
              :controls="false"
              placeholder="数值"
              class="cond-num"
            />
            <el-select
              v-if="kindOf(c) === 'num' && c.op === 'in'"
              v-model="c.inValues"
              multiple
              filterable
              allow-create
              placeholder="数值集合（回车添加）"
              class="cond-value"
            >
              <el-option v-for="v in valuesOf(c)" :key="String(v)" :value="v" :label="String(v)" />
            </el-select>
            <div v-if="kindOf(c) === 'num' && c.op === 'range'" class="cond-range">
              <el-input-number v-model="c.lo" :controls="false" placeholder="下界" class="cond-num" />
              <span class="cond-tilde">~</span>
              <el-input-number v-model="c.hi" :controls="false" placeholder="上界" class="cond-num" />
            </div>

            <el-button
              text
              type="danger"
              :icon="DeleteIcon"
              :disabled="condRows.length <= 1"
              @click="removeRow(i)"
            />
          </div>
          <div v-if="fieldInfo(c)" class="cond-bench">
            {{ fieldInfo(c).desc }}<template v-if="fieldInfo(c).bench"> —— {{ fieldInfo(c).bench }}</template>
          </div>
        </div>

        <el-button size="small" text type="primary" @click="addRow">+ 添加条件（多条件 = And 交集）</el-button>
      </template>
      <template #footer>
        <div class="cond-footer">
          <el-popover v-if="guideRules.length" placement="top-start" :width="460" trigger="hover">
            <template #reference>
              <el-link type="primary" :underline="false">口径规则与速查（{{ guideRules.length }} + {{ guideQuick.length }}）</el-link>
            </template>
            <div class="guide-pop">
              <p v-for="(r, i) in guideRules" :key="'r' + i" class="guide-line">{{ i + 1 }}. {{ r }}</p>
              <div v-for="(q, i) in guideQuick" :key="'q' + i" class="guide-q">
                <b>{{ q.want }}</b> → {{ q.cond }}
              </div>
            </div>
          </el-popover>
          <div class="flex-fill"></div>
          <el-button @click="condVisible = false">取消</el-button>
          <el-button
            type="primary"
            :loading="condSearching"
            :disabled="!condRows.some(rowReady)"
            @click="doCondSearch"
          >
            检索
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import { Search as SearchIcon, Filter as FilterIcon, Delete as DeleteIcon } from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';
import {
  getFiles,
  searchFiles,
  getFileMetadata,
  getIndexFields,
  moveFile,
  copyFile,
  analyzeFile,
  downloadFileBlob,
} from '../api/files';
import { formatBytes, formatDate } from '../utils/format';
import { downloadBlob as saveBlob } from '../utils/download';

// ---- 树与表 ----
const tree = ref([]);
const flat = ref([]); // 全量平铺（表格底料）
const rows = ref([]);
const currentDir = ref('');
const mode = ref('all'); // all | dir | search | cond
const query = ref('');
const condTotal = ref(0);

// normRow HTTP 检索行（repo.FileMetadata，file_path 键）归一到表格/详情的
// path 口径——q 搜索与 cond 搜索同用（2026-09-10 顺带修 q 搜索行键错位）
const normRow = (it) => ({ ...it, path: it.path || it.file_path });

const headTitle = computed(() => {
  if (mode.value === 'search') return `搜索结果 · ${query.value}`;
  if (mode.value === 'cond') return `条件检索 · ${condTotal.value} 命中`;
  return currentDir.value || '全部文件';
});

const walk = (nodes, out) => {
  for (const n of nodes || []) {
    if (n.type === 'dir') walk(n.children, out);
    else out.push(n);
  }
};

const refresh = async () => {
  const { data } = await getFiles();
  tree.value = data.tree || [];
  const out = [];
  walk(tree.value, out);
  flat.value = out;
  applyMode();
};

const applyMode = () => {
  if (mode.value === 'search') return; // 搜索结果由 doSearch 维护
  if (mode.value === 'dir' && currentDir.value) {
    const prefix = currentDir.value + '/';
    rows.value = flat.value.filter((f) => f.path.startsWith(prefix));
  } else {
    rows.value = flat.value;
  }
};

const onNodeClick = (data) => {
  if (data.type === 'dir') {
    mode.value = 'dir';
    currentDir.value = data.path;
    applyMode();
  } else {
    openDetail(data.path);
  }
};

const doSearch = async () => {
  if (!query.value.trim()) return clearSearch();
  const { data } = await searchFiles({ q: query.value.trim(), size: 100 });
  rows.value = (data.items || []).map(normRow);
  mode.value = 'search';
};

const clearSearch = () => {
  query.value = '';
  mode.value = 'all';
  currentDir.value = '';
  condTotal.value = 0;
  applyMode();
};

// ---- 条件检索（索引机直查，2026-09-10）----
const condVisible = ref(false);
const condSearching = ref(false);
const catalog = ref([]); // 字段目录（含 kind/values/min-max/desc/bench）
const catalogLoaded = ref(false);
const catalogError = ref('');
const guideRules = ref([]);
const guideQuick = ref([]);
const condRows = ref([]); // [{field, op, value, inValues, lo, hi}]

const openCond = async () => {
  condVisible.value = true;
  if (!catalogLoaded.value && !catalogError.value) await loadCatalog();
  if (!condRows.value.length) addRow();
};

const loadCatalog = async () => {
  try {
    const { data } = await getIndexFields();
    catalog.value = data.fields || [];
    guideRules.value = data.guide?.rules || [];
    guideQuick.value = data.guide?.quick_ref || [];
    catalogLoaded.value = true;
  } catch (e) {
    catalogError.value =
      '字段目录不可用：' + (e?.response?.data?.error || e.message) +
      '（索引机未装配时目录端点 503——检索仍可用 q 关键词走 SQL）';
  }
};

const fieldInfo = (c) => catalog.value.find((f) => f.field === c.field);
const kindOf = (c) => fieldInfo(c)?.kind || '';
const valuesOf = (c) => fieldInfo(c)?.values || [];

const opsFor = (c) => {
  if (kindOf(c) === 'num') return ['eq', 'in', 'gt', 'lt', 'range'];
  return ['eq', 'in'];
};

const addRow = () => {
  condRows.value.push({ field: '', op: 'eq', value: '', inValues: [], lo: null, hi: null });
};

const removeRow = (i) => {
  if (condRows.value.length > 1) condRows.value.splice(i, 1);
};

const onFieldChange = (c) => {
  c.op = 'eq';
  onOpChange(c);
};

const onOpChange = (c) => {
  c.value = c.op === 'eq' && kindOf(c) === 'num' ? null : '';
  c.inValues = [];
  c.lo = null;
  c.hi = null;
};

const rowReady = (c) => {
  if (!c.field || !c.op) return false;
  if (c.op === 'in') return (c.inValues || []).length > 0;
  if (c.op === 'range') return c.lo != null && c.hi != null && c.lo <= c.hi;
  return c.value !== '' && c.value != null;
};

const buildCond = (c) => {
  if (c.op === 'in') {
    const vals = kindOf(c) === 'num' ? c.inValues.map(Number) : c.inValues;
    return { field: c.field, op: 'in', value: vals };
  }
  if (c.op === 'range') return { field: c.field, op: 'range', value: [c.lo, c.hi] };
  return { field: c.field, op: c.op, value: c.value };
};

const doCondSearch = async () => {
  const conds = condRows.value.filter(rowReady).map(buildCond);
  if (!conds.length) return;
  condSearching.value = true;
  try {
    const { data } = await searchFiles({ cond: JSON.stringify(conds), size: 100 });
    rows.value = (data.items || []).map(normRow);
    condTotal.value = data.total ?? rows.value.length;
    mode.value = 'cond';
    condVisible.value = false;
    if (!rows.value.length) {
      ElMessage.info('0 命中——空集是合法答案，试试放宽分档（目录的 min/max 是本库实况）');
    }
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '条件检索失败');
  } finally {
    condSearching.value = false;
  }
};

// ---- 统计 ----
const stats = computed(() => {
  let totalBytes = 0;
  const types = {};
  const dirSet = new Set();
  for (const f of flat.value) {
    totalBytes += f.size_bytes || 0;
    const t = f.path.endsWith('/') ? undefined : (f.file_type || 'other');
    if (t) types[t] = (types[t] || 0) + 1;
    const segs = f.path.split('/');
    for (let i = 0; i < segs.length - 1; i++) {
      dirSet.add(segs.slice(0, i + 1).join('/'));
    }
  }
  return { count: flat.value.length, totalBytes, types, dirs: dirSet.size };
});

// ---- 详情抽屉 ----
const detailVisible = ref(false);
const detail = ref(null);
const analyzing = ref(false);

const openDetail = async (path) => {
  const { data } = await getFileMetadata(path);
  detail.value = data;
  detailVisible.value = true;
};

const lineage = computed(() => {
  const d = detail.value;
  if (!d) return [];
  const out = [];
  if (d.copied_from) out.push({ k: '复制自', v: d.copied_from });
  if (d.moved_from) out.push({ k: '移动自', v: d.moved_from });
  return out;
});

const attrGroups = computed(() => {
  const attrs = detail.value?.attributes || {};
  const groups = [
    { name: 'basic', prefix: 'cod-basic-', tagType: 'info' },
    { name: 'text', prefix: 'cod-text-', tagType: 'success' },
    { name: 'code', prefix: 'cod-code-', tagType: 'warning' },
    { name: 'image', prefix: 'cod-image-', tagType: 'danger' },
    { name: 'llm 语义', prefix: 'llm-', tagType: 'primary' },
  ];
  const out = [];
  for (const g of groups) {
    const items = Object.entries(attrs)
      .filter(([k]) => k.startsWith(g.prefix))
      .map(([k, v]) => ({ key: k, short: k.slice(g.prefix.length), value: v }));
    if (items.length) out.push({ name: g.name, tagType: g.tagType, items });
  }
  return out;
});

const reanalyze = async (path) => {
  analyzing.value = true;
  try {
    await analyzeFile(path);
    ElMessage.success('重分析完成');
    await openDetail(path);
    await refresh();
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '重分析失败');
  } finally {
    analyzing.value = false;
  }
};

const download = async (path) => {
  const { data } = await downloadFileBlob(path);
  saveBlob(data, path.split('/').pop());
};

// ---- 移动 / 复制 ----
const moveVisible = ref(false);
const moveFrom = ref('');
const moveTo = ref('');
const moving = ref(false);

const openMove = (path) => {
  moveFrom.value = path;
  moveTo.value = '';
  moveVisible.value = true;
};

const doMove = async () => {
  if (!moveTo.value.trim()) return;
  moving.value = true;
  try {
    await moveFile({ from: moveFrom.value, to: moveTo.value.trim() });
    ElMessage.success('已移动（uuid 不变，键已迁移）');
    moveVisible.value = false;
    detailVisible.value = false;
    await refresh();
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '移动失败');
  } finally {
    moving.value = false;
  }
};

const copyVisible = ref(false);
const copyFrom = ref('');
const copyTo = ref('');
const copying = ref(false);

const openCopy = (path) => {
  copyFrom.value = path;
  copyTo.value = '';
  copyVisible.value = true;
};

const doCopy = async () => {
  if (!copyTo.value.trim()) return;
  copying.value = true;
  try {
    await copyFile({ source: copyFrom.value, target: copyTo.value.trim() });
    ElMessage.success('已复制');
    copyVisible.value = false;
    await refresh();
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '复制失败');
  } finally {
    copying.value = false;
  }
};

onMounted(refresh);
</script>

<style scoped>
.stat-bar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 10px 16px;
  background-color: var(--mabel-surface);
  border: 1px solid var(--mabel-border);
  border-radius: 10px;
}
.stat {
  display: flex;
  align-items: baseline;
  gap: 6px;
}
.stat-num {
  font-size: 1.05rem;
  font-weight: 700;
  color: var(--mabel-accent);
}
.stat-label {
  font-size: 0.78rem;
  color: var(--mabel-text-muted);
}
.stat-tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.search-box {
  width: 300px;
}
.pane-tree {
  width: 300px;
  flex: none;
}
.pane-table {
  flex: 1;
}
.table-body {
  padding: 0;
}
.tree-node {
  display: flex;
  align-items: center;
  gap: 6px;
  overflow: hidden;
}
.tree-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tree-size {
  margin-left: auto;
  font-size: 0.72rem;
  color: var(--mabel-text-muted);
}
.detail-body h4 {
  margin: 18px 0 8px;
  font-size: 0.85rem;
  color: var(--mabel-text-muted);
  border-bottom: 1px solid var(--mabel-border);
  padding-bottom: 4px;
}
.detail-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.desc {
  font-size: 0.88rem;
  line-height: 1.6;
  margin: 0 0 8px;
}
.mono {
  font-family: Consolas, Monaco, monospace;
  font-size: 0.8rem;
}
.lineage-item {
  display: flex;
  gap: 10px;
  font-size: 0.82rem;
  margin-bottom: 4px;
}
.attr-group {
  margin-bottom: 10px;
}
.attr-group-name {
  font-size: 0.78rem;
  color: var(--mabel-text-muted);
  margin-bottom: 4px;
}
.attr-tag {
  max-width: 100%;
}
.hint {
  font-size: 0.78rem;
  color: var(--mabel-text-muted);
  margin: 4px 0 0;
}
.flex-fill {
  flex: 1;
}

/* ---- 条件检索对话框 ---- */
.cond-item {
  margin-bottom: 10px;
}
.cond-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.cond-field {
  width: 280px;
}
.cond-op {
  width: 96px;
  flex: none;
}
.cond-value {
  flex: 1;
  min-width: 140px;
}
.cond-num {
  flex: 1;
  min-width: 100px;
}
.cond-range {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 6px;
}
.cond-tilde {
  color: var(--mabel-text-muted);
}
.cond-bench {
  margin: 2px 0 0 4px;
  font-size: 0.76rem;
  line-height: 1.5;
  color: var(--mabel-text-muted);
}
.cond-footer {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}
.guide-pop {
  max-height: 380px;
  overflow-y: auto;
  font-size: 0.78rem;
  line-height: 1.6;
}
.guide-line {
  margin: 0 0 4px;
}
.guide-q {
  margin: 4px 0;
  padding-top: 4px;
  border-top: 1px dashed var(--mabel-border, #e3e7ee);
}
</style>
