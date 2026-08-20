<template>
  <div class="files-view">
    <div class="view-header">
      <h2>Files</h2>
      <button class="btn-ghost" @click="fetchFiles">Refresh</button>
    </div>

    <div v-if="loading" class="state">Loading...</div>
    <div v-else-if="error" class="state error">{{ error }}</div>
    <div v-else-if="tree.length === 0" class="state">No files found.</div>

    <ul v-else class="tree">
      <FileTreeNode
        v-for="node in tree"
        :key="node.path"
        :node="node"
        @download="handleDownload"
      />
    </ul>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import { getFiles } from '../api/files';
import FileTreeNode from '../components/FileTreeNode.vue';

const tree = ref([]);
const loading = ref(true);
const error = ref('');

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

const handleDownload = (path) => {
  const token = import.meta.env.VITE_ACCESS_TOKEN || '';
  const query = `path=${encodeURIComponent(path)}` +
    (token ? `&token=${encodeURIComponent(token)}` : '');
  window.location.href = `/api/files/download?${query}`;
};

onMounted(fetchFiles);
</script>

<style scoped>
.tree {
  background-color: var(--color-surface);
  border-radius: 8px;
  padding: 0.75rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  margin: 0;
}
</style>
