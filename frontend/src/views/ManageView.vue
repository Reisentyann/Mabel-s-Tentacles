<template>
  <div class="page-shell">
    <div class="page-heading">
      <h1>文件管理</h1>
      <p>{{ stats.count }} 个文件 · {{ formatBytes(stats.totalBytes) }}</p>
    </div>
    <div class="stat-bar">
      <el-select v-model="currentDir" filterable clearable placeholder="全部目录"
        class="directory-filter" aria-label="筛选目录" @change="filterDirectory">
        <el-option v-for="dir in directories" :key="dir" :label="dir" :value="dir" />
      </el-select>
      <div class="flex-fill"></div>

      <el-input
        v-model="query"
        placeholder="搜索文件、标签或描述"
        aria-label="搜索文件"
        clearable
        class="search-box"
        @keyup.enter="doSearch"
        @clear="clearSearch"
      >
        <template #append>
          <el-button :icon="SearchIcon" aria-label="搜索" :loading="listLoading" @click="doSearch" />
        </template>
      </el-input>
      <el-button :icon="FilterIcon" @click="openCond">高级筛选</el-button>
    </div>

    <!-- 批量操作悬浮条（当有选中项时出现） -->
    <transition name="el-fade-in-linear">
      <div v-if="selectedRows.length > 0" class="batch-bar">
        <div class="batch-info">
          <span class="batch-count">已选择 {{ selectedRows.length }} 项</span>
          <span class="batch-size">（总计 {{ formatBytes(selectedTotalBytes) }}）</span>
        </div>
        <div class="batch-actions">
          <el-button size="small" type="primary" :icon="DownloadIcon" @click="batchDownload">
            打包下载 (ZIP)
          </el-button>
          <el-popconfirm
            title="确认软删除所选文件？删除后可在审计记录中追溯。"
            confirm-button-text="删除"
            cancel-button-text="取消"
            confirm-button-type="danger"
            width="260px"
            @confirm="batchDelete"
          >
            <template #reference>
              <el-button size="small" type="danger" :icon="DeleteIcon" :loading="batchDeleting">
                批量删除
              </el-button>
            </template>
          </el-popconfirm>
          <el-button size="small" text @click="clearSelection">取消选择</el-button>
        </div>
      </div>
    </transition>

    <!-- 文件列表 -->
    <div class="pane-row">
      <div class="pane pane-table">
        <div class="pane-head">
          <span class="pane-title">{{ headTitle }}</span>
          <span class="result-count">{{ rows.length }} 项</span>
          <el-button
            v-if="mode === 'search' || mode === 'cond'"
            size="small"
            text
            type="primary"
            @click="clearSearch"
          >
            返回全部
          </el-button>
          <div class="flex-fill"></div>
          <el-button size="small" text :icon="RefreshIcon" :loading="listLoading" @click="refresh">刷新</el-button>
        </div>
        <div class="pane-body table-body">
          <el-table
            ref="tableRef"
            v-loading="listLoading"
            :data="pagedRows"
            height="100%"
            empty-text="暂无文件，请调整筛选条件或稍后刷新"
            highlight-current-row
            row-key="path"
            @selection-change="handleSelectionChange"
            @sort-change="sortFiles"
            @row-click="(row) => openDetail(row.path)"
          >
            <el-table-column type="selection" width="45" align="center" />
            <el-table-column prop="path" label="文件" min-width="260" sortable="custom" show-overflow-tooltip>
              <template #default="{ row }">
                <div class="file-name">{{ row.name || row.path.split('/').pop() }}</div>
                <div class="file-path">{{ row.path }}</div>
              </template>
            </el-table-column>
            <el-table-column prop="file_type" label="类型" width="100" align="center" sortable="custom">
              <template #default="{ row }">
                <el-tag size="small" effect="plain">{{ row.file_type || '-' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="size_bytes" label="大小" width="110" align="right" sortable="custom">
              <template #default="{ row }">{{ formatBytes(row.size_bytes) }}</template>
            </el-table-column>
            <el-table-column prop="updated_at" label="更新时间" width="170" sortable="custom">
              <template #default="{ row }">{{ formatDate(row.updated_at) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="190" fixed="right">
              <template #default="{ row }">
                <el-button size="small" text @click.stop="preview(row.path)">
                  预览
                </el-button>
                <el-button size="small" text @click.stop="download(row.path)">
                  下载
                </el-button>
                <el-dropdown trigger="click" @command="(cmd) => handleRowCommand(cmd, row.path)">
                  <el-button size="small" text @click.stop>更多 ▾</el-button>
                  <template #dropdown>
                    <el-dropdown-menu>
                      <el-dropdown-item command="detail">文件详情</el-dropdown-item>
                      <el-dropdown-item command="share">分享 24h 短链</el-dropdown-item>
                      <el-dropdown-item command="move">移动 / 重命名</el-dropdown-item>
                      <el-dropdown-item command="copy">复制副本</el-dropdown-item>
                      <el-dropdown-item command="reanalyze">重新分析事实</el-dropdown-item>
                      <el-dropdown-item command="delete" divided style="color: #f56c6c;">
                        软删除
                      </el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
              </template>
            </el-table-column>
          </el-table>
        </div>
        <div class="file-pagination">
          <el-pagination v-model:current-page="page" v-model:page-size="pageSize"
            :total="rows.length" :page-sizes="[20, 50, 100]"
            layout="total, sizes, prev, pager, next" />
        </div>
      </div>
    </div>

    <!-- 详情抽屉 -->
    <el-drawer v-model="detailVisible" size="440px" :title="detail?.file_path || '文件详情'">
      <div v-if="detail" class="detail-body">
        <div class="detail-actions">
          <el-button size="small" type="primary" plain :icon="ViewIcon" @click="preview(detail.file_path)">
            在线预览
          </el-button>
          <el-button size="small" plain :icon="ShareIcon" @click="shareLink(detail.file_path)">
            分享短链
          </el-button>
          <el-button size="small" plain :icon="DownloadIcon" @click="download(detail.file_path)">
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
          <el-popconfirm
            title="确认软删除该文件？可在数据库审计中追溯。"
            confirm-button-text="删除"
            cancel-button-text="取消"
            confirm-button-type="danger"
            @confirm="doDelete(detail.file_path)"
          >
            <template #reference>
              <el-button size="small" type="danger" plain :icon="DeleteIcon">删除</el-button>
            </template>
          </el-popconfirm>
        </div>

        <h4>基本信息</h4>
        <div class="meta-grid">
          <span class="k">UUID</span><span class="v mono clickable" title="点击复制" @click="copyText(detail.uuid)">{{ detail.uuid }}</span>
          <span class="k">类型</span><span class="v">{{ detail.file_type || '-' }}</span>
          <span class="k">MIME</span><span class="v">{{ detail.mime_type || '-' }}</span>
          <span class="k">大小</span><span class="v">{{ formatBytes(detail.size_bytes) }}</span>
          <span class="k">归属</span><span class="v">{{ detail.owner_id || '无主（管家）' }}</span>
          <span class="k">可见性</span><span class="v">{{ detail.visibility || '-' }}</span>
          <span class="k">下载次数</span><span class="v">{{ detail.download_count ?? 0 }}</span>
          <span class="k">更新时间</span><span class="v">{{ formatDate(detail.updated_at) }}</span>
        </div>

        <div class="section-head">
          <h4>描述与标签</h4>
          <el-button size="small" text type="primary" :icon="EditIcon" @click="openEditDesc">
            编辑描述
          </el-button>
        </div>
        <p class="desc-title" v-if="detail.title"><b>标题：</b>{{ detail.title }}</p>
        <p class="desc">{{ detail.description || '（暂无描述内容）' }}</p>
        <div v-if="detail.tags?.length" class="attr-tag-list">
          <el-tag v-for="t in detail.tags" :key="t" size="small" effect="plain">{{ t }}</el-tag>
        </div>

        <template v-if="lineage.length">
          <h4>谱系追溯</h4>
          <div class="lineage">
            <div v-for="(l, i) in lineage" :key="i" class="lineage-item">
              <span class="k">{{ l.k }}</span>
              <span class="v mono">{{ l.v }}</span>
            </div>
          </div>
        </template>

        <h4>
           文件属性
          <span class="hint-small">点击属性标签可一键复制</span>
        </h4>
        <div v-for="g in attrGroups" :key="g.name" class="attr-group">
          <div class="attr-group-name">{{ g.name }} · {{ g.items.length }}</div>
          <div class="attr-tag-list">
            <el-tooltip
              v-for="a in g.items"
              :key="a.key"
              :content="`${a.key}: ${a.value}`"
              placement="top"
              :show-after="300"
            >
              <el-tag
                size="small"
                :type="g.tagType"
                effect="plain"
                class="attr-tag clickable"
                @click="copyText(`${a.key}: ${a.value}`, `${a.short} 属性`)"
              >
                {{ a.short }}: {{ a.value }}
              </el-tag>
            </el-tooltip>
          </div>
        </div>
        <el-empty
          v-if="!attrGroups.length"
          description="暂无文件属性"
          :image-size="50"
        />
      </div>
    </el-drawer>

    <!-- 在线内容预览模态框 -->
    <el-dialog
      v-model="previewVisible"
      :title="`文件预览 · ${previewPath}`"
      width="780px"
      class="preview-dialog"
      destroy-on-close
    >
      <div v-loading="previewLoading" class="preview-content">
        <!-- 图像预览 -->
        <div v-if="previewIsImage" class="preview-image-wrap">
          <img :src="previewImageUrl" alt="preview" class="preview-img" />
        </div>

        <!-- 文本/代码预览 -->
        <div v-else-if="previewText !== null" class="preview-text-wrap">
          <div class="preview-meta-bar">
            <span>字符数: {{ previewText.length }}</span>
            <span v-if="previewTruncated" class="warn-badge">已截断显示前 100KB</span>
            <div class="flex-fill"></div>
            <el-button size="small" text :icon="DocumentCopyIcon" @click="copyText(previewText, '全文内容')">
              复制内容
            </el-button>
          </div>
          <pre class="preview-pre"><code>{{ previewText }}</code></pre>
        </div>

        <el-empty v-else-if="!previewLoading" description="无法预览此文件类型或内容为空" />
      </div>
      <template #footer>
        <el-button @click="previewVisible = false">关闭</el-button>
        <el-button type="primary" :icon="DownloadIcon" @click="download(previewPath)">下载原文件</el-button>
      </template>
    </el-dialog>

    <!-- 编辑描述与标签对话框 -->
    <el-dialog v-model="editDescVisible" title="编辑描述与元数据" width="520px">
      <el-form :model="editForm" label-width="70px">
        <el-form-item label="路径">
          <el-input :model-value="detail?.file_path" disabled />
        </el-form-item>
        <el-form-item label="标题">
          <el-input v-model="editForm.title" placeholder="简短标题（便于检索）" clearable />
        </el-form-item>
        <el-form-item label="描述">
          <el-input
            v-model="editForm.description"
            type="textarea"
            :rows="3"
            placeholder="描述正文，供 Agent 与管理台全文检索"
          />
        </el-form-item>
        <el-form-item label="更新模式">
          <el-radio-group v-model="editForm.mode">
             <el-radio-button value="replace">替换描述</el-radio-button>
             <el-radio-button value="append">追加描述</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="标签">
          <div class="tag-input-wrap">
            <el-tag
              v-for="(t, idx) in editForm.tags"
              :key="t"
              closable
              size="small"
              @close="removeEditTag(idx)"
            >
              {{ t }}
            </el-tag>
            <el-input
              v-if="newTagVisible"
              ref="newTagInputRef"
              v-model="newTagValue"
              size="small"
              class="new-tag-input"
              @keyup.enter="confirmNewTag"
              @blur="confirmNewTag"
            />
            <el-button v-else size="small" text @click="showNewTagInput">+ 新标签</el-button>
          </div>
        </el-form-item>
        <el-form-item label="可见性">
          <el-select v-model="editForm.visibility" placeholder="选择可见性">
            <el-option label="公开 (public) - 所有人可读写" value="public" />
            <el-option label="私密 (private) - 仅主人与管家可动" value="private" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDescVisible = false">取消</el-button>
        <el-button type="primary" :loading="savingDesc" @click="saveDesc">保存元数据</el-button>
      </template>
    </el-dialog>

    <!-- 移动对话框 -->
    <el-dialog v-model="moveVisible" title="移动或重命名" width="480px">
      <el-form label-width="70px">
        <el-form-item label="源">
          <el-input :model-value="moveFrom" disabled />
        </el-form-item>
        <el-form-item label="目标">
          <el-input v-model="moveTo" placeholder="例：~agent/书房档案/新名.txt" />
        </el-form-item>
      </el-form>
      <p class="hint">请输入完整目标路径。移动后文件描述和标识保持不变。</p>
      <template #footer>
        <el-button @click="moveVisible = false">取消</el-button>
        <el-button type="primary" :loading="moving" @click="doMove">移动</el-button>
      </template>
    </el-dialog>

    <!-- 复制对话框 -->
    <el-dialog v-model="copyVisible" title="复制文件" width="480px">
      <el-form label-width="70px">
        <el-form-item label="源">
          <el-input :model-value="copyFrom" disabled />
        </el-form-item>
        <el-form-item label="目标">
           <el-input v-model="copyTo" placeholder="输入目标文件路径" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="copyVisible = false">取消</el-button>
        <el-button type="primary" :loading="copying" @click="doCopy">复制</el-button>
      </template>
    </el-dialog>

    <!-- 条件检索对话框（索引机直查） -->
    <el-dialog v-model="condVisible" title="高级筛选" width="760px">
      <div v-if="catalogError" class="hint">{{ catalogError }}</div>
      <template v-else>
        <div v-for="(c, i) in condRows" :key="i" class="cond-item">
          <div class="cond-row">
            <el-select
              v-model="c.field"
              filterable
              placeholder="选择字段（来自索引目录）"
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

        <el-button size="small" text type="primary" @click="addRow">添加条件（同时满足）</el-button>
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
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import {
  Search as SearchIcon,
  Filter as FilterIcon,
  Delete as DeleteIcon,
  Download as DownloadIcon,
  View as ViewIcon,
  Edit as EditIcon,
  Refresh as RefreshIcon,
  DocumentCopy as DocumentCopyIcon,
  Share as ShareIcon,
} from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';
import {
  getFiles,
  searchFiles,
  getFileMetadata,
  getIndexFields,
  moveFile,
  copyFile,
  analyzeFile,
  describeFile,
  deleteFile,
  getShareLink,
  downloadFileBlob,
  downloadZipBlob,
} from '../api/files';
import { formatBytes, formatDate } from '../utils/format';
import { downloadBlob as saveBlob } from '../utils/download';

// ---- 文件数据与筛选 ----
const tree = ref([]);
const flat = ref([]); // 全量平铺（表格底料）
const rows = ref([]);
const currentDir = ref('');
const mode = ref('all'); // all | dir | search | cond
const query = ref('');
const condTotal = ref(0);
const tableRef = ref(null);
const listLoading = ref(false);
const page = ref(1);
const pageSize = ref(20);
const sort = ref({ prop: '', order: null });
const directories = computed(() => {
  const paths = [];
  const visit = (nodes) => {
    for (const node of nodes) {
      if (node.type === 'dir') { paths.push(node.path); visit(node.children || []); }
    }
  };
  visit(tree.value);
  return paths.sort((a, b) => a.localeCompare(b));
});
const sortedRows = computed(() => {
  const { prop, order } = sort.value;
  if (!prop || !order) return rows.value;
  return [...rows.value].sort((a, b) => {
    const left = a[prop] ?? '';
    const right = b[prop] ?? '';
    const comparison = prop === 'size_bytes' ? Number(left) - Number(right)
      : String(left).localeCompare(String(right), 'zh-CN', { numeric: true });
    return order === 'ascending' ? comparison : -comparison;
  });
});
const pagedRows = computed(() => sortedRows.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value));
const sortFiles = ({ prop, order }) => { sort.value = { prop, order }; page.value = 1; clearSelection(); };
watch([rows, pageSize], () => { page.value = 1; clearSelection(); });
watch(page, () => clearSelection());

// 表格多选
const selectedRows = ref([]);
const selectedTotalBytes = computed(() =>
  selectedRows.value.reduce((acc, cur) => acc + (cur.size_bytes || 0), 0)
);

const handleSelectionChange = (selection) => {
  selectedRows.value = selection;
};

const clearSelection = () => {
  tableRef.value?.clearSelection();
  selectedRows.value = [];
};

const filterDirectory = () => {
  query.value = '';
  mode.value = currentDir.value ? 'dir' : 'all';
  applyMode();
};

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
  listLoading.value = true;
  try {
    const { data } = await getFiles();
    tree.value = data.tree || [];
    const out = [];
    walk(tree.value, out);
    flat.value = out;
    if (mode.value === 'search') await doSearch();
    else if (mode.value === 'cond') await doCondSearch();
    else applyMode();
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '加载文件列表失败');
  } finally { listLoading.value = false; }
};

