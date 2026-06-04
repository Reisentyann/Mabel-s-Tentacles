<template>
  <div class="files-container">
    <div class="header">
      <div class="nav-links">
        <h2>Workspace Files</h2>
        <router-link to="/dashboard" class="nav-btn">Dashboard</router-link>
      </div>
      <div class="actions">
        <button @click="fetchFiles" class="refresh-btn">Refresh</button>
        <button @click="handleLogout" class="logout-btn">Logout</button>
      </div>
    </div>

    <div v-if="loading">Loading files...</div>
    <div v-else-if="error" class="error">{{ error }}</div>
    
    <div v-else-if="files.length === 0" class="empty-state">
      No files found in the data directory.
    </div>
    
    <div v-else class="files-list">
      <table>
        <thead>
          <tr>
            <th>Filename</th>
            <th>Size (Bytes)</th>
            <th>Last Modified</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="file in files" :key="file.name">
            <td><strong>{{ file.name }}</strong></td>
            <td>{{ formatBytes(file.size) }}</td>
            <td>{{ new Date(file.modified_at * 1000).toLocaleString() }}</td>
            <td>
              <button @click="handleDownload(file.name)" class="action-btn">Download</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';
import { getFiles, downloadFileDirectly } from '../api/files';

const router = useRouter();
const authStore = useAuthStore();
const files = ref([]);
const loading = ref(true);
const error = ref('');

const fetchFiles = async () => {
  loading.value = true;
  error.value = '';
  try {
    const response = await getFiles();
    files.value = response.data.files || [];
  } catch (err) {
    error.value = 'Failed to load files.';
    console.error(err);
  } finally {
    loading.value = false;
  }
};

const formatBytes = (bytes, decimals = 2) => {
  if (!+bytes) return '0 Bytes';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
};

const handleDownload = async (filename) => {
  try {
    const response = await downloadFileDirectly(filename);
    const blob = new Blob([response.data]);
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  } catch (err) {
    alert('Failed to download file.');
    console.error(err);
  }
};

onMounted(() => {
  fetchFiles();
});

const handleLogout = async () => {
  await authStore.logoutAction();
  router.push('/login');
};
</script>

<style scoped>
.files-container { padding: 2rem; max-width: 1000px; margin: 0 auto; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 2rem; }
.nav-links { display: flex; align-items: center; gap: 1.5rem; }
.nav-links h2 { margin: 0; }
.nav-btn { text-decoration: none; padding: 0.4rem 0.8rem; background-color: #e9ecef; color: #333; border-radius: 4px; font-weight: 500; }
.nav-btn:hover { background-color: #dde0e3; }
.actions { display: flex; gap: 1rem; align-items: center; }
.refresh-btn { padding: 0.5rem 1rem; background-color: #28a745; color: white; border: none; border-radius: 4px; cursor: pointer; }
.refresh-btn:hover { background-color: #218838; }
.logout-btn { padding: 0.5rem 1rem; background-color: #f8f9fa; border: 1px solid #ddd; border-radius: 4px; cursor: pointer; }
.logout-btn:hover { background-color: #e2e6ea; }
.empty-state { text-align: center; padding: 2rem; background-color: #f8f9fa; border-radius: 8px; color: #6c757d; }
table { width: 100%; border-collapse: collapse; margin-bottom: 1rem; background-color: #fff; box-shadow: 0 1px 3px rgba(0,0,0,0.1); border-radius: 8px; overflow: hidden; }
th, td { border-bottom: 1px solid #ddd; padding: 1rem; text-align: left; }
th { background-color: #f4f4f4; font-weight: 600; }
tr:hover { background-color: #f8f9fa; }
.action-btn { padding: 0.3rem 0.8rem; background-color: #0066cc; color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 0.8rem; }
.action-btn:hover { background-color: #0052a3; }
.error { color: red; margin-bottom: 1rem; }
</style>
