<template>
  <div class="page-shell">
    <!-- 顶部过滤与控制栏 -->
    <div class="ctrl-bar">
      <div class="title-area">
        <span class="view-title">Agent 工具调用日志</span>
        <span class="total-tag">共 {{ total }} 条记录</span>
      </div>

      <div class="filter-group">
        <!-- 状态过滤 -->
        <el-radio-group v-model="filterStatus" size="small" @change="handleFilterChange">
          <el-radio-button value="">全部</el-radio-button>
          <el-radio-button value="success">成功</el-radio-button>
          <el-radio-button value="failed">失败</el-radio-button>
          <el-radio-button value="denied">拒绝</el-radio-button>
        </el-radio-group>

        <!-- 工具名搜索 -->
        <el-input
          v-model="filterTool"
          placeholder="按工具名过滤..."
          size="small"
          clearable
          class="tool-input"
          @input="handleFilterChange"
        />
      </div>

      <div class="flex-fill"></div>

      <!-- 实时轮询开关 -->
      <div class="poll-switch">
        <span class="poll-label">实时刷新</span>
        <el-switch
          v-model="autoRefresh"
          size="small"
          inline-prompt
          active-text="开"
          inactive-text="关"
          @change="toggleAutoRefresh"
        />
        <span v-if="autoRefresh" class="pulse-dot" title="实时轮询中（每 3 秒刷新）"></span>
      </div>

      <el-button size="small" :icon="RefreshIcon" :loading="loading" @click="fetchList">
        刷新
      </el-button>
    </div>

    <!-- 主表格区 -->
    <div class="pane table-pane">
      <div class="pane-body table-body">
        <el-table
          v-loading="loading"
          :data="filteredItems"
          height="100%"
          size="small"
          highlight-current-row
          row-key="id"
        >
          <el-table-column prop="created_at" label="调用时间" width="170">
            <template #default="{ row }">
              <span class="mono">{{ formatDate(row.created_at) }}</span>
            </template>
          </el-table-column>

          <el-table-column prop="tool_name" label="工具名称" width="160">
            <template #default="{ row }">
              <span class="tool-tag">{{ row.tool_name }}</span>
            </template>
          </el-table-column>

          <el-table-column prop="status" label="状态" width="90" align="center">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small" effect="plain">
                {{ row.status }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column prop="file_path" label="目标路径 / 键" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">
              <span v-if="row.file_path" class="mono file-path">{{ row.file_path }}</span>
              <span v-else class="text-muted">—</span>
            </template>
          </el-table-column>

          <el-table-column label="调用参数概览" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="mono params-preview">{{ formatParamsPreview(row.params) }}</span>
            </template>
          </el-table-column>

          <el-table-column prop="error" label="错误 / 拒绝原因" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">
              <span v-if="row.error" class="error-text">{{ row.error }}</span>
              <span v-else class="text-muted">—</span>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="90" align="center" fixed="right">
            <template #default="{ row }">
              <el-button size="small" text type="primary" @click="showDetail(row)">
                详情
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 分页栏 -->
      <div class="pagination-bar">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[20, 50, 100]"
          size="small"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="fetchList"
          @current-change="fetchList"
        />
      </div>
    </div>

    <!-- 单次调用详情弹窗 -->
    <el-dialog v-model="detailVisible" title="调用详情与参数报文" width="600px" destroy-on-close>
      <div v-if="currentOp" class="op-detail">
        <div class="meta-grid">
          <span class="k">记录 ID</span><span class="v mono">{{ currentOp.id }}</span>
          <span class="k">调用时间</span><span class="v">{{ formatDate(currentOp.created_at) }}</span>
          <span class="k">工具名称</span><span class="v"><span class="tool-tag">{{ currentOp.tool_name }}</span></span>
          <span class="k">执行状态</span>
          <span class="v">
            <el-tag :type="statusTagType(currentOp.status)" size="small" effect="plain">
              {{ currentOp.status }}
            </el-tag>
          </span>
          <span class="k">目标文件</span><span class="v mono">{{ currentOp.file_path || '（无文件目标）' }}</span>
          <span class="k">会话 Session</span><span class="v mono">{{ currentOp.session_id || '（无）' }}</span>
        </div>

        <div v-if="currentOp.error" class="error-block">
          <div class="error-title">异常信息 / 拒绝理由：</div>
          <div class="error-content">{{ currentOp.error }}</div>
        </div>

        <div class="params-block">
          <div class="params-title">
            <span>入参明细 (Params JSON)</span>
            <div class="flex-fill"></div>
            <el-button size="small" text @click="copyJson(currentOp.params)">复制 JSON</el-button>
          </div>
          <pre class="json-pre"><code>{{ formatPrettyJson(currentOp.params) }}</code></pre>
        </div>
      </div>
      <template #footer>
        <el-button @click="detailVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { Refresh as RefreshIcon } from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';
import { getOperations } from '../api/operations';
import { formatDate } from '../utils/format';

const items = ref([]);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const loading = ref(false);

const filterStatus = ref('');
const filterTool = ref('');

// 自动轮询
const autoRefresh = ref(true);
let timer = null;

const filteredItems = computed(() => {
  return items.value.filter((it) => {
    if (filterStatus.value && it.status !== filterStatus.value) return false;
    if (filterTool.value && !it.tool_name?.toLowerCase().includes(filterTool.value.toLowerCase())) {
      return false;
    }
    return true;
  });
});

const statusTagType = (st) => {
  switch (st) {
    case 'success':
      return 'success';
    case 'failed':
      return 'danger';
    case 'denied':
      return 'warning';
    default:
      return 'info';
  }
};

const formatParamsPreview = (raw) => {
  if (!raw) return '—';
  try {
    const obj = typeof raw === 'string' ? JSON.parse(raw) : raw;
    const entries = Object.entries(obj || {});
    if (entries.length === 0) return '{}';
    return entries.map(([k, v]) => `${k}=${JSON.stringify(v)}`).join(', ');
  } catch {
    return String(raw);
  }
};

const formatPrettyJson = (raw) => {
  if (!raw) return '{}';
  try {
    const obj = typeof raw === 'string' ? JSON.parse(raw) : raw;
    return JSON.stringify(obj, null, 2);
  } catch {
    return String(raw);
  }
};

const fetchList = async () => {
  loading.value = true;
  try {
    const { data } = await getOperations(page.value, size.value);
    items.value = data.items || [];
    total.value = data.total || 0;
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '获取调用日志失败');
  } finally {
    loading.value = false;
  }
};