const applyMode = () => {
  if (mode.value === 'search' || mode.value === 'cond') return;
  if (mode.value === 'dir' && currentDir.value) {
    const prefix = currentDir.value + '/';
    rows.value = flat.value.filter((f) => f.path.startsWith(prefix));
  } else {
    rows.value = flat.value;
  }
};

const doSearch = async () => {
  if (!query.value.trim()) return clearSearch();
  listLoading.value = true;
  try {
    const { data } = await searchFiles({ q: query.value.trim(), size: 100 });
    rows.value = (data.items || []).map(normRow);
    mode.value = 'search';
    currentDir.value = '';
  } catch (e) { ElMessage.error(e?.response?.data?.error || '搜索失败'); }
  finally { listLoading.value = false; }
};

const clearSearch = () => {
  query.value = '';
  mode.value = 'all';
  currentDir.value = '';
  condTotal.value = 0;
  applyMode();
};

// ---- 条件检索 ----
const condVisible = ref(false);
const condSearching = ref(false);
const catalog = ref([]);
const catalogLoaded = ref(false);
const catalogError = ref('');
const guideRules = ref([]);
const guideQuick = ref([]);
const condRows = ref([]);

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
      '（索引机未装配时目录端点 503）';
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
    currentDir.value = '';
    condVisible.value = false;
    if (!rows.value.length) {
      ElMessage.info('未找到符合条件的文件，请尝试放宽条件');
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
  try {
    const { data } = await getFileMetadata(path);
    detail.value = data;
    detailVisible.value = true;
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '获取文件详情失败');
  }
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
    { name: '基础事实', prefix: 'cod-basic-', tagType: 'info' },
    { name: '文本结构与指纹', prefix: 'cod-text-', tagType: 'success' },
    { name: '代码语言与统计', prefix: 'cod-code-', tagType: 'warning' },
    { name: '图像色彩与尺寸', prefix: 'cod-image-', tagType: 'danger' },
    { name: 'LLM 语义标注', prefix: 'llm-', tagType: 'primary' },
    { name: '自定义扩展', prefix: 'sp-', tagType: 'info' },
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
  try {
    const { data } = await downloadFileBlob(path);
    saveBlob(data, path.split('/').pop());
  } catch (e) {
    ElMessage.error('下载失败：' + (e?.response?.data?.error || e.message));
  }
};

