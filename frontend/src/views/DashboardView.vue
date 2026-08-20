<template>
  <div class="activity-view">
    <div class="view-header">
      <h2>Activity</h2>
    </div>

    <div v-if="loading" class="state">Loading...</div>
    <div v-else-if="error" class="state error">{{ error }}</div>
    <div v-else-if="operations.length === 0" class="state">No activity yet.</div>

    <template v-else>
      <table class="table">
        <thead>
          <tr>
            <th>Time</th>
            <th>Tool</th>
            <th>File</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="op in operations" :key="op.id">
            <td>{{ formatDate(op.created_at) }}</td>
            <td><code>{{ op.tool_name }}</code></td>
            <td>{{ op.file_path || '—' }}</td>
            <td><StatusBadge :status="op.status" /></td>
          </tr>
        </tbody>
      </table>

      <div class="pagination">
        <button
          class="btn-ghost"
          :disabled="page <= 1"
          @click="fetchOperations(page - 1)"
        >
          Previous
        </button>
        <span>Page {{ page }}</span>
        <button
          class="btn-ghost"
          :disabled="operations.length < size"
          @click="fetchOperations(page + 1)"
        >
          Next
        </button>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import { getOperations } from '../api/operations';
import { formatDate } from '../utils/format';
import StatusBadge from '../components/StatusBadge.vue';

const operations = ref([]);
const loading = ref(true);
const error = ref('');
const page = ref(1);
const size = ref(20);

const fetchOperations = async (newPage = 1) => {
  loading.value = true;
  error.value = '';
  try {
    const response = await getOperations(newPage, size.value);
    operations.value = response.data.items;
    page.value = newPage;
  } catch (err) {
    error.value = 'Failed to load activity.';
  } finally {
    loading.value = false;
  }
};

onMounted(() => fetchOperations());
</script>

<style scoped>
.pagination {
  display: flex;
  gap: 1rem;
  align-items: center;
  justify-content: center;
  margin-top: 1rem;
}
</style>