const handleFilterChange = () => {
  // 本地即时计算 filteredItems
};

const toggleAutoRefresh = (val) => {
  if (val) {
    startTimer();
  } else {
    stopTimer();
  }
};

const startTimer = () => {
  stopTimer();
  timer = setInterval(() => {
    // 仅在当前第一页时做后台轮询更新，避免翻页时打扰用户阅读
    if (page.value === 1) {
      getOperations(1, size.value).then(({ data }) => {
        items.value = data.items || [];
        total.value = data.total || 0;
      }).catch(() => {});
    }
  }, 3000);
};

const stopTimer = () => {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
};

// 详情弹窗
const detailVisible = ref(false);
const currentOp = ref(null);

const showDetail = (row) => {
  currentOp.value = row;
  detailVisible.value = true;
};

const copyJson = (data) => {
  const str = formatPrettyJson(data);
  navigator.clipboard.writeText(str).then(() => {
    ElMessage.success('已复制参数 JSON');
  });
};

onMounted(() => {
  fetchList();
  if (autoRefresh.value) {
    startTimer();
  }
});

onUnmounted(() => {
  stopTimer();
});
</script>

<style scoped>
.ctrl-bar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 16px;
  background: linear-gradient(135deg, var(--mabel-surface) 0%, #201a2c 100%);
  border: 1px solid var(--mabel-border);
  border-radius: 10px;
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.2);
}
.title-area {
  display: flex;
  align-items: baseline;
  gap: 10px;
}
.view-title {
  font-size: 1rem;
  font-weight: 700;
  color: var(--mabel-text);
}
.total-tag {
  font-size: 0.78rem;
  color: var(--mabel-accent);
}
.filter-group {
  display: flex;
  align-items: center;
  gap: 12px;
}
.tool-input {
  width: 160px;
}
.poll-switch {
  display: flex;
  align-items: center;
  gap: 8px;
}
.poll-label {
  font-size: 0.78rem;
  color: var(--mabel-text-muted);
}
.pulse-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-color: #67c23a;
  box-shadow: 0 0 8px #67c23a;
  animation: pulse 1.5s infinite;
}
@keyframes pulse {
  0% { opacity: 0.4; }
  50% { opacity: 1; transform: scale(1.15); }
  100% { opacity: 0.4; }
}

.table-pane {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.table-body {
  flex: 1;
  min-height: 0;
  padding: 0;
}
.pagination-bar {
  flex: none;
  display: flex;
  justify-content: flex-end;
  padding: 10px 16px;
  border-top: 1px solid var(--mabel-border);
  background: var(--mabel-surface);
}

.tool-tag {
  font-family: Consolas, Monaco, monospace;
  font-size: 0.82rem;
  color: var(--el-color-primary-light-3);
  background: rgba(184, 118, 217, 0.15);
  padding: 2px 6px;
  border-radius: 4px;
}
.file-path {
  color: #c0b8d4;
  font-size: 0.8rem;
}
.params-preview {
  color: var(--mabel-text-muted);
  font-size: 0.78rem;
}
.error-text {
  color: #f89898;
  font-size: 0.78rem;
}
.text-muted {
  color: #655e77;
}

/* 详情弹窗 */
.op-detail {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.error-block {
  background: rgba(245, 108, 108, 0.1);
  border: 1px solid rgba(245, 108, 108, 0.3);
  border-radius: 6px;
  padding: 10px 12px;
}
.error-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: #f56c6c;
  margin-bottom: 4px;
}
.error-content {
  font-size: 0.82rem;
  color: #fbc4c4;
  line-height: 1.5;
  word-break: break-all;
}
.params-block {
  display: flex;
  flex-direction: column;
}
.params-title {
  display: flex;
  align-items: center;
  font-size: 0.82rem;
  font-weight: 600;
  color: var(--mabel-text-muted);
  margin-bottom: 6px;
}
.json-pre {
  margin: 0;
  background: #110f17;
  border: 1px solid var(--mabel-border);
  padding: 12px;
  border-radius: 6px;
  font-family: Consolas, Monaco, monospace;
  font-size: 0.82rem;
  line-height: 1.5;
  color: #cfc8de;
  max-height: 280px;
  overflow: auto;
}
</style>