// ---- 在线预览 ----
const previewVisible = ref(false);
const previewPath = ref('');
const previewLoading = ref(false);
const previewText = ref(null);
const previewIsImage = ref(false);
const previewImageUrl = ref('');
const previewTruncated = ref(false);

const preview = async (path) => {
  previewPath.value = path;
  previewText.value = null;
  previewIsImage.value = false;
  if (previewImageUrl.value) {
    URL.revokeObjectURL(previewImageUrl.value);
    previewImageUrl.value = '';
  }
  previewTruncated.value = false;
  previewVisible.value = true;
  previewLoading.value = true;

  try {
    const { data } = await downloadFileBlob(path);
    const ext = path.split('.').pop()?.toLowerCase();
    const imageExts = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'ico'];

    if (imageExts.includes(ext) || data.type?.startsWith('image/')) {
      previewIsImage.value = true;
      previewImageUrl.value = URL.createObjectURL(data);
    } else {
      // 文本读取
      const rawText = await data.text();
      const maxLen = 100 * 1024;
      if (rawText.length > maxLen) {
        previewText.value = rawText.slice(0, maxLen);
        previewTruncated.value = true;
      } else {
        previewText.value = rawText;
      }
    }
  } catch (e) {
    ElMessage.error('读取预览失败：' + (e?.response?.data?.error || e.message));
  } finally {
    previewLoading.value = false;
  }
};

