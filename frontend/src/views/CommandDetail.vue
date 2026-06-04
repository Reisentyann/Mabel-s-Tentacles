<template>
  <div class="command-detail-container">
    <div class="header">
      <router-link to="/dashboard">&larr; Back to Dashboard</router-link>
      <h2>Command Detail #{{ $route.params.id }}</h2>
    </div>

    <div v-if="loading">Loading...</div>
    <div v-else-if="error" class="error">{{ error }}</div>
    
    <div v-else-if="command" class="detail-card">
      <div class="meta-info">
        <p><strong>Status:</strong> <span :class="['status', command.status]">{{ command.status }}</span></p>
        <p><strong>Type:</strong> {{ command.command_type }}</p>
        <p><strong>Exit Code:</strong> {{ command.exit_code !== null ? command.exit_code : 'N/A' }}</p>
        <p><strong>Created:</strong> {{ new Date(command.created_at).toLocaleString() }}</p>
        <p v-if="command.finished_at"><strong>Finished:</strong> {{ new Date(command.finished_at).toLocaleString() }}</p>
      </div>

      <div class="code-section">
        <h3>Command</h3>
        <pre><code>{{ command.command_text }}</code></pre>
      </div>

      <div class="code-section" v-if="command.result">
        <div class="section-header">
          <h3>Output (stdout)</h3>
          <button @click="downloadText(command.result, 'stdout.txt')" class="download-btn">Download Output</button>
        </div>
        <pre class="output"><code>{{ command.result }}</code></pre>
      </div>

      <div class="code-section error-section" v-if="command.error_message">
        <div class="section-header">
          <h3>Error (stderr)</h3>
          <button @click="downloadText(command.error_message, 'stderr.txt')" class="download-btn">Download Error</button>
        </div>
        <pre class="error-output"><code>{{ command.error_message }}</code></pre>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import { useRoute } from 'vue-router';
import { getCommandDetail } from '../api/commands';

const route = useRoute();
const command = ref(null);
const loading = ref(true);
const error = ref('');

const fetchDetail = async () => {
  loading.value = true;
  try {
    const response = await getCommandDetail(route.params.id);
    command.value = response.data;
  } catch (err) {
    error.value = 'Failed to load command details.';
  } finally {
    loading.value = false;
  }
};

const downloadText = (content, filename) => {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
};

onMounted(() => {
  fetchDetail();
});
</script>

<style scoped>
.command-detail-container { padding: 2rem; max-width: 1000px; margin: 0 auto; }
.header { margin-bottom: 2rem; }
.header a { text-decoration: none; color: #0066cc; font-weight: bold; }
.detail-card { border: 1px solid #ddd; border-radius: 8px; padding: 1.5rem; background-color: #fff; }
.meta-info { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin-bottom: 1.5rem; padding-bottom: 1.5rem; border-bottom: 1px solid #eee; }
.meta-info p { margin: 0; }
.status { padding: 0.2rem 0.5rem; border-radius: 4px; font-size: 0.8rem; font-weight: bold; }
.status.pending { background-color: #fff3cd; color: #555; }
.status.running { background-color: #cce5ff; color: #856404; }
.status.done { background-color: #d4edda; color: #155724; }
.status.error { background-color: #f8d7da; color: #721c24; }
.code-section { margin-bottom: 1.5rem; }
.code-section h3 { margin-bottom: 0.5rem; font-size: 1.1rem; }
.section-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem; }
.section-header h3 { margin-bottom: 0; }
.download-btn { padding: 0.3rem 0.6rem; background-color: #0066cc; color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 0.8rem; }
.download-btn:hover { background-color: #0052a3; }
pre { background-color: #f4f4f4; padding: 1rem; border-radius: 4px; overflow-x: auto; white-space: pre-wrap; word-wrap: break-word; }
.output { background-color: #2d2d2d; color: #f8f8f2; }
.error-output { background-color: #fbeaea; color: #d32f2f; border: 1px solid #f5c6c6; }
.error { color: red; }
</style>