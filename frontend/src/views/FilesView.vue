<template>
  <div class="files-view">
    <div class="view-header">
      <h2>Files</h2>
      <button class="btn-ghost" @click="fetchFiles">Refresh</button>
    </div>

    <div class="search-bar">
      <input v-model="searchQuery" type="text" placeholder="关键词" @keyup.enter="handleSearch" />
      <input v-model="searchTag" type="text" placeholder="标签" @keyup.enter="handleSearch" />
      <button class="btn" @click="handleSearch">搜索</button>
      <button v-if="searching" class="btn-ghost" @click="clearSearch">清除</button>
    </div>

    <div v-if="loading" class="state">Loading...</div>
    <div v-else-if="error" class="state error">{{ error }}</div>

    <!-- 搜索结果 -->
    <template v-else-if="searching">
      <div v-if="searchResults.length === 0" class="state">No files matched.</div>
      <ul v-else class="result-list">
        <li v-for="f in searchResults" :key="f.file_path" class="result-item">
          <div class="result-main">
            <div class="result-title">
              <span class="name">{{ f.file_path }}</span>
              <span v-if="f.title" class="title">{{ f.title }}</span>
              <span v-if="f.file_type" class="badge">{{ f.file_type }}</span>
            </div>
            <div v-if="f.description" class="desc">{{ f.description }}</div>
            <div class="tags">
              <span v-for="t in f.tags" :key="t" class="tag">{{ t }}</span>
            </div>
          </div>
          <div class="result-actions">
            <button class="btn" @click="handleDownload(f.file_path)">Download</button>
            <button class="btn-ghost" @click="openDescribe(f.file_path)">Describe</button>
            <button class="btn-ghost" @click="openCopy(f.file_path)">Copy</button>
          </div>
        </li>
      </ul>
    </template>

    <!-- 目录树 -->
    <div v-else-if="tree.length === 0" class="state">No files found.</div>
    <ul v-else class="tree">
      <FileTreeNode
        v-for="node in tree"
        :key="node.path"
        :node="node"
        @download="handleDownload"
        @describe="openDescribe"
        @copy="openCopy"
      />
    </ul>

    <!-- Describe 弹窗 -->
    <div v-if="describeModal.open" class="modal-mask" @click.self="describeModal.open = false">
      <div class="modal">
        <h3>描述文件</h3>
        <label class="field"><span>文件</span><input :value="describeModal.path" disabled /></label>
        <label class="field"><span>标题</span><input v-model="describeModal.title" /></label>
        <label class="field"><span>描述</span><textarea v-model="describeModal.description" rows="3"></textarea></label>
        <label class="field"><span>标签（逗号分隔）</span><input v-model="describeModal.tags" /></label>
        <label class="field"><span>类型</span><input v-model="describeModal.fileType" placeholder="text / image / code / other" /></label>
        <div class="modal-actions">
          <button class="btn" @click="saveDescribe">保存</button>
          <button class="btn-ghost" @click="describeModal.open = false">取消</button>
        </div>
      </div>
    </div>

    <!-- Copy 弹窗 -->
    <div v-if="copyModal.open" class="modal-mask" @click.self="copyModal.open = false">
      <div class="modal">
        <h3>复制文件</h3>
        <label class="field"><span>源文件</span><input :value="copyModal.source" disabled /></label>
        <label class="field"><span>目标路径</span><input v-model="copyModal.target" placeholder="sub/new-name.txt" /></label>
        <div class="modal-actions">
          <button class="btn" @click="saveCopy">复制</button>
          <button class="btn-ghost" @click="copyModal.open = false">取消</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import { getFiles, searchFiles, getFileMetadata, describeFile, copyFile } from '../api/files';
import FileTreeNode from '../components/FileTreeNode.vue';

const tree = ref([]);
const loading = ref(true);
const error = ref('');
const searching = ref(false);
const searchQuery = ref('');
const searchTag = ref('');
const searchResults = ref([]);

const describeModal = ref({ open: false, path: '', title: '', description: '', tags: '', fileType: '' });
const copyModal = ref({ open: false, source: '', target: '' });

