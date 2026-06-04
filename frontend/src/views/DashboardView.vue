<template>
  <div class="dashboard-container">
    <div class="header">
      <div class="nav-links">
        <h2>Dashboard</h2>
        <router-link to="/files" class="nav-btn">Workspace Files</router-link>
      </div>
      <button @click="handleLogout">Logout</button>
    </div>

    <div v-if="loading">Loading commands...</div>
    <div v-else-if="error" class="error">{{ error }}</div>
    
    <div v-else class="commands-list">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>Command</th>
            <th>Status</th>
            <th>Created At</th>
            <th>Action</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="cmd in commands" :key="cmd.id">
            <td>{{ cmd.id }}</td>
            <td><code>{{ cmd.command_text.substring(0, 30) }}{{ cmd.command_text.length > 30 ? '...' : '' }}</code></td>
            <td>
              <span :class="['status', cmd.status]">{{ cmd.status }}</span>
            </td>
            <td>{{ new Date(cmd.created_at).toLocaleString() }}</td>
            <td>
              <router-link :to="`/commands/${cmd.id}`">View Details</router-link>
            </td>
          </tr>
        </tbody>
      </table>

      <div class="pagination">
        <button :disabled="page <= 1" @click="fetchCommands(page - 1)">Previous</button>
        <span>Page {{ page }}</span>
        <button :disabled="commands.length < size" @click="fetchCommands(page + 1)">Next</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';
import { getCommands } from '../api/commands';

const router = useRouter();
const authStore = useAuthStore();

const commands = ref([]);
const loading = ref(true);
const error = ref('');
const page = ref(1);
const size = ref(10);
const total = ref(0);

const fetchCommands = async (newPage = 1) => {
  loading.value = true;
  error.value = '';
  try {
    const response = await getCommands(newPage, size.value);
    commands.value = response.data.items;
    total.value = response.data.total;
    page.value = newPage;
  } catch (err) {
    error.value = 'Failed to fetch commands.';
    console.error(err);
  } finally {
    loading.value = false;
  }
};

onMounted(() => {
  fetchCommands();
});

const handleLogout = async () => {
  await authStore.logoutAction();
  router.push('/login');
};
</script>

<style scoped>
.dashboard-container { padding: 2rem; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 2rem; }
.nav-links { display: flex; align-items: center; gap: 1.5rem; }
.nav-links h2 { margin: 0; }
.nav-btn { text-decoration: none; padding: 0.4rem 0.8rem; background-color: #e9ecef; color: #333; border-radius: 4px; font-weight: 500; }
.nav-btn:hover { background-color: #dde0e3; }
table { width: 100%; border-collapse: collapse; margin-bottom: 1rem; }
th, td { border: 1px solid #ddd; padding: 0.5rem; text-align: left; }
th { background-color: #f4f4f4; }
.status { padding: 0.2rem 0.5rem; border-radius: 4px; font-size: 0.8rem; font-weight: bold; }
.status.pending { background-color: #fff3cd; color: #555; }
.status.running { background-color: #cce5ff; color: #856404; }
.status.done { background-color: #d4edda; color: #155724; }
.status.error { background-color: #f8d7da; color: #721c24; }
.pagination { display: flex; gap: 1rem; align-items: center; justify-content: center; }
.error { color: red; }
</style>