// ---- 编辑描述与元数据 ----
const editDescVisible = ref(false);
const savingDesc = ref(false);
const newTagVisible = ref(false);
const newTagValue = ref('');
const newTagInputRef = ref(null);
const editForm = ref({
  title: '',
  description: '',
  tags: [],
  visibility: 'public',
  mode: 'replace',
});

const openEditDesc = () => {
  if (!detail.value) return;
  editForm.value = {
    title: detail.value.title || '',
    description: detail.value.description || '',
    tags: [...(detail.value.tags || [])],
    visibility: detail.value.visibility || 'public',
    mode: 'replace',
  };
  editDescVisible.value = true;
};

const removeEditTag = (idx) => {
  editForm.value.tags.splice(idx, 1);
};

const showNewTagInput = () => {
  newTagVisible.value = true;
  newTagValue.value = '';
  nextTick(() => newTagInputRef.value?.focus());
};

const confirmNewTag = () => {
  const val = newTagValue.value.trim();
  if (val && !editForm.value.tags.includes(val)) {
    editForm.value.tags.push(val);
  }
  newTagVisible.value = false;
  newTagValue.value = '';
};

const saveDesc = async () => {
  if (!detail.value) return;
  savingDesc.value = true;
  try {
    await describeFile({
      path: detail.value.file_path,
      title: editForm.value.title || null,
      description: editForm.value.description || null,
      tags: editForm.value.tags,
      visibility: editForm.value.visibility,
      mode: editForm.value.mode,
    });
    ElMessage.success('元数据保存成功');
    editDescVisible.value = false;
    await openDetail(detail.value.file_path);
    await refresh();
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '保存元数据失败');
  } finally {
    savingDesc.value = false;
  }
};