const fetchFiles = async () => {
  loading.value = true;
  error.value = '';
  try {
    const response = await getFiles();
    tree.value = response.data.tree || [];
  } catch (err) {
    error.value = 'Failed to load files.';
  } finally {
    loading.value = false;
  }
};

const handleSearch = async () => {
  searching.value = true;
  error.value = '';
  try {
    const params = {};
    if (searchQuery.value) params.q = searchQuery.value;
    if (searchTag.value) params.tag = searchTag.value;
    const response = await searchFiles(params);
    searchResults.value = response.data.items || [];
  } catch (err) {
    error.value = '搜索失败。';
  }
};

const clearSearch = () => {
  searching.value = false;
  searchQuery.value = '';
  searchTag.value = '';
  searchResults.value = [];
};

const openDescribe = async (path) => {
  describeModal.value = { open: true, path, title: '', description: '', tags: '', fileType: '' };
  try {
    const response = await getFileMetadata(path);
    const m = response.data;
    describeModal.value.title = m.title || '';
    describeModal.value.description = m.description || '';
    describeModal.value.tags = (m.tags || []).join(', ');
    describeModal.value.fileType = m.file_type || '';
  } catch {
    // 没有元数据时留空
  }
};

const saveDescribe = async () => {
  const tags = describeModal.value.tags
    .split(',')
    .map((t) => t.trim())
    .filter((t) => t);
  try {
    await describeFile({
      path: describeModal.value.path,
      title: describeModal.value.title || undefined,
      description: describeModal.value.description || undefined,
      tags,
      file_type: describeModal.value.fileType || undefined,
    });
    describeModal.value.open = false;
    if (searching.value) handleSearch();
    else fetchFiles();
  } catch (err) {
    alert('描述失败：' + (err.response?.data?.detail || err.message));
  }
};

const openCopy = (source) => {
  copyModal.value = { open: true, source, target: '' };
};

const saveCopy = async () => {
  if (!copyModal.value.target) {
    alert('请填写目标路径');
    return;
  }
  try {
    await copyFile({ source: copyModal.value.source, target: copyModal.value.target });
    copyModal.value.open = false;
    fetchFiles();
  } catch (err) {
    alert('复制失败：' + (err.response?.data?.detail || err.message));
  }
};

const handleDownload = (path) => {
  const token = import.meta.env.VITE_ACCESS_TOKEN || '';
  const query = `path=${encodeURIComponent(path)}` +
    (token ? `&token=${encodeURIComponent(token)}` : '');
  window.location.href = `/api/files/download?${query}`;
};

onMounted(fetchFiles);
</script>

<style scoped>
.search-bar {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 1rem;
}
.search-bar input {
  flex: 1;
  padding: 0.5rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
}
.result-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.result-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.75rem;
  border-bottom: 1px solid var(--color-border);
}
.result-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.result-title .name {
  font-weight: 600;
}
.result-title .title {
  color: var(--color-text);
}
.badge {
  font-size: 0.75rem;
  padding: 0.1rem 0.4rem;
  background: var(--color-bg);
  border-radius: 4px;
  color: var(--color-text-muted);
}
.desc {
  margin-top: 0.25rem;
  color: var(--color-text-muted);
  font-size: 0.9rem;
}
.tags {
  margin-top: 0.25rem;
  display: flex;
  gap: 0.3rem;
  flex-wrap: wrap;
}
.tag {
  font-size: 0.75rem;
  padding: 0.1rem 0.5rem;
  background: var(--color-primary);
  color: #fff;
  border-radius: 10px;
}
.result-actions {
  display: flex;
  gap: 0.4rem;
  flex-shrink: 0;
}
.tree {
  background-color: var(--color-surface);
  border-radius: 8px;
  padding: 0.75rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  margin: 0;
}
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 10;
}
.modal {
  background: var(--color-surface);
  padding: 1.5rem;
  border-radius: 10px;
  width: 90%;
  max-width: 460px;
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.2);
}
.modal h3 {
  margin: 0 0 1rem;
}
.field {
  display: block;
  margin-bottom: 0.75rem;
}
.field span {
  display: block;
  margin-bottom: 0.3rem;
  font-size: 0.85rem;
  color: var(--color-text-muted);
}
.field input,
.field textarea {
  width: 100%;
  padding: 0.5rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}
</style>