// ---- 删除与批量操作 ----
const doDelete = async (path) => {
  try {
    await deleteFile(path);
    ElMessage.success('文件已软删除');
    detailVisible.value = false;
    await refresh();
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '删除失败');
  }
};

const batchDeleting = ref(false);
const batchDelete = async () => {
  if (!selectedRows.value.length) return;
  batchDeleting.value = true;
  let successCount = 0;
  for (const row of selectedRows.value) {
    try {
      await deleteFile(row.path);
      successCount++;
    } catch (_) {}
  }
  ElMessage.success(`成功删除 ${successCount} 个文件`);
  selectedRows.value = [];
  batchDeleting.value = false;
  await refresh();
};

const batchDownload = async () => {
  if (!selectedRows.value.length) return;
  try {
    const paths = selectedRows.value.map((r) => r.path);
    const { data } = await downloadZipBlob(paths);
    saveBlob(data, `mabel-files-${Date.now()}.zip`);
    ElMessage.success('ZIP 打包下载开始');
  } catch (e) {
    ElMessage.error('打包下载失败：' + (e?.response?.data?.error || e.message));
  }
};

const shareLink = async (path) => {
  try {
    const { data } = await getShareLink(path);
    if (data.short_url) {
      await navigator.clipboard.writeText(data.short_url);
      ElMessage.success({
        message: `已复制短链到剪贴板：${data.short_url}（24小时有效）`,
        duration: 4000,
      });
    }
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '生成分享短链失败');
  }
};

const handleRowCommand = (cmd, path) => {
  if (cmd === 'detail') openDetail(path);
  if (cmd === 'share') shareLink(path);
  if (cmd === 'move') openMove(path);
  if (cmd === 'copy') openCopy(path);
  if (cmd === 'reanalyze') reanalyze(path);
  if (cmd === 'delete') {
    doDelete(path);
  }
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
    ElMessage.success('文件已移动');
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

// 辅助工具：剪贴板复制
const copyText = (text, label = '内容') => {
  if (!text) return;
  navigator.clipboard.writeText(String(text)).then(() => {
    ElMessage.success(`已复制 ${label}`);
  }).catch(() => {
    ElMessage.info('复制失败，请手动选择复制');
  });
};

onMounted(refresh);
</script>

<style scoped>
.stat-bar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  flex-wrap: wrap;
  background: var(--mabel-surface);
  border: 1px solid var(--mabel-border);
  border-radius: 10px;
}
.stat {
  display: flex;
  align-items: baseline;
  gap: 6px;
}
.stat-num {
  font-size: 1.15rem;
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
  width: 340px;
  max-width: 100%;
}
.directory-filter { width: 240px; }
.file-name { font-weight: 500; color: var(--mabel-text); }
.file-path { font-size: 12px; color: var(--mabel-text-muted); overflow: hidden; text-overflow: ellipsis; }
.result-count { font-size: 12px; font-weight: 400; }
.file-pagination { padding: 14px 16px; border-top: 1px solid var(--mabel-border); display: flex; justify-content: flex-end; overflow: auto; }

/* 批量操作悬浮条 */
.batch-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 16px;
  background: var(--el-color-primary-light-9);
  border: 1px solid var(--el-color-primary-light-8);
  border-radius: 8px;
  flex-wrap: wrap;
  gap: 10px;
}
.batch-info {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.batch-count {
  font-weight: 600;
  color: var(--el-color-primary);
  font-size: 0.9rem;
}
.batch-size {
  color: var(--mabel-text-muted);
  font-size: 0.8rem;
}
.batch-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.pane-tree {
  width: 300px;
  flex: none;
}
.pane-table {
  flex: 1;
}
.pane-title {
  font-weight: 600;
  color: var(--mabel-text);
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
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 18px;
  border-bottom: 1px solid var(--mabel-border);
  padding-bottom: 4px;
}
.section-head h4 {
  margin: 0;
  border: none;
  padding: 0;
}
.hint-small {
  font-size: 0.72rem;
  font-weight: 400;
  color: #7b748f;
  margin-left: 6px;
}
.detail-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--mabel-border);
}
.desc-title {
  font-size: 0.88rem;
  margin: 8px 0 4px;
  color: var(--mabel-text);
}
.desc {
  font-size: 0.86rem;
  line-height: 1.6;
  margin: 0 0 10px;
  color: var(--mabel-text);
  background: var(--mabel-surface-2);
  padding: 8px 12px;
  border-radius: 6px;
}
.mono {
  font-family: Consolas, Monaco, monospace;
  font-size: 0.8rem;
}
.clickable {
  cursor: pointer;
  transition: opacity 0.2s;
}
.clickable:hover {
  opacity: 0.8;
  text-decoration: underline;
}
.lineage-item {
  display: flex;
  gap: 10px;
  font-size: 0.82rem;
  margin-bottom: 4px;
}
.attr-group {
  margin-bottom: 12px;
}
.attr-group-name {
  font-size: 0.78rem;
  color: var(--mabel-text-muted);
  margin-bottom: 5px;
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

/* 预览样式 */
.preview-dialog :deep(.el-dialog__body) {
  padding: 12px 20px;
}
.preview-content {
  min-height: 240px;
  max-height: 520px;
  display: flex;
  flex-direction: column;
}
.preview-image-wrap {
  display: flex;
  justify-content: center;
  align-items: center;
  background: #000;
  border-radius: 8px;
  padding: 16px;
  overflow: auto;
}
.preview-img {
  max-width: 100%;
  max-height: 480px;
  object-fit: contain;
  border-radius: 4px;
}
.preview-text-wrap {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}
.preview-meta-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 0.78rem;
  color: var(--mabel-text-muted);
  margin-bottom: 8px;
}
.warn-badge {
  color: #e6a23c;
  background: rgba(230, 162, 60, 0.15);
  padding: 2px 6px;
  border-radius: 4px;
}
.preview-pre {
  margin: 0;
  flex: 1;
  overflow: auto;
  background: var(--mabel-surface-2);
  border: 1px solid var(--mabel-border);
  padding: 12px;
  border-radius: 6px;
  font-family: Consolas, Monaco, monospace;
  font-size: 0.82rem;
  line-height: 1.5;
  color: var(--mabel-text);
  white-space: pre-wrap;
  word-break: break-all;
}

/* 标签输入 */
.tag-input-wrap {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.new-tag-input {
  width: 90px;
}

/* 条件检索 */
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
@media (max-width: 720px) {
  .directory-filter, .search-box { width: 100%; }
  .stat-bar > .flex-fill { display: none; }
  .batch-actions { flex-wrap: wrap; gap: 6px; }
  .cond-row { flex-wrap: wrap; }
  .cond-field { width: 100%; }
}
</style>